package loop

import (
	"context"

	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

func (v *recordingVC) PostReview(context.Context, string, string, string, []versioncontrol.ReviewComment) error {
	return versioncontrol.ErrUnsupported
}

func (v *recordingVC) ReviewedCommits(context.Context, string, string) ([]string, error) {
	return nil, versioncontrol.ErrUnsupported
}
