package software

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// Phase 40 step 10: a proposal stream repairs an unmergeable pull request by
// merging main into its branch. The files main changed arrive in that push but
// are not the run's change, so they are not a path violation.

// mergeMainFixture is a backlog branch already on origin with its own
// docs/roadmap.md, and an action declaring item.txt and docs/roadmap.md.
type mergeMainFixture struct {
	pushedRefsFixture
	action environment.Action
	branch string
}

func newMergeMainFixture(t *testing.T) mergeMainFixture {
	t.Helper()
	const branch = "karakuri/tech-backlog-2026-11-18"
	f := mergeMainFixture{pushedRefsFixture: newPushedRefsFixture(t), branch: branch}
	if err := os.MkdirAll(filepath.Join(f.worktree, "docs"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(f.worktree, "docs/roadmap.md"), "branch roadmap\n")
	gitIn(t, f.worktree, "add", "docs/roadmap.md")
	gitIn(t, f.worktree, "commit", "-m", "branch roadmap")
	gitIn(t, f.worktree, "push", "origin", "HEAD:refs/heads/"+branch)
	f.action = salvageAction(f.worktree)
	f.action.Params["branch"] = branch
	f.action.Params["paths"] = strs("item.txt", "docs/roadmap.md")
	return f
}

// mainMoves commits the named files on origin's main, as other merged pull
// requests do after the branch was cut.
func (f mergeMainFixture) mainMoves(t *testing.T, names ...string) {
	t.Helper()
	gitIn(t, f.clone, "checkout", "main")
	for _, name := range names {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(f.clone, name)), 0o750); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(f.clone, name), "main "+name+"\n")
		gitIn(t, f.clone, "add", name)
	}
	gitIn(t, f.clone, "commit", "-m", "main moves")
	gitIn(t, f.clone, "push", "origin", "HEAD:refs/heads/main")
}

// mergeMainResolvingRoadmap merges origin/main into the worktree's branch,
// resolves the docs/roadmap.md conflict by keeping both sides, and pushes.
func mergeMainResolvingRoadmap(t *testing.T, wt, branch string) {
	t.Helper()
	gitIn(t, wt, "fetch", "origin")
	merge := exec.Command("git", "merge", "--no-edit", "origin/main")
	merge.Dir = wt
	if out, err := merge.CombinedOutput(); err == nil {
		t.Fatalf("expected a conflict on docs/roadmap.md, merge went clean: %s", out)
	}
	writeFile(t, filepath.Join(wt, "docs/roadmap.md"), "branch roadmap\nmain roadmap\n")
	gitIn(t, wt, "add", "docs/roadmap.md")
	gitIn(t, wt, "commit", "--no-edit")
	gitIn(t, wt, "push", "origin", "HEAD:refs/heads/"+branch)
}

func TestAProposalActionThatMergedMainWithAConflictIsNotAPathViolation(t *testing.T) {
	f := newMergeMainFixture(t)
	f.mainMoves(t, "internal/x.go", "README.md", "docs/roadmap.md")
	res := actWith(t, f.worktree, f.action, func(t *testing.T, wt string) error {
		mergeMainResolvingRoadmap(t, wt, f.branch)
		return nil
	})
	if !res.Success || res.BoundsViolation != nil {
		t.Fatalf("a merge of main resolved in docs/roadmap.md was reported as leaving the declared paths: error %q, violation %+v", res.Error, res.BoundsViolation)
	}
}

func TestAProposalActionThatCommittedOutsideItsPathsOnABacklogBranchIsCaught(t *testing.T) {
	f := newMergeMainFixture(t)
	res := actWith(t, f.worktree, f.action, func(t *testing.T, wt string) error {
		if err := os.MkdirAll(filepath.Join(wt, "internal/feature"), 0o750); err != nil {
			t.Fatal(err)
		}
		commitIn(t, wt, "internal/feature/x.go")
		gitIn(t, wt, "push", "origin", "HEAD:refs/heads/"+f.branch)
		return nil
	})
	if res.Success || res.BoundsViolation == nil || !strings.Contains(res.Error, "internal/feature/x.go") {
		t.Fatalf("a commit of internal/feature/x.go passed or went unnamed: success=%v error %q violation %+v", res.Success, res.Error, res.BoundsViolation)
	}
}

func TestAProposalActionThatMergedMainAndCommittedOutsideNamesOnlyItsOwnFile(t *testing.T) {
	f := newMergeMainFixture(t)
	f.mainMoves(t, "internal/x.go", "README.md")
	res := actWith(t, f.worktree, f.action, func(t *testing.T, wt string) error {
		gitIn(t, wt, "fetch", "origin")
		gitIn(t, wt, "merge", "--no-edit", "origin/main")
		if err := os.MkdirAll(filepath.Join(wt, "internal/feature"), 0o750); err != nil {
			t.Fatal(err)
		}
		commitIn(t, wt, "internal/feature/x.go")
		gitIn(t, wt, "push", "origin", "HEAD:refs/heads/"+f.branch)
		return nil
	})
	if res.Success || res.BoundsViolation == nil {
		t.Fatalf("a commit of internal/feature/x.go after merging main passed: %+v", res)
	}
	if !strings.Contains(res.Error, "internal/feature/x.go") {
		t.Errorf("Error = %q, want it to name internal/feature/x.go", res.Error)
	}
	for _, mains := range []string{"internal/x.go", "README.md"} {
		if strings.Contains(res.Error, mains) {
			t.Errorf("Error = %q names %s, a file main changed and the run did not", res.Error, mains)
		}
	}
}
