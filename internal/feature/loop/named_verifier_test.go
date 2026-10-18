package loop

import (
	"context"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 41 step 3: a criterion that names a verifier is settled by that
// verifier and nothing else. When it did not run, the state is
// inconclusive: not_run, and no model is asked; the judge's reply below would
// make it met if it were.
func TestANamedVerifierThatDidNotRunIsNotRunAndNotJudged(t *testing.T) {
	agent := &capturingAgent{reply: "PASS"}
	sc := &stepContext{
		svc:   &serviceImpl{hub: event.NewHub()},
		agent: agent,
		obj: objective.Objective{
			ID: "obj-1",
			SuccessCriteria: []objective.Criterion{{
				ID:          "ci",
				Description: "every required check is green at the pull request's head",
				Verifier:    "software.verify.ci_green",
				Weight:      1.0,
			}},
		},
	}

	// Another capability's success is audited but settles nothing.
	score, allMet := stepVerify(context.Background(), sc, []actionOutcome{
		{CapabilityID: "software.verify.run_tests", Result: environment.ActionResult{Success: true}},
	})

	if allMet || score != 0.0 {
		t.Errorf("a criterion whose verifier never ran scored (%v, %v)", score, allMet)
	}
	if agent.lastTask != "" {
		t.Error("the judge was asked about a criterion that names a verifier")
	}
	got := sc.obj.SuccessCriteria[0]
	if got.State != objective.CriterionInconclusive || got.Reason != objective.ReasonNotRun {
		t.Errorf("state = %q, reason = %q; want inconclusive: not_run", got.State, got.Reason)
	}
}

// A docs-only one-shot whose required criteria all name verifiers completes
// from their results alone, with no judge call for any of them.
func TestADocsOnlyOneShotCompletesWithoutAJudge(t *testing.T) {
	agent := &capturingAgent{reply: "FAIL"}
	sc := &stepContext{
		svc:   &serviceImpl{hub: event.NewHub()},
		agent: agent,
		obj: objective.Objective{
			ID: "obj-docs",
			SuccessCriteria: []objective.Criterion{
				{ID: "pr", Description: "a pull request is open", Verifier: "software.verify.stream_pr", Weight: 1.0},
				{ID: "paths", Description: "the diff touches only docs/**", Verifier: "software.verify.paths_declared", Weight: 1.0},
				{ID: "ci", Description: "checks are green at the head", Verifier: "software.verify.ci_green", Weight: 1.0},
			},
		},
	}
	met := environment.ActionResult{Success: true, StateDelta: map[string]any{"state": "met"}}

	_, allMet := stepVerify(context.Background(), sc, []actionOutcome{
		{CapabilityID: "software.act.write_doc", Result: environment.ActionResult{Success: true}},
		{CapabilityID: "software.verify.stream_pr", Result: met},
		{CapabilityID: "software.verify.paths_declared", Result: met},
		{CapabilityID: "software.verify.ci_green", Result: met},
	})

	if !allMet {
		t.Errorf("a docs pass whose verifiers all reported met did not complete: %+v", sc.obj.SuccessCriteria)
	}
	if agent.lastTask != "" {
		t.Error("the judge was called for a docs-only one-shot")
	}
}

// A discovery pass that opened no pull request leaves stream_pr inconclusive
// with no_data, which is not a failed pass; a judge must not turn it into one.
func TestADiscoveryPassWithNoPullRequestIsInconclusive(t *testing.T) {
	agent := &capturingAgent{reply: "FAIL"}
	sc := &stepContext{
		svc:   &serviceImpl{hub: event.NewHub()},
		agent: agent,
		obj: objective.Objective{
			ID: "obj-discovery",
			SuccessCriteria: []objective.Criterion{
				{ID: "pr", Description: "a pull request on the stream's branch", Verifier: "software.verify.stream_pr", Weight: 1.0},
				{ID: "ci", Description: "checks are green at the head", Verifier: "software.verify.ci_green", Weight: 1.0},
			},
		},
	}
	noPR := environment.ActionResult{StateDelta: map[string]any{"state": "inconclusive", "reason": "no_data"}}

	_, allMet := stepVerify(context.Background(), sc, []actionOutcome{
		{CapabilityID: "software.verify.stream_pr", Result: noPR},
	})

	if allMet {
		t.Error("a pass with no pull request completed")
	}
	for _, c := range sc.obj.SuccessCriteria {
		if c.State == objective.CriterionNotMet {
			t.Errorf("criterion %s is not_met; a pass with no pull request is inconclusive", c.ID)
		}
	}
	if got := sc.obj.SuccessCriteria[1]; got.Reason != objective.ReasonNotRun {
		t.Errorf("ci_green never ran: reason = %q, want not_run", got.Reason)
	}
	if agent.lastTask != "" {
		t.Error("the judge was asked about criteria that name verifiers")
	}
}
