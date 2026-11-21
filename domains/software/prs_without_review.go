package software

// A stream's open pull request whose head has no review (Phase 40 step 11).
//
// The review objective builds and runs a branch; without a list of what is
// unreviewed it has nothing to wake on. The git observation lists each open
// pull request of a stream whose head commit no review was left at, so a push
// after a review makes the pull request unreviewed again.

import (
	"context"
	"fmt"
	"slices"

	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// observePRsWithoutReview writes prs_without_review and its planner lines into
// state from the open pull requests the caller already listed. A pull request
// whose branch no stream owns is skipped: it is not the reviewer's to review.
// A forge that cannot read the reviews leaves the list out and says why
// (AGENTS.md rule 10): an empty list would tell the planner nothing is waiting.
func observePRsWithoutReview(ctx context.Context, adapter versioncontrol.VersionControlAdapter, repo string, prs []versioncontrol.PRSummary, state map[string]any) {
	listed := []map[string]any{}
	lines := []string{}
	for _, pr := range prs {
		stream := branchStream(pr.HeadBranch)
		if stream == "" {
			continue
		}
		reviewed, err := adapter.ReviewedCommits(ctx, repo, pr.ID)
		if err != nil {
			state["prs_without_review_error"] = err.Error()
			state["prs_without_review_lines"] = []string{
				"The stream pull requests with no review could not be read: " + err.Error() +
					". Do not take this as an empty list.",
			}
			return
		}
		// Reviewed at the head, not merely reviewed: a review of an older commit
		// does not cover what was pushed since.
		if slices.Contains(reviewed, pr.HeadSHA) {
			continue
		}
		listed = append(listed, map[string]any{
			"pr": pr.ID, "title": pr.Title, "url": pr.URL,
			"branch": pr.HeadBranch, "head_sha": pr.HeadSHA, "stream": stream,
		})
		lines = append(lines, fmt.Sprintf(
			"Pull request %s of stream %s has no review at its head %s: review it by building and running its branch.",
			pr.ID, stream, shortSHA(pr.HeadSHA)))
	}
	state["prs_without_review"] = listed
	state["prs_without_review_lines"] = lines
}
