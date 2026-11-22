package software

import (
	"context"
	"fmt"
	"strings"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// closedProposalWindow is how many recently closed pull requests are read to
// find the newest discovery proposal.
const closedProposalWindow = 30

// newClosingComment reads the last comment Karakuri did not post on the newest
// closed, unmerged discovery proposal; "" when there is none. A comment from an
// account only Karakuri uses is skipped; one from a shared account is skipped
// when the record says Karakuri posted it (the policy of Phase 40 step 12a),
// and counts when no record is available. Any read error is an error, never "".
func newClosingComment(vc versioncontrol.VersionControlAdapter, ownAuthors config.OwnAuthorsConfig, posted postedCommentIDs) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		closed, err := vc.ListClosedPRs(ctx, "", closedProposalWindow)
		if err != nil {
			return "", fmt.Errorf("closed pull requests: %w", err)
		}
		for _, pr := range closed {
			if !strings.HasPrefix(pr.HeadBranch, discoveryBranchPrefix) {
				continue
			}
			comments, err := vc.ListPullRequestComments(ctx, "", pr.Number)
			if err != nil {
				return "", fmt.Errorf("comments on pull request %d: %w", pr.Number, err)
			}
			var record map[int64]bool
			recordRead := false
			last := ""
			for _, c := range comments {
				if loginIn(ownAuthors.Karakuri, c.Author) {
					if !loginIn(ownAuthors.Owners, c.Author) {
						continue
					}
					if posted != nil {
						if !recordRead {
							m, err := posted(ctx)
							if err != nil {
								return "", fmt.Errorf("the record of what Karakuri posted could not be read: %w", err)
							}
							record, recordRead = m, true
						}
						if record[c.ID] {
							continue
						}
					}
				}
				last = c.Body
			}
			return last, nil
		}
		return "", nil
	}
}
