package loop

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/bsenel/karakuri/internal/platform/storage"
)

// A write action's planned branch survives worktree provisioning as
// planned_branch, beside the worktree's own branch in params.branch, so the
// pushed-refs check (Phase 40 step 3) declares the branch the plan named.
// Before, act replaced it and every push to the item branch read as a push
// outside the action's declared refs.
func TestWriteActionKeepsItsPlannedBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	const planned = "karakuri/phase-99-step-01-planned"

	repo := scratchRepo(t)
	sc := writePathContext(t, repo, &scriptedCLI{}, &recordingVC{})
	results := stepAct(context.Background(), sc, plan{
		Confidence: 0.9,
		Actions: []plannedAction{{CapabilityID: "software.act.write_code", Params: map[string]any{
			"prompt": "make the router resolve by declaration",
			"branch": planned,
		}}},
	})
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	rows, err := sc.svc.store.ListToolEvents(context.Background(), storage.ToolEventFilter{ObjectiveID: string(sc.obj.ID)})
	if err != nil {
		t.Fatalf("list tool events: %v", err)
	}
	var params map[string]any
	for _, r := range rows {
		if r.Capability != "software.act.write_code" {
			continue
		}
		var p struct {
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal([]byte(r.PayloadJSON), &p); err == nil && p.Params != nil {
			params = p.Params
		}
	}
	if params == nil {
		t.Fatal("no write_code audit row with params")
	}
	if got := params["planned_branch"]; got != planned {
		t.Errorf("planned_branch = %v, want %q", got, planned)
	}
	// The worktree is made on the planned branch itself (Phase 40 step 6a),
	// so params.branch names it too; what matters is that it names one.
	if got, _ := params["branch"].(string); got == "" {
		t.Errorf("branch is empty, want the worktree's branch")
	}
}
