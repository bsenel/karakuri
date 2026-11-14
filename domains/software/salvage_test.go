package software

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/cliagent"
)

// Phase 44 step 2: a coding action that ends without a commit of its own
// (killed, out of quota, exit 1) leaves its edits in a worktree nothing reads.
// The software pack commits them to karakuri/salvage/<objective>/<action>,
// never to the item's branch, and names the branch and SHA in the result.

const (
	salvageObjectiveID = "1e450f12-7c1d-4b9e-9a51-3f0c2d8e6b44"
	salvageTaskID      = "1e450f12-0-1-dmuwlmxb16xy"
	salvageItemBranch  = "karakuri/1e450f12/" + salvageTaskID
	salvageWantBranch  = "karakuri/salvage/" + salvageObjectiveID + "/" + salvageTaskID
)

var fullSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// funcCLI is a wired coding agent whose run is the function the test passes:
// it works in the worktree it is given and returns what the run returned.
type funcCLI struct {
	run func(t *testing.T, worktree string) error
	t   *testing.T
}

func (*funcCLI) Name() string { return "func" }
func (*funcCLI) Active() bool { return true }
func (c *funcCLI) Delegate(_ context.Context, in cliagent.DelegateInput) (cliagent.DelegateOutput, error) {
	return cliagent.DelegateOutput{Summary: "ran", RawOutput: "ran"}, c.run(c.t, in.WorktreePath)
}
func (*funcCLI) Stream(context.Context, cliagent.DelegateInput) (<-chan cliagent.DelegateChunk, error) {
	ch := make(chan cliagent.DelegateChunk)
	close(ch)
	return ch, nil
}

// salvageFixture is the layout the loop leaves: a bare origin, a clone with
// the item branch pushed, and that branch checked out as a worktree under
// <base>/worktrees/<objective>/<task>, as GoGitWorktreeManager.Create does.
type salvageFixture struct {
	origin   string
	clone    string
	worktree string
	itemSHA  string
}

// isolateGit keeps the developer's or CI's git configuration (signing, hooks,
// default branch) out of the fixture and gives every commit an identity.
func isolateGit(t *testing.T) {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Fixture")
	t.Setenv("GIT_AUTHOR_EMAIL", "fixture@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "Fixture")
	t.Setenv("GIT_COMMITTER_EMAIL", "fixture@example.invalid")
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newSalvageFixture(t *testing.T) salvageFixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	isolateGit(t)
	root := t.TempDir()
	f := salvageFixture{
		origin: filepath.Join(root, "origin.git"),
		clone:  filepath.Join(root, "repo"),
	}
	f.worktree = filepath.Join(f.clone, "worktrees", salvageObjectiveID, salvageTaskID)

	gitIn(t, root, "init", "--bare", "-b", "main", f.origin)
	gitIn(t, root, "clone", f.origin, f.clone)
	writeFile(t, filepath.Join(f.clone, "tracked.txt"), "original\n")
	writeFile(t, filepath.Join(f.clone, ".gitignore"), "worktrees/\n")
	gitIn(t, f.clone, "add", ".")
	gitIn(t, f.clone, "commit", "-m", "base")
	gitIn(t, f.clone, "push", "origin", "HEAD:refs/heads/main")

	if err := os.MkdirAll(filepath.Dir(f.worktree), 0o750); err != nil {
		t.Fatal(err)
	}
	gitIn(t, f.clone, "worktree", "add", "-b", salvageItemBranch, f.worktree, "main")
	writeFile(t, filepath.Join(f.worktree, "item.txt"), "item work\n")
	gitIn(t, f.worktree, "add", "item.txt")
	gitIn(t, f.worktree, "commit", "-m", "item commit")
	gitIn(t, f.worktree, "push", "origin", "HEAD:refs/heads/"+salvageItemBranch)
	f.itemSHA = gitIn(t, f.worktree, "rev-parse", "HEAD")
	return f
}

// originRefs maps each branch on the bare origin to its SHA.
func (f salvageFixture) originRefs(t *testing.T) map[string]string {
	t.Helper()
	refs := map[string]string{}
	out := gitIn(t, f.clone, "ls-remote", "--heads", f.origin)
	for _, line := range strings.Split(out, "\n") {
		sha, ref, ok := strings.Cut(line, "\t")
		if ok {
			refs[strings.TrimPrefix(ref, "refs/heads/")] = sha
		}
	}
	return refs
}

func salvageAction(worktree string) environment.Action {
	return environment.Action{
		CapabilityID: "software.act.delegate_to_cli",
		Params: map[string]any{
			"prompt":        "implement the step",
			"allowed_tools": []any{"Read", "Edit"},
			// As the loop's stepAct sets them (internal/feature/loop/act.go).
			"worktree_path": worktree,
			"branch":        salvageItemBranch,
		},
	}
}

func actWith(t *testing.T, worktree string, action environment.Action, run func(t *testing.T, worktree string) error) environment.ActionResult {
	t.Helper()
	env := &cliEnv{id: "software.env.cli_agent", cli: &funcCLI{run: run, t: t}}
	res, err := env.Act(context.Background(), action)
	if err != nil {
		t.Fatalf("act returned an error instead of a failed result: %v", err)
	}
	return res
}

// exit1 is what the adapters return for a CLI that exited 1.
var exit1 = errors.New("claude_code exited with status 1: usage limit reached")

// An edit left uncommitted by a run that failed ends up on a salvage branch on
// origin, holding every edit, parented on the item's head, while the item's
// branch does not move.
func TestSalvageCommitsAFailedRunsEditsToASideBranch(t *testing.T) {
	f := newSalvageFixture(t)

	res := actWith(t, f.worktree, salvageAction(f.worktree), func(t *testing.T, wt string) error {
		writeFile(t, filepath.Join(wt, "new.txt"), "new file\n")
		writeFile(t, filepath.Join(wt, "tracked.txt"), "edited\n")
		return exit1
	})

	if res.Success {
		t.Fatal("a failed run must stay a failed result after salvage")
	}
	if res.Error != exit1.Error() {
		t.Errorf("Error = %q, want the delegate's own %q", res.Error, exit1.Error())
	}
	if got := res.StateDelta["salvage_branch"]; got != salvageWantBranch {
		t.Fatalf("salvage_branch = %v, want %q (delta %v)", got, salvageWantBranch, res.StateDelta)
	}
	sha, _ := res.StateDelta["salvage_sha"].(string)
	if !fullSHA.MatchString(sha) {
		t.Fatalf("salvage_sha = %v, want a 40-hex SHA", res.StateDelta["salvage_sha"])
	}

	refs := f.originRefs(t)
	if refs[salvageWantBranch] != sha {
		t.Errorf("origin %s = %q, want the reported %s", salvageWantBranch, refs[salvageWantBranch], sha)
	}
	if refs[salvageItemBranch] != f.itemSHA {
		t.Errorf("origin item branch moved to %q, want %s", refs[salvageItemBranch], f.itemSHA)
	}
	if local := gitIn(t, f.clone, "rev-parse", "refs/heads/"+salvageItemBranch); local != f.itemSHA {
		t.Errorf("local item branch moved to %q, want %s", local, f.itemSHA)
	}

	if got := gitIn(t, f.origin, "show", sha+":new.txt"); got != "new file" {
		t.Errorf("salvage commit's new.txt = %q, want the untracked file the run wrote", got)
	}
	if got := gitIn(t, f.origin, "show", sha+":tracked.txt"); got != "edited" {
		t.Errorf("salvage commit's tracked.txt = %q, want the run's edit", got)
	}
	if parent := gitIn(t, f.origin, "rev-parse", sha+"^"); parent != f.itemSHA {
		t.Errorf("salvage commit's parent = %s, want the item's head %s so it can be cherry-picked", parent, f.itemSHA)
	}
}

// Nothing is salvaged unless a run failed and left uncommitted changes.
func TestSalvageOnlyWhenAFailedRunLeftUncommittedWork(t *testing.T) {
	cases := map[string]func(t *testing.T, wt string) error{
		// The run committed its own work before it failed: the tree is clean.
		"failed after committing its own work": func(t *testing.T, wt string) error {
			writeFile(t, filepath.Join(wt, "tracked.txt"), "committed by the agent\n")
			gitIn(t, wt, "add", "tracked.txt")
			gitIn(t, wt, "commit", "-m", "agent commit")
			return exit1
		},
		"failed having changed nothing": func(*testing.T, string) error {
			return exit1
		},
		// Only a run that ended without success is salvaged.
		"succeeded and left edits": func(t *testing.T, wt string) error {
			writeFile(t, filepath.Join(wt, "new.txt"), "left behind\n")
			return nil
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			f := newSalvageFixture(t)

			res := actWith(t, f.worktree, salvageAction(f.worktree), run)

			for _, key := range []string{"salvage_branch", "salvage_sha", "salvage_error"} {
				if v, ok := res.StateDelta[key]; ok {
					t.Errorf("StateDelta[%q] = %v, want no salvage", key, v)
				}
			}
			for ref := range f.originRefs(t) {
				if strings.HasPrefix(ref, "karakuri/salvage/") {
					t.Errorf("origin has %s, want no salvage branch", ref)
				}
			}
		})
	}
}

// A worktree that is not a git repository cannot be salvaged, and the result
// says so rather than staying quiet (AGENTS.md rule 10). The run's own error
// is still the action's error.
func TestSalvageOfANonRepositorySaysWhy(t *testing.T) {
	isolateGit(t)
	dir := filepath.Join(t.TempDir(), "worktrees", salvageObjectiveID, salvageTaskID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}

	res := actWith(t, dir, salvageAction(dir), func(t *testing.T, wt string) error {
		writeFile(t, filepath.Join(wt, "new.txt"), "nowhere to keep this\n")
		return exit1
	})

	if res.Success {
		t.Fatal("a failed run must stay a failed result")
	}
	if res.Error != exit1.Error() {
		t.Errorf("Error = %q, want the delegate's own %q", res.Error, exit1.Error())
	}
	reason, _ := res.StateDelta["salvage_error"].(string)
	if strings.TrimSpace(reason) == "" {
		t.Errorf("salvage_error = %v, want a reason the worktree could not be salvaged (delta %v)",
			res.StateDelta["salvage_error"], res.StateDelta)
	}
	if v, ok := res.StateDelta["salvage_branch"]; ok {
		t.Errorf("salvage_branch = %v, want none for a worktree that is not a repository", v)
	}
}
