package loop

import (
	"context"
	"strings"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// A negated verdict must not score as a positive one.
//
// The old parser searched the whole reply for "pass", "met", "approved" or
// "yes" and returned true on any hit. "This does not pass" contains "pass".
// "The criterion is not met" contains "met". Both scored the criterion as
// satisfied — a scoring bug failing in the worst available direction, since it
// turns a model's rejection into a completed objective.
func TestVerdictDoesNotInvertOnNegation(t *testing.T) {
	for _, reply := range []string{
		"FAIL",
		"fail",
		"This does not pass.",
		"The criterion is not met.",
		"No — the results show nothing of the kind.",
		"The proposal doesn't name any telemetry, so this fails.",
		"There is no evidence the pull request was opened.",
		"Insufficient: the action failed.",
		"unmet",
	} {
		t.Run(reply, func(t *testing.T) {
			if verdictIsPass(reply) {
				t.Errorf("%q scored as met", reply)
			}
		})
	}
}

// And a genuine pass still passes, or the fix is just "always fail".
func TestVerdictAcceptsAPass(t *testing.T) {
	for _, reply := range []string{
		"PASS",
		"pass",
		"Pass.",
		"yes",
		"met",
		"PASS — the results show the pull request URL.",
	} {
		t.Run(reply, func(t *testing.T) {
			if !verdictIsPass(reply) {
				t.Errorf("%q scored as unmet", reply)
			}
		})
	}
}

// An answer nobody can parse is a FAIL. Scoring a criterion met on an empty or
// unintelligible reply is the silent success this codebase keeps finding.
func TestVerdictFailsOnAnUnparseableAnswer(t *testing.T) {
	for _, reply := range []string{"", "   ", "\n", "???", "42"} {
		if verdictIsPass(reply) {
			t.Errorf("%q scored as met", reply)
		}
	}
}

// ── the judge sees the evidence ──────────────────────────────────────────────

// capturingAgent records the task it was asked and answers as told.
type capturingAgent struct {
	lastTask string
	reply    string
}

func (a *capturingAgent) Run(_ context.Context, in coreagent.Input) (coreagent.Output, error) {
	a.lastTask = in.Task
	return coreagent.Output{Content: a.reply}, nil
}

func (a *capturingAgent) Stream(context.Context, coreagent.Input) (<-chan coreagent.OutputChunk, error) {
	ch := make(chan coreagent.OutputChunk)
	close(ch)
	return ch, nil
}

// The prompt said "based on the action results" and the results were never
// included: WorldState and Memory were nil and the results parameter was
// accepted and ignored. Every unverifiable criterion in every domain was
// scored by a model shown nothing but the criterion's own description.
func TestTheJudgeIsShownWhatTheActionsProduced(t *testing.T) {
	agent := &capturingAgent{reply: "PASS"}
	sc := &stepContext{
		svc:   &serviceImpl{hub: event.NewHub()},
		agent: agent,
		obj:   objective.Objective{ID: "obj-1"},
	}

	outcomes := []actionOutcome{{
		CapabilityID: "software.act.create_pr",
		Result: environment.ActionResult{
			Success:    true,
			StateDelta: map[string]any{"pr_url": "https://example.invalid/pr/7"},
		},
	}, {
		CapabilityID: "software.act.write_code",
		Result:       environment.ActionResult{Success: false, Error: "no worktree"},
	}}

	evaluateWithAgent(context.Background(), sc, objective.Criterion{
		Description: "A pull request is open with the change",
	}, outcomes)

	for _, want := range []string{
		"software.act.create_pr",       // which capability ran
		"https://example.invalid/pr/7", // what it produced
		"software.act.write_code",
		"no worktree", // and what failed, with its reason
		"A pull request is open with the change",
	} {
		if !strings.Contains(agent.lastTask, want) {
			t.Errorf("the judge was not shown %q\n--- task ---\n%s", want, agent.lastTask)
		}
	}
}

// With no actions there is nothing to judge, and asking anyway invites a model
// to reason from the criterion's plausibility — which is how an objective
// scores well for having done nothing.
func TestTheJudgeIsNotAskedAboutAnEmptyIteration(t *testing.T) {
	agent := &capturingAgent{reply: "PASS"}
	sc := &stepContext{
		svc:   &serviceImpl{hub: event.NewHub()},
		agent: agent,
		obj:   objective.Objective{ID: "obj-1"},
	}

	met := evaluateWithAgent(context.Background(), sc, objective.Criterion{
		Description: "something plausible happened",
	}, nil)

	if met {
		t.Error("a criterion was met with no actions at all")
	}
	if agent.lastTask != "" {
		t.Error("the agent was asked to judge an iteration that did nothing")
	}
}

// ── verifier matching ────────────────────────────────────────────────────────

// A criterion verified by a capability is settled by that capability's result,
// not by any action succeeding.
//
// The old rule was "if the verifier ID contains run_tests or lint, the
// criterion is met when *any* action succeeded" — so a criterion about the
// test suite was satisfied by an unrelated send_message, and every other
// verifier fell through to a model that had been shown no results.
func TestACriterionIsSettledByItsOwnVerifier(t *testing.T) {
	// The agent must not be consulted when the verifier ran; if it is, the
	// reply below would flip the answer and the test would catch it.
	agent := &capturingAgent{reply: "PASS"}
	sc := &stepContext{
		svc:   &serviceImpl{hub: event.NewHub()},
		agent: agent,
		obj: objective.Objective{
			ID: "obj-1",
			SuccessCriteria: []objective.Criterion{{
				ID:          "tests-pass",
				Description: "the test suite passes",
				Verifier:    "software.verify.run_tests",
				Weight:      1.0,
			}},
		},
	}

	score, allMet := stepVerify(context.Background(), sc, []actionOutcome{
		// An unrelated success, which used to be enough.
		{CapabilityID: "software.act.send_message", Result: environment.ActionResult{Success: true}},
		// And the verifier itself, failing.
		{CapabilityID: "software.verify.run_tests", Result: environment.ActionResult{Success: false, Error: "3 tests failed"}},
	})

	if allMet || score != 0.0 {
		t.Errorf("a failing test run scored (%v, %v); an unrelated success satisfied the criterion", score, allMet)
	}
	if agent.lastTask != "" {
		t.Error("the agent was consulted for a criterion its own verifier settled")
	}
}

// One failing run is a failing run, however many passed beside it.
func TestOneFailingRunOfTheVerifierIsEnoughToFail(t *testing.T) {
	sc := &stepContext{
		svc:   &serviceImpl{hub: event.NewHub()},
		agent: &capturingAgent{reply: "PASS"},
		obj: objective.Objective{
			ID: "obj-1",
			SuccessCriteria: []objective.Criterion{{
				ID: "tests-pass", Verifier: "software.verify.run_tests", Weight: 1.0,
			}},
		},
	}
	_, allMet := stepVerify(context.Background(), sc, []actionOutcome{
		{CapabilityID: "software.verify.run_tests", Result: environment.ActionResult{Success: true}},
		{CapabilityID: "software.verify.run_tests", Result: environment.ActionResult{Success: false}},
	})
	if allMet {
		t.Error("a failing run was outvoted by a passing one")
	}
}

// The verifier succeeding settles it the other way too, so the rule is not
// "always fail".
func TestTheVerifierSucceedingMeetsTheCriterion(t *testing.T) {
	agent := &capturingAgent{reply: "FAIL"} // must not be consulted
	sc := &stepContext{
		svc:   &serviceImpl{hub: event.NewHub()},
		agent: agent,
		obj: objective.Objective{
			ID: "obj-1",
			SuccessCriteria: []objective.Criterion{{
				ID: "tests-pass", Verifier: "software.verify.run_tests", Weight: 1.0,
			}},
		},
	}
	score, allMet := stepVerify(context.Background(), sc, []actionOutcome{
		{CapabilityID: "software.verify.run_tests", Result: environment.ActionResult{Success: true}},
	})
	if !allMet || score != 1.0 {
		t.Errorf("a passing verifier scored (%v, %v)", score, allMet)
	}
}

// A verifier that never ran falls to the agent — which now sees the outcomes,
// so it is a judgement rather than a guess. It must not be treated as met by
// default.
func TestAVerifierThatNeverRanIsJudgedNotAssumed(t *testing.T) {
	agent := &capturingAgent{reply: "FAIL"}
	sc := &stepContext{
		svc:   &serviceImpl{hub: event.NewHub()},
		agent: agent,
		obj: objective.Objective{
			ID: "obj-1",
			SuccessCriteria: []objective.Criterion{{
				ID: "pr-open", Verifier: "software.act.create_pr", Weight: 1.0,
			}},
		},
	}
	_, allMet := stepVerify(context.Background(), sc, []actionOutcome{
		{CapabilityID: "software.act.send_message", Result: environment.ActionResult{Success: true}},
	})
	if allMet {
		t.Error("a criterion whose verifier never ran was met anyway")
	}
	if agent.lastTask == "" {
		t.Error("the agent was not asked about a criterion nothing settled")
	}
}
