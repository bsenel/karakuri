package software

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/domain"
)

// Phase 34 step 6, "Use it". Steps 2 to 5 put the deployment's own data —
// audit log, checkpoints, cost report, reconcile status, objectives, digest,
// telemetry — behind MCP tools a delegated agent can be handed for one action
// through params.karakuri_tools. Nothing tells the planner that, so a plan
// that needs that data still writes "run `krk audit list`" into a prompt and
// the agent shells out with whatever credential happens to be lying around.
//
// The hint is the only place a model learns the field exists. These tests pin
// what it has to say, and that every tool it names is one the server serves:
// a hint naming a tool that does not exist produces a delegation whose agent
// is handed nothing.

// karakuriToolsHint returns the hint that tells the planner about
// params.karakuri_tools.
func karakuriToolsHint(t *testing.T) domain.PlannerHint {
	t.Helper()
	var found []domain.PlannerHint
	for _, h := range softwarePlannerHints() {
		if strings.Contains(h.Guidance, "params.karakuri_tools") {
			found = append(found, h)
		}
	}
	if len(found) != 1 {
		t.Fatalf("softwarePlannerHints() has %d hints whose Guidance names params.karakuri_tools, want exactly 1", len(found))
	}
	return found[0]
}

// servedKarakuriTools reads the tool names out of (*MCPHandler).tools() in
// internal/api/handler/mcp.go. The list is unexported and lives in the
// delivery layer, which a domain pack may not import, so the test reads the
// source: a tool renamed or removed there fails here.
func servedKarakuriTools(t *testing.T) map[string]bool {
	t.Helper()
	const path = "../../internal/api/handler/mcp.go"
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	body := string(src)
	start := strings.Index(body, "func (h *MCPHandler) tools() []mcpTool {")
	if start < 0 {
		t.Fatalf("%s no longer declares (*MCPHandler).tools(); update this test to wherever the tool list moved", path)
	}
	body = body[start:]
	if end := strings.Index(body, "\n}\n"); end >= 0 {
		body = body[:end]
	}
	served := map[string]bool{}
	for _, m := range regexp.MustCompile(`Name:\s+"([a-z_]+)"`).FindAllStringSubmatch(body, -1) {
		served[m[1]] = true
	}
	if len(served) == 0 {
		t.Fatalf("found no tool names in (*MCPHandler).tools() in %s", path)
	}
	return served
}

func TestKarakuriToolsHintAppliesToDelegatingCapabilities(t *testing.T) {
	hint := karakuriToolsHint(t)

	// The three capabilities cliEnv serves, the only actions that read
	// params.karakuri_tools.
	for _, id := range []string{
		"software.act.write_code",
		"software.act.write_test",
		"software.act.delegate_to_cli",
	} {
		if !strings.Contains(hint.Condition, "'"+id+"'") {
			t.Errorf("Condition %q does not apply to %s", hint.Condition, id)
		}
	}
	if !strings.HasPrefix(hint.Condition, "capability.id") {
		t.Errorf("Condition %q is not a condition on capability.id", hint.Condition)
	}
	if hint.Priority <= 0 {
		t.Errorf("Priority = %d, want a positive priority", hint.Priority)
	}
}

func TestKarakuriToolsHintSaysToAskInsteadOfShellingOutToKrk(t *testing.T) {
	guidance := karakuriToolsHint(t).Guidance

	if !strings.Contains(guidance, "krk") {
		t.Errorf("Guidance does not name krk, the thing not to shell out to: %q", guidance)
	}
	if !strings.Contains(guidance, "instead of") {
		t.Errorf("Guidance does not say the tools are asked for instead of shelling out: %q", guidance)
	}
}

// cliEnv refuses an action that asks for karakuri_tools when the cli_agents
// instance does not set attach_karakuri_mcp, so a hint that says "always ask"
// turns every such plan into a failed action on a deployment that has not
// opted in.
func TestKarakuriToolsHintIsConditionalOnAttachKarakuriMCP(t *testing.T) {
	guidance := karakuriToolsHint(t).Guidance

	if !strings.Contains(guidance, "attach_karakuri_mcp") {
		t.Errorf("Guidance does not name the attach_karakuri_mcp option: %q", guidance)
	}
	if !strings.Contains(guidance, "cli_agents") {
		t.Errorf("Guidance does not say which instance sets attach_karakuri_mcp (cli_agents): %q", guidance)
	}
	if !strings.Contains(guidance, "only") {
		t.Errorf("Guidance does not say the tools are only available when attach_karakuri_mcp is set: %q", guidance)
	}
}

func TestKarakuriToolsHintNamesOnlyServedTools(t *testing.T) {
	served := servedKarakuriTools(t)
	for _, want := range []string{"audit_list", "checkpoints_list", "cost_report"} {
		if !served[want] {
			t.Fatalf("read %d tool names from mcp.go and %s is not among them; the reader is out of step with the file", len(served), want)
		}
	}
	guidance := karakuriToolsHint(t).Guidance

	// Every snake_case word in the guidance is either one of these, which are
	// the field and the options, or a tool name. A misspelled tool name is
	// neither, and fails.
	notTools := map[string]bool{
		"karakuri_tools":      true,
		"attach_karakuri_mcp": true,
		"karakuri_mcp_url":    true,
		"cli_agents":          true,
	}
	named := 0
	for _, word := range regexp.MustCompile(`[a-z]+(?:_[a-z]+)+`).FindAllString(guidance, -1) {
		if notTools[word] {
			continue
		}
		if !served[word] {
			t.Errorf("Guidance names %q, which internal/api/handler/mcp.go does not serve", word)
			continue
		}
		named++
	}
	if named == 0 {
		t.Errorf("Guidance names no Karakuri tool; the planner has to know what to put in the list: %q", guidance)
	}
}
