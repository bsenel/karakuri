package reconcile

import (
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// Phase 41 step 7 part B: a relative target with no baseline blocks
// convergence and is not a failed pass, so passes that cannot compare with a
// baseline never trip the breaker (docs/roadmap.md, Phase 41 decision 3
// 'Baselines').
func TestANoBaselinePassLeavesTheBreakerCountUnchanged(t *testing.T) {
	f := newFixture(t, Config{BreakerFailures: 2, StallReconciles: 100})
	f.use(t, map[string]string{"git": "aaa"})
	f.loops.status = objective.StatusFailed
	f.loops.criteriaMet = 0.5
	f.loops.criteria = []objective.Criterion{
		settled("built", objective.CriterionMet, true),
		{
			ID: "latency", Kind: objective.KindMeasured, Change: objective.ChangePercent,
			Baseline: &objective.Baseline{AtStart: true},
			State:    objective.CriterionInconclusive, Reason: objective.ReasonNoBaseline,
		},
	}
	obj := f.declare(t, actingObjective())

	for i := 0; i < 3; i++ {
		f.pass(t, obj.ID, reconcile.TriggerManual)
		f.clock = f.clock.Add(time.Hour)
	}

	st := f.state(t, obj.ID)
	if st.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d after three no_baseline passes, want 0", st.ConsecutiveFailures)
	}
	if st.Paused {
		t.Errorf("the breaker tripped on passes with no baseline to compare with: %s", st.PausedReason)
	}
	if st.LastConvergedAt != nil {
		t.Error("an objective with a required criterion inconclusive: no_baseline was recorded as converged")
	}
}
