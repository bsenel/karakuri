package agent_test

import (
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// Phase 44 step 3, "plan from what is delivered": a plan whose write names
// work an observation says is already delivered goes to a human with the
// reason, and a plan for work nobody delivered runs as before.

// stepFiveDelivered is the evidence of an iteration whose observation says
// phase 44 step 5 is ticked on pull request #12.
func stepFiveDelivered() agent.Evidence {
	return agent.Evidence{Delivered: []environment.DeliveredWork{
		{Ref: "phase 44 step 5", Where: "pull request #12", SHAs: []string{"abc1234"}},
	}}
}

// writeCode is a one-action plan of a capability that writes files.
var writeCode = []capability.CapabilityID{"software.act.write_code"}

func TestPlanThatWritesADeliveredStepEscalatesWithTheRedoReason(t *testing.T) {
	bounds := agent.AuthorityBounds{MaxAutonomousActions: agent.UnlimitedActions}

	for _, text := range []string{
		"Deliver phase 44 step 5: mark the group ready.",
		// Case does not make it a different step.
		"Implement Phase 44 Step 5 of docs/roadmap.md",
	} {
		v := bounds.Decide(1.0, 0, writeCode, []string{text}, stepFiveDelivered())
		if !v.Escalate {
			t.Errorf("a plan writing %q ran although that step is ticked", text)
			continue
		}
		for _, want := range []string{"redoes delivered work", "phase 44 step 5", "pull request #12", "abc1234"} {
			if !strings.Contains(v.Reason, want) {
				t.Errorf("reason %q does not name %q", v.Reason, want)
			}
		}
		// Shown intact to the reviewer, like every other escalation.
		if v.Allowed != 1 {
			t.Errorf("Allowed = %d, want the whole plan", v.Allowed)
		}
	}
}

func TestPlanThatWritesAStepNobodyDeliveredDoesNotEscalate(t *testing.T) {
	bounds := agent.AuthorityBounds{MaxAutonomousActions: agent.UnlimitedActions}

	for name, writes := range map[string][]string{
		"another step": {"Deliver phase 44 step 6: plan from what is delivered."},
		// Word boundaries: step 50 is not step 5.
		"a step whose number starts the same": {"Deliver phase 44 step 50."},
		"no step at all":                      {"Fix the flaky test in the CLI."},
		"no write text":                       nil,
	} {
		v := bounds.Decide(1.0, 0, writeCode, writes, stepFiveDelivered())
		if v.Escalate {
			t.Errorf("%s: a plan writing %q escalated: %s", name, writes, v.Reason)
		}
	}
}

// The redo reason is the most specific one: "this is already on pull request
// #12" is something a reviewer checks in one click, where a low confidence or
// a capability that always asks says nothing about this plan in particular.
func TestRedoReasonWinsOverConfidenceAndCapabilityReasons(t *testing.T) {
	bounds := agent.AuthorityBounds{
		MaxAutonomousActions: agent.UnlimitedActions,
		RequiresApprovalFor:  writeCode,
	}

	v := bounds.Decide(0.2, 0.9, writeCode, []string{"Deliver phase 44 step 5."}, stepFiveDelivered())
	if !v.Escalate {
		t.Fatal("an under-confident plan needing approval ran")
	}
	if !strings.Contains(v.Reason, "redoes delivered work") {
		t.Errorf("reason = %q, want the redo reason", v.Reason)
	}
}

// Naming a source keeps the delivered work: the loop threads one Evidence
// value through its steps, and a third-party observation arriving after the
// checklist must not erase it.
func TestWithSourceKeepsTheDeliveredWork(t *testing.T) {
	ev := stepFiveDelivered().WithSource("software.env.chat")
	if len(ev.Delivered) != 1 || ev.Delivered[0].Ref != "phase 44 step 5" {
		t.Errorf("Delivered after WithSource = %#v, want phase 44 step 5 kept", ev.Delivered)
	}
}
