package loop

import (
	"context"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 44 step 4: a run_tests run killed at its time limit reports
// inconclusive: timeout, with the packages that finished. Running out of time
// says nothing about the work, so its criterion stays inconclusive with that
// reason; before this step the same run was a failure and the criterion was
// not_met. The judge is not asked either.
func TestARunTestsTimeoutLeavesItsCriterionInconclusiveTimeout(t *testing.T) {
	judge := &scriptedAgent{scripted: []coreagent.Output{{Content: "FAIL"}}}
	sc := &stepContext{
		svc:   &serviceImpl{hub: event.NewHub()},
		agent: judge,
		obj: objective.Objective{
			ID: "obj-timeout",
			SuccessCriteria: []objective.Criterion{
				{ID: "tests", Description: "go test ./... passes", Verifier: "software.verify.run_tests", Weight: 1},
			},
		},
	}
	timedOut := environment.ActionResult{
		Success: false,
		Error:   "command timed out after 10m0s",
		StateDelta: map[string]any{
			"verifier": "software.verify.run_tests",
			"status":   "timeout",
			"state":    string(objective.CriterionInconclusive),
			"reason":   string(objective.ReasonTimeout),
			"finished_packages": []map[string]any{
				{"package": "example.com/a", "result": "pass"},
			},
		},
	}

	score, allMet := stepVerify(context.Background(), sc, []actionOutcome{
		{CapabilityID: "software.verify.run_tests", Result: timedOut},
	})

	if allMet || score != 0 {
		t.Errorf("a timed-out run scored (%v, %v)", score, allMet)
	}
	if judge.calls != 0 {
		t.Errorf("the judge was asked %d times about a verifier that reported", judge.calls)
	}
	c := sc.obj.SuccessCriteria[0]
	if c.State != objective.CriterionInconclusive || c.Reason != objective.ReasonTimeout {
		t.Errorf("state=%q reason=%q, want inconclusive/timeout, not not_met", c.State, c.Reason)
	}
}
