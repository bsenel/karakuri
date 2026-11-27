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
	env := NewHandedBlockersEnvironment(EnvHandedBlockers, func(context.Context) ([]reconcile.State, error) { return states, nil })
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
	})
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

	pack := New()
	pack.WithStorage(s)
	var env environment.Environment
	for _, fac := range pack.EnvironmentFactories() {
		if fac.EnvID != EnvHandedBlockers {
			continue
		}
		built, err := fac.Build(environment.BuildContext{TwinID: "twin-1"})
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
}
