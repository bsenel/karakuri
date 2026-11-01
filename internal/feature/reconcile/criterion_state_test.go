package reconcile

import (
	"context"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// Phase 41 Step 1: only a required criterion that ended not_met is a failed
// pass. One that could not be settled blocks convergence, and is not counted
// towards the circuit breaker.

func optional() *bool { no := false; return &no }

func settled(id string, state objective.CriterionState, required bool) objective.Criterion {
	c := objective.Criterion{ID: id, State: state}
	if state == objective.CriterionInconclusive {
		c.Reason = objective.ReasonVerifierError
	}
	if !required {
		c.Required = optional()
	}
	return c
}

func actingObjective() objective.Objective {
	return objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	}
}

// The acceptance case. Passes whose only unmet required criterion could not
// be settled leave the failure count where it was, however many of them there
// are, so a verifier that is down does not trip the breaker on work that may
// well be done. The optional not_met criterion beside it counts for nothing.
func TestAnInconclusivePassLeavesTheBreakerCountUnchanged(t *testing.T) {
	f := newFixture(t, Config{BreakerFailures: 2, StallReconciles: 100})
	f.use(t, map[string]string{"git": "aaa"})
	f.loops.status = objective.StatusFailed
	f.loops.criteriaMet = 1.0 / 3
	f.loops.criteria = []objective.Criterion{
		settled("built", objective.CriterionMet, true),
		settled("tested", objective.CriterionInconclusive, true),
		settled("tidy", objective.CriterionNotMet, false),
	}
	obj := f.declare(t, actingObjective())

	for i := 0; i < 3; i++ {
		f.pass(t, obj.ID, reconcile.TriggerManual)
		f.clock = f.clock.Add(time.Hour)
	}

	st := f.state(t, obj.ID)
	if st.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d after three inconclusive passes, want 0", st.ConsecutiveFailures)
	}
	if st.Paused {
		t.Errorf("the breaker tripped on passes nobody could settle: %s", st.PausedReason)
	}
	// Not a failure, and not convergence either: an inconclusive required
	// criterion still blocks it.
	if st.LastConvergedAt != nil {
		t.Error("an objective with a required criterion inconclusive was recorded as converged")
	}
}

// The other half: a required criterion that ended not_met is a failed pass,
// with or without an inconclusive one beside it, and a failed loop that
// recorded no criterion states at all is counted as it always was.
func TestANotMetRequiredCriterionStillCountsTowardsTheBreaker(t *testing.T) {
	for _, tc := range []struct {
		name     string
		criteria []objective.Criterion
	}{
		{"not met beside inconclusive", []objective.Criterion{
			settled("built", objective.CriterionNotMet, true),
			settled("tested", objective.CriterionInconclusive, true),
		}},
		{"no states recorded", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, Config{BreakerFailures: 3, StallReconciles: 100})
			f.use(t, map[string]string{"git": "aaa"})
			f.loops.status = objective.StatusFailed
			f.loops.criteria = tc.criteria
			obj := f.declare(t, actingObjective())

			f.pass(t, obj.ID, reconcile.TriggerManual)

			if st := f.state(t, obj.ID); st.ConsecutiveFailures != 1 {
				t.Errorf("consecutive_failures = %d, want 1", st.ConsecutiveFailures)
			}
		})
	}
}

// Completion is every required criterion met. A pass that met them all has
// converged, though an optional criterion beside them held the weighted score
// below 1.0.
func TestAPassThatMetEveryRequiredCriterionConverges(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	f.loops.status = objective.StatusCompleted
	f.loops.criteriaMet = 0.5
	f.loops.criteria = []objective.Criterion{
		settled("tested", objective.CriterionMet, true),
		settled("tidy", objective.CriterionNotMet, false),
	}
	obj := f.declare(t, actingObjective())

	f.pass(t, obj.ID, reconcile.TriggerManual)

	st := f.state(t, obj.ID)
	if st.LastConvergedAt == nil {
		t.Error("a pass that met every required criterion did not converge")
	}
	got, err := f.store.GetObjective(context.Background(), obj.ID)
	if err != nil {
		t.Fatalf("get objective: %v", err)
	}
	if got.Status != objective.StatusConverged {
		t.Errorf("objective status = %q, want %q", got.Status, objective.StatusConverged)
	}
}
