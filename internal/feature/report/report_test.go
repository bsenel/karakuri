package report

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/digest"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	"github.com/bsenel/karakuri/internal/core/twin"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

func newService(t *testing.T) (*Service, storage.StorageAdapter, *time.Time) {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "report.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := storage.NewGORMStorage(db)
	clock := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	// No agent factory and no tool registry: the digest has to be complete
	// without a model, because that is what gets delivered when one is
	// unavailable and it is appended beneath the prose when one is not.
	svc := NewService(store, nil, nil, karakuriquota.Deps{}, Config{})
	svc.now = func() time.Time { return clock }
	return svc, store, &clock
}

func seedTwin(t *testing.T, store storage.StorageAdapter, id, name string) {
	t.Helper()
	if err := store.SaveTwin(context.Background(), twin.DigitalTwin{
		ID: id, Name: name, Kind: twin.KindPerson, Domain: "software",
	}); err != nil {
		t.Fatalf("seed twin: %v", err)
	}
}

func seedStanding(t *testing.T, store storage.StorageAdapter, twinID, id, title string) objective.Objective {
	t.Helper()
	obj := objective.Objective{
		ID: objective.ObjectiveID(id), Title: title, Domain: "software",
		TwinID: twinID, Mode: objective.ModeStanding, Status: objective.StatusConverged,
	}
	if err := store.SaveObjective(context.Background(), obj); err != nil {
		t.Fatalf("seed objective: %v", err)
	}
	return obj
}

// The ratio between cheap and expensive passes is the answer to "is this
// costing me anything", so a digest reports both — a summary that showed only
// the reconciles would make a well-behaved objective look idle.
func TestDigestCountsCheapAndExpensivePassesSeparately(t *testing.T) {
	svc, store, clock := newService(t)
	ctx := context.Background()
	seedTwin(t, store, "twin-1", "CTO twin")
	obj := seedStanding(t, store, "twin-1", "obj-1", "keep the build green")

	since := clock.Add(-24 * time.Hour)
	for i := 0; i < 90; i++ {
		mustOutcome(t, store, reconcile.Outcome{
			ID: id(i), ObjectiveID: obj.ID, Trigger: reconcile.TriggerSchedule,
			StartedAt: since.Add(time.Duration(i) * time.Minute),
		})
	}
	mustOutcome(t, store, reconcile.Outcome{
		ID: "r1", ObjectiveID: obj.ID, LoopID: "loop-1", Trigger: reconcile.TriggerDrift,
		Drift:     reconcile.Drift{Changed: true, Environments: []string{"git"}},
		Converged: true, CriteriaMet: 1,
		StartedAt: since.Add(2 * time.Hour),
	})
	// Outside the window, and must not be counted.
	mustOutcome(t, store, reconcile.Outcome{
		ID: "old", ObjectiveID: obj.ID, LoopID: "loop-0", Trigger: reconcile.TriggerSchedule,
		StartedAt: since.Add(-48 * time.Hour),
	})

	d, err := svc.Assemble(ctx, "twin-1", since, *clock)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(d.Objectives) != 1 {
		t.Fatalf("got %d objectives, want 1", len(d.Objectives))
	}
	o := d.Objectives[0]
	if o.Senses != 90 {
		t.Errorf("senses = %d, want 90 — the cheap passes are what prove the split works", o.Senses)
	}
	if o.Reconciles != 1 {
		t.Errorf("reconciles = %d, want 1", o.Reconciles)
	}
	if o.DriftDetected != 1 || o.Converged != 1 {
		t.Errorf("drift = %d converged = %d, want 1/1", o.DriftDetected, o.Converged)
	}
	if d.TwinName != "CTO twin" {
		t.Errorf("twin name = %q, want the twin's name", d.TwinName)
	}
}

// Oldest first, against the usual newest-first: the checkpoint that has been
// waiting three days is the one blocking work, and burying it under this
// morning's is how a queue grows.
func TestDecisionsAreOldestFirst(t *testing.T) {
	svc, store, clock := newService(t)
	ctx := context.Background()
	seedTwin(t, store, "twin-1", "CTO twin")
	seedStanding(t, store, "twin-1", "obj-1", "keep the build green")

	for i, age := range []time.Duration{time.Hour, 72 * time.Hour, 24 * time.Hour} {
		cp := corecheckpoint.Checkpoint{
			ID: id(i), ObjectiveID: "obj-1", TwinID: "twin-1",
			Summary: id(i), Status: corecheckpoint.StatusPending,
			CreatedAt: clock.Add(-age),
			Actions:   []corecheckpoint.Action{{CapabilityID: "code.write"}},
		}
		if err := store.SaveCheckpoint(ctx, cp); err != nil {
			t.Fatalf("seed checkpoint: %v", err)
		}
	}

	d, err := svc.Assemble(ctx, "twin-1", clock.Add(-24*time.Hour), *clock)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(d.Decisions) != 3 {
		t.Fatalf("got %d decisions, want 3", len(d.Decisions))
	}
	if !d.Decisions[0].WaitingAt.Before(d.Decisions[1].WaitingAt) {
		t.Error("decisions are not oldest first")
	}
	if got := d.Decisions[0].Age(*clock); got < 71*time.Hour {
		t.Errorf("oldest decision age = %s, want about 72h", got)
	}
	if len(d.Decisions[0].Proposed) != 1 {
		t.Error("the proposed capabilities did not survive — a reviewer should not have to open anything to see what they are approving")
	}
}

// A daily mail that says "nothing happened" is a mail people stop reading,
// which costs the ones that matter their audience.
func TestEmptyDigest(t *testing.T) {
	svc, store, clock := newService(t)
	ctx := context.Background()
	seedTwin(t, store, "twin-1", "CTO twin")
	seedStanding(t, store, "twin-1", "obj-1", "keep the build green")

	d, err := svc.Assemble(ctx, "twin-1", clock.Add(-24*time.Hour), *clock)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if !d.Empty() {
		t.Error("a window in which nothing happened did not report itself empty")
	}

	// Sense passes alone are not activity worth mailing about — an objective
	// that looked ninety times and found nothing is exactly the case this
	// suppression exists for.
	for i := 0; i < 90; i++ {
		mustOutcome(t, store, reconcile.Outcome{
			ID: id(i), ObjectiveID: "obj-1", Trigger: reconcile.TriggerSchedule,
			StartedAt: clock.Add(-time.Duration(i) * time.Minute),
		})
	}
	d, _ = svc.Assemble(ctx, "twin-1", clock.Add(-24*time.Hour), *clock)
	if !d.Empty() {
		t.Error("ninety quiet checks were treated as news")
	}

	// One pending decision is always news.
	if err := store.SaveCheckpoint(ctx, corecheckpoint.Checkpoint{
		ID: "cp-1", ObjectiveID: "obj-1", TwinID: "twin-1",
		Summary: "wants to force-push", Status: corecheckpoint.StatusPending,
		CreatedAt: *clock,
	}); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}
	d, _ = svc.Assemble(ctx, "twin-1", clock.Add(-24*time.Hour), *clock)
	if d.Empty() {
		t.Error("a pending decision was treated as nothing to report")
	}
}

// The noisiest objective goes first. A reader skimming a morning brief should
// meet the one that failed four times before the one that quietly converged.
func TestObjectivesAreOrderedByAttentionNeeded(t *testing.T) {
	svc, store, clock := newService(t)
	ctx := context.Background()
	seedTwin(t, store, "twin-1", "CTO twin")
	seedStanding(t, store, "twin-1", "quiet", "quietly converging")
	seedStanding(t, store, "twin-1", "noisy", "failing repeatedly")

	since := clock.Add(-24 * time.Hour)
	mustOutcome(t, store, reconcile.Outcome{
		ID: "a", ObjectiveID: "quiet", LoopID: "l1", Converged: true, StartedAt: since,
	})
	for i := 0; i < 3; i++ {
		mustOutcome(t, store, reconcile.Outcome{
			ID: "f" + id(i), ObjectiveID: "noisy", LoopID: "l" + id(i),
			Error: "adapter unreachable", StartedAt: since.Add(time.Duration(i) * time.Hour),
		})
	}

	d, err := svc.Assemble(ctx, "twin-1", since, *clock)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(d.Objectives) != 2 || d.Objectives[0].ID != "noisy" {
		t.Fatalf("order = %v, want the failing objective first", titles(d))
	}
	if d.Objectives[0].Failures != 3 {
		t.Errorf("failures = %d, want 3", d.Objectives[0].Failures)
	}
}

func TestPlainRendering(t *testing.T) {
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	d := digest.Digest{
		TwinID: "twin-1", TwinName: "CTO twin",
		Since: now.Add(-24 * time.Hour), Until: now,
		Objectives: []digest.ObjectiveSummary{{
			ID: "obj-1", Title: "keep the build green", Status: objective.StatusBlocked,
			Senses: 90, Reconciles: 2, Actions: 4, Failures: 3,
			Paused: true, PausedWhy: "3 consecutive failed reconciles",
		}},
		Decisions: []digest.Decision{{
			CheckpointID: "cp-1", ObjectiveTitle: "keep the build green",
			Summary: "wants to force-push to main", Options: []string{"approve", "reject"},
			Proposed: []string{"code.write"}, WaitingAt: now.Add(-72 * time.Hour),
		}},
		AutonomyChanges: []digest.AutonomyChange{{
			ObjectiveTitle: "keep the build green",
			From:           objective.AutonomyActWithNotice, To: objective.AutonomyPropose,
			Reason: "checkpoint_rejected", At: now,
		}},
	}

	out := Plain(d)

	// Decisions lead. This is why the report is sent rather than left in a
	// console somebody might open.
	decisions := strings.Index(out, "Decisions I need from you")
	objectives := strings.Index(out, "Standing objectives")
	if decisions < 0 || objectives < 0 || decisions > objectives {
		t.Error("the decisions section does not lead the report")
	}
	if !strings.Contains(out, "waiting 3 days") {
		t.Error("a three-day-old decision is not rendered as three days — nobody should have to subtract dates")
	}
	if !strings.Contains(out, "krk checkpoint resolve cp-1") {
		t.Error("the report does not say how to answer")
	}
	if !strings.Contains(out, "narrowed to propose") {
		t.Error("a demotion is not reported as a narrowing")
	}
	if !strings.Contains(out, "90 checks, 2 reconciles") {
		t.Error("the cheap passes are missing from the summary line")
	}
	if !strings.Contains(out, "PAUSED") {
		t.Error("a paused objective does not say so")
	}
	// An unpriced deployment says so rather than showing a zero that reads as
	// "this was free".
	if !strings.Contains(out, "not priced") {
		t.Error("an unpriced digest rendered a bare zero")
	}
}

func TestRoughly(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Second:    "less than a minute",
		time.Minute:         "1 minute",
		90 * time.Minute:    "1 hour",
		5 * time.Hour:       "5 hours",
		24 * time.Hour:      "24 hours",
		72 * time.Hour:      "3 days",
		30 * 24 * time.Hour: "30 days",
	}
	for d, want := range cases {
		if got := roughly(d); got != want {
			t.Errorf("roughly(%s) = %q, want %q", d, got, want)
		}
	}
}

// Since resolves from the last delivery rather than a fixed offset, so a
// sender that was down for a day sends one digest covering two days instead of
// silently losing one.
func TestScheduleWindowCatchesUp(t *testing.T) {
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	twoDaysAgo := now.Add(-48 * time.Hour)

	sch := digest.Schedule{LastSentAt: &twoDaysAgo}
	if got := sch.Since(now); !got.Equal(twoDaysAgo) {
		t.Errorf("since = %s, want the last delivery %s — a missed run must catch up", got, twoDaysAgo)
	}

	// An explicit window overrides it: somebody who asked for "the last 24
	// hours" means that even after an outage.
	sch.Window = "24h"
	if got := sch.Since(now); !got.Equal(now.Add(-24 * time.Hour)) {
		t.Errorf("since = %s, want the declared window", got)
	}

	// A schedule that has never sent falls back to a day.
	fresh := digest.Schedule{}
	if got := fresh.Since(now); !got.Equal(now.Add(-24 * time.Hour)) {
		t.Errorf("since = %s on a fresh schedule, want 24h back", got)
	}
}

func TestDeclareRejectsBadSchedules(t *testing.T) {
	svc, store, _ := newService(t)
	ctx := context.Background()
	seedTwin(t, store, "twin-1", "CTO twin")

	bad := []struct {
		name string
		sch  digest.Schedule
	}{
		{"no twin", digest.Schedule{Channel: "messaging", Target: "#eng"}},
		{"unknown channel", digest.Schedule{TwinID: "twin-1", Channel: "carrier-pigeon"}},
		{"malformed cron", digest.Schedule{TwinID: "twin-1", Channel: "email",
			Cadence: objective.Cadence{Cron: "not a cron"}}},
		{"two schedules", digest.Schedule{TwinID: "twin-1", Channel: "email",
			Cadence: objective.Cadence{Every: "1h", DailyAt: "08:00"}}},
		{"bad window", digest.Schedule{TwinID: "twin-1", Channel: "email", Window: "a while"}},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Declare(ctx, tc.sch); err == nil {
				t.Error("Declare accepted a schedule it should have rejected")
			}
		})
	}

	good, err := svc.Declare(ctx, digest.Schedule{
		TwinID: "twin-1", Channel: "messaging", Target: "#eng",
		Cadence: objective.Cadence{DailyAt: "08:00", Timezone: "Europe/Istanbul"},
	})
	if err != nil {
		t.Fatalf("Declare rejected a valid schedule: %v", err)
	}
	if good.NextDueAt == nil {
		t.Error("a declared schedule was not armed")
	}
	// Armed for the cadence's next firing rather than for now: a digest
	// declared at noon for an 8am schedule must not fire immediately, because
	// the window it would cover is not the one anybody asked about.
	if !good.NextDueAt.After(time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("next due = %s, want a future firing rather than now", good.NextDueAt)
	}
}

func mustOutcome(t *testing.T, store storage.StorageAdapter, o reconcile.Outcome) {
	t.Helper()
	if err := store.SaveReconcileOutcome(context.Background(), o); err != nil {
		t.Fatalf("save outcome: %v", err)
	}
}

func id(i int) string { return string(rune('a'+i%26)) + string(rune('a'+i/26)) }

func titles(d digest.Digest) []string {
	out := make([]string, len(d.Objectives))
	for i, o := range d.Objectives {
		out[i] = string(o.ID)
	}
	return out
}

// A quiet window on a fully priced deployment is quiet, not unpriced.
//
// Priced was derived as `Cost > 0`, which made it a restatement of the total:
// any window in which nothing was spent printed "not priced — no rate table is
// configured" into the operator's morning brief, which for most deployments is
// simply untrue. It asks whether prices exist, not whether money was spent.
func TestQuietWindowIsNotReportedAsUnpriced(t *testing.T) {
	svc, store, clock := newService(t)
	seedTwin(t, store, "twin-1", "Ops")
	seedStanding(t, store, "twin-1", "obj-1", "Watch the repo")

	d, err := svc.Assemble(context.Background(), "twin-1", clock.Add(-24*time.Hour), *clock)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if d.Spend.Cost != 0 {
		t.Fatalf("expected a quiet window, got cost %v", d.Spend.Cost)
	}
	if !d.Spend.Priced {
		t.Error("a window in which nothing was spent was reported as having no rate table")
	}
}

// A failed send must not retry on every tick. The failure path leaves
// LastSentAt untouched, so without a backoff the schedule reads as never-run
// and is due again immediately — 1,440 failed deliveries and 1,440 audit rows
// a day against a channel nobody has fixed yet.
func TestFailedSendBacksOff(t *testing.T) {
	if got := sendBackoff(0); got != 0 {
		t.Errorf("sendBackoff(0) = %v, want 0", got)
	}
	first := sendBackoff(1)
	if first <= 0 {
		t.Fatalf("sendBackoff(1) = %v, want a positive delay", first)
	}
	if second := sendBackoff(2); second <= first {
		t.Errorf("sendBackoff(2) = %v, want more than sendBackoff(1) = %v", second, first)
	}
	// It stops growing, so a long-broken channel still gets retried rather
	// than effectively never.
	if capped := sendBackoff(50); capped != time.Hour {
		t.Errorf("sendBackoff(50) = %v, want it capped at an hour", capped)
	}
}

// An objective that stopped for want of money gets its own section.
//
// Separate from the objective roll-call because the two look identical there:
// an objective with no reconciles because nothing drifted is healthy, and one
// with no reconciles because it hit its ceiling is an operator decision waiting
// to be made. Phase 23 declared the budget and shipped the deferral; the
// section that tells anybody about it was step 6 and was not built.
func TestDigestNamesObjectivesThatRanOutOfBudget(t *testing.T) {
	svc, store, clock := newService(t)
	ctx := context.Background()
	seedTwin(t, store, "twin-1", "CTO twin")

	obj := seedStanding(t, store, "twin-1", "obj-1", "improve this deployment")
	obj.Budget = &objective.Budget{Daily: 5}
	if err := store.SaveObjective(ctx, obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}
	// A second, unbudgeted objective that must not appear.
	quiet := seedStanding(t, store, "twin-1", "obj-2", "watch the build")

	since := clock.Add(-24 * time.Hour)
	resets := clock.Add(2 * time.Hour)
	for i, at := range []time.Time{since.Add(time.Hour), since.Add(5 * time.Hour)} {
		mustOutcome(t, store, reconcile.Outcome{
			ID: id(100 + i), ObjectiveID: obj.ID, LoopID: "loop-x",
			Trigger:       reconcile.TriggerSchedule,
			Deferred:      "budget_exhausted",
			DeferredUntil: resets,
			CriteriaMet:   0.8,
			StartedAt:     at,
		})
	}
	mustOutcome(t, store, reconcile.Outcome{
		ID: "q1", ObjectiveID: quiet.ID, LoopID: "loop-y",
		Trigger: reconcile.TriggerSchedule, StartedAt: since.Add(time.Hour),
	})

	d, err := svc.Assemble(ctx, "twin-1", since, *clock)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	if len(d.Exhausted) != 1 {
		t.Fatalf("got %d exhausted objectives, want 1: %+v", len(d.Exhausted), d.Exhausted)
	}
	x := d.Exhausted[0]
	if x.ObjectiveID != obj.ID {
		t.Errorf("named %q, want %q", x.ObjectiveID, obj.ID)
	}
	// The count is what separates a budget doing its job from a ceiling set
	// below what the cadence asks for.
	if x.Times != 2 {
		t.Errorf("Times = %d, want 2", x.Times)
	}
	if x.Ceiling != 5 {
		t.Errorf("Ceiling = %v, want 5", x.Ceiling)
	}
	if !x.ResumesAt.Equal(resets) {
		t.Errorf("ResumesAt = %v, want %v", x.ResumesAt, resets)
	}
	// What it was mid-way through: 0.8 of the way is a different message from
	// having had nothing left to do.
	if x.InterruptedBy == "" {
		t.Error("nothing said what it was interrupted mid-way through")
	}

	// And it reaches the reader.
	prose := Plain(d)
	if !strings.Contains(prose, "Stopped for want of budget") {
		t.Error("the rendered digest has no budget section")
	}
	if !strings.Contains(prose, "improve this deployment") {
		t.Error("the rendered digest does not name the objective that stopped")
	}
	// A budget clears itself, so it must not be filed under decisions.
	for _, dec := range d.Decisions {
		if dec.ObjectiveID == obj.ID {
			t.Error("a budget exhaustion was filed as a decision somebody has to make")
		}
	}
}

// A digest whose only news is an exhausted budget is still worth sending: it
// is the reason every other number reads as quiet.
func TestADigestWithOnlyAnExhaustedBudgetIsNotEmpty(t *testing.T) {
	d := digest.Digest{Exhausted: []digest.BudgetExhaustion{{
		ObjectiveID: "obj-1", Times: 1,
	}}}
	if d.Empty() {
		t.Error("a digest reporting an exhausted budget was treated as nothing to send")
	}
}
