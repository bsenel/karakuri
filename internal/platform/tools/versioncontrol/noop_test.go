package versioncontrol

import (
	"context"
	"errors"
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
