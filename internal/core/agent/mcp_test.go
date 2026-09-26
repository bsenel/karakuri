package agent_test

import (
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
)

// The third bound: on a fresh deployment — bounds that name nothing, an agent
// with unlimited autonomy and full confidence — an MCP action still escalates.
// Nobody could have listed it in RequiresApprovalFor; the policy lists it.
func TestMCPActionEscalatesWithBoundsThatNameNothing(t *testing.T) {
	bounds := agent.AuthorityBounds{MaxAutonomousActions: agent.UnlimitedActions}
	mcpTool := capability.MCPCapabilityID("acme_files", "read_file")

	v := bounds.Decide(1.0, 0, []capability.CapabilityID{"test.act.real", mcpTool}, agent.Evidence{})
	if !v.Escalate {
		t.Fatal("a discovered tool ran without asking on a fresh deployment")
	}
	if !strings.Contains(v.Reason, string(mcpTool)) || !strings.Contains(v.Reason, "MCP") {
		t.Errorf("reason = %q, want it to name the tool and say it came from an MCP server", v.Reason)
	}
	if v.Allowed != 2 {
		t.Errorf("Allowed = %d, want 2: an approved plan reaches act intact", v.Allowed)
	}
}

// The same bounds without the MCP action do not escalate, so the test above is
// measuring the namespace and not something else about the plan.
func TestThePlanWithoutTheMCPActionRunsUnasked(t *testing.T) {
	bounds := agent.AuthorityBounds{MaxAutonomousActions: agent.UnlimitedActions}
	v := bounds.Decide(1.0, 0, []capability.CapabilityID{"test.act.real"}, agent.Evidence{})
	if v.Escalate {
		t.Fatalf("a plan with no MCP action escalated: %s", v.Reason)
	}
}
