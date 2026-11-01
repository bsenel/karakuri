package reconcile

import (
	"context"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// Phase 41 step 8 part C: a person's change through the API restarts a
// criterion's sustain window, and the criterion records when as SustainSince
// (docs/roadmap.md, Phase 41 section 6 'Targets belong to their author').
// Reconcile reads it from the stored objective on the next pass and passes it
// to Sustain.Evaluate as since, so the passes recorded before the restart are
// out of the counts and the window starts at max(now - window, since).

// restartSustain records on the stored objective that a person's change
// restarted uptime's window at the given time, as SetBaseline does through the
// API. The service's clock is the fixture's, not the wall clock SetBaseline
// reads, so the stored objective is written directly.
func (f *fixture) restartSustain(t *testing.T, id objective.ObjectiveID, at time.Time) {
	t.Helper()
	obj, err := f.store.GetObjective(context.Background(), id)
	if err != nil {
		t.Fatalf("get objective: %v", err)
	}
	for i := range obj.SuccessCriteria {
		if obj.SuccessCriteria[i].ID == "uptime" {
			obj.SuccessCriteria[i].SustainSince = &at
		}
	}
	if err := f.store.SaveObjective(context.Background(), obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}
}

// Ten passes with one not_met, then a person's change: the next three met
// passes are all the window holds, so the old not_met no longer spends the
// budget, and the window starts at the restart rather than 28 days back.
func TestPassesBeforeARestartAreOutOfTheSustainCounts(t *testing.T) {
	f, obj := sustainFixture(t)
	f.tenPasses(t, obj.ID, 3)
	checkSustain(t, sustainOf(t, f.state(t, obj.ID), "uptime"), reconcile.CriterionSustain{
		State: objective.CriterionMet, Settled: 10, NotMet: 1, BudgetLeft: 0, Latest: objective.CriterionMet,
	})

	restart := f.clock.Add(-day)
	f.restartSustain(t, obj.ID, restart)
	f.settlePass(t, obj.ID, objective.CriterionMet, 2*day)
	f.settlePass(t, obj.ID, objective.CriterionMet, 2*day)
	f.settlePass(t, obj.ID, objective.CriterionMet, 2*day)

	checkSustain(t, sustainOf(t, f.state(t, obj.ID), "uptime"), reconcile.CriterionSustain{
		State: objective.CriterionMet, Settled: 3, NotMet: 0, BudgetLeft: 0.1,
		Latest: objective.CriterionMet, WindowStart: restart,
	})
}

// The restart is a fresh window, so min_passes applies to it from zero: two
// passes since the change are too few, though the window held ten before it.
func TestTwoPassesAfterARestartAreTooFewSamples(t *testing.T) {
	f, obj := sustainFixture(t)
	f.tenPasses(t, obj.ID, -1)

	restart := f.clock.Add(-day)
	f.restartSustain(t, obj.ID, restart)
	f.settlePass(t, obj.ID, objective.CriterionMet, 2*day)
	f.settlePass(t, obj.ID, objective.CriterionMet, 2*day)

	checkSustain(t, sustainOf(t, f.state(t, obj.ID), "uptime"), reconcile.CriterionSustain{
		State: objective.CriterionInconclusive, Reason: objective.ReasonTooFewSamples,
		Settled: 2, NotMet: 0, BudgetLeft: 0.1, Latest: objective.CriterionMet, WindowStart: restart,
	})
}

// A not_met pass after the restart counts as any pass does: one in three
// overspends the 0.1 budget.
func TestANotMetAfterARestartSpendsTheFreshBudget(t *testing.T) {
	f, obj := sustainFixture(t)
	f.tenPasses(t, obj.ID, -1)

	restart := f.clock.Add(-day)
	f.restartSustain(t, obj.ID, restart)
	f.settlePass(t, obj.ID, objective.CriterionMet, 2*day)
	f.settlePass(t, obj.ID, objective.CriterionNotMet, 2*day)
	f.settlePass(t, obj.ID, objective.CriterionMet, 2*day)

	checkSustain(t, sustainOf(t, f.state(t, obj.ID), "uptime"), reconcile.CriterionSustain{
		State: objective.CriterionNotMet, Settled: 3, NotMet: 1, BudgetLeft: 0,
		Latest: objective.CriterionMet, WindowStart: restart,
	})
}

// A restart older than the window changes nothing: the window is the last 28
// days, as it was before.
func TestARestartOlderThanTheWindowLeavesTheRollingWindow(t *testing.T) {
	f, obj := sustainFixture(t)
	f.restartSustain(t, obj.ID, f.clock.Add(-60*day))

	last := f.tenPasses(t, obj.ID, 3)

	checkSustain(t, sustainOf(t, f.state(t, obj.ID), "uptime"), reconcile.CriterionSustain{
		State: objective.CriterionMet, Settled: 10, NotMet: 1, BudgetLeft: 0,
		Latest: objective.CriterionMet, WindowStart: last.Add(-28 * day),
	})
}
