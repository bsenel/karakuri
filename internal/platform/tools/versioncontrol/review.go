package versioncontrol

import (
	"context"
	"fmt"
)

func (n *NoOp) PostReview(_ context.Context, _, _, _ string, _ []ReviewComment) error {
	return fmt.Errorf("noop: post review: %w", ErrUnsupported)
}
