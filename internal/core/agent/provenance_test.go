package agent_test

import (
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
)

func plan(n int) []capability.CapabilityID {
	out := make([]capability.CapabilityID, n)
	for i := range out {
		out[i] = "test.act.real"
	}
	return out
}

// The acceptance property, stated at the level the policy answers it: the same
// plan, the same confidence, the same bounds — and the only difference is where
// the material came from.
func TestSamePlanEscalatesOnUntrustedEvidenceAndNotOtherwise(t *testing.T) {
	// "act" autonomy: an agent that has earned the right to act unsupervised is
	// exactly the one this bound is for. An agent that escalates everything
	// anyway would prove nothing.
	bounds := agent.AuthorityBounds{MaxAutonomousActions: agent.UnlimitedActions}

	trusted := bounds.Decide(0.95, 0, plan(2), agent.Evidence{})
	if trusted.Escalate {
		t.Fatalf("a plan built from the operator's own infrastructure escalated: %s", trusted.Reason)
	}

	untrusted := bounds.Decide(0.95, 0, plan(2), agent.Evidence{}.WithSource("software.env.communication"))
	if !untrusted.Escalate {
		t.Fatal("a plan built from somebody else's writing ran without asking")
	}
	if !strings.Contains(untrusted.Reason, "software.env.communication") {
		t.Errorf("reason = %q, want it to name the source a reviewer has to go and read", untrusted.Reason)
	}
	if untrusted.Allowed != 2 {
		t.Errorf("Allowed = %d, want 2: an approval falls through to act, so the plan must reach the reviewer intact", untrusted.Allowed)
	}
}

// Earned autonomy does not buy an exemption. This is the whole difference
// between provenance and the other bounds: a cap and a threshold are things an
// agent can be trusted out of, and this is not one of them.
func TestUntrustedEvidenceEscalatesAtEveryLevelOfAutonomy(t *testing.T) {
	ev := agent.Evidence{}.WithSource("software.env.research")
	for _, tc := range []struct {
		name   string
		bounds agent.AuthorityBounds
	}{
		{"unlimited", agent.AuthorityBounds{MaxAutonomousActions: agent.UnlimitedActions}},
		{"a generous cap", agent.AuthorityBounds{MaxAutonomousActions: 20}},
		{"no threshold", agent.AuthorityBounds{MaxAutonomousActions: 5, ConfidenceThreshold: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := tc.bounds.Decide(1.0, 0, plan(1), ev)
			if !v.Escalate {
				t.Errorf("%s ran a plan drafted from untrusted material without asking", tc.name)
			}
		})
	}
}

// Provenance wins over confidence when both apply, because it is the reason an
// operator can act on: they can go and read the named source. "The model was
// only 40%% sure" tells them nothing they can check.
func TestProvenanceIsTheReasonWhenConfidenceAlsoFails(t *testing.T) {
	v := agent.AuthorityBounds{
		MaxAutonomousActions: agent.UnlimitedActions,
		ConfidenceThreshold:  0.9,
	}.Decide(0.4, 0.9, plan(2), agent.Evidence{}.WithSource("software.env.git"))

	if !v.Escalate {
		t.Fatal("did not escalate")
	}
	if !strings.Contains(v.Reason, "software.env.git") {
		t.Errorf("reason = %q, want the provenance one", v.Reason)
	}
}

// Nothing to approve. Drafting is not acting, and a checkpoint proposing no
// actions is noise a reviewer cannot answer — the same guard the zero cap has.
func TestUntrustedEvidenceWithNothingPlannedDoesNotEscalate(t *testing.T) {
	v := agent.AuthorityBounds{MaxAutonomousActions: agent.UnlimitedActions}.
		Decide(1.0, 0, nil, agent.Evidence{}.WithSource("software.env.git"))
	if v.Escalate {
		t.Errorf("an empty plan escalated on provenance: %s", v.Reason)
	}
}

// A cap still trims an escalating plan, because an approval falls through to
// act and the reviewer must be shown what will actually run.
func TestProvenanceEscalationStillRespectsTheCap(t *testing.T) {
	v := agent.AuthorityBounds{MaxAutonomousActions: 2}.
		Decide(1.0, 0, plan(5), agent.Evidence{}.WithSource("software.env.git"))
	if !v.Escalate {
		t.Fatal("did not escalate")
	}
	if v.Allowed != 2 {
		t.Errorf("Allowed = %d, want 2 — the reviewer approves the plan that will run", v.Allowed)
	}
}

// Evidence is a set, and its order does not depend on which environment the
// loop happened to reach first. An escalation reason that reordered itself
// between runs would be undiffable in the audit log.
func TestEvidenceDeduplicatesAndSorts(t *testing.T) {
	ev := agent.Evidence{}.
		WithSource("software.env.research").
		WithSource("software.env.git").
		WithSource("software.env.research").
		WithSource("")

	if got := len(ev.ThirdParty); got != 2 {
		t.Fatalf("sources = %v, want two distinct ones", ev.ThirdParty)
	}
	if ev.ThirdParty[0] != "software.env.git" || ev.ThirdParty[1] != "software.env.research" {
		t.Errorf("sources = %v, want them sorted", ev.ThirdParty)
	}
}

// WithSource returns a copy. The loop threads one Evidence value through six
// steps, and a policy that could widen its caller's evidence would be a second
// gate in the one place ADR 015 says there must only ever be one.
func TestWithSourceDoesNotMutateItsReceiver(t *testing.T) {
	base := agent.Evidence{}.WithSource("software.env.git")
	_ = base.WithSource("software.env.research")

	if len(base.ThirdParty) != 1 {
		t.Errorf("the original evidence grew to %v", base.ThirdParty)
	}
}
