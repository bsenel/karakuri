package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/git"
)

func TestWorktreeCreateRemove(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	writeFile := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("README.md", "test")
	for _, args := range [][]string{
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
		{"add", "."},
		{"commit", "-m", "init"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if err := c.Run(); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.GitConfig{RepoPath: dir, WorktreeBase: "worktrees", BaseBranch: "main", BranchPrefix: "karakuri"}
	mgr, err := git.NewGoGitWorktreeManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	objID := objective.ObjectiveID("obj-abc12345")
	wt, err := mgr.Create(ctx, git.WorktreeOptions{ObjectiveID: objID, TaskID: "task-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(wt.Path)); err != nil {
		t.Fatalf("worktree dir missing: %v", err)
	}
	if err := mgr.Remove(ctx, "task-1"); err != nil {
		t.Fatal(err)
	}
}
