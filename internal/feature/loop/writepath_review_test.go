package loop

import (
	"context"

	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

func (v *recordingVC) PostReview(context.Context, string, string, string, []versioncontrol.ReviewComment) error {
	return versioncontrol.ErrUnsupported
}

func (v *recordingVC) ListReviews(context.Context, string, string) ([]versioncontrol.Review, error) {
	return nil, versioncontrol.ErrUnsupported
}

func (v *recordingVC) ReviewedCommits(context.Context, string, string) ([]string, error) {
	return nil, versioncontrol.ErrUnsupported
}
