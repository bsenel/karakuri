package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// A checkpoint's expiry is what the sweep compares against the clock: if the
// instant does not come back as it was saved, a checkpoint lapses at the wrong
// time or never.

func pendingByID(t *testing.T, s *storage.GORMStorage) map[string]checkpoint.Checkpoint {
	t.Helper()
	cps, err := s.ListPendingCheckpoints(context.Background(), "")
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	out := make(map[string]checkpoint.Checkpoint, len(cps))
	for _, c := range cps {
		out[c.ID] = c
	}
	return out
}

func TestCheckpointExpiresAt_RoundTrips(t *testing.T) {
	s := newStore(t)
	want := time.Date(2026, 10, 10, 12, 30, 0, 0, time.UTC)
	err := s.SaveCheckpoint(context.Background(), checkpoint.Checkpoint{
		ID: "exp", ObjectiveID: "obj-exp", TwinID: "twin-a",
		Summary: "summary exp", Options: []string{"approve", "reject", "modify"},
		Status:    checkpoint.StatusPending,
		ExpiresAt: &want,
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	if got := getCheckpoint(t, s, "exp").ExpiresAt; got == nil || !got.Equal(want) {
		t.Errorf("GetCheckpoint ExpiresAt = %v, want %v", got, want)
	}
	listed, ok := pendingByID(t, s)["exp"]
	if !ok {
		t.Fatalf("checkpoint missing from the pending list")
	}
	if got := listed.ExpiresAt; got == nil || !got.Equal(want) {
		t.Errorf("ListPendingCheckpoints ExpiresAt = %v, want %v", got, want)
	}
}

func TestCheckpointExpiresAt_NilWhenUnset(t *testing.T) {
	s := newStore(t)
	saveCheckpointWithWorldState(t, s, "plain", nil)

	if got := getCheckpoint(t, s, "plain").ExpiresAt; got != nil {
		t.Errorf("GetCheckpoint ExpiresAt = %v, want nil", got)
	}
	listed, ok := pendingByID(t, s)["plain"]
	if !ok {
		t.Fatalf("checkpoint missing from the pending list")
	}
	if listed.ExpiresAt != nil {
		t.Errorf("ListPendingCheckpoints ExpiresAt = %v, want nil", listed.ExpiresAt)
	}
}
