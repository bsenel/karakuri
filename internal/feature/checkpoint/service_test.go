package checkpoint

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// fakeStore is a tiny in-process StorageAdapter stub that records
// SaveToolEvent calls so the Resolve audit-kind matrix can be asserted
// without spinning up a real database.
type fakeStore struct {
	storage.StorageAdapter // embed for unused methods — calling them panics
	mu                     sync.Mutex
	checkpoints            map[string]corecheckpoint.Checkpoint
	events                 []storage.ToolEvent
}

func newFakeStore() *fakeStore {
	return &fakeStore{checkpoints: make(map[string]corecheckpoint.Checkpoint)}
}

func (f *fakeStore) SaveCheckpoint(_ context.Context, c corecheckpoint.Checkpoint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checkpoints[c.ID] = c
	return nil
}

func (f *fakeStore) GetCheckpoint(_ context.Context, id string) (corecheckpoint.Checkpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checkpoints[id], nil
}

func (f *fakeStore) ResolveCheckpoint(_ context.Context, id string, d corecheckpoint.Decision) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := f.checkpoints[id]
	cp.Status = corecheckpoint.StatusResolved
	cp.Decision = &d
	f.checkpoints[id] = cp
	return nil
}

func (f *fakeStore) SaveToolEvent(_ context.Context, e storage.ToolEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
	return nil
}

func TestServiceCreate_PersistsPlannerDraft(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, event.NewHub())

	cp, err := svc.Create(context.Background(), objective.ObjectiveID("obj-1"), "twin-1",
		"confidence 0.84 below threshold 0.90",
		"Loop xyz requires human decision",
		[]string{"approve", "reject", "modify"},
		CreateOptions{
			Capability: capability.CapabilityID("code.write"),
			Confidence: 0.84,
			Actions: []corecheckpoint.Action{
				{CapabilityID: "code.write", Reason: "scaffold auth module"},
				{CapabilityID: "test.run"},
			},
			AuditEventID: "audit-123",
		},
	)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if cp.Capability != "code.write" {
		t.Errorf("expected Capability=code.write on returned checkpoint, got %q", cp.Capability)
	}
	if cp.Confidence != 0.84 {
		t.Errorf("expected Confidence=0.84, got %v", cp.Confidence)
	}
	if len(cp.Actions) != 2 || cp.Actions[0].CapabilityID != "code.write" {
		t.Errorf("expected 2 actions starting with code.write, got %+v", cp.Actions)
	}
	if cp.AuditEventID != "audit-123" {
		t.Errorf("expected AuditEventID=audit-123, got %q", cp.AuditEventID)
	}

	stored, _ := store.GetCheckpoint(context.Background(), cp.ID)
	if stored.Confidence != 0.84 || len(stored.Actions) != 2 || stored.AuditEventID != "audit-123" {
		t.Errorf("planner draft lost on round-trip: %+v", stored)
	}
}

func TestServiceResolve_AuditKindMatrix(t *testing.T) {
	cases := []struct {
		choice   string
		wantKind string
		wantSucc bool
	}{
		{"approve", storage.ToolEventApproval, true},
		{"reject", storage.ToolEventRejection, false},
		{"modify", storage.ToolEventModification, true},
	}
	for _, c := range cases {
		t.Run("choice="+c.choice, func(t *testing.T) {
			store := newFakeStore()
			svc := NewService(store, event.NewHub())
			cp, _ := svc.Create(context.Background(), "obj", "twin", "r", "s",
				[]string{"approve", "reject", "modify"}, CreateOptions{AuditEventID: "audit-X"})

			if err := svc.Resolve(context.Background(), cp.ID, corecheckpoint.Decision{
				Choice:   c.choice,
				Note:     "rationale",
				Approver: "bsenel",
			}); err != nil {
				t.Fatalf("Resolve failed: %v", err)
			}

			if len(store.events) != 1 {
				t.Fatalf("expected 1 audit event, got %d", len(store.events))
			}
			ev := store.events[0]
			if ev.Kind != c.wantKind {
				t.Errorf("expected Kind=%q, got %q", c.wantKind, ev.Kind)
			}
			if ev.Success != c.wantSucc {
				t.Errorf("expected Success=%v, got %v", c.wantSucc, ev.Success)
			}
			if ev.Approver != "bsenel" {
				t.Errorf("expected Approver=bsenel, got %q", ev.Approver)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(ev.PayloadJSON), &payload); err != nil {
				t.Fatalf("payload not JSON: %v", err)
			}
			if payload["linked_audit_event"] != "audit-X" {
				t.Errorf("expected linked_audit_event=audit-X in payload, got %v", payload["linked_audit_event"])
			}
		})
	}
}

func TestServiceResolve_ModifyPayloadCarriesDiff(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, event.NewHub())
	cp, _ := svc.Create(context.Background(), "obj", "twin", "r", "s",
		[]string{"approve", "reject", "modify"}, CreateOptions{})

	floor := 0.75
	if err := svc.Resolve(context.Background(), cp.ID, corecheckpoint.Decision{
		Choice:   "modify",
		Approver: "bsenel",
		Modifications: &corecheckpoint.Modifications{
			RemovedActions:    []string{"code.write"},
			AddedConstraints:  []string{"scaffold only"},
			RevisedConfidence: &floor,
		},
	}); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	var payload map[string]any
	_ = json.Unmarshal([]byte(store.events[0].PayloadJSON), &payload)
	mods, ok := payload["modifications"].(map[string]any)
	if !ok {
		t.Fatalf("expected modifications block in payload, got %v", payload)
	}
	if removed, _ := mods["removed_actions"].([]any); len(removed) != 1 || removed[0] != "code.write" {
		t.Errorf("expected removed_actions=[code.write], got %v", mods["removed_actions"])
	}
	if added, _ := mods["added_constraints"].([]any); len(added) != 1 || added[0] != "scaffold only" {
		t.Errorf("expected added_constraints=[scaffold only], got %v", mods["added_constraints"])
	}
	if got, _ := mods["revised_confidence"].(float64); got != 0.75 {
		t.Errorf("expected revised_confidence=0.75, got %v", mods["revised_confidence"])
	}
}
