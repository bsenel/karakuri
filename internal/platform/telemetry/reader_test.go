package telemetry

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
	coretelemetry "github.com/bsenel/karakuri/internal/core/telemetry"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

func newReader(t *testing.T) (*Reader, storage.StorageAdapter) {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := storage.NewGORMStorage(db)
	return New(store, karakuriquota.Deps{}), store
}

// seedTenant gives a twin one objective and n escalation rows against it.
func seedTenant(t *testing.T, store storage.StorageAdapter, twinID, objID string, escalations int) {
	t.Helper()
	ctx := context.Background()
	if err := store.SaveObjective(ctx, objective.Objective{
		ID: objective.ObjectiveID(objID), Title: objID, Domain: "software",
		TwinID: twinID, Mode: objective.ModeStanding,
	}); err != nil {
		t.Fatalf("seed objective: %v", err)
	}
	for i := 0; i < escalations; i++ {
		if err := store.SaveToolEvent(ctx, storage.ToolEvent{
			ID:          objID + "-esc-" + string(rune('a'+i)),
			ObjectiveID: objID,
			Kind:        storage.ToolEventEscalation,
			CreatedAt:   time.Now().UTC(),
		}); err != nil {
			t.Fatalf("seed tool event: %v", err)
		}
	}
}

// A tenant-scoped snapshot must not count another tenant's activity.
//
// tool_events carries no twin, and the audit queries simply did not filter —
// so a twin with three escalations reported everybody's. A pack reasoning
// about its own trustworthiness from another tenant's approval rate is worse
// off than one with no number at all, and the numbers leak across a boundary
// the rest of the system enforces.
func TestSnapshotDoesNotCountOtherTenants(t *testing.T) {
	r, store := newReader(t)
	seedTenant(t, store, "twin-a", "obj-a", 3)
	seedTenant(t, store, "twin-b", "obj-b", 7)

	snap, err := r.Snapshot(context.Background(), coretelemetry.Query{TwinID: "twin-a"})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.Escalation.Escalations != 3 {
		t.Errorf("escalations = %d, want 3 — twin-b's 7 are not twin-a's",
			snap.Escalation.Escalations)
	}
	if snap.Objectives.Total != 1 {
		t.Errorf("objectives = %d, want 1", snap.Objectives.Total)
	}
}

// The deployment-wide query is the one case where everything is the right
// answer, so the scoping fix must not narrow it.
func TestUnscopedSnapshotSeesEverything(t *testing.T) {
	r, store := newReader(t)
	seedTenant(t, store, "twin-a", "obj-a", 3)
	seedTenant(t, store, "twin-b", "obj-b", 7)

	snap, err := r.Snapshot(context.Background(), coretelemetry.Query{})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.Escalation.Escalations != 10 {
		t.Errorf("escalations = %d, want 10 across both tenants", snap.Escalation.Escalations)
	}
}

// A twin that owns no objectives owns no events. The filter must read an
// empty scope as "nothing", never as "no filter" — that inversion is how a
// scoped listing turns into a full one.
func TestTwinWithNoObjectivesSeesNothing(t *testing.T) {
	r, store := newReader(t)
	seedTenant(t, store, "twin-a", "obj-a", 5)

	snap, err := r.Snapshot(context.Background(), coretelemetry.Query{TwinID: "twin-empty"})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.Escalation.Escalations != 0 {
		t.Errorf("escalations = %d for a twin with no objectives, want 0", snap.Escalation.Escalations)
	}
}

// seedDecision saves the audit rows the two production writers save for one
// checkpoint decision: the row the checkpoint service writes when a person
// decides (internal/feature/checkpoint/service.go, Record), and, when the
// decision ends the loop, the terminal row the loop writes after it
// (internal/feature/loop/runner.go, recordCheckpointTerminal). Each row is
// built field by field the way its writer builds it.
func seedDecision(t *testing.T, store storage.StorageAdapter, objID, checkpointID, choice, terminalReason string) {
	t.Helper()
	ctx := context.Background()
	kind, success := storage.ToolEventApproval, true
	switch choice {
	case "reject":
		kind, success = storage.ToolEventRejection, false
	case "modify":
		kind, success = storage.ToolEventModification, true
	}
	decision, _ := json.Marshal(map[string]any{
		"checkpoint_id": checkpointID, "choice": choice, "note": "", "linked_audit_event": "",
	})
	if err := store.SaveToolEvent(ctx, storage.ToolEvent{
		ID:          checkpointID + "-decision",
		ObjectiveID: objID,
		Kind:        kind,
		Approver:    "operator@example.test",
		PayloadJSON: string(decision),
		Success:     success,
	}); err != nil {
		t.Fatalf("seed checkpoint decision: %v", err)
	}
	if terminalReason == "" {
		return
	}
	terminal, _ := json.Marshal(map[string]any{
		"reason": terminalReason, "choice": choice, "note": "", "approver": "operator@example.test",
	})
	if err := store.SaveToolEvent(ctx, storage.ToolEvent{
		ID:               checkpointID + "-terminal",
		ObjectiveID:      objID,
		Kind:             storage.ToolEventRejection,
		EscalationReason: terminalReason,
		Approver:         "operator@example.test",
		PayloadJSON:      string(terminal),
		Success:          false,
	}); err != nil {
		t.Fatalf("seed loop terminal row: %v", err)
	}
}

// One human rejection is one rejection in the approval rate.
//
// A rejected checkpoint leaves two rows of kind rejection: the checkpoint
// service's record of the decision, and the loop's record that the decision
// ended it. Counting both made one approval and one rejection read as 1/3, so
// a criterion such as "approval rate >= 0.7" failed on a deployment that met
// it. A modify that ran past the loop's modify limit leaves a terminal row of
// kind rejection too, and nobody rejected anything there.
func TestEscalationApprovalRateCountsEachDecisionOnce(t *testing.T) {
	cases := []struct {
		name           string
		seed           func(t *testing.T, store storage.StorageAdapter)
		wantApprovals  int
		wantRejections int
		wantRate       float64
	}{
		{
			name: "one approval and one rejection",
			seed: func(t *testing.T, store storage.StorageAdapter) {
				seedDecision(t, store, "obj-a", "cp-approve", "approve", "")
				seedDecision(t, store, "obj-a", "cp-reject", "reject", "rejected_at_checkpoint")
			},
			wantApprovals: 1, wantRejections: 1, wantRate: 0.5,
		},
		{
			name: "a modify that exceeded the modify loop is not a rejection",
			seed: func(t *testing.T, store storage.StorageAdapter) {
				seedDecision(t, store, "obj-a", "cp-approve", "approve", "")
				seedDecision(t, store, "obj-a", "cp-modify", "modify", "modify_loop_exceeded")
			},
			wantApprovals: 1, wantRejections: 0, wantRate: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, store := newReader(t)
			seedTenant(t, store, "twin-a", "obj-a", 0)
			tc.seed(t, store)

			snap, err := r.Snapshot(context.Background(), coretelemetry.Query{})
			if err != nil {
				t.Fatalf("snapshot: %v", err)
			}
			if snap.Escalation.Approvals != tc.wantApprovals {
				t.Errorf("approvals = %d, want %d", snap.Escalation.Approvals, tc.wantApprovals)
			}
			if snap.Escalation.Rejections != tc.wantRejections {
				t.Errorf("rejections = %d, want %d", snap.Escalation.Rejections, tc.wantRejections)
			}
			if got := snap.Escalation.ApprovalRate(); got != tc.wantRate {
				t.Errorf("ApprovalRate() = %v, want %v", got, tc.wantRate)
			}
		})
	}
}
