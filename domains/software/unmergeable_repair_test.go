package software

// Phase 40 step 6a, the git part of the repair. A note for the next action on
// the plan's item "a plan that rebases or force-pushes is refused (step 2)":
// it cannot be shown, so no test claims it. Step 2 checks a coding action's
// tool allow-list against the stream's exact entries (`Bash(git:*)` is one of
// them, in every stream) and its branch against the stream's patterns; it
// reads neither the git subcommand nor its flags, so `git rebase` and
// `git push --force` are inside what it admits. The guard against a rewrite
// is step 3's check after the push (TestUnmergeableRepairByRebaseAndForcePushIsCaught):
// it detects, it does not prevent. Only a narrower tool entry or branch
// protection on the remote would prevent.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	repairBranch    = "karakuri/phase-99-repair"
	repairBase      = "intro\nshared line\noutro\n"
	repairBranchHas = "branch side"
	repairMainHas   = "main side"
)

// repairFixture is a bare origin whose main and stream branch conflict in
// docs/roadmap.md, and a clone of it checked out on the branch.
type repairFixture struct {
	origin    string
	work      string
	branchTip string
}

func newRepairFixture(t *testing.T) repairFixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	isolateGit(t)
	// The host may hand git config through the environment (core.hooksPath);
	// pushLog.env builds on GIT_CONFIG_COUNT, so start from none.
	t.Setenv("GIT_CONFIG_PARAMETERS", "")
	t.Setenv("GIT_CONFIG_COUNT", "0")

	root := t.TempDir()
	seed := filepath.Join(root, "seed")
	f := repairFixture{origin: filepath.Join(root, "origin.git"), work: filepath.Join(root, "work")}
	roadmap := filepath.Join(seed, "docs", "roadmap.md")

	gitIn(t, root, "init", "--bare", "-b", "main", f.origin)
	gitIn(t, root, "clone", f.origin, seed)
	if err := os.MkdirAll(filepath.Dir(roadmap), 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, roadmap, repairBase)
	gitIn(t, seed, "add", ".")
	gitIn(t, seed, "commit", "-m", "base")
	gitIn(t, seed, "push", "origin", "HEAD:refs/heads/main")

	gitIn(t, seed, "checkout", "-b", repairBranch)
	writeFile(t, roadmap, "intro\n"+repairBranchHas+"\noutro\n")
	gitIn(t, seed, "commit", "-am", "branch edit")
	f.branchTip = gitIn(t, seed, "rev-parse", "HEAD")
	gitIn(t, seed, "push", "origin", "HEAD:refs/heads/"+repairBranch)

	gitIn(t, seed, "checkout", "main")
	writeFile(t, roadmap, "intro\n"+repairMainHas+"\noutro\n")
	gitIn(t, seed, "commit", "-am", "main edit")
	gitIn(t, seed, "push", "origin", "HEAD:refs/heads/main")

	gitIn(t, root, "clone", "--branch", repairBranch, f.origin, f.work)
	return f
}

// gitMayFail runs git in dir with extra environment and returns its output and
// error: a conflicting merge or rebase exits non-zero by design.
func gitMayFail(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// agentEnv is the push log's environment as KEY=VALUE entries.
func agentEnv(m map[string]string, extra ...string) []string {
	out := append([]string{}, extra...)
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

// check installs the push log, takes the before refs, runs run with the
// environment the agent would get, and returns what checkPushedRefs says.
func (f repairFixture) check(t *testing.T, run func(env map[string]string)) (reason string, violated bool) {
	t.Helper()
	ctx := context.Background()
	log := installPushLog(ctx, f.work)
	t.Cleanup(func() {
		if err := log.remove(); err != nil {
			t.Errorf("removing the push log: %v", err)
		}
	})
	before, beforeErr := readRemoteRefs(ctx, f.work)
	run(log.env(nil))
	v, err := checkPushedRefs(ctx, f.work, declaredRefs(repairBranch, f.work), before, beforeErr, log)
	if err != nil {
		t.Fatalf("checkPushedRefs: %v; want a comparison, not an error", err)
	}
	if v == nil {
		return "", false
	}
	return v.Reason, true
}

func (f repairFixture) resolveKeepingBoth(t *testing.T) {
	t.Helper()
	writeFile(t, filepath.Join(f.work, "docs", "roadmap.md"),
		"intro\n"+repairBranchHas+"\n"+repairMainHas+"\noutro\n")
	gitIn(t, f.work, "add", "docs/roadmap.md")
}

func TestUnmergeableRepairMergeKeepsBothSidesOfARoadmapConflict(t *testing.T) {
	f := newRepairFixture(t)
	var newTip string
	reason, violated := f.check(t, func(env map[string]string) {
		e := agentEnv(env)
		if out, err := gitMayFail(f.work, e, "merge", "origin/main"); err == nil {
			t.Fatalf("git merge origin/main succeeded, want a conflict in docs/roadmap.md:\n%s", out)
		}
		f.resolveKeepingBoth(t)
		if out, err := gitMayFail(f.work, e, "commit", "--no-edit"); err != nil {
			t.Fatalf("git commit --no-edit: %v\n%s", err, out)
		}
		if out, err := gitMayFail(f.work, e, "push", "origin", "HEAD:refs/heads/"+repairBranch); err != nil {
			t.Fatalf("git push: %v\n%s", err, out)
		}
		newTip = gitIn(t, f.work, "rev-parse", "HEAD")
	})
	if violated {
		t.Errorf("checkPushedRefs reported %q for a merge of main into the branch, want none", reason)
	}
	if parents := strings.Fields(gitIn(t, f.work, "rev-list", "--parents", "-n1", newTip)); len(parents) != 3 {
		t.Errorf("new tip %s has %d parents, want exactly 2 (%v)", newTip, len(parents)-1, parents)
	}
	pushed := gitIn(t, f.work, "--git-dir", f.origin, "show", repairBranch+":docs/roadmap.md")
	for _, line := range []string{repairBranchHas, repairMainHas} {
		if !strings.Contains(pushed, line) {
			t.Errorf("pushed docs/roadmap.md lacks %q:\n%s", line, pushed)
		}
	}
	if out, err := gitMayFail(f.work, nil, "merge-base", "--is-ancestor", f.branchTip, newTip); err != nil {
		t.Errorf("previous tip %s is not an ancestor of the new tip %s: %v\n%s", f.branchTip, newTip, err, out)
	}
	if remote := gitIn(t, f.work, "ls-remote", "origin", "refs/heads/"+repairBranch); !strings.HasPrefix(remote, newTip) {
		t.Errorf("origin's %s = %q, want the new tip %s", repairBranch, remote, newTip)
	}
}

func TestUnmergeableRepairByRebaseAndForcePushIsCaught(t *testing.T) {
	f := newRepairFixture(t)
	reason, violated := f.check(t, func(env map[string]string) {
		e := agentEnv(env)
		if out, err := gitMayFail(f.work, e, "rebase", "origin/main"); err == nil {
			t.Fatalf("git rebase origin/main succeeded, want a conflict in docs/roadmap.md:\n%s", out)
		}
		f.resolveKeepingBoth(t)
		if out, err := gitMayFail(f.work, agentEnv(env, "GIT_EDITOR=true"), "rebase", "--continue"); err != nil {
			t.Fatalf("git rebase --continue: %v\n%s", err, out)
		}
		if out, err := gitMayFail(f.work, e, "push", "--force", "origin", "HEAD:refs/heads/"+repairBranch); err != nil {
			t.Fatalf("git push --force: %v\n%s", err, out)
		}
	})
	if !violated {
		t.Fatalf("checkPushedRefs reported no violation for a rebased and force-pushed %s, want one that names it", repairBranch)
	}
	if !strings.Contains(reason, repairBranch) {
		t.Errorf("violation %q does not name %s", reason, repairBranch)
	}
	if !strings.Contains(reason, "drops its previous tip") {
		t.Errorf("violation %q does not say the previous tip was dropped", reason)
	}
}
