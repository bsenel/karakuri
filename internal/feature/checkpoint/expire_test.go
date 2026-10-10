package checkpoint

// Phase 35 step 1: a checkpoint nobody answered ends as a rejection.
//
// What storage does today, read before writing these:
//   - A checkpoint is stored as columns (schema.CheckpointModel), with JSON
//     only for options, actions, decision and world state. SaveCheckpoint and
//     checkpointFromModel map field by field, so ExpiresAt needs its own
//     column and a line in each; nothing carries it for free.
//   - Migration is gorm AutoMigrate over the models (platform/db/migrate.go),
//     which adds a new nullable column to an existing table. No SQL file.
//   - ListPendingCheckpoints(ctx, "") returns every twin's pending
//     checkpoints, oldest first: the twin filter is applied only when the id
//     is non-empty. The sweep needs no new storage method.
//   - Record writes one tool event per decision: Kind "rejection" and
//     Success false for a reject, Approver from the decision, ObjectiveID from
//     the checkpoint, and a payload of checkpoint_id, choice, note and
//     linked_audit_event.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

const timeoutApprover = "system:timeout"

// ListPendingCheckpoints mirrors the GORM adapter: an empty twin id lists
// every twin's pending checkpoints.
func (f *fakeStore) ListPendingCheckpoints(_ context.Context, twinID string) ([]corecheckpoint.Checkpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []corecheckpoint.Checkpoint
	for _, c := range f.checkpoints {
		if c.Status != corecheckpoint.StatusPending {
			continue
		}
		if twinID != "" && c.TwinID != twinID {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

var expireNow = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

// seedPending writes a pending checkpoint straight into the store, so these
// tests fail on ExpireDue and not on Create.
func seedPending(t *testing.T, store *fakeStore, id, twinID string, expiresAt *time.Time) {
	t.Helper()
	err := store.SaveCheckpoint(context.Background(), corecheckpoint.Checkpoint{
		ID: id, ObjectiveID: objective.ObjectiveID("obj-" + id), TwinID: twinID,
		Summary:      "summary " + id,
		Options:      []string{"approve", "reject", "modify"},
		AuditEventID: "audit-" + id,
		Status:       corecheckpoint.StatusPending,
		CreatedAt:    expireNow.Add(-2 * time.Hour),
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func at(d time.Duration) *time.Time {
	ts := expireNow.Add(d)
	return &ts
}

func TestServiceCreate_CarriesExpiresAt(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		store := newFakeStore()
		svc := NewService(store, event.NewHub())
		want := expireNow.Add(30 * time.Minute)

		cp, err := svc.Create(context.Background(), "obj", "twin", "r", "s",
			[]string{"approve", "reject", "modify"}, CreateOptions{ExpiresAt: &want})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if cp.ExpiresAt == nil || !cp.ExpiresAt.Equal(want) {
			t.Errorf("returned ExpiresAt = %v, want %v", cp.ExpiresAt, want)
		}
		if stored := store.checkpoints[cp.ID]; stored.ExpiresAt == nil || !stored.ExpiresAt.Equal(want) {
			t.Errorf("stored ExpiresAt = %v, want %v", stored.ExpiresAt, want)
		}
	})

	t.Run("unset", func(t *testing.T) {
		store := newFakeStore()
		svc := NewService(store, event.NewHub())

		cp, err := svc.Create(context.Background(), "obj", "twin", "r", "s",
			[]string{"approve", "reject", "modify"}, CreateOptions{})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if cp.ExpiresAt != nil {
			t.Errorf("returned ExpiresAt = %v, want nil", cp.ExpiresAt)
		}
		if stored := store.checkpoints[cp.ID]; stored.ExpiresAt != nil {
			t.Errorf("stored ExpiresAt = %v, want nil", stored.ExpiresAt)
		}
	})
}

func TestExpireDue_RejectsPastDueAsSystemTimeout(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, event.NewHub())
	seedPending(t, store, "due", "twin-a", at(-time.Minute))

	n, err := svc.ExpireDue(context.Background(), expireNow)
	if err != nil {
		t.Fatalf("ExpireDue failed: %v", err)
	}
	if n != 1 {
		t.Errorf("ExpireDue = %d, want 1", n)
	}

	cp := store.checkpoints["due"]
	if cp.Status != corecheckpoint.StatusResolved {
		t.Errorf("Status = %q, want resolved", cp.Status)
	}
	if cp.Decision == nil {
		t.Fatalf("no decision recorded on the expired checkpoint")
	}
	if cp.Decision.Choice != "reject" {
		t.Errorf("Choice = %q, want reject", cp.Decision.Choice)
	}
	if cp.Decision.Approver != timeoutApprover {
		t.Errorf("Approver = %q, want %q", cp.Decision.Approver, timeoutApprover)
	}
	if cp.Decision.Note == "" {
		t.Errorf("Note is empty; an expiry must say why it rejected")
	}
	if cp.Decision.Modifications != nil {
		t.Errorf("Modifications = %+v, want nil", cp.Decision.Modifications)
	}

	// The row Record writes for a reject, and only that one.
	if len(store.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(store.events))
	}
	ev := store.events[0]
	if ev.Kind != storage.ToolEventRejection {
		t.Errorf("Kind = %q, want %q", ev.Kind, storage.ToolEventRejection)
	}
	if ev.Success {
		t.Errorf("Success = true, want false for a rejection")
	}
	if ev.Approver != timeoutApprover {
		t.Errorf("audit Approver = %q, want %q", ev.Approver, timeoutApprover)
	}
	if ev.ObjectiveID != "obj-due" {
		t.Errorf("audit ObjectiveID = %q, want obj-due", ev.ObjectiveID)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(ev.PayloadJSON), &payload); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if payload["checkpoint_id"] != "due" {
		t.Errorf("payload checkpoint_id = %v, want due", payload["checkpoint_id"])
	}
	if payload["choice"] != "reject" {
		t.Errorf("payload choice = %v, want reject", payload["choice"])
	}
	if note, _ := payload["note"].(string); note == "" || note != cp.Decision.Note {
		t.Errorf("payload note = %q, want the decision's note %q", note, cp.Decision.Note)
	}
	if payload["linked_audit_event"] != "audit-due" {
		t.Errorf("payload linked_audit_event = %v, want audit-due", payload["linked_audit_event"])
	}
}

func TestExpireDue_LeavesOthersUntouched(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, event.NewHub())
	seedPending(t, store, "not-yet", "twin-a", at(time.Minute))
	seedPending(t, store, "no-expiry", "twin-a", nil)
	seedPending(t, store, "answered", "twin-a", at(-time.Minute))
	person := corecheckpoint.Decision{Choice: "approve", Note: "looks right", Approver: "bsenel"}
	if err := svc.Resolve(context.Background(), "answered", person); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	// One checkpoint that is due, so a sweep that does nothing cannot pass.
	seedPending(t, store, "due", "twin-a", at(-time.Minute))

	n, err := svc.ExpireDue(context.Background(), expireNow)
	if err != nil {
		t.Fatalf("ExpireDue failed: %v", err)
	}
	if n != 1 {
		t.Errorf("ExpireDue = %d, want 1 (only the due one)", n)
	}

	for _, id := range []string{"not-yet", "no-expiry"} {
		cp := store.checkpoints[id]
		if cp.Status != corecheckpoint.StatusPending || cp.Decision != nil {
			t.Errorf("%s: status %q decision %+v, want still pending with none", id, cp.Status, cp.Decision)
		}
	}
	answered := store.checkpoints["answered"]
	if answered.Decision == nil || *answered.Decision != person {
		t.Errorf("answered: decision = %+v, want the person's %+v", answered.Decision, person)
	}
	// The person's approval and the one expiry; nothing for the three left alone.
	if len(store.events) != 2 {
		t.Fatalf("expected 2 audit events, got %d", len(store.events))
	}
	if ev := store.events[0]; ev.Kind != storage.ToolEventApproval || ev.Approver != "bsenel" {
		t.Errorf("first audit event = kind %q approver %q, want the person's approval", ev.Kind, ev.Approver)
	}
	if ev := store.events[1]; ev.Kind != storage.ToolEventRejection || ev.Approver != timeoutApprover {
		t.Errorf("second audit event = kind %q approver %q, want a rejection by %s", ev.Kind, ev.Approver, timeoutApprover)
	}
}

func TestExpireDue_NeverApproves(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, event.NewHub())
	seedPending(t, store, "due-1", "twin-a", at(-time.Hour))
	seedPending(t, store, "due-2", "twin-a", at(-time.Second))
	seedPending(t, store, "not-yet", "twin-a", at(time.Hour))
	seedPending(t, store, "no-expiry", "twin-a", nil)
	for id, choice := range map[string]string{"p-approve": "approve", "p-reject": "reject", "p-modify": "modify"} {
		seedPending(t, store, id, "twin-a", at(-time.Minute))
		if err := svc.Resolve(context.Background(), id, corecheckpoint.Decision{Choice: choice, Approver: "bsenel"}); err != nil {
			t.Fatalf("Resolve %s failed: %v", id, err)
		}
	}
	personChoice := map[string]string{"p-approve": "approve", "p-reject": "reject", "p-modify": "modify"}

	n, err := svc.ExpireDue(context.Background(), expireNow)
	if err != nil {
		t.Fatalf("ExpireDue failed: %v", err)
	}
	if n != 2 {
		t.Errorf("ExpireDue = %d, want 2", n)
	}

	timeouts := 0
	for id, cp := range store.checkpoints {
		if cp.Decision == nil {
			continue
		}
		switch cp.Decision.Approver {
		case timeoutApprover:
			timeouts++
			if cp.Decision.Choice != "reject" {
				t.Errorf("%s: %s decided %q; an expiry may only reject", id, timeoutApprover, cp.Decision.Choice)
			}
		case "bsenel":
			if cp.Decision.Choice != personChoice[id] {
				t.Errorf("%s: the person's choice became %q, want %q", id, cp.Decision.Choice, personChoice[id])
			}
		default:
			t.Errorf("%s: decision by unexpected approver %q", id, cp.Decision.Approver)
		}
	}
	if timeouts != 2 {
		t.Errorf("%d checkpoints decided by %s, want 2", timeouts, timeoutApprover)
	}
	for _, ev := range store.events {
		if ev.Approver == timeoutApprover && ev.Kind != storage.ToolEventRejection {
			t.Errorf("audit row of kind %q attributed to %s; want only rejections", ev.Kind, timeoutApprover)
		}
	}
}

func TestExpireDue_SweepsEveryTwin(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, event.NewHub())
	seedPending(t, store, "a-due", "twin-a", at(-time.Minute))
	seedPending(t, store, "b-due", "twin-b", at(-time.Minute))
	seedPending(t, store, "b-not-yet", "twin-b", at(time.Minute))

	n, err := svc.ExpireDue(context.Background(), expireNow)
	if err != nil {
		t.Fatalf("ExpireDue failed: %v", err)
	}
	if n != 2 {
		t.Errorf("ExpireDue = %d, want 2", n)
	}
	for _, id := range []string{"a-due", "b-due"} {
		cp := store.checkpoints[id]
		if cp.Decision == nil || cp.Decision.Choice != "reject" || cp.Decision.Approver != timeoutApprover {
			t.Errorf("%s: decision = %+v, want a rejection by %s", id, cp.Decision, timeoutApprover)
		}
	}
	if cp := store.checkpoints["b-not-yet"]; cp.Status != corecheckpoint.StatusPending {
		t.Errorf("b-not-yet: status %q, want pending", cp.Status)
	}
}
