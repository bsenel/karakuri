package loop

import (
	"context"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// TestStepVerify_NoCriteriaFailsHonestly checks that an objective
// without SuccessCriteria no longer auto-completes with score 1.0.
// Previously this path returned (1.0, true), which is how the
// Phase 13.5 dogfood objective got status=completed despite all 17
// of its actions being no-ops. Honest failure: score 0, allMet false,
// reason recorded on the step_completed payload.
func TestStepVerify_NoCriteriaFailsHonestly(t *testing.T) {
	svc := &serviceImpl{hub: event.NewHub()}
	sc := &stepContext{
		svc: svc,
		obj: objective.Objective{
			ID:              objective.ObjectiveID("obj-1"),
			SuccessCriteria: nil, // no criteria — used to trivially pass
		},
	}
	score, allMet := stepVerify(context.Background(), sc, []actionOutcome{
		// Even with a successful action, no criteria = no truth.
		{CapabilityID: "test.act.thing", Result: environment.ActionResult{Success: true}},
	})
	if score != 0.0 {
		t.Errorf("expected score=0.0 when no criteria defined, got %v", score)
	}
	if allMet {
		t.Errorf("expected allMet=false when no criteria defined, got true")
	}
}

// TestStepVerify_NoCriteriaIgnoresActionSuccess proves the result
// payload is irrelevant when criteria are absent — the no-criteria
// path must fail regardless of what happened upstream.
func TestStepVerify_NoCriteriaIgnoresActionSuccess(t *testing.T) {
	svc := &serviceImpl{hub: event.NewHub()}
	sc := &stepContext{
		svc: svc,
		obj: objective.Objective{
			ID:              objective.ObjectiveID("obj-2"),
			SuccessCriteria: nil,
		},
	}
	ok := func(success bool) actionOutcome {
		return actionOutcome{CapabilityID: "test.act.thing", Result: environment.ActionResult{Success: success}}
	}
	cases := []struct {
		name     string
		outcomes []actionOutcome
	}{
		{"all success", []actionOutcome{ok(true), ok(true)}},
		{"all failure", []actionOutcome{ok(false), ok(false)}},
		{"empty results", []actionOutcome{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			score, allMet := stepVerify(context.Background(), sc, c.outcomes)
			if score != 0.0 || allMet {
				t.Errorf("expected (0.0, false) regardless of results, got (%v, %v)", score, allMet)
			}
		})
	}
}
