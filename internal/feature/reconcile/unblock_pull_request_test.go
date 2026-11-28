package reconcile

// Phase 40 step 14b, owner's review Blocker 1: the planner is told to post its
// diagnosis on the stream's pull request only when Unblock.PullRequest is set,
// so reconcile must fill it on the real pass path. For unmergeable_pr the
// blocker's subject is the branch, and the pull request is the one whose head
// is that branch, as the sensed snapshot names it. The loop's half (a set
// number makes the prompt ask for `gh pr comment <n>`, zero asks for nothing)
// is held by TestABlockerWithAnOpenPullRequestTellsThePlannerToCommentTheDiagnosis
// and TestABlockerWithNoPullRequestSaysNothingAboutAComment in feature/loop.

import (
	"testing"

	"github.com/bsenel/karakuri/internal/core/reconcile"
)

const blockedBranch = "karakuri/phase-99-x"

// listUnmergeablePRs makes the sensed git snapshot list these branches, with
// their pull request numbers, as the stream's pull requests that cannot be
// merged, the way the delivery-work environment's snapshot does.
func (f *fixture) listUnmergeablePRs(prs map[string]int) {
	branches := []string{}
	for b := range prs {
		branches = append(branches, b)
	}
	for _, e := range f.envs {
		if e.id == branchEnvironment {
			e.state = map[string]any{"unmergeable_prs": branches, "unmergeable_pr_numbers": prs}
		}
	}
}

func TestAnUnmergeableBlockerNamesTheStreamsPullRequestOnItsBranch(t *testing.T) {
	f, obj := plainStream(t)
	f.listUnmergeablePRs(map[string]int{blockedBranch: 152, "karakuri/phase-99-y": 153})

	req := unblockRequest(t, f, obj, blk(reconcile.BlockerUnmergeablePR, blockedBranch, 3, t0))

	if req.Unblock == nil {
		t.Fatal("request.Unblock is nil, want the unmergeable_pr blocker")
	}
	if req.Unblock.Subject != blockedBranch {
		t.Fatalf("Unblock.Subject = %q, want %q", req.Unblock.Subject, blockedBranch)
	}
	if req.Unblock.PullRequest != 152 {
		t.Errorf("Unblock.PullRequest = %d, want 152: the open pull request whose head is %s, so the planner is told to post its diagnosis there",
			req.Unblock.PullRequest, blockedBranch)
	}
}

func TestABlockerWithNoOpenPullRequestOnItsSubjectNamesNone(t *testing.T) {
	cases := map[string]struct {
		listed  map[string]int
		blocker reconcile.Blocker
	}{
		"no pull request listed": {
			listed:  nil,
			blocker: blk(reconcile.BlockerUnmergeablePR, blockedBranch, 3, t0),
		},
		"a pull request on another branch only": {
			listed:  map[string]int{"karakuri/phase-99-y": 153},
			blocker: blk(reconcile.BlockerUnmergeablePR, blockedBranch, 3, t0),
		},
		"a blocker whose subject is not a branch": {
			listed:  map[string]int{blockedBranch: 152},
			blocker: blk(reconcile.BlockerVerifyFailed, "tests-pass", 3, t0),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f, obj := plainStream(t)
			f.listUnmergeablePRs(tc.listed)
			req := unblockRequest(t, f, obj, tc.blocker)
			if req.Unblock == nil {
				t.Fatal("request.Unblock is nil, want the blocker")
			}
			if req.Unblock.PullRequest != 0 {
				t.Errorf("Unblock.PullRequest = %d, want 0: no open pull request is on %q, so no comment is asked for",
					req.Unblock.PullRequest, tc.blocker.Subject)
			}
		})
	}
}
