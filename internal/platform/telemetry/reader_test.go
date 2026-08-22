package telemetry

import (
	"context"
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
