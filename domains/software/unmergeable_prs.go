package software

// A stream's open pull request that cannot be merged as it stands (Phase 40
// step 6a): it conflicts with main, a check failed at its head, or no check
// was reported at its head noChecksAfter after its last update. Repairing it
// comes first in a pass, before the stream's branches without a pull request.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// noChecksAfter is how long after a pull request's last update its head may
// have no check before the pull request counts as broken. The conflict on the
// Phase 34 pull request showed itself this way: no checks at all.
const noChecksAfter = 15 * time.Minute

// observeUnmergeablePRs writes unmergeable_prs and its planner lines into
// state, for the open pull requests on branches a stream owns. A forge that
// cannot list the pull requests leaves the list out and says why (AGENTS.md
// rule 10): an empty list would tell the planner nothing is broken.
func observeUnmergeablePRs(ctx context.Context, adapter versioncontrol.VersionControlAdapter, repo string, state map[string]any) {
	// A zero since, as in step 6: a conflicting pull request nobody touched
	// lately is exactly the one a window would drop.
	prs, err := adapter.ListPRs(ctx, repo, time.Time{})
	if err != nil {
		state["unmergeable_prs_error"] = err.Error()
		state["unmergeable_prs_lines"] = []string{
			"The stream pull requests that cannot be merged could not be read: " + err.Error() +
				". Do not take this as there being none.",
		}
		return
	}

	listed := []map[string]any{}
	lines := []string{}
	for _, pr := range prs {
		stream := branchStream(pr.HeadBranch)
		if stream == "" {
			continue
		}
		name := fmt.Sprintf("Pull request #%s on branch %s of stream %s", pr.ID, pr.HeadBranch, stream)
		mergeState, mergeErr := adapter.MergeState(ctx, repo, pr.ID)
		reason := ""
		if mergeErr == nil && mergeState == "conflicting" {
			reason = "conflicts with main"
		} else {
			var checkErr error
			reason, checkErr = checkReason(ctx, adapter, repo, pr)
			if checkErr != nil {
				lines = append(lines, fmt.Sprintf("%s: its checks could not be read: %v.", name, checkErr))
			}
		}
		if reason != "" {
			listed = append(listed, map[string]any{
				"pr": pr.ID, "number": pr.Number, "branch": pr.HeadBranch, "stream": stream, "reason": reason,
			})
			lines = append(lines, fmt.Sprintf("%s %s: repair it before any other work.", name, reason))
			continue
		}
		// Neither listed nor called mergeable: the forge did not say.
		if mergeErr != nil {
			lines = append(lines, fmt.Sprintf("%s: whether it can be merged is not known (%v).", name, mergeErr))
		} else if mergeState != "clean" {
			lines = append(lines, fmt.Sprintf("%s: whether it can be merged is not known yet; the forge is still computing it.", name))
		}
	}
	state["unmergeable_prs"] = listed
	state["unmergeable_prs_lines"] = lines
}

// checkReason is why the checks at pr's head keep it from being merged: a
// failed check, by name, or no check at all noChecksAfter after its last
// update. "" when the checks do not stand in the way.
func checkReason(ctx context.Context, adapter versioncontrol.VersionControlAdapter, repo string, pr versioncontrol.PRSummary) (string, error) {
	checks, err := adapter.ListChecks(ctx, repo, pr.HeadSHA)
	if err != nil {
		return "", err
	}
	runs := latestRuns(atHead(checks, pr.HeadSHA))
	if failing, _, _, _, _ := sortChecks(runs); len(failing) > 0 {
		return "check failed: " + strings.Join(failing, ", "), nil
	}
	if len(runs) == 0 && time.Since(pr.UpdatedAt) > noChecksAfter {
		return "no check reported", nil
	}
	return "", nil
}

// unmergeablePRsConstraint tells every stream's planner to repair its
// unmergeable pull requests first, before branchesWithoutPRConstraint. The
// repair it names is a coding action on the stream's own branch, which
// branchConstraint already holds to that branch with no merge tool and no
// unrestricted shell, so it never asks for a force push, a rebase or main.
var unmergeablePRsConstraint = objective.Constraint{
	ID: "unmergeable-prs",
	Description: "The git observation's unmergeable_prs lists open pull requests that cannot be merged as they stand, each with its branch, its stream and the reason. " +
		"The plan repairs each one of this stream first, before any other work, including the branches in branches_without_pr: " +
		"for a pull request that conflicts with main, one software.act.write_code action with params.branch set to the listed branch whose task is to merge main into the branch keeping both sides, " +
		"never rebase and never force-push, rerun the tests of what the merge touched, and push the merge commit to that branch; the pull request itself is left for a human to merge. " +
		"A conflict that cannot keep both sides, a failed check, or checks still missing after a clean merge is reported as a blocker, not a guess. " +
		"When unmergeable_prs_error is set instead, the pass reports that it could not see the pull requests and does not treat the list as empty.",
	Hard: true,
}
