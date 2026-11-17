package software

// A stream's open pull request that cannot be merged as it stands (Phase 40
// step 6a): it conflicts with main, a check failed at its head, or no check
// was reported at its head noChecksAfter after its last update. Repairing it
// comes first in a pass, before the stream's branches without a pull request.

import (
	"context"
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// noChecksAfter is how long after a pull request's last update its head may
// have no check before the pull request counts as broken. The conflict on the
// Phase 34 pull request showed itself this way: no checks at all.
const noChecksAfter = 15 * time.Minute

// observeUnmergeablePRs writes unmergeable_prs and its planner lines into
// state. Not implemented yet: the tests in unmergeable_prs_test.go specify it.
func observeUnmergeablePRs(ctx context.Context, adapter versioncontrol.VersionControlAdapter, repo string, state map[string]any) {
}

// unmergeablePRsConstraint tells every stream's planner to repair its
// unmergeable pull requests first. Not written yet: the tests in
// unmergeable_prs_test.go specify it.
var unmergeablePRsConstraint = objective.Constraint{}
