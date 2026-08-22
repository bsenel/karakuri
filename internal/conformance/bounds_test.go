package conformance_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/conformance"
	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
)

// boundsPack is routingPack with agents, so the bounds check has something to
// run and everything else in the suite still passes.
type boundsPack struct {
	routingPack
	agents []agent.Definition
}

func (p *boundsPack) AgentDefinitions() []agent.Definition { return p.agents }

func boundsResult(t *testing.T, p domain.Pack) conformance.Result {
	t.Helper()
	for _, r := range conformance.New().Run(context.Background(), p) {
		if r.Check == "agent_bounds_behave" {
			return r
		}
	}
	t.Fatal("the suite no longer runs agent_bounds_behave")
	return conformance.Result{}
}

// packWith returns a pack declaring one capability and one agent with the
// given bounds.
func packWith(b agent.AuthorityBounds) *boundsPack {
	p := &boundsPack{
		agents: []agent.Definition{{
			ID:           "fixture.agent.one",
			Domain:       "routing",
			Capabilities: []capability.CapabilityID{"routing.act.real"},
			Authority:    b,
		}},
	}
	p.caps = []capability.Capability{routingCap("routing.act.real", false)}
	return p
}

// The declarations that are honest pass — every rung of the ladder, so the
// check cannot be satisfied by escalating everything.
func TestBoundsCheckPassesEachRungOfTheLadder(t *testing.T) {
	for _, tc := range []struct {
		name   string
		bounds agent.AuthorityBounds
	}{
		{"draft and ask", agent.AuthorityBounds{MaxAutonomousActions: 0}},
		{"a cap of three", agent.AuthorityBounds{MaxAutonomousActions: 3}},
		{"unlimited", agent.AuthorityBounds{MaxAutonomousActions: agent.UnlimitedActions}},
		{"a cap and a threshold", agent.AuthorityBounds{MaxAutonomousActions: 5, ConfidenceThreshold: 0.8}},
		{"approval for a declared capability", agent.AuthorityBounds{
			MaxAutonomousActions: agent.UnlimitedActions,
			RequiresApprovalFor:  []capability.CapabilityID{"routing.act.real"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := boundsResult(t, packWith(tc.bounds)); !got.Passed {
				t.Errorf("check failed an honest declaration: %s", got.Message)
			}
		})
	}
}

// And a declaration that names something the pack does not have fails, because
// a list nobody can act on is the same defect one field over.
func TestBoundsCheckCatchesApprovalForAnUndeclaredCapability(t *testing.T) {
	p := packWith(agent.AuthorityBounds{
		MaxAutonomousActions: agent.UnlimitedActions,
		RequiresApprovalFor:  []capability.CapabilityID{"routing.act.imaginary"},
	})
	got := boundsResult(t, p)
	if got.Passed {
		t.Fatal("check passed a pack requiring approval for a capability it does not declare")
	}
	// The acceptance criterion asks for a message naming both.
	if !strings.Contains(got.Message, "fixture.agent.one") || !strings.Contains(got.Message, "routing.act.imaginary") {
		t.Errorf("message %q does not name both the agent and the capability", got.Message)
	}
}

// The check has to run the policy rather than read the field, or it verifies
// nothing. These assert the policy directly: each is a bug that shipped, or
// would have been indistinguishable from one.
func TestTheDecisionPolicyHonoursEachBound(t *testing.T) {
	plan := func(n int) []capability.CapabilityID {
		out := make([]capability.CapabilityID, n)
		for i := range out {
			out[i] = "routing.act.real"
		}
		return out
	}

	t.Run("zero means none, not unlimited", func(t *testing.T) {
		// The bug: `MaxAutonomousActions > 0` meant a zero cap was no cap.
		// Four packs wrote zero to mean "plans but never acts".
		v := agent.AuthorityBounds{MaxAutonomousActions: 0}.Decide(1.0, 0, plan(3))
		if !v.Escalate {
			t.Error("a zero cap ran three actions without asking")
		}
		if v.Allowed != 3 {
			t.Errorf("Allowed = %d, want 3: an escalating plan is shown to the reviewer intact", v.Allowed)
		}
	})

	t.Run("an escalated plan is not emptied", func(t *testing.T) {
		// An approval falls straight through to the act step, so trimming an
		// escalating plan to zero would discard the work that was approved.
		v := agent.AuthorityBounds{
			MaxAutonomousActions: agent.UnlimitedActions,
			ConfidenceThreshold:  0.9,
		}.Decide(0.5, 0.9, plan(4))
		if !v.Escalate {
			t.Fatal("a plan below the threshold did not escalate")
		}
		if v.Allowed != 4 {
			t.Errorf("Allowed = %d, want 4", v.Allowed)
		}
	})

	t.Run("a cap trims to exactly the cap", func(t *testing.T) {
		v := agent.AuthorityBounds{MaxAutonomousActions: 2}.Decide(1.0, 0, plan(5))
		if v.Allowed != 2 {
			t.Errorf("Allowed = %d, want 2", v.Allowed)
		}
		if v.Escalate {
			t.Errorf("trimming to the cap escalated: %s", v.Reason)
		}
	})

	t.Run("unlimited is not trimmed", func(t *testing.T) {
		v := agent.AuthorityBounds{MaxAutonomousActions: agent.UnlimitedActions}.Decide(1.0, 0, plan(50))
		if v.Allowed != 50 {
			t.Errorf("Allowed = %d, want 50", v.Allowed)
		}
	})

	t.Run("a zero threshold opts out rather than escalating everything", func(t *testing.T) {
		v := agent.AuthorityBounds{MaxAutonomousActions: agent.UnlimitedActions}.Decide(0.0, 0, plan(1))
		if v.Escalate {
			t.Errorf("an agent that declared no threshold escalated: %s", v.Reason)
		}
	})

	t.Run("the more specific reason wins", func(t *testing.T) {
		// Both under-confident and bounded to nothing. The operator can act on
		// the confidence; "bounded to no autonomous actions" tells them
		// nothing they can change about this plan.
		v := agent.AuthorityBounds{
			MaxAutonomousActions: 0,
			ConfidenceThreshold:  0.9,
		}.Decide(0.4, 0.9, plan(2))
		if !v.Escalate {
			t.Fatal("did not escalate")
		}
		if !strings.Contains(v.Reason, "confidence") {
			t.Errorf("reason = %q, want the confidence one", v.Reason)
		}
	})

	t.Run("an empty plan is not escalated by a zero cap", func(t *testing.T) {
		// Nothing to ask about. Escalating here would raise a checkpoint on a
		// plan with no actions in it.
		v := agent.AuthorityBounds{MaxAutonomousActions: 0}.Decide(1.0, 0, nil)
		if v.Escalate {
			t.Errorf("an empty plan escalated: %s", v.Reason)
		}
	})
}
