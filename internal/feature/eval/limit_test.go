package eval

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/objective"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// A calibration spends one model call per checkpoint, so the limit is a spend
// bound. This runs against the real store because the bound is only as good as
// the listing that applies it: a fake store would be asserting its own cap.
func TestCalibrate_LimitBoundsJudgeCalls(t *testing.T) {
	ctx := context.Background()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "eval.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := storage.NewGORMStorage(db)

	judge := &fakeJudge{replies: map[string]string{}, errs: map[string]error{}}
	for _, id := range []string{"c1", "c2", "c3", "c4", "c5"} {
		objID := objective.ObjectiveID("obj-" + id)
		title := "Objective title <" + id + ">"
		if err := store.SaveObjective(ctx, objective.Objective{ID: objID, Title: title, TwinID: "twin-a"}); err != nil {
			t.Fatalf("save objective %s: %v", id, err)
		}
		err := store.SaveCheckpoint(ctx, checkpoint.Checkpoint{
			ID: id, ObjectiveID: objID, TwinID: "twin-a",
			Actions: []checkpoint.Action{{CapabilityID: "vcs.open_pr", Reason: "draft for " + id}},
			Status:  checkpoint.StatusPending,
		})
		if err != nil {
			t.Fatalf("save checkpoint %s: %v", id, err)
		}
		// A few milliseconds apart so resolved_at orders them.
		time.Sleep(5 * time.Millisecond)
		if err := store.ResolveCheckpoint(ctx, id, checkpoint.Decision{Choice: choiceApprove}); err != nil {
			t.Fatalf("resolve %s: %v", id, err)
		}
		judge.replies[title] = "PASS"
	}

	const limit = 2
	rep := calibrateStore(t, store, judge, storage.ResolvedCheckpointFilter{Limit: limit})
	if c := judge.callCount(); c > limit {
		t.Errorf("judge called %d times, want at most %d", c, limit)
	}
	if rep.N != limit {
		t.Fatalf("N = %d, want %d", rep.N, limit)
	}
	// The newest decisions are the ones judged, oldest of them first.
	if got := rep.Items[0].CheckpointID + "," + rep.Items[1].CheckpointID; got != "c4,c5" {
		t.Errorf("judged %s, want c4,c5", got)
	}

	// Without a limit the whole corpus is judged.
	judge.calls = 0
	rep = calibrateStore(t, store, judge, storage.ResolvedCheckpointFilter{})
	if c := judge.callCount(); c != 5 || rep.N != 5 {
		t.Errorf("unlimited: judge called %d times, N = %d, want 5 and 5", c, rep.N)
	}
}

func calibrateStore(t *testing.T, store Store, judge *fakeJudge, f storage.ResolvedCheckpointFilter) CalibrationReport {
	t.Helper()
	rep, err := NewService(store, fixedJudge(judge), nil).Calibrate(context.Background(), f)
	if err != nil {
		t.Fatalf("Calibrate: %v", err)
	}
	return rep
}
