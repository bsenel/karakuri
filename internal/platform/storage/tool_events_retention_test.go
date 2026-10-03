package storage_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/platform/db/schema"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Pruning the audit log is the one place Karakuri destroys its own record, so
// the boundary has to be exact: a row at the cutoff is still inside the
// retention window and must survive.
func TestDeleteToolEventsBefore(t *testing.T) {
	ctx := context.Background()
	s, db := newStoreWithDB(t)

	cutoff := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	events := []struct {
		id, kind string
		at       time.Time
	}{
		{"old-execute", storage.ToolEventExecute, cutoff.Add(-48 * time.Hour)},
		{"old-approval", storage.ToolEventApproval, cutoff.Add(-time.Hour)},
		{"old-rejection", storage.ToolEventRejection, cutoff.Add(-time.Second)},
		{"at-execute", storage.ToolEventExecute, cutoff},
		{"at-approval", storage.ToolEventApproval, cutoff},
		{"new-rejection", storage.ToolEventRejection, cutoff.Add(time.Second)},
		{"new-execute", storage.ToolEventExecute, cutoff.Add(48 * time.Hour)},
	}
	for _, e := range events {
		if err := s.SaveToolEvent(ctx, storage.ToolEvent{ID: e.id, ObjectiveID: "obj-1", Kind: e.kind, Success: true}); err != nil {
			t.Fatalf("save %s: %v", e.id, err)
		}
		// SaveToolEvent does not carry a timestamp; the column is stamped on
		// insert. Backdate it directly so the test owns the boundary.
		if err := db.Model(&schema.ToolEventModel{}).Where("id = ?", e.id).UpdateColumn("created_at", e.at).Error; err != nil {
			t.Fatalf("backdate %s: %v", e.id, err)
		}
	}

	n, err := s.DeleteToolEventsBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 3 {
		t.Errorf("deleted = %d, want 3 (rows strictly older than the cutoff)", n)
	}

	left, err := s.ListToolEvents(ctx, storage.ToolEventFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := make([]string, 0, len(left))
	for _, e := range left {
		got = append(got, e.ID)
	}
	slices.Sort(got)
	want := []string{"at-approval", "at-execute", "new-execute", "new-rejection"}
	if !slices.Equal(got, want) {
		t.Errorf("remaining = %v, want %v", got, want)
	}

	again, err := s.DeleteToolEventsBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("second delete: %v", err)
	}
	if again != 0 {
		t.Errorf("second delete = %d, want 0", again)
	}
}
