package software

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// salvageTimeout bounds every git command one salvage runs, together. It runs
// after a killed run, when the action's own context may already be cancelled.
const salvageTimeout = 60 * time.Second

// salvageBranchPrefix is where a failed run's uncommitted work is kept. It is
// never the item's branch, so nothing a failed run wrote lands on a pull
// request until a later pass decides to take it (Phase 44 step 2).
const salvageBranchPrefix = "karakuri/salvage/"

// urlCredentials matches the user-info part of a URL, which git may echo back
// in a failed push's message when origin carries a token.
var urlCredentials = regexp.MustCompile(`://[^/@\s]+@`)

// salvage commits whatever a failed run left uncommitted in worktree, untracked
// files included, to karakuri/salvage/<objective>/<action> on origin and
// returns the keys to put on the action's result: salvage_branch and
// salvage_sha, or salvage_error when it could not, or nothing when the tree is
// clean. The loop lays worktrees out as worktrees/<objective>/<task>, so the
// two names are the worktree's parent directory and its own.
//
// The item's branch never moves: the commit is built from a temporary index
// with write-tree and commit-tree, parented on the worktree's HEAD, and pushed
// by SHA, so neither the checked-out branch nor its index is touched.
func salvage(ctx context.Context, worktree string) map[string]any {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), salvageTimeout)
	defer cancel()
	sha, branch, err := salvageWorktree(ctx, worktree)
	switch {
	case err != nil:
		return map[string]any{"salvage_error": err.Error()}
	case sha == "":
		return nil
	}
	return map[string]any{"salvage_branch": branch, "salvage_sha": sha}
}

// salvageWorktree does the work of salvage; an empty sha with no error means
// there was nothing to keep.
func salvageWorktree(ctx context.Context, worktree string) (sha, branch string, err error) {
	dir := filepath.Clean(worktree)
	top, err := gitOutput(ctx, dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", fmt.Errorf("worktree %s is not a git repository: %w", dir, err)
	}
	// A directory inside some other repository would answer rev-parse with
	// that repository's root, and salvaging it would commit the wrong tree.
	if !sameDir(top, dir) {
		return "", "", fmt.Errorf("worktree %s is not the root of a git worktree (its repository is %s)", dir, top)
	}
	status, err := gitOutput(ctx, dir, nil, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return "", "", fmt.Errorf("reading the worktree's status: %w", err)
	}
	if status == "" {
		return "", "", nil
	}
	head, err := gitOutput(ctx, dir, nil, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", "", fmt.Errorf("reading the worktree's HEAD: %w", err)
	}

	tmp, err := os.MkdirTemp("", "karakuri-salvage-")
	if err != nil {
		return "", "", fmt.Errorf("creating a temporary index: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	index := []string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}
	if _, err := gitOutput(ctx, dir, index, "read-tree", head); err != nil {
		return "", "", fmt.Errorf("loading HEAD into a temporary index: %w", err)
	}
	if _, err := gitOutput(ctx, dir, index, "add", "--all", "--", "."); err != nil {
		return "", "", fmt.Errorf("staging the worktree's changes: %w", err)
	}
	tree, err := gitOutput(ctx, dir, index, "write-tree")
	if err != nil {
		return "", "", fmt.Errorf("writing the salvage tree: %w", err)
	}

	branch = salvageBranchPrefix + filepath.Base(filepath.Dir(dir)) + "/" + filepath.Base(dir)
	msg := "salvage: uncommitted work of a failed coding action\n\n" +
		"Kept by Karakuri from worktree " + filepath.Base(dir) + " after its run failed; " +
		"parented on " + head + ", the item's head when the run ended.\n"
	sha, err = gitOutput(ctx, dir, nil, "commit-tree", tree, "-p", head, "-m", msg)
	if err != nil {
		return "", "", fmt.Errorf("committing the salvage tree: %w", err)
	}
	if _, err := gitOutput(ctx, dir, nil, "push", "origin", sha+":refs/heads/"+branch); err != nil {
		return "", "", fmt.Errorf("pushing %s to origin: %w", branch, err)
	}
	return sha, branch, nil
}

// sameDir reports whether a and b name the same directory once symlinks are
// resolved; git reports its toplevel with them resolved.
func sameDir(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

// gitOutput runs git in dir with env added to the server's own environment and
// returns its trimmed stdout. A failure carries git's first line of stderr,
// with any URL credentials removed.
func gitOutput(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	c := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- fixed git subcommands; the variable arguments are SHAs git printed and a branch name built from the worktree's path; no shell
	c.Dir = dir
	// A push that needs a password fails rather than waiting on a terminal
	// nobody is watching.
	c.Env = append(append(os.Environ(), "GIT_TERMINAL_PROMPT=0"), env...)
	c.WaitDelay = 5 * time.Second
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		line, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		line = urlCredentials.ReplaceAllString(line, "://[redacted]@")
		if line == "" {
			return "", fmt.Errorf("git %s: %w", args[0], err)
		}
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, line)
	}
	return strings.TrimSpace(stdout.String()), nil
}
