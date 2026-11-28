package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/feature/gateway"
	"github.com/bsenel/karakuri/internal/platform/git"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/cliagent"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

// Phase 39 step 4, slice S1, wired as bootstrap wires it: a write_code action
// dispatched through the gateway with the software pack's server (and its
// keeper of worktrees) behind it reaches the pack, and the loop's perform,
// the only place the loop provisions a worktree, never runs.

// writingAgent is a coding agent CLI that records the worktree it was handed
// and writes one file there.
type writingAgent struct {
	mu   sync.Mutex
	dirs []string
}

func (*writingAgent) Name() string { return "writing-agent" }
func (*writingAgent) Active() bool { return true }
func (a *writingAgent) Delegate(_ context.Context, in cliagent.DelegateInput) (cliagent.DelegateOutput, error) {
	a.mu.Lock()
	a.dirs = append(a.dirs, in.WorktreePath)
	a.mu.Unlock()
	if err := os.WriteFile(filepath.Join(in.WorktreePath, "feature.go"), []byte("package feature\n"), 0o600); err != nil {
		return cliagent.DelegateOutput{}, err
	}
	return cliagent.DelegateOutput{Summary: "wrote feature.go in " + in.WorktreePath}, nil
}
func (*writingAgent) Stream(context.Context, cliagent.DelegateInput) (<-chan cliagent.DelegateChunk, error) {
	ch := make(chan cliagent.DelegateChunk)
	close(ch)
	return ch, nil
}

func (a *writingAgent) worktrees() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.dirs...)
}

// provisionCounter stands for the loop's WorktreeManager and counts the
// worktrees asked of it.
type provisionCounter struct {
	git.WorktreeManager
	dir     string
	creates int
}

func (w *provisionCounter) Create(_ context.Context, opts git.WorktreeOptions) (git.Worktree, error) {
	w.creates++
	return git.Worktree{TaskID: opts.TaskID, ObjectiveID: opts.ObjectiveID, Path: w.dir, Branch: "provisioned/" + opts.TaskID, CreatedAt: time.Now()}, nil
}

// actingRepo is a git repository with one commit, for the pack's keeper.
func actingRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		base := []string{"-c", "user.name=karakuri-test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}
		cmd := exec.Command("git", append(base, args...)...) // #nosec G204 -- fixed binary, test arguments
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("worktrees/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", ".gitignore")
	run("commit", "-q", "-m", "init")
	return root
}

func TestAWriteCodeActionThroughTheGatewayReachesTheSoftwarePackAndTheLoopProvisionsNothing(t *testing.T) {
	repo := actingRepo(t)
	agent := &writingAgent{}
	reg := tools.NewRegistry()
	reg.CLIAgents.Set("agent", "stub", agent)
	bindings := func(context.Context, domain.CallContract) map[string]string {
		return map[string]string{"cli_agents": "agent"}
	}
	workspaces := software.NewWorkspaces(repo, time.Hour, time.Now)
	server := software.NewWithTools(reg).MCPServer(bindings).WithWorkspaces(workspaces)

	gw := gateway.New(&rowStore{}, karakuriquota.Deps{}, &karakuriquota.Recorder{})
	if err := gw.ServePack(context.Background(), "software", server); err != nil {
		t.Fatalf("serve the software pack: %v", err)
	}

	call := fetchPRsCall()
	call.Capability = "software.act.write_code"
	call.EnvID = "software.env.cli_agent"
	call.Params = map[string]any{
		"prompt":        "implement the step",
		"allowed_tools": []any{"Read", "Edit"},
		"branch":        "karakuri/phase-39-step-04-planned-group",
	}
	loopTrees := &provisionCounter{dir: t.TempDir()}

	// The loop's perform: it provisions a worktree for a capability that
	// writes files, then runs the environment in process.
	got, err := gw.Call(context.Background(), call, func(ctx context.Context) (environment.ActionResult, error) {
		wt, err := loopTrees.Create(ctx, git.WorktreeOptions{ObjectiveID: "obj-1", TaskID: "task-1", BranchName: "karakuri/phase-39-step-04-planned-group"})
		if err != nil {
			return environment.ActionResult{}, err
		}
		return environment.ActionResult{Success: false, Error: "ran in process in " + wt.Path}, nil
	})
	if err != nil {
		t.Fatalf("write_code through the gateway: %v", err)
	}
	if loopTrees.creates != 0 {
		t.Errorf("the loop's WorktreeManager received %d Create calls; the software pack serves write_code in its own worktree", loopTrees.creates)
	}
	dirs := agent.worktrees()
	if len(dirs) != 1 {
		t.Fatalf("the software pack's coding agent ran %d times, want once; the action did not reach the pack (result %+v)", len(dirs), got)
	}
	if !got.Success {
		t.Fatalf("write_code through the software pack failed: %s", got.Error)
	}
	if !strings.HasPrefix(filepath.Clean(dirs[0]), filepath.Clean(repo)+string(filepath.Separator)) {
		t.Errorf("the agent ran in %q, not in a worktree of the pack's repository %q", dirs[0], repo)
	}
	if text := fmt.Sprintf("%+v", got); strings.Contains(text, dirs[0]) || strings.Contains(text, repo) {
		t.Errorf("the result names the pack's filesystem: %s", text)
	}
	for _, key := range []string{"workdir", "worktree_path", "planned_branch"} {
		if _, ok := call.Params[key]; ok {
			t.Errorf("the call's params gained %q; the orchestrator holds no workspace", key)
		}
	}
}
