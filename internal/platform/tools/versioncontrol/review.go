package versioncontrol

import (
	"context"
	"fmt"
)

func (n *NoOp) PostReview(_ context.Context, _, _, _ string, _ []ReviewComment) error {
	return fmt.Errorf("noop: post review: %w", ErrUnsupported)
}

func (n *NoOp) ReviewedCommits(_ context.Context, _, _ string) ([]string, error) {
	return nil, fmt.Errorf("noop: reviewed commits: %w", ErrUnsupported)
}
