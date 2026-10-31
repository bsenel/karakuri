package reconcile

import (
	"context"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// Phase 41 step 8 part B: each pass that settled criteria records their states
// with the pass time, and reconcile evaluates every criterion that declares a
// sustain over the objective's recorded passes, beside ScoreStreak rather than
// in its place (docs/roadmap.md, Phase 41 section '4. Sustain for standing
// objectives'). Drift still comes from the fingerprint alone.

const day = 24 * time.Hour

// stepSustain is step 8's acceptance sustain: window 28d, budget 0.1,
// min_passes 3.
func stepSustain() *objective.Sustain {
	return &objective.Sustain{Window: "28d", Budget: 0.1, MinPasses: 3}
}

// uptime is the sustained criterion as a pass's verify step settled it. It
// carries the sustain the way the loop's copy of the objective's criteria does.
func uptime(state objective.CriterionState) objective.Criterion {
	c := settled("uptime", state, true)
	c.Sustain = stepSustain()
	return c
}

// sustainedObjective is an acting standing objective whose one criterion
// declares the acceptance sustain.
func sustainedObjective() objective.Objective {
	obj := actingObjective()
	obj.SuccessCriteria = []objective.Criterion{{ID: "uptime", Sustain: stepSustain()}}
	return obj
}

// sustainFixture never trips the breaker or the stall detector on its own, so
// the passes below are about sustain only.
func sustainFixture(t *testing.T) (*fixture, objective.Objective) {
	t.Helper()
	f := newFixture(t, Config{BreakerFailures: 100, StallReconciles: 100})
	f.use(t, map[string]string{"git": "aaa"})
	return f, f.declare(t, sustainedObjective())
}

// settlePass runs one manual pass whose loop settled uptime as given, the way
// a real loop ends for that state, and returns the pass time. The clock then
// moves on by gap.
func (f *fixture) settlePass(t *testing.T, id objective.ObjectiveID, state objective.CriterionState, gap time.Duration) time.Time {
	t.Helper()
	f.loops.criteria = []objective.Criterion{uptime(state)}
	switch state {
	case objective.CriterionMet:
		f.loops.status, f.loops.criteriaMet = objective.StatusCompleted, 1.0
	default:
		f.loops.status, f.loops.criteriaMet = objective.StatusFailed, 0
	}
	at := f.clock
	f.pass(t, id, reconcile.TriggerManual)
	f.clock = f.clock.Add(gap)
	return at
}

// tenPasses records ten passes two days apart, the given one of them not_met,
// and returns the last pass time.
func (f *fixture) tenPasses(t *testing.T, id objective.ObjectiveID, notMetAt int) time.Time {
	t.Helper()
	var last time.Time
	for i := 0; i < 10; i++ {
		state := objective.CriterionMet
		if i == notMetAt {
			state = objective.CriterionNotMet
		}
		last = f.settlePass(t, id, state, 2*day)
	}
	return last
}

func sustainOf(t *testing.T, st reconcile.State, id string) reconcile.CriterionSustain {
	t.Helper()
	for _, s := range st.Sustain {
		if s.CriterionID == id {
			return s
		}
	}
	t.Fatalf("no sustain state for criterion %q in %+v", id, st.Sustain)
	return reconcile.CriterionSustain{}
}

func checkSustain(t *testing.T, got, want reconcile.CriterionSustain) {
	t.Helper()
	if got.State != want.State || got.Reason != want.Reason || got.Settled != want.Settled ||
		got.NotMet != want.NotMet || got.BudgetLeft != want.BudgetLeft || got.Latest != want.Latest {
		t.Errorf("sustain = %s/%q settled=%d not_met=%d budget_left=%v latest=%s, want %s/%q settled=%d not_met=%d budget_left=%v latest=%s",
			got.State, got.Reason, got.Settled, got.NotMet, got.BudgetLeft, got.Latest,
			want.State, want.Reason, want.Settled, want.NotMet, want.BudgetLeft, want.Latest)
	}
	if !want.WindowStart.IsZero() && !got.WindowStart.Equal(want.WindowStart) {
		t.Errorf("window_start = %v, want %v", got.WindowStart, want.WindowStart)
	}
}

func latestOutcome(t *testing.T, f *fixture, id objective.ObjectiveID) reconcile.Outcome {
	t.Helper()
	history, err := f.svc.History(context.Background(), id, 1)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("no outcome recorded")
	}
	return history[0]
}

// Part B's first half: the outcome of a pass whose loop settled criteria
// records each one's state and reason with the pass time.
func TestAPassRecordsTheCriterionStatesItSettled(t *testing.T) {
	f := newFixture(t, Config{BreakerFailures: 100, StallReconciles: 100})
	f.use(t, map[string]string{"git": "aaa"})
	f.loops.status = objective.StatusFailed
	f.loops.criteria = []objective.Criterion{
		settled("built", objective.CriterionMet, true),
		settled("tested", objective.CriterionInconclusive, true),
	}
	obj := f.declare(t, actingObjective())
	at := f.clock

	f.pass(t, obj.ID, reconcile.TriggerManual)

	got := latestOutcome(t, f, obj.ID).Criteria
	want := []objective.CriterionPass{
		{CriterionID: "built", State: objective.CriterionMet, At: at},
		{CriterionID: "tested", State: objective.CriterionInconclusive, Reason: objective.ReasonVerifierError, At: at},
	}
	if len(got) != len(want) {
		t.Fatalf("outcome criteria = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].CriterionID != want[i].CriterionID || got[i].State != want[i].State ||
			got[i].Reason != want[i].Reason || !got[i].At.Equal(want[i].At) {
			t.Errorf("criteria[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// A pass that settled nothing records nothing: a look, a deferral and an
// escalation leave the outcome's criteria empty, whatever the loop row holds.
func TestAPassThatSettledNothingRecordsNoCriteria(t *testing.T) {
	t.Run("sense only", func(t *testing.T) {
		f := newFixture(t, Config{})
		f.use(t, map[string]string{"git": "aaa"})
		f.loops.criteria = []objective.Criterion{uptime(objective.CriterionMet)}
		obj := sustainedObjective()
		obj.Cadence = &objective.Cadence{Sense: "15m"}
		obj = f.declare(t, obj)

		f.pass(t, obj.ID, "")

		if f.loops.count() != 0 {
			t.Fatalf("a quiet sense ran %d loops, want 0", f.loops.count())
		}
		if got := latestOutcome(t, f, obj.ID).Criteria; len(got) != 0 {
			t.Errorf("a sense-only pass recorded criteria %+v", got)
		}
	})
	t.Run("deferred", func(t *testing.T) {
		f, obj := rateLimitFixture(t, Config{})
		f.loops.criteria = []objective.Criterion{uptime(objective.CriterionNotMet)}

		f.pass(t, obj.ID, reconcile.TriggerManual)

		o := latestOutcome(t, f, obj.ID)
		if o.Deferred == "" {
			t.Fatalf("the pass was not deferred: %+v", o)
		}
		if len(o.Criteria) != 0 {
			t.Errorf("a deferred pass recorded criteria %+v", o.Criteria)
		}
	})
	t.Run("escalated", func(t *testing.T) {
		f, obj := sustainFixture(t)
		f.loops.paused = true
		f.loops.checkpoint = "cp-1"
		f.loops.criteria = []objective.Criterion{uptime(objective.CriterionMet)}

		f.pass(t, obj.ID, reconcile.TriggerManual)

		o := latestOutcome(t, f, obj.ID)
		if !o.Escalated {
			t.Fatalf("the pass did not escalate: %+v", o)
		}
		if len(o.Criteria) != 0 {
			t.Errorf("an escalated pass recorded criteria %+v", o.Criteria)
		}
		if st := f.state(t, obj.ID); len(st.Sustain) != 0 {
			t.Errorf("an escalated pass evaluated sustain %+v", st.Sustain)
		}
	})
}

// (a) The acceptance case: ten recorded passes in the window, one not_met,
// spend exactly the 0.1 budget and the criterion is sustained.
func TestTenPassesWithOneNotMetAreSustained(t *testing.T) {
	f, obj := sustainFixture(t)

	last := f.tenPasses(t, obj.ID, 3)

	checkSustain(t, sustainOf(t, f.state(t, obj.ID), "uptime"), reconcile.CriterionSustain{
		State: objective.CriterionMet, Settled: 10, NotMet: 1, BudgetLeft: 0,
		Latest: objective.CriterionMet, WindowStart: last.Add(-28 * day),
	})
}

// (b) An eleventh pass with a second not_met spends more than the budget.
func TestASecondNotMetOverspendsTheBudget(t *testing.T) {
	f, obj := sustainFixture(t)
	f.tenPasses(t, obj.ID, 3)

	last := f.settlePass(t, obj.ID, objective.CriterionNotMet, 2*day)

	checkSustain(t, sustainOf(t, f.state(t, obj.ID), "uptime"), reconcile.CriterionSustain{
		State: objective.CriterionNotMet, Settled: 11, NotMet: 2, BudgetLeft: 0,
		Latest: objective.CriterionNotMet, WindowStart: last.Add(-28 * day),
	})
}

// (c) An inconclusive pass spends no budget and counts for nothing: settled,
// not_met and budget_left stay where they were, and so does the breaker count
// the not_met pass before it left.
func TestAnInconclusivePassLeavesTheSustainCountsUnchanged(t *testing.T) {
	f, obj := sustainFixture(t)
	f.tenPasses(t, obj.ID, 9)
	before := f.state(t, obj.ID)
	if before.ConsecutiveFailures != 1 {
		t.Fatalf("consecutive_failures = %d after a not_met pass, want 1", before.ConsecutiveFailures)
	}
	was := sustainOf(t, before, "uptime")

	f.settlePass(t, obj.ID, objective.CriterionInconclusive, 2*day)

	after := f.state(t, obj.ID)
	if after.ConsecutiveFailures != before.ConsecutiveFailures {
		t.Errorf("consecutive_failures = %d after an inconclusive pass, want %d", after.ConsecutiveFailures, before.ConsecutiveFailures)
	}
	now := sustainOf(t, after, "uptime")
	checkSustain(t, now, reconcile.CriterionSustain{
		State: objective.CriterionMet, Settled: was.Settled, NotMet: was.NotMet, BudgetLeft: was.BudgetLeft,
		Latest: objective.CriterionInconclusive,
	})
	if was.Settled != 10 || was.NotMet != 1 {
		t.Errorf("before the inconclusive pass settled=%d not_met=%d, want 10/1", was.Settled, was.NotMet)
	}
}

// (d) Two settled passes are too few for the budget to say anything.
func TestTwoSettledPassesAreTooFewSamples(t *testing.T) {
	f, obj := sustainFixture(t)

	f.settlePass(t, obj.ID, objective.CriterionMet, 2*day)
	f.settlePass(t, obj.ID, objective.CriterionMet, 2*day)

	checkSustain(t, sustainOf(t, f.state(t, obj.ID), "uptime"), reconcile.CriterionSustain{
		State: objective.CriterionInconclusive, Reason: objective.ReasonTooFewSamples,
		Settled: 2, NotMet: 0, BudgetLeft: 0.1, Latest: objective.CriterionMet,
	})
}

// (e) The window rolls: a not_met pass counts while it is inside the last 28
// days and drops out once it is older.
func TestAPassOlderThanTheWindowDropsOut(t *testing.T) {
	f, obj := sustainFixture(t)

	f.settlePass(t, obj.ID, objective.CriterionNotMet, 10*day) // day 0
	f.settlePass(t, obj.ID, objective.CriterionMet, 10*day)    // day 10
	f.settlePass(t, obj.ID, objective.CriterionMet, 5*day)     // day 20
	f.settlePass(t, obj.ID, objective.CriterionMet, 5*day)     // day 25

	checkSustain(t, sustainOf(t, f.state(t, obj.ID), "uptime"), reconcile.CriterionSustain{
		State: objective.CriterionNotMet, Settled: 4, NotMet: 1, BudgetLeft: 0,
		Latest: objective.CriterionMet,
	})

	last := f.settlePass(t, obj.ID, objective.CriterionMet, day) // day 30

	checkSustain(t, sustainOf(t, f.state(t, obj.ID), "uptime"), reconcile.CriterionSustain{
		State: objective.CriterionMet, Settled: 4, NotMet: 0, BudgetLeft: 0.1,
		Latest: objective.CriterionMet, WindowStart: last.Add(-28 * day),
	})
}

// (f) Sustain is read, never sensed: a criterion's sustain state moving from
// met to not_met while the fingerprint stands still triggers no run. Drift
// still comes from the fingerprint alone, and a look evaluates nothing.
func TestASustainChangeWithAStillFingerprintIsNotDrift(t *testing.T) {
	f := newFixture(t, Config{BreakerFailures: 100, StallReconciles: 100})
	f.use(t, map[string]string{"git": "aaa"})
	obj := sustainedObjective()
	obj.Cadence = &objective.Cadence{Sense: "15m"}
	obj = f.declare(t, obj)

	for i := 0; i < 3; i++ {
		f.settlePass(t, obj.ID, objective.CriterionMet, day)
	}
	if got := sustainOf(t, f.state(t, obj.ID), "uptime").State; got != objective.CriterionMet {
		t.Fatalf("sustain state = %s after three met passes, want met", got)
	}
	f.settlePass(t, obj.ID, objective.CriterionNotMet, 15*time.Minute)
	was := sustainOf(t, f.state(t, obj.ID), "uptime")
	if was.State != objective.CriterionNotMet {
		t.Fatalf("sustain state = %s after one not_met in four, want not_met", was.State)
	}
	runs := f.loops.count()

	f.pass(t, obj.ID, "")

	if got := f.loops.count(); got != runs {
		t.Errorf("the loop ran %d more times with the fingerprint unchanged, want 0", got-runs)
	}
	o := latestOutcome(t, f, obj.ID)
	if o.LoopID != "" || o.Trigger != "" || o.Drift.Changed {
		t.Errorf("the sense after a sustain change = trigger %q loop %q drift %v, want a quiet look", o.Trigger, o.LoopID, o.Drift.Changed)
	}
	checkSustain(t, sustainOf(t, f.state(t, obj.ID), "uptime"), was)
}

// (g) Sustain sits beside the stall detector and the breaker, not in their
// place: for criteria with no sustain both behave exactly as before, and such
// criteria get no sustain entry.
func TestCriteriaWithoutSustainKeepTheStreakAndBreaker(t *testing.T) {
	plain := func() objective.Objective {
		obj := actingObjective()
		obj.SuccessCriteria = []objective.Criterion{{ID: "built"}}
		return obj
	}

	t.Run("stall detector", func(t *testing.T) {
		f := newFixture(t, Config{StallReconciles: 3, BreakerFailures: 100})
		f.use(t, map[string]string{"git": "aaa"})
		f.loops.status = objective.StatusCompleted
		f.loops.criteriaMet = 0.4
		f.loops.criteria = []objective.Criterion{settled("built", objective.CriterionNotMet, true)}
		obj := f.declare(t, plain())

		// The first pass improves on the 0 the state started at; the next
		// three are flat.
		for i := 0; i < 3; i++ {
			f.pass(t, obj.ID, reconcile.TriggerManual)
			f.clock = f.clock.Add(time.Hour)
		}
		st := f.state(t, obj.ID)
		if st.ScoreStreak != 2 || st.Paused {
			t.Fatalf("score_streak = %d paused = %v after two flat passes, want 2/false", st.ScoreStreak, st.Paused)
		}
		f.pass(t, obj.ID, reconcile.TriggerManual)

		st = f.state(t, obj.ID)
		if !st.Paused || st.ScoreStreak != 3 {
			t.Errorf("score_streak = %d paused = %v after three flat passes, want 3/true", st.ScoreStreak, st.Paused)
		}
		if st.ConsecutiveFailures != 0 {
			t.Errorf("consecutive_failures = %d on a stall, want 0", st.ConsecutiveFailures)
		}
		if len(st.Sustain) != 0 {
			t.Errorf("a criterion with no sustain got sustain state %+v", st.Sustain)
		}
		if got := latestOutcome(t, f, obj.ID).Criteria; len(got) != 1 || got[0].CriterionID != "built" {
			t.Errorf("outcome criteria = %+v, want built's state recorded as for any criterion", got)
		}
	})

	t.Run("circuit breaker", func(t *testing.T) {
		f := newFixture(t, Config{BreakerFailures: 3, StallReconciles: 100})
		f.use(t, map[string]string{"git": "aaa"})
		f.loops.status = objective.StatusFailed
		f.loops.criteria = []objective.Criterion{settled("built", objective.CriterionNotMet, true)}
		obj := f.declare(t, plain())

		for i := 0; i < 3; i++ {
			f.pass(t, obj.ID, reconcile.TriggerManual)
			f.clock = f.clock.Add(time.Hour)
		}

		st := f.state(t, obj.ID)
		if !st.Paused || st.ConsecutiveFailures != 3 {
			t.Errorf("consecutive_failures = %d paused = %v after three not_met passes, want 3/true", st.ConsecutiveFailures, st.Paused)
		}
		if len(st.Sustain) != 0 {
			t.Errorf("a criterion with no sustain got sustain state %+v", st.Sustain)
		}
	})

	t.Run("beside a sustained criterion", func(t *testing.T) {
		f := newFixture(t, Config{BreakerFailures: 100, StallReconciles: 100})
		f.use(t, map[string]string{"git": "aaa"})
		obj := actingObjective()
		obj.SuccessCriteria = []objective.Criterion{{ID: "built"}, {ID: "uptime", Sustain: stepSustain()}}
		obj = f.declare(t, obj)
		f.loops.status = objective.StatusCompleted
		f.loops.criteriaMet = 1.0
		f.loops.criteria = []objective.Criterion{
			settled("built", objective.CriterionMet, true),
			uptime(objective.CriterionMet),
		}

		f.pass(t, obj.ID, reconcile.TriggerManual)

		st := f.state(t, obj.ID)
		if len(st.Sustain) != 1 || st.Sustain[0].CriterionID != "uptime" {
			t.Errorf("sustain = %+v, want one entry, for uptime only", st.Sustain)
		}
		if st.ScoreStreak != 0 || st.LastConvergedAt == nil {
			t.Errorf("score_streak = %d converged = %v, want a converged pass to reset the streak as before", st.ScoreStreak, st.LastConvergedAt != nil)
		}
	})
}
