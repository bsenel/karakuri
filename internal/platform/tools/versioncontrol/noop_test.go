package versioncontrol

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// With no forge configured nothing was marked ready, and saying so is the only
// honest answer (AGENTS.md rule 10): a nil here would let the stream believe a
// draft is up for review.
func TestNoOpMarkReadyIsUnsupported(t *testing.T) {
	err := NewNoOp().MarkReady(context.Background(), "acme/api", "244")
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("MarkReady = %v, want an error wrapping ErrUnsupported", err)
	}
}

// With no forge there are no commits to compare, and an empty list would read
// as "nothing delivered on this branch" (AGENTS.md rule 10).
func TestNoOpCompareCommitsIsUnsupported(t *testing.T) {
	commits, err := NewNoOp().CompareCommits(context.Background(), "acme/api", "main", "karakuri/x")
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("CompareCommits = %v, %v; want an error wrapping ErrUnsupported", commits, err)
	}
	if !strings.HasPrefix(err.Error(), "noop") {
		t.Errorf("CompareCommits error %q does not name the adapter", err)
	}
}
