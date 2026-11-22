package git_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/platform/git"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// A second action on the same planned branch must get its worktree even though
// the first action's worktree still has that branch checked out, and it starts
// from origin's current tip.
func TestWorktreeCreateTwiceOnANamedBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	// The host may hand git config through the environment (core.hooksPath).
	t.Setenv("GIT_CONFIG_PARAMETERS", "")
	origin := t.TempDir()
	gitIn(t, origin, "init", "-q", "--bare", "-b", "main")
	repo := t.TempDir()
	initGitRepo(t, repo)
	gitIn(t, repo, "remote", "add", "origin", origin)
	gitIn(t, repo, "push", "-q", "origin", "main")

	const branch = "karakuri/phase-99-repair"
	other := t.TempDir()
	gitIn(t, other, "clone", "-q", origin, ".")
	gitIn(t, other, "config", "user.email", "a@b.c")
	gitIn(t, other, "config", "user.name", "t")
	gitIn(t, other, "checkout", "-q", "-b", branch)
	gitIn(t, other, "commit", "-q", "--allow-empty", "-m", "first")
	gitIn(t, other, "push", "-q", "origin", branch)

	mgr, err := git.NewGoGitWorktreeManager(config.GitConfig{RepoPath: repo, WorktreeBase: "worktrees", BaseBranch: "main", BranchPrefix: "karakuri"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := mgr.Create(ctx, git.WorktreeOptions{ObjectiveID: "obj-abc12345", TaskID: "task-1", BranchName: branch}); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	gitIn(t, other, "commit", "-q", "--allow-empty", "-m", "second")
	gitIn(t, other, "push", "-q", "origin", branch)
	tip := gitIn(t, other, "rev-parse", "HEAD")

	wt2, err := mgr.Create(ctx, git.WorktreeOptions{ObjectiveID: "obj-abc12345", TaskID: "task-2", BranchName: branch})
	if err != nil {
		t.Fatalf("second Create on the same branch: %v", err)
	}
	if got := gitIn(t, wt2.Path, "rev-parse", "HEAD"); got != tip {
		t.Errorf("second worktree HEAD = %s, want origin's tip %s", got, tip)
	}
}
