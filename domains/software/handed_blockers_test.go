package software

// Phase 40 step 14c, the engineering_backlog stream's half: its observation
// lists the blockers the streams handed over, one per kind, each with the
// diagnosis, every attempt and its audit rows, and how often the kind came
// back, so its next pass is told to file one Engineering Backlog entry per
// kind and add later evidence to it rather than a second one.

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

var handedAt = time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)

const handedDiagnosis = "TestClock reads time.Now and the fixture pins no clock"

func handedAttempts() []reconcile.Attempt {
	return []reconcile.Attempt{
		{LoopID: "loop-1", At: handedAt, Diagnosis: handedDiagnosis, Remedy: "pin the clock in the fixture", AuditRows: []string{"row-11", "row-12"}},
		{LoopID: "loop-2", At: handedAt.Add(time.Hour), Diagnosis: handedDiagnosis, Remedy: "inject a clock into the service", AuditRows: []string{"row-21"}},
		{LoopID: "loop-3", At: handedAt.Add(2 * time.Hour), Diagnosis: handedDiagnosis, Remedy: "freeze time in TestMain", AuditRows: []string{"row-31"}},
	}
}

// handedStates are two streams: delivery spent its attempts on a red verify;
// UX saw a red verify (same kind) and a conflict come back inside the window.
func handedStates() []reconcile.State {
	return []reconcile.State{
		{
			ObjectiveID: "obj-delivery", TwinID: "twin-1",
			Handovers: []reconcile.Handover{{
				Kind: reconcile.BlockerVerifyFailed, Subject: "tests-pass", Reason: reconcile.HandoverAttemptsSpent,
				Diagnosis: handedDiagnosis, Attempts: handedAttempts(), At: handedAt.Add(2 * time.Hour),
			}},
		},
		{
			ObjectiveID: "obj-ux", TwinID: "twin-1",
			Handovers: []reconcile.Handover{
				{
					Kind: reconcile.BlockerVerifyFailed, Subject: "lint-clean", Reason: reconcile.HandoverRecurring,
					Cleared: []time.Time{handedAt, handedAt.Add(time.Hour)}, At: handedAt.Add(time.Hour),
				},
				{
					Kind: reconcile.BlockerUnmergeablePR, Subject: "karakuri/ux-2026-11-20", Reason: reconcile.HandoverRecurring,
					Cleared: []time.Time{handedAt, handedAt.Add(3 * time.Hour), handedAt.Add(5 * time.Hour)}, At: handedAt.Add(5 * time.Hour),
				},
			},
		},
		{ObjectiveID: "obj-quiet", TwinID: "twin-1"},
	}
}

func observeHanded(t *testing.T, states []reconcile.State) environment.Observation {
	t.Helper()
	return observeHandedWith(t, states, &backlogForge{fakeForge: &fakeForge{}})
}

func observeHandedWith(t *testing.T, states []reconcile.State, forge versioncontrol.VersionControlAdapter) environment.Observation {
	t.Helper()
	env := NewHandedBlockersEnvironment(EnvHandedBlockers, func(context.Context) ([]reconcile.State, error) { return states, nil }, forge)
	obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	return obs
}

func handedOf(t *testing.T, obs environment.Observation) []HandedBlocker {
	t.Helper()
	got, ok := obs.State["handed_blockers"].([]HandedBlocker)
	if !ok {
		t.Fatalf("state[handed_blockers] = %#v, want []HandedBlocker", obs.State["handed_blockers"])
	}
	return got
}

func TestTheBacklogObservationListsOneHandedBlockerPerKindWithItsEvidence(t *testing.T) {
	obs := observeHanded(t, handedStates())
	got := handedOf(t, obs)
	if len(got) != 2 {
		t.Fatalf("handed blockers = %+v, want two: one per kind", got)
	}
	// Sorted by kind.
	unmergeable, verify := got[0], got[1]
	if verify.Kind != "verify_failed" || unmergeable.Kind != "unmergeable_pr" {
		t.Fatalf("kinds = %q, %q; want unmergeable_pr then verify_failed", unmergeable.Kind, verify.Kind)
	}

	if want := []string{"obj-delivery", "obj-ux"}; !reflect.DeepEqual(verify.Streams, want) {
		t.Errorf("verify_failed streams = %v, want %v", verify.Streams, want)
	}
	if want := []string{"lint-clean", "tests-pass"}; !reflect.DeepEqual(verify.Subjects, want) {
		t.Errorf("verify_failed subjects = %v, want %v", verify.Subjects, want)
	}
	if verify.Diagnosis != handedDiagnosis {
		t.Errorf("verify_failed diagnosis = %q, want %q", verify.Diagnosis, handedDiagnosis)
	}
	if !reflect.DeepEqual(verify.Attempts, handedAttempts()) {
		t.Errorf("verify_failed attempts = %+v, want every attempt with its audit rows", verify.Attempts)
	}
	if verify.Occurrences != 2 {
		t.Errorf("verify_failed occurrences = %d, want 2 (the UX stream's two clears)", verify.Occurrences)
	}
	if unmergeable.Occurrences != 3 || !reflect.DeepEqual(unmergeable.Streams, []string{"obj-ux"}) {
		t.Errorf("unmergeable_pr = %+v, want obj-ux with 3 occurrences", unmergeable)
	}

	if hasWork, _ := obs.State["has_work"].(bool); !hasWork {
		t.Errorf("has_work = %v with blockers handed over, want true: the backlog stream has an entry to file", obs.State["has_work"])
	}
}

// The planner reads the lines: it is told to file one Engineering Backlog
// entry per kind, or add to the one already filed, with all the evidence.
func TestTheBacklogPassIsToldToFileOneEntryPerKindWithTheEvidence(t *testing.T) {
	obs := observeHanded(t, handedStates())
	lines, ok := obs.State["handed_blockers_lines"].([]string)
	if !ok || len(lines) == 0 {
		t.Fatalf("state[handed_blockers_lines] = %#v, want the planner's lines", obs.State["handed_blockers_lines"])
	}
	text := strings.Join(lines, "\n")
	for _, want := range []string{
		"one Engineering Backlog entry", "verify_failed", "unmergeable_pr", handedDiagnosis,
		"obj-delivery", "obj-ux", "tests-pass", "lint-clean",
		"loop-1", "loop-2", "loop-3",
		"pin the clock in the fixture", "inject a clock into the service", "freeze time in TestMain",
		"row-11", "row-12", "row-21", "row-31",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the lines do not hold %q:\n%s", want, text)
		}
	}
	if n := strings.Count(text, "one Engineering Backlog entry"); n != 2 {
		t.Errorf("the lines ask for %d entries, want one per kind (2):\n%s", n, text)
	}
}

// Nothing handed over: no work, an empty listing, and a hash that does not
// move with the clock.
func TestNoHandedBlockerGivesTheBacklogStreamNoWork(t *testing.T) {
	obs := observeHanded(t, []reconcile.State{{ObjectiveID: "obj-quiet", TwinID: "twin-1"}})
	if got := handedOf(t, obs); len(got) != 0 {
		t.Errorf("handed blockers = %+v, want none", got)
	}
	if hasWork, ok := obs.State["has_work"].(bool); !ok || hasWork {
		t.Errorf("has_work = %#v, want false", obs.State["has_work"])
	}
	again := observeHanded(t, []reconcile.State{{ObjectiveID: "obj-quiet", TwinID: "twin-1"}})
	if obs.Version == "" || obs.Version != again.Version {
		t.Errorf("versions %q and %q, want one stable non-empty version", obs.Version, again.Version)
	}
}

// A third occurrence changes the evidence, so the version moves and the backlog
// stream gets a pass to add it to its entry.
func TestANewOccurrenceMovesTheBacklogObservation(t *testing.T) {
	before := observeHanded(t, handedStates())
	more := handedStates()
	h := &more[1].Handovers[0]
	h.Cleared = append(h.Cleared, handedAt.Add(4*time.Hour))
	after := observeHanded(t, more)
	if before.Version == after.Version {
		t.Errorf("version %q unchanged by a new occurrence: the entry's evidence would never be added", before.Version)
	}
	if got := handedOf(t, after); len(got) != 2 || got[1].Occurrences != 3 {
		t.Errorf("handed blockers = %+v, want still two kinds, verify_failed with 3 occurrences", got)
	}
}

// Blind, not quiet: states that cannot be read are an error and an empty SHA,
// never "nothing handed over".
func TestUnreadableStatesLeaveTheBacklogObservationBlind(t *testing.T) {
	env := NewHandedBlockersEnvironment(EnvHandedBlockers, func(context.Context) ([]reconcile.State, error) {
		return nil, errors.New("database is locked")
	}, &backlogForge{fakeForge: &fakeForge{}})
	if _, err := env.Observe(context.Background(), environment.ObservationQuery{}); err == nil {
		t.Error("observe returned no error for states it could not read")
	}
	snap, _ := env.Snapshot(context.Background())
	if snap.SHA != "" {
		t.Errorf("snapshot SHA = %q, want empty", snap.SHA)
	}
}

// The pack binds the environment to the store, scoped to the twin it is built
// for: another twin's handovers are not this backlog's to file.
func TestThePackReadsTheTwinsHandedBlockersFromTheStore(t *testing.T) {
	s := newPostedStore(t)
	ctx := context.Background()
	for _, st := range append(handedStates(), reconcile.State{
		ObjectiveID: "obj-other-twin", TwinID: "twin-2",
		Handovers: []reconcile.Handover{{Kind: reconcile.BlockerPlanError, Subject: "obj-other-twin", Reason: reconcile.HandoverAttemptsSpent, At: handedAt}},
	}) {
		if err := s.SaveObjective(ctx, objective.Objective{ID: st.ObjectiveID, TwinID: st.TwinID, Mode: objective.ModeStanding, Domain: "software"}); err != nil {
			t.Fatalf("save objective: %v", err)
		}
		if err := s.SaveReconcileState(ctx, st); err != nil {
			t.Fatalf("save state: %v", err)
		}
	}

	// The twin's forge, bound the way production binds it, holds the backlog
	// stream's merged entry for verify_failed.
	forge := &backlogForge{fakeForge: &fakeForge{merged: []versioncontrol.PRSummary{
		backlogPR(1, "karakuri/tech-backlog-2026-11-21", true, backlogEntryLine("verify_failed", verifyEvidence)),
	}}}
	reg := tools.NewRegistry()
	reg.VC.Set("forge", "stub", versioncontrol.VersionControlAdapter(forge))
	pack := New()
	pack.tools = reg
	pack.WithStorage(s)
	var env environment.Environment
	for _, fac := range pack.EnvironmentFactories() {
		if fac.EnvID != EnvHandedBlockers {
			continue
		}
		built, err := fac.Build(environment.BuildContext{TwinID: "twin-1", AdapterBindings: map[string]string{"versioncontrol": "forge"}})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		env = built
	}
	if env == nil {
		t.Fatalf("the pack declares no %s environment", EnvHandedBlockers)
	}
	obs, err := env.Observe(ctx, environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	got := handedOf(t, obs)
	if len(got) != 2 {
		t.Fatalf("handed blockers = %+v, want twin-1's two kinds", got)
	}
	for _, h := range got {
		if h.Kind == string(reconcile.BlockerPlanError) {
			t.Errorf("twin-2's plan_error handover is listed for twin-1")
		}
	}
	if lines := linesOf(t, obs); len(lines) != 1 || !strings.Contains(lines[0], "unmergeable_pr") {
		t.Errorf("lines = %q, want one, for unmergeable_pr: verify_failed is filed on the bound forge", lines)
	}
}

// The handovers' newest evidence per kind in handedStates.
var (
	verifyEvidence      = handedAt.Add(2 * time.Hour) // delivery's third attempt
	unmergeableEvidence = handedAt.Add(5 * time.Hour) // UX's third clear
)

// backlogForge is a forge whose open, merged and closed-unmerged pull requests
// a test sets, and which counts how often it was read.
type backlogForge struct {
	*fakeForge
	closed []versioncontrol.PRSummary
	reads  int
}

func (f *backlogForge) ListPRs(ctx context.Context, repo string, since time.Time) ([]versioncontrol.PRSummary, error) {
	f.reads++
	return f.fakeForge.ListPRs(ctx, repo, since)
}

func (f *backlogForge) ListClosedPRs(context.Context, string, int) ([]versioncontrol.PRSummary, error) {
	return f.closed, nil
}

func backlogPR(number int, branch string, own bool, body ...string) versioncontrol.PRSummary {
	return versioncontrol.PRSummary{
		Number: number, HeadBranch: branch, OwnAuthor: own,
		Body: "Adds the Engineering Backlog entries.\n\n" + strings.Join(body, "\r\n") + "\n",
	}
}

func linesOf(t *testing.T, obs environment.Observation) []string {
	t.Helper()
	lines, ok := obs.State["handed_blockers_lines"].([]string)
	if !ok {
		t.Fatalf("state[handed_blockers_lines] = %#v, want []string", obs.State["handed_blockers_lines"])
	}
	return lines
}

func hasWork(obs environment.Observation) bool {
	work, _ := obs.State["has_work"].(bool)
	return work
}

// (1) A kind no entry is filed for is work, and its line hands the pass the
// exact marker line covering the kind's newest evidence.
func TestAnUnfiledHandedBlockerIsWorkWithItsMarkerLine(t *testing.T) {
	obs := observeHanded(t, handedStates())
	if !hasWork(obs) {
		t.Fatal("has_work = false with nothing filed, want true")
	}
	text := strings.Join(linesOf(t, obs), "\n")
	for _, want := range []string{
		backlogEntryLine("verify_failed", verifyEvidence),
		backlogEntryLine("unmergeable_pr", unmergeableEvidence),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the lines do not hand over the marker %q:\n%s", want, text)
		}
	}
}

// (2) Once the pass's pull request carries the marker lines, the next
// observation has no work and no line, keeps the evidence, and has moved.
func TestAFiledHandedBlockerIsTheOwnersTurn(t *testing.T) {
	before := observeHanded(t, handedStates())
	forge := &backlogForge{fakeForge: &fakeForge{prs: []versioncontrol.PRSummary{
		backlogPR(7, "karakuri/tech-backlog-2026-11-28", true,
			backlogEntryLine("verify_failed", verifyEvidence), backlogEntryLine("unmergeable_pr", unmergeableEvidence)),
	}}}
	after := observeHandedWith(t, handedStates(), forge)
	if hasWork(after) {
		t.Error("has_work = true with every kind filed, want false: it is the owner's turn")
	}
	if lines := linesOf(t, after); len(lines) != 0 {
		t.Errorf("lines = %q, want none", lines)
	}
	got := handedOf(t, after)
	if len(got) != 2 || got[0].FiledThrough == nil || got[1].FiledThrough == nil {
		t.Errorf("handed blockers = %+v, want both still listed, each with the evidence its entry covers", got)
	}
	if before.Version == after.Version {
		t.Errorf("version %q unchanged by the filing: drift would not see it", after.Version)
	}
}

// (3) An occurrence after the filing brings the kind back as work, as evidence
// for the same entry; a closed-unmerged entry counts as answered too.
func TestNewEvidenceAfterTheFilingIsWorkAgain(t *testing.T) {
	forge := &backlogForge{
		fakeForge: &fakeForge{merged: []versioncontrol.PRSummary{
			backlogPR(7, "karakuri/tech-backlog-2026-11-28", true, backlogEntryLine("unmergeable_pr", unmergeableEvidence)),
		}},
		closed: []versioncontrol.PRSummary{
			backlogPR(8, "karakuri/tech-backlog-2026-11-27", true, backlogEntryLine("verify_failed", verifyEvidence)),
		},
	}
	filed := observeHandedWith(t, handedStates(), forge)
	if hasWork(filed) {
		t.Fatalf("has_work = true before the new occurrence, lines %q", linesOf(t, filed))
	}

	later := handedStates()
	h := &later[1].Handovers[1]
	h.Cleared = append(h.Cleared, unmergeableEvidence.Add(time.Hour))
	obs := observeHandedWith(t, later, forge)
	if !hasWork(obs) {
		t.Fatal("has_work = false after a new occurrence, want true")
	}
	lines := linesOf(t, obs)
	if len(lines) != 1 {
		t.Fatalf("lines = %q, want one, for unmergeable_pr", lines)
	}
	for _, want := range []string{
		"unmergeable_pr", "covers the evidence up to " + unmergeableEvidence.Format(time.RFC3339),
		backlogEntryLine("unmergeable_pr", unmergeableEvidence.Add(time.Hour)),
	} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("line does not hold %q:\n%s", want, lines[0])
		}
	}
	if obs.Version == filed.Version {
		t.Error("version unchanged by the new occurrence")
	}
}

// (4) Two kinds, one filed: only the other has a line.
func TestOnlyTheUnfiledKindHasALine(t *testing.T) {
	forge := &backlogForge{fakeForge: &fakeForge{prs: []versioncontrol.PRSummary{
		backlogPR(7, "karakuri/tech-backlog-2026-11-28", true, backlogEntryLine("verify_failed", verifyEvidence)),
	}}}
	obs := observeHandedWith(t, handedStates(), forge)
	lines := linesOf(t, obs)
	if len(lines) != 1 || !strings.Contains(lines[0], "unmergeable_pr") || strings.Contains(lines[0], "verify_failed") {
		t.Errorf("lines = %q, want one, for unmergeable_pr only", lines)
	}
	if !hasWork(obs) {
		t.Error("has_work = false with unmergeable_pr unfiled")
	}
}

// (5) A forge that cannot be read, or none bound, leaves the observation
// blind while something is handed over; with nothing handed over the forge is
// not read at all.
func TestAnUnreadableForgeLeavesTheBacklogObservationBlind(t *testing.T) {
	states := func(context.Context) ([]reconcile.State, error) { return handedStates(), nil }
	for name, vc := range map[string]versioncontrol.VersionControlAdapter{
		"forge errors":    &backlogForge{fakeForge: &fakeForge{prsErr: errors.New("502 bad gateway")}},
		"no-op adapter":   versioncontrol.NewNoOp(),
		"nothing bound":   nil,
		"merged unlisted": &noMergedForge{backlogForge{fakeForge: &fakeForge{}}},
	} {
		t.Run(name, func(t *testing.T) {
			env := NewHandedBlockersEnvironment(EnvHandedBlockers, states, vc)
			if _, err := env.Observe(context.Background(), environment.ObservationQuery{}); err == nil {
				t.Error("observe returned no error for a forge it could not read")
			}
			if snap, _ := env.Snapshot(context.Background()); snap.SHA != "" {
				t.Errorf("snapshot SHA = %q, want empty", snap.SHA)
			}
		})
	}

	forge := &backlogForge{fakeForge: &fakeForge{prsErr: errors.New("502 bad gateway")}}
	obs := observeHandedWith(t, []reconcile.State{{ObjectiveID: "obj-quiet", TwinID: "twin-1"}}, forge)
	if hasWork(obs) || forge.reads != 0 {
		t.Errorf("has_work = %v, forge reads = %d with nothing handed over, want false and 0", hasWork(obs), forge.reads)
	}
}

type noMergedForge struct{ backlogForge }

func (f *noMergedForge) ListMergedPRs(context.Context, string, int) ([]versioncontrol.PRSummary, error) {
	return nil, versioncontrol.ErrUnsupported
}

// (6) A marker on a pull request that is not the backlog stream's own, on
// another stream's branch, on a stranger's pull request from the backlog's
// branch, or on main, retires nothing.
func TestAPullRequestNotTheBacklogStreamsRetiresNothing(t *testing.T) {
	markers := []string{backlogEntryLine("verify_failed", verifyEvidence), backlogEntryLine("unmergeable_pr", unmergeableEvidence)}
	forge := &backlogForge{
		fakeForge: &fakeForge{
			prs:    []versioncontrol.PRSummary{backlogPR(1, "karakuri/ux-2026-11-28", true, markers...)},
			merged: []versioncontrol.PRSummary{backlogPR(2, "karakuri/tech-backlog-2026-11-28", false, markers...)},
		},
		closed: []versioncontrol.PRSummary{backlogPR(3, "fix-the-clock", true, markers...)},
	}
	obs := observeHandedWith(t, handedStates(), forge)
	if !hasWork(obs) || len(linesOf(t, obs)) != 2 {
		t.Errorf("has_work = %v, lines = %q; want work and both kinds' lines", hasWork(obs), linesOf(t, obs))
	}
	if obs.Trust == environment.TrustThirdParty {
		t.Error("trust = third party, but no pull request text is carried")
	}
}
