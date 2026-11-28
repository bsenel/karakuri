package software

// Phase 39 step 4, the remove half, slice S1: the software pack's server
// serves the acting capabilities that write files, and runs each in the
// worktree its keeper holds for the call's objective and pass. The loop no
// longer has to provision one (slice S2): a call reaches the pack with the
// plan's params and nothing the orchestrator added, and the result goes back
// naming no path of the pack's.
//
// The coding agent CLI is a fake (actingAgent): it records what it was handed
// and runs a scripted body as the agent, in the directory it was given.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/cliagent"
)

// packWritingTools are the acting capabilities that write files today: the
// ones capabilities.go's writes helper marks. Named here rather than read off
// NeedsWorkspace, because slice S2 takes that field out of the core and this
// list must outlive it.
var packWritingTools = []string{
	"software.act.create_pr",
	"software.act.delegate_to_cli",
	"software.act.write_code",
	"software.act.write_test",
}

// packPlannedBranch is the branch a plan names for a delivery item.
const packPlannedBranch = "karakuri/phase-39-step-04-planned-group"

// actingAgent is a coding agent CLI that records every delegation and runs
// run, when set, as the agent in the worktree it was handed.
type actingAgent struct {
	t   *testing.T
	run func(t *testing.T, worktree string)

	mu    sync.Mutex
	calls []cliagent.DelegateInput
}

func (*actingAgent) Name() string { return "pack-agent" }
func (*actingAgent) Active() bool { return true }
func (a *actingAgent) Delegate(_ context.Context, in cliagent.DelegateInput) (cliagent.DelegateOutput, error) {
	a.mu.Lock()
	a.calls = append(a.calls, in)
	a.mu.Unlock()
	if a.run != nil {
		withEnv(in.WorktreePath, in.Env, func() { a.run(a.t, in.WorktreePath) })
	}
	// An agent's summary names where it worked, as a real one's does.
	said := "wrote the change in " + in.WorktreePath
	return cliagent.DelegateOutput{Summary: said, RawOutput: said}, nil
}
func (*actingAgent) Stream(context.Context, cliagent.DelegateInput) (<-chan cliagent.DelegateChunk, error) {
	ch := make(chan cliagent.DelegateChunk)
	close(ch)
	return ch, nil
}

func (a *actingAgent) delegations() []cliagent.DelegateInput {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]cliagent.DelegateInput(nil), a.calls...)
}

// packActingFixture is the software pack's server over a clone of a bare
// origin, with the pack's keeper of worktrees on the clone and the fake agent
// bound as the twin's cli_agents instance.
type packActingFixture struct {
	srv    *MCPServer
	ws     *Workspaces
	agent  *actingAgent
	origin string
	repo   string
}

func newPackActingFixture(t *testing.T, run func(t *testing.T, worktree string)) packActingFixture {
	t.Helper()
	isolateGit(t)
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	repo := filepath.Join(root, "repo")
	gitIn(t, root, "init", "--bare", "-b", "main", origin)
	gitIn(t, root, "clone", origin, repo)
	writeFile(t, filepath.Join(repo, "README.md"), "pack repo\n")
	writeFile(t, filepath.Join(repo, ".gitignore"), "worktrees/\n")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", "base")
	gitIn(t, repo, "push", "origin", "HEAD:refs/heads/main")

	agent := &actingAgent{t: t, run: run}
	reg := tools.NewRegistry()
	reg.CLIAgents.Set("agent", "stub", agent)
	bindings := func(context.Context, domain.CallContract) map[string]string {
		return map[string]string{"cli_agents": "agent"}
	}
	clock := &packClock{t: time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)}
	ws := NewWorkspaces(repo, workspaceAgeBound, clock.now)
	return packActingFixture{
		srv:    NewWithTools(reg).MCPServer(bindings).WithWorkspaces(ws),
		ws:     ws,
		agent:  agent,
		origin: origin,
		repo:   repo,
	}
}

// writeCodeParams are a plan's params for write_code: the task and the
// branch the plan names, nothing the orchestrator adds.
func writeCodeParams() map[string]any {
	return map[string]any{
		"prompt":        "implement the step",
		"allowed_tools": []any{"Read", "Edit", "Bash"},
		"branch":        packPlannedBranch,
	}
}

func TestTheSoftwarePacksServerListsItsFileWritingActingToolsAndNoOtherActingTool(t *testing.T) {
	pack := packWithForge(&vcStub{})
	served, err := pack.MCPServer(boundTo).ListTools(context.Background())
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	writing := map[string]bool{}
	for _, id := range packWritingTools {
		writing[id] = true
	}
	// Today the list is the writes helper's; S2 drops this cross-check with
	// the field it reads.
	for _, c := range pack.Capabilities() {
		if c.NeedsWorkspace != writing[string(c.ID)] {
			t.Errorf("%s: marked as writing files = %v, in packWritingTools = %v; the list is out of date", c.ID, c.NeedsWorkspace, writing[string(c.ID)])
		}
	}

	var want []string
	for _, c := range pack.Capabilities() {
		if strings.HasPrefix(string(c.ID), "software.observe.") || c.Verifiable || writing[string(c.ID)] {
			want = append(want, string(c.ID))
		}
	}
	got := make([]string, 0, len(served))
	for _, id := range served {
		got = append(got, string(id))
	}
	sort.Strings(want)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("served tools = %v, want the observe capabilities, the verifiers and the file-writing acting tools %v", got, want)
	}

	listed := map[string]bool{}
	for _, id := range got {
		listed[id] = true
	}
	for _, id := range packWritingTools {
		if !listed[id] {
			t.Errorf("the server does not serve %s, an acting capability that writes files", id)
		}
	}
	for _, c := range pack.Capabilities() {
		id := string(c.ID)
		acting := !strings.HasPrefix(id, "software.observe.") && !c.Verifiable
		if acting && listed[id] && !writing[id] {
			t.Errorf("the server serves the acting capability %s, which writes no files", id)
		}
	}
}

func TestAWriteThroughThePacksServerRunsInThePassesWorktreeAndAVerifierOfThatPassSeesIt(t *testing.T) {
	f := newPackActingFixture(t, func(t *testing.T, wt string) {
		writeWork(t, wt, "dist/app.js", "console.log(1)\n")
	})
	call := passCall("obj-1", 1, "write-code")

	res, err := f.srv.CallTool(context.Background(), call, "software.act.write_code", writeCodeParams())
	if err != nil {
		t.Fatalf("write_code through the pack's server: %v", err)
	}
	if !res.Success {
		t.Fatalf("write_code through the pack's server failed: %s", res.Error)
	}
	runs := f.agent.delegations()
	if len(runs) != 1 {
		t.Fatalf("the coding agent ran %d times, want once", len(runs))
	}
	want := workspaceFor(t, f.ws, passCall("obj-1", 1, "lookup"))
	if filepath.Clean(runs[0].WorktreePath) != filepath.Clean(want) {
		t.Errorf("the agent ran in %q, want the pack's worktree for objective obj-1 pass 1 %q", runs[0].WorktreePath, want)
	}

	if text, ok := measureBundle(t, f.srv, passCall("obj-1", 1, "measure")); !ok {
		t.Errorf("the verifier of the same pass does not see what write_code wrote: %s", text)
	}
	for name, other := range map[string]domain.CallContract{
		"next pass":       passCall("obj-1", 2, "measure"),
		"other objective": passCall("obj-2", 1, "measure"),
	} {
		if text, ok := measureBundle(t, f.srv, other); ok {
			t.Errorf("%s: the verifier measured the bundle objective obj-1 pass 1 wrote: %s", name, text)
		}
	}
}

// The plan's params.branch is the ref the action is declared to push, as the
// loop declared it under params.planned_branch: a push to it is inside the
// action's refs (Phase 40 step 3).
func TestAWriteThroughThePacksServerDeclaresThePlansBranchForThePushedRefsCheck(t *testing.T) {
	f := newPackActingFixture(t, func(t *testing.T, wt string) {
		commitIn(t, wt, "step.txt")
		gitIn(t, wt, "push", "origin", "HEAD:refs/heads/"+packPlannedBranch)
	})

	res, err := f.srv.CallTool(context.Background(), passCall("obj-1", 1, "write-code"), "software.act.write_code", writeCodeParams())
	if err != nil {
		t.Fatalf("write_code through the pack's server: %v", err)
	}
	if !res.Success || res.BoundsViolation != nil {
		t.Fatalf("a push to the plan's branch failed the action: %q (violation %+v)", res.Error, res.BoundsViolation)
	}
	if out := gitIn(t, f.repo, "ls-remote", "--heads", f.origin, packPlannedBranch); !strings.Contains(out, packPlannedBranch) {
		t.Errorf("origin has no %s after the agent pushed it: %q", packPlannedBranch, out)
	}
}

func TestAWriteThroughThePacksServerReturnsNoPathOfThePacks(t *testing.T) {
	f := newPackActingFixture(t, func(t *testing.T, wt string) {
		writeWork(t, wt, "feature.go", "package feature\n")
	})
	call := passCall("obj-1", 1, "write-code")

	res, err := f.srv.CallTool(context.Background(), call, "software.act.write_code", writeCodeParams())
	if err != nil {
		t.Fatalf("write_code through the pack's server: %v", err)
	}
	if !res.Success {
		t.Fatalf("write_code through the pack's server failed: %s", res.Error)
	}
	if len(f.agent.delegations()) != 1 {
		t.Fatalf("the coding agent ran %d times, want once", len(f.agent.delegations()))
	}
	for _, key := range []string{"workdir", "worktree_path", "worktree", "workspace", "path"} {
		if _, ok := res.StateDelta[key]; ok {
			t.Errorf("the result's state carries %q, a workspace reference the orchestrator would hold", key)
		}
	}
	dir := workspaceFor(t, f.ws, call)
	text := fmt.Sprintf("%+v", res)
	for _, ref := range []string{dir, f.repo, filepath.Join(f.repo, packWorktreeBase)} {
		if strings.Contains(text, ref) {
			t.Errorf("the result names the pack's filesystem %q: %s", ref, text)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "feature.go")); err != nil {
		t.Errorf("the agent's file is not in the pack's worktree for the pass: %v", err)
	}
}
