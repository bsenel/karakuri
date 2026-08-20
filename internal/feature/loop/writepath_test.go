package loop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/domains/software"
	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/git"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/cliagent"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// ── stub adapters ────────────────────────────────────────────────────────────

// scriptedCLI stands in for a coding-agent CLI. It writes the file it is told
// to write, in the worktree it is handed, and records both — which is the only
// way to tell "the CLI ran" from "the CLI was called with nothing".
type scriptedCLI struct {
	calls []cliagent.DelegateInput
}

func (c *scriptedCLI) Name() string { return "scripted" }
func (c *scriptedCLI) Active() bool { return true }

func (c *scriptedCLI) Delegate(_ context.Context, in cliagent.DelegateInput) (cliagent.DelegateOutput, error) {
	c.calls = append(c.calls, in)
	name := "written-by-cli.txt"
	if strings.Contains(strings.ToLower(in.Prompt), "test") {
		name = "written-by-cli_test.txt"
	}
	if err := os.WriteFile(filepath.Join(in.WorktreePath, name), []byte(in.Prompt), 0o600); err != nil {
		return cliagent.DelegateOutput{ExitCode: 1}, err
	}
	return cliagent.DelegateOutput{Summary: "wrote " + name, ExitCode: 0}, nil
}

func (c *scriptedCLI) Stream(context.Context, cliagent.DelegateInput) (<-chan cliagent.DelegateChunk, error) {
	ch := make(chan cliagent.DelegateChunk)
	close(ch)
	return ch, nil
}

// recordingVC stands in for GitHub. It records the pull request it was asked
// to open, worktree path included — the field the loop never used to supply.
type recordingVC struct {
	prs []versioncontrol.PullRequest
}

func (v *recordingVC) Name() string { return "recording" }
func (v *recordingVC) Active() bool { return true }

func (v *recordingVC) CreatePR(_ context.Context, pr versioncontrol.PullRequest) (string, error) {
	v.prs = append(v.prs, pr)
	return "https://example.invalid/pr/1", nil
}

func (v *recordingVC) ListPRs(context.Context, string, time.Time) ([]versioncontrol.PRSummary, error) {
	return nil, nil
}

func (v *recordingVC) GetCommits(context.Context, string, time.Time) ([]versioncontrol.Commit, error) {
	return nil, nil
}

// ── the acceptance test ──────────────────────────────────────────────────────

// The write path, end to end, against a scratch repository.
//
// ADR 017 divides self-improvement in two: analyse and draft on one side,
// write and open a pull request on the other. The second half did not exist —
// not because any single piece was missing, but because the pieces were wired
// to each other by a capability-name suffix and a model-chosen env_id, and the
// two disagreed. write_code got a worktree and had no implementation;
// delegate_to_cli had an implementation and got no worktree; create_pr took a
// worktree path the loop never supplied.
//
// This runs the chain a self_improve objective needs and asserts the three
// things that were each individually false: the drafting step is served, the
// writing step reaches the CLI *in a real worktree*, and the pull-request step
// is handed that worktree's path and branch. No env_id is set on any action —
// the pack's declarations do the routing, which is the point.
func TestWritePathEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo := scratchRepo(t)
	cli := &scriptedCLI{}
	vc := &recordingVC{}
	sc := writePathContext(t, repo, cli, vc)

	results := stepAct(context.Background(), sc, plan{
		Confidence: 0.9,
		Actions: []plannedAction{
			{CapabilityID: "software.act.write_design_doc", Params: map[string]any{
				"design": "route capabilities to the environment that serves them",
			}},
			{CapabilityID: "software.act.write_test", Params: map[string]any{
				"prompt": "a failing test for the router",
			}},
			{CapabilityID: "software.act.write_code", Params: map[string]any{
				"prompt": "make the router resolve by declaration",
			}},
			{CapabilityID: "software.act.create_pr", Params: map[string]any{
				"title": "Route by declaration",
				"body":  "See ADR 019.",
			}},
		},
	})

	if len(results) != 4 {
		t.Fatalf("got %d results, want 4", len(results))
	}
	for i, o := range results {
		if !o.Result.Success {
			t.Errorf("action %d (%s) failed: %s", i, o.CapabilityID, o.Result.Error)
		}
	}

	// The CLI ran twice, each time in a real worktree with a real task. Before
	// Phase 26 it ran with an empty worktree_path, or did not run at all.
	if len(cli.calls) != 2 {
		t.Fatalf("the CLI was called %d times, want 2 (write_test and write_code)", len(cli.calls))
	}
	for _, call := range cli.calls {
		if call.WorktreePath == "" {
			t.Error("the CLI was called with no worktree; this is the bug Phase 26 exists for")
			continue
		}
		if !strings.HasPrefix(call.WorktreePath, repo) {
			t.Errorf("worktree %q is outside the scratch repository", call.WorktreePath)
		}
		if _, err := os.Stat(call.WorktreePath); err != nil {
			t.Errorf("worktree %q was reported but does not exist: %v", call.WorktreePath, err)
		}
		if strings.TrimSpace(call.Prompt) == "" {
			t.Error("the CLI was handed an empty task")
		}
	}
	if cli.calls[0].WorktreePath == cli.calls[1].WorktreePath {
		t.Error("both actions shared one worktree; isolation per action is the point of provisioning them")
	}

	// The files actually landed, in the worktree rather than the checked-out
	// tree — the one outcome a planner hint explicitly forbids.
	for _, call := range cli.calls {
		entries, err := os.ReadDir(call.WorktreePath)
		if err != nil {
			t.Fatalf("read worktree: %v", err)
		}
		var wrote bool
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "written-by-cli") {
				wrote = true
			}
		}
		if !wrote {
			t.Errorf("nothing was written in %s", call.WorktreePath)
		}
	}

	// And the pull request was opened with a worktree to push, which is the
	// half of the chain that was unreachable by the same gap.
	if len(vc.prs) != 1 {
		t.Fatalf("%d pull requests opened, want 1", len(vc.prs))
	}
	pr := vc.prs[0]
	if pr.WorktreePath == "" {
		t.Error("create_pr got no worktree_path: there is nothing to push")
	}
	if pr.HeadBranch == "" {
		t.Error("create_pr got no branch")
	}
	if pr.Title != "Route by declaration" {
		t.Errorf("PR title = %q", pr.Title)
	}
}

// A plan may still name an environment, and naming the wrong one must not
// break the chain — the pack's declaration wins. This is the live failure that
// started the phase, reproduced deliberately.
func TestWritePathSurvivesAWrongEnvID(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo := scratchRepo(t)
	cli := &scriptedCLI{}
	sc := writePathContext(t, repo, cli, &recordingVC{})

	results := stepAct(context.Background(), sc, plan{
		Confidence: 0.9,
		Actions: []plannedAction{{
			CapabilityID: "software.act.write_code",
			EnvID:        "software.env.codebase", // the model's wrong guess
			Params:       map[string]any{"prompt": "implement the router"},
		}},
	})

	if len(results) != 1 || !results[0].Result.Success {
		t.Fatalf("write_code failed despite the pack declaring who serves it: %+v", results)
	}
	if len(cli.calls) != 1 {
		t.Fatalf("the CLI was called %d times, want 1: the wrong env_id was obeyed", len(cli.calls))
	}
}

// ── fixtures ─────────────────────────────────────────────────────────────────

// scratchRepo returns a committed git repository in a temp dir.
func scratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@test.invalid")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("scratch"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

// writePathContext wires the real software pack — its registry, its
// environments, its routing — onto a real worktree manager and the two stub
// adapters, so the only fake things are the CLI and the forge.
func writePathContext(t *testing.T, repo string, cli cliagent.CLIAgentAdapter, vc versioncontrol.VersionControlAdapter) *stepContext {
	t.Helper()

	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "writepath.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := storage.NewGORMStorage(db)

	reg := &tools.Registry{}
	reg.CLIAgents.Set("scripted", "stub", cli)
	reg.VC.Set("recording", "stub", vc)

	pack := software.NewWithTools(reg)

	capReg := capability.NewRegistry()
	for _, c := range pack.Capabilities() {
		if err := capReg.Register(c); err != nil {
			t.Fatalf("register capability %q: %v", c.ID, err)
		}
	}
	envReg := environment.NewRegistry()
	var envs []environment.Environment
	for _, f := range pack.EnvironmentFactories() {
		if err := envReg.Register(f); err != nil {
			t.Fatalf("register %q: %v", f.EnvID, err)
		}
		env, err := f.Build(environment.BuildContext{
			TwinID:          "twin-1",
			AdapterBindings: map[string]string{"cli_agents": "scripted", "versioncontrol": "recording"},
		})
		if err != nil {
			t.Fatalf("build %q: %v", f.EnvID, err)
		}
		envs = append(envs, env)
	}

	wt, err := git.NewGoGitWorktreeManager(config.GitConfig{
		RepoPath:     repo,
		WorktreeBase: "worktrees",
		BaseBranch:   "main",
		BranchPrefix: "karakuri",
	})
	if err != nil {
		t.Fatalf("worktree manager: %v", err)
	}

	return &stepContext{
		loopID:   "loop-writepath-0001",
		agentDef: coreagent.Definition{ID: "software.agent.maintainer"},
		envs:     envs,
		obj: objective.Objective{
			ID:     "obj-writepath",
			Title:  "close the write path",
			Domain: "software",
		},
		twinID: "twin-1",
		svc: &serviceImpl{
			store:  store,
			capReg: capReg,
			envReg: envReg,
			wt:     wt,
			hub:    event.NewHub(),
		},
	}
}
