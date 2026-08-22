package loop

import (
	"context"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// decideFixture builds a step context whose only interesting property is the
// agent's authority. Confidence is deliberately high and the threshold zero,
// so nothing except the action cap can cause an escalation — otherwise a
// passing test would not tell us which bound did the work.
func decideFixture(t *testing.T, bounds coreagent.AuthorityBounds) *stepContext {
	t.Helper()
	svc, _, _ := newResumeFixture(t)
	return &stepContext{
		agentDef: coreagent.Definition{
			ID:                "test-agent",
			ReasoningStrategy: coreagent.ReasoningChainOfThought,
			Authority:         bounds,
		},
		obj:       objective.Objective{ID: "obj-1", Title: "test objective"},
		twinID:    "twin-1",
		loopID:    "loop-1",
		iteration: 1,
		svc:       svc,
		state: &loopState{
			id:         "loop-1",
			decisionCh: make(chan corecheckpoint.Decision, 1),
		},
	}
}

func threeActions() plan {
	return plan{
		Confidence: 0.99,
		Actions: []plannedAction{
			{CapabilityID: "code.write"},
			{CapabilityID: "code.write"},
			{CapabilityID: "test.run"},
		},
	}
}

// The bug this pins: MaxAutonomousActions was declared 0 all over the
// codebase to mean "plans but never acts", and the decide step read 0 as "no
// cap" — so the bound did nothing at all. Every existing test asserted the
// field was 0; none asserted what 0 caused, which is exactly how it survived.
//
// The reconcile supervisor's propose level and the karakuri maintainer's
// "cannot act unsupervised" guarantee both rest on this.
func TestZeroAutonomousActionsEscalatesRatherThanActing(t *testing.T) {
	sc := decideFixture(t, coreagent.AuthorityBounds{
		MaxAutonomousActions: 0,
		ConfidenceThreshold:  0, // not the bound under test
	})

	got, paused := stepDecide(context.Background(), sc, threeActions(), nil)

	if !paused {
		t.Fatal("an agent bounded to zero autonomous actions acted without asking")
	}
	// The plan must survive intact. An approval falls straight through to
	// act, so a plan emptied on the way to the checkpoint would silently
	// discard the very work the reviewer approved — and the checkpoint
	// would show them nothing to approve in the first place.
	if len(got.Actions) != 3 {
		t.Errorf("escalated plan carries %d actions, want all 3 for the reviewer to see", len(got.Actions))
	}
}

// The opposite bound, so the fix cannot be "escalate everything".
func TestUnlimitedActionsDoesNotEscalate(t *testing.T) {
	sc := decideFixture(t, coreagent.AuthorityBounds{
		MaxAutonomousActions: coreagent.UnlimitedActions,
		ConfidenceThreshold:  0,
	})

	got, paused := stepDecide(context.Background(), sc, threeActions(), nil)

	if paused {
		t.Error("an agent with an explicit unlimited cap was made to ask")
	}
	if len(got.Actions) != 3 {
		t.Errorf("actions trimmed to %d under an unlimited cap", len(got.Actions))
	}
}

// A positive cap keeps its existing meaning: trim to the cap and carry on
// without asking. This is the behaviour the fix had to leave alone.
func TestPositiveCapStillTrimsWithoutEscalating(t *testing.T) {
	sc := decideFixture(t, coreagent.AuthorityBounds{
		MaxAutonomousActions: 2,
		ConfidenceThreshold:  0,
	})

	got, paused := stepDecide(context.Background(), sc, threeActions(), nil)

	if paused {
		t.Error("a plan within reach of its cap escalated")
	}
	if len(got.Actions) != 2 {
		t.Errorf("actions = %d, want 2 (trimmed to the cap)", len(got.Actions))
	}
}

// An empty plan under a zero cap has nothing to ask about. Escalating here
// would raise a checkpoint proposing nothing, which is noise a reviewer
// cannot act on.
func TestZeroCapWithNothingPlannedDoesNotEscalate(t *testing.T) {
	sc := decideFixture(t, coreagent.AuthorityBounds{
		MaxAutonomousActions: 0,
		ConfidenceThreshold:  0,
	})

	if _, paused := stepDecide(context.Background(), sc, plan{Confidence: 0.99}, nil); paused {
		t.Error("an empty plan raised a checkpoint with nothing to approve")
	}
}
