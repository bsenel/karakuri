package software

import "testing"

// The loop puts the worktree's own branch in params.branch and keeps the
// plan's branch as planned_branch. A push to the planned branch is the action
// staying inside its declared refs, not a bounds violation; before, every such
// push failed the action.
func TestPushToThePlannedBranchIsInsideTheDeclaredRefs(t *testing.T) {
	const planned = "karakuri/phase-40-step-03-planned-group"
	f := newPushedRefsFixture(t)
	action := salvageAction(f.worktree)
	action.Params["planned_branch"] = planned
	action.Params["branch"] = "karakuri/objective/task-worktree-branch"

	res := actWith(t, f.worktree, action, func(t *testing.T, wt string) error {
		commitIn(t, wt, "planned.txt")
		gitIn(t, wt, "push", "origin", "HEAD:refs/heads/"+planned)
		return nil
	})
	if !res.Success || res.BoundsViolation != nil {
		t.Fatalf("a push to the planned branch failed: %q (violation %+v)", res.Error, res.BoundsViolation)
	}
}

// plannedBranch prefers the loop's planned_branch and falls back to branch,
// which is what an action run outside the loop names.
func TestPlannedBranchPrefersThePlansBranch(t *testing.T) {
	cases := []struct {
		params map[string]any
		want   string
	}{
		{map[string]any{"planned_branch": "karakuri/phase-1-a", "branch": "karakuri/obj/task"}, "karakuri/phase-1-a"},
		{map[string]any{"branch": "karakuri/phase-1-a"}, "karakuri/phase-1-a"},
		{map[string]any{}, ""},
	}
	for _, tc := range cases {
		if got := plannedBranch(tc.params); got != tc.want {
			t.Errorf("plannedBranch(%v) = %q, want %q", tc.params, got, tc.want)
		}
	}
}
