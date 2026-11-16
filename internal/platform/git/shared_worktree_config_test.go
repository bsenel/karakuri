package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gitlib "github.com/go-git/go-git/v5"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/platform/git"
)

// Phase 40 step 3, review of ecfae91, case (e): the software pack's pre-push
// log (domains/software installPushLog) runs `git config
// extensions.worktreeConfig true` in a delegated run's worktree. Without
// --worktree that writes the SHARED repository config, the deployment's main
// repository that GoGitWorktreeManager opens with go-git, and it is left on.
// The server must still open that repository afterwards: a failed open makes
// NewGoGitWorktreeManager try PlainInit on an existing repository, and the
// server does not start. The steps installPushLog takes are done here with the
// git CLI, so the import direction stays inward.

func sharedGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestSharedWorktreeConfigStillOpensTheDeploymentRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
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

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	deployment := filepath.Join(root, "deployment")
	action := filepath.Join(root, "worktrees", "objective", "task")
	hooks := filepath.Join(root, "push-log")
	sharedGit(t, root, "init", "--bare", "-b", "main", origin)
	sharedGit(t, root, "clone", origin, deployment)
	if err := os.WriteFile(filepath.Join(deployment, "README.md"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sharedGit(t, deployment, "add", ".")
	sharedGit(t, deployment, "commit", "-m", "base")
	sharedGit(t, deployment, "push", "origin", "HEAD:refs/heads/main")
	mainSHA := sharedGit(t, deployment, "rev-parse", "HEAD")
	if err := os.MkdirAll(filepath.Dir(action), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(hooks, 0o750); err != nil {
		t.Fatal(err)
	}
	sharedGit(t, deployment, "worktree", "add", "-b", "karakuri/objective/task", action, "main")

	// What installPushLog does in the action's worktree.
	sharedGit(t, action, "config", "extensions.worktreeConfig", "true")
	sharedGit(t, action, "config", "--worktree", "core.hooksPath", hooks)
	if got := sharedGit(t, deployment, "config", "--get", "extensions.worktreeConfig"); got != "true" {
		t.Fatalf("fixture: the deployment repository's config has extensions.worktreeConfig = %q, want true written to the shared config", got)
	}

	repo, err := gitlib.PlainOpen(deployment)
	if err != nil {
		t.Fatalf("gitlib.PlainOpen(deployment) after installPushLog set extensions.worktreeConfig: %v; want the deployment repository to keep opening, or GoGitWorktreeManager cannot start", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("HEAD of the deployment repository does not resolve after extensions.worktreeConfig was set: %v", err)
	}
	if head.Hash().String() != mainSHA {
		t.Errorf("HEAD = %s, want %s, the deployment's main", head.Hash(), mainSHA)
	}

	cfg := config.GitConfig{RepoPath: deployment, WorktreeBase: "worktrees", BaseBranch: "main", BranchPrefix: "karakuri"}
	if _, err := git.NewGoGitWorktreeManager(cfg); err != nil {
		t.Errorf("NewGoGitWorktreeManager on the deployment repository after extensions.worktreeConfig was set: %v; want the server to start", err)
	}
}
