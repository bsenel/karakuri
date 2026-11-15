package software

// What was pushed is checked, not assumed (Phase 40 step 3). The tool
// allow-list bounds what a coding agent is told it may run, but the push
// happens inside its subprocess, so the pack reads origin's refs around the
// run and fails an action that moved a ref it did not declare. This detects;
// branch protection on the remote is what prevents.

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bsenel/karakuri/internal/core/environment"
)

const headsPrefix = "refs/heads/"

// readRemoteRefs is remoteRefs under salvageTimeout and outside the action's
// own cancellation: the read after a run comes when that run's context may be
// the reason it ended.
func readRemoteRefs(ctx context.Context, worktree string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), salvageTimeout)
	defer cancel()
	return remoteRefs(ctx, worktree)
}

// checkPushedRefs reads origin's refs after a run and compares them with
// before, the read taken before it (nil when that read failed with
// beforeErr). It returns the bounds violation, typed so the loop and reconcile
// never read it out of Error's text, or nil when every ref that moved was
// declared and kept its previous tip, and an error when the comparison could
// not be made, which the caller must not treat as a pass.
func checkPushedRefs(ctx context.Context, worktree string, declared map[string]bool, before map[string]string, beforeErr error) (*environment.BoundsViolation, error) {
	if beforeErr != nil {
		return nil, fmt.Errorf("what the run pushed could not be checked: %w", beforeErr)
	}
	after, err := readRemoteRefs(ctx, worktree)
	if err != nil {
		return nil, fmt.Errorf("what the run pushed could not be checked: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), salvageTimeout)
	defer cancel()
	violations, refs, err := pushedRefsViolations(ctx, worktree, declared, before, after)
	if err != nil {
		return nil, fmt.Errorf("what the run pushed could not be checked: %w", err)
	}
	if len(violations) == 0 {
		return nil, nil
	}
	return &environment.BoundsViolation{
		Reason: "bounds violation: " + strings.Join(violations, "; "),
		Refs:   refs,
	}, nil
}

// remoteRefs maps every branch and tag on the worktree's origin to its SHA. An
// origin that cannot be read is an error, never an empty map (AGENTS.md rule
// 10): an empty map would read as "nothing moved".
func remoteRefs(ctx context.Context, worktree string) (map[string]string, error) {
	out, err := gitOutput(ctx, filepath.Clean(worktree), nil, "ls-remote", "--refs", "--heads", "--tags", "origin")
	if err != nil {
		return nil, fmt.Errorf("reading origin's refs: %w", err)
	}
	refs := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if sha, ref, ok := strings.Cut(strings.TrimSpace(line), "\t"); ok {
			refs[ref] = sha
		}
	}
	return refs, nil
}

// declaredRefs are the refs a coding action may move: params.branch, which the
// decide step has already held to the stream template's branch patterns
// (stream_limits.go), and the salvage branch the pack itself pushes for this
// worktree. An action with no branch declares only the salvage branch.
func declaredRefs(branch, worktree string) map[string]bool {
	dir := filepath.Clean(worktree)
	declared := map[string]bool{
		headsPrefix + salvageBranchPrefix + filepath.Base(filepath.Dir(dir)) + "/" + filepath.Base(dir): true,
	}
	if branch != "" {
		declared[headsPrefix+branch] = true
	}
	return declared
}

// pushedRefsViolations compares origin's refs before and after a run and
// returns one line per bounds violation: an undeclared ref that was created,
// moved or deleted, or a declared ref that no longer contains its previous
// tip, and the refs those lines name. An error means the comparison could not
// be made, which is not a pass.
func pushedRefsViolations(ctx context.Context, worktree string, declared map[string]bool, before, after map[string]string) ([]string, []string, error) {
	names := map[string]bool{}
	for ref := range before {
		names[ref] = true
	}
	for ref := range after {
		names[ref] = true
	}
	sorted := make([]string, 0, len(names))
	for ref := range names {
		sorted = append(sorted, ref)
	}
	sort.Strings(sorted)

	var violations, refs []string
	for _, ref := range sorted {
		old, now := before[ref], after[ref]
		if old == now {
			continue
		}
		if declared[ref] {
			// Created, or moved forward, is what a declared push does.
			if old == "" {
				continue
			}
			if now != "" {
				kept, err := containsTip(ctx, worktree, ref, old, now)
				if err != nil {
					return nil, nil, err
				}
				if kept {
					continue
				}
			}
			violations = append(violations, refMove(ref, old, now)+", which drops its previous tip")
			refs = append(refs, ref)
			continue
		}
		violations = append(violations, refMove(ref, old, now)+", outside the action's declared refs")
		refs = append(refs, ref)
	}
	return violations, refs, nil
}

// containsTip reports whether commit now has old in its history. now is
// fetched from origin when the worktree's repository does not hold it.
func containsTip(ctx context.Context, worktree, ref, old, now string) (bool, error) {
	dir := filepath.Clean(worktree)
	if _, err := gitOutput(ctx, dir, nil, "cat-file", "-e", now+"^{commit}"); err != nil {
		if _, err := gitOutput(ctx, dir, nil, "fetch", "--no-tags", "origin", ref); err != nil {
			return false, fmt.Errorf("fetching %s to check that it keeps %s: %w", ref, old, err)
		}
	}
	_, err := gitOutput(ctx, dir, nil, "merge-base", "--is-ancestor", old, now)
	var exit *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return false, nil
	}
	return false, fmt.Errorf("checking that %s keeps %s: %w", ref, old, err)
}

// refMove names a ref and its old and new SHA in the words an operator reads.
func refMove(ref, old, now string) string {
	name := strings.TrimPrefix(ref, headsPrefix)
	switch {
	case old == "":
		return fmt.Sprintf("%s was created at %s", name, now)
	case now == "":
		return fmt.Sprintf("%s was deleted (was %s)", name, old)
	}
	return fmt.Sprintf("%s moved from %s to %s", name, old, now)
}
