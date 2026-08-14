package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/git"
)

// Regression test for SECURITY_AUDIT.md F-01: an attacker-controlled objective
// ID (from POST /loops) must not let the worktree manager create a directory
// outside its base. Before the fix, os.MkdirAll created a directory outside the
// repo root; after the fix, Create rejects the traversal and creates nothing.
func TestWorktreeObjectiveIDTraversalRejected(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "F01-ESCAPE")

	initGitRepo(t, root)
	cfg := config.GitConfig{RepoPath: root, WorktreeBase: "worktrees", BaseBranch: "main", BranchPrefix: "karakuri"}
	mgr, err := git.NewGoGitWorktreeManager(cfg)
	if err != nil {
		t.Fatal(err)
	}

	up := strings.Repeat("../", strings.Count(root, "/")+2)
	cases := []objective.ObjectiveID{
		objective.ObjectiveID(up + strings.TrimPrefix(target, "/")),
		"../../../../etc",
		"..",
		"a/b",
	}
	for _, evil := range cases {
		if _, err := mgr.Create(context.Background(), git.WorktreeOptions{ObjectiveID: evil, TaskID: "t1"}); err == nil {
			t.Errorf("Create(%q) returned nil error; expected rejection", evil)
		}
	}
	if _, err := os.Stat(target); err == nil {
		t.Errorf("F-01: directory was created outside the repo root at %s", target)
	}
}

// A well-formed objective ID still produces a worktree inside the base.
func TestWorktreeCreateWithinBase(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	initGitRepo(t, root)
	cfg := config.GitConfig{RepoPath: root, WorktreeBase: "worktrees", BaseBranch: "main", BranchPrefix: "karakuri"}
	mgr, err := git.NewGoGitWorktreeManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := mgr.Create(context.Background(), git.WorktreeOptions{ObjectiveID: "obj-abc12345", TaskID: "task-1"})
	if err != nil {
		t.Fatalf("Create with a valid id failed: %v", err)
	}
	base := filepath.Join(root, "worktrees")
	if rel, err := filepath.Rel(base, wt.Path); err != nil || strings.HasPrefix(rel, "..") {
		t.Errorf("worktree path %q is not within base %q", wt.Path, base)
	}
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("config", "user.email", "a@b.c")
	run("config", "user.name", "t")
	run("add", ".")
	run("commit", "-m", "init")
}
