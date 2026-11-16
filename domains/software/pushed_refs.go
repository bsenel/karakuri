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
	"os"
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
// beforeErr). log is the pre-push log installed for the run. It returns the
// bounds violation, typed so the loop and reconcile never read it out of
// Error's text, or nil when every ref the action moved was declared and kept
// its previous tip, and an error when the comparison could not be made, which
// the caller must not treat as a pass.
func checkPushedRefs(ctx context.Context, worktree string, declared map[string]bool, before map[string]string, beforeErr error, log *pushLog) (*environment.BoundsViolation, error) {
	if beforeErr != nil {
		return nil, fmt.Errorf("what the run pushed could not be checked: %w", beforeErr)
	}
	after, err := readRemoteRefs(ctx, worktree)
	if err != nil {
		return nil, fmt.Errorf("what the run pushed could not be checked: %w", err)
	}
	deleted, deletedErr := log.deleted()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), salvageTimeout)
	defer cancel()
	violations, refs, err := pushedRefsViolations(ctx, worktree, declared, before, after, deleted, deletedErr)
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
// returns one line per bounds violation, and the refs those lines name: a
// declared ref that no longer contains its previous tip, or an undeclared ref
// the action moved. Other actors move refs on origin during a run (the owner
// merges, other streams and dependabot push), so an undeclared ref counts only
// when the action could have moved it: created or updated to a commit the
// worktree holds that no ref origin had before already reached, or deleted by
// a push its pre-push hook logged (deleted; deletedErr when the log could not
// be kept or read). An error means the comparison could not be made, which is
// not a pass.
func pushedRefsViolations(ctx context.Context, worktree string, declared map[string]bool, before, after map[string]string, deleted map[string]bool, deletedErr error) ([]string, []string, error) {
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
		if now == "" {
			if deletedErr != nil {
				return nil, nil, fmt.Errorf("%s was deleted and whether the action's push deleted it is not known: %w",
					strings.TrimPrefix(ref, headsPrefix), deletedErr)
			}
			if !deleted[ref] {
				continue
			}
		} else {
			made, err := madeByAction(ctx, worktree, now, before)
			if err != nil {
				return nil, nil, err
			}
			if !made {
				continue
			}
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

// madeByAction reports whether the action could have moved an undeclared ref
// to now: the worktree holds commit now, and no ref origin had before the run
// already reached it. The after read brings in no objects and nothing fetches
// an undeclared ref, so a commit another actor pushed is one the worktree
// lacks. A before tip the worktree lacks is skipped (--ignore-missing).
func madeByAction(ctx context.Context, worktree, now string, before map[string]string) (bool, error) {
	dir := filepath.Clean(worktree)
	if _, err := gitOutput(ctx, dir, nil, "cat-file", "-e", now+"^{commit}"); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && ctx.Err() == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking whether the worktree holds %s: %w", now, err)
	}
	tips := map[string]bool{}
	for _, sha := range before {
		tips[sha] = true
	}
	args := []string{"rev-list", "-n", "1", "--ignore-missing", now}
	if len(tips) > 0 {
		args = append(args, "--not")
		for sha := range tips {
			args = append(args, sha)
		}
		sort.Strings(args[len(args)-len(tips):])
	}
	out, err := gitOutput(ctx, dir, nil, args...)
	if err != nil {
		return false, fmt.Errorf("checking whether origin already reached %s: %w", now, err)
	}
	return out != "", nil
}

// pushLog is a pre-push hook installed for one delegated run's worktree,
// which appends each line git hands it (<local ref> <local sha> <remote ref>
// <remote sha>) to a log. A ref ls-remote shows deleted is the action's only
// when this log names it: a deletion leaves no commit to recognise. Best
// effort: a push with --no-verify skips the hook. The hook and its log live in
// a directory outside the worktree, so the run leaves nothing to commit.
type pushLog struct {
	worktree     string
	dir          string // holds the hooks and the log; removed with the hook
	hadHooksPath bool   // the worktree's own core.hooksPath before the run
	hooksPath    string
	err          error // why the log could not be kept, when it could not
}

// pushLogFile is the log's name inside pushLog.dir.
const pushLogFile = "pushes.log"

// installPushLog points the worktree's own core.hooksPath at a fresh
// directory holding the logging pre-push hook. The setting is per worktree
// (extensions.worktreeConfig, which is enabled and left on, since a sibling
// run may rely on it), so the clone's other worktrees keep their hooks; and it
// is git configuration, not environment, so every push from the worktree runs
// the hook whoever starts it. The hooks the worktree had are linked in, and
// the hook runs the previous pre-push after logging. A failure to install is
// recorded on the log and surfaces only when a deletion needs it.
func installPushLog(ctx context.Context, worktree string) *pushLog {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), salvageTimeout)
	defer cancel()
	l := &pushLog{worktree: filepath.Clean(worktree)}
	if err := l.install(ctx); err != nil {
		l.err = fmt.Errorf("the pre-push log could not be installed: %w", err)
	}
	return l
}

func (l *pushLog) install(ctx context.Context) error {
	previous, err := gitOutput(ctx, l.worktree, nil, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	if err != nil {
		return err
	}
	if _, err := gitOutput(ctx, l.worktree, nil, "config", "extensions.worktreeConfig", "true"); err != nil {
		return err
	}
	if path, err := gitOutput(ctx, l.worktree, nil, "config", "--worktree", "--get", "core.hooksPath"); err == nil {
		l.hadHooksPath, l.hooksPath = true, path
	}
	dir, err := os.MkdirTemp("", "karakuri-push-log-")
	if err != nil {
		return err
	}
	l.dir = dir
	if entries, err := os.ReadDir(previous); err == nil {
		for _, e := range entries {
			if name := e.Name(); name != "pre-push" && !strings.HasSuffix(name, ".sample") {
				if err := os.Symlink(filepath.Join(previous, name), filepath.Join(dir, name)); err != nil {
					return err
				}
			}
		}
	}
	script := "#!/bin/sh\n" +
		"# Installed by Karakuri for one delegated run: logs what each push sends.\n" +
		"input=$(cat)\n" +
		"printf '%s\\n' \"$input\" >> " + shellQuote(filepath.Join(dir, pushLogFile)) + "\n" +
		"prev=" + shellQuote(filepath.Join(previous, "pre-push")) + "\n" +
		"if [ -x \"$prev\" ]; then printf '%s\\n' \"$input\" | \"$prev\" \"$@\"; fi\n"
	// #nosec G306 -- git runs a hook only when it is executable; the file sits in a 0o700 directory of the server's own and holds no secret
	if err := os.WriteFile(filepath.Join(dir, "pre-push"), []byte(script), 0o700); err != nil {
		return err
	}
	_, err = gitOutput(ctx, l.worktree, nil, "config", "--worktree", "core.hooksPath", dir)
	return err
}

// deleted names the refs the log shows a push deleting (an all-zero local
// SHA). A log never written is a run that pushed nothing.
func (l *pushLog) deleted() (map[string]bool, error) {
	if l == nil {
		return nil, errors.New("no pre-push log was installed")
	}
	if l.err != nil {
		return nil, l.err
	}
	data, err := os.ReadFile(filepath.Join(l.dir, pushLogFile))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the pre-push log: %w", err)
	}
	deleted := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) == 4 && strings.Trim(f[1], "0") == "" {
			deleted[f[2]] = true
		}
	}
	return deleted, nil
}

// pushed maps every remote ref the log shows a push sending (created,
// updated or deleted) to the SHA it was sent, "" for a deletion. Stub: the
// code slice of Phase 40 step 3 fills it in.
func (l *pushLog) pushed() (map[string]string, error) {
	return nil, nil
}

// remove restores the worktree's core.hooksPath and deletes the hook and its
// log. It runs outside the action's cancellation, which may be why it ended.
func (l *pushLog) remove(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), salvageTimeout)
	defer cancel()
	if l.dir == "" {
		return nil
	}
	var err error
	if l.hadHooksPath {
		_, err = gitOutput(ctx, l.worktree, nil, "config", "--worktree", "core.hooksPath", l.hooksPath)
	} else {
		_, err = gitOutput(ctx, l.worktree, nil, "config", "--worktree", "--unset", "core.hooksPath")
		// Exit 5: install failed before it set the key, so nothing to unset.
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 5 {
			err = nil
		}
	}
	if err != nil {
		err = fmt.Errorf("restoring the worktree's core.hooksPath: %w", err)
	}
	return errors.Join(err, os.RemoveAll(l.dir))
}

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
