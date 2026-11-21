package software

import (
	"context"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// observeUnansweredReviewComments writes unanswered_review_comments and its
// planner lines into state (Phase 40 step 12). Not implemented yet.
func observeUnansweredReviewComments(ctx context.Context, adapter versioncontrol.VersionControlAdapter, repo string, authors config.OwnAuthorsConfig, state map[string]any) {
}
