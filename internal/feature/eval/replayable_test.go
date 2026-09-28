package eval

import (
	"context"
	"testing"
	"time"

	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// withWorldState gives the named checkpoints a recorded world state, the way
// escalations after slice 2 carry one.
func withWorldState(store *fakeStore, ids ...string) {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	for i := range store.checkpoints {
		if want[store.checkpoints[i].ID] {
			store.checkpoints[i].WorldState = &coreloop.WorldState{Version: "ws-" + store.checkpoints[i].ID}
		}
	}
}

func TestCountReplayable(t *testing.T) {
	store, judge := fixture(
		fixtureCase{id: "a", choice: choiceApprove, reply: "PASS"},
		fixtureCase{id: "b", choice: choiceReject, reply: "FAIL"},
		fixtureCase{id: "c", choice: choiceModify, reply: "PASS"},
	)
	withWorldState(store, "a", "c")
	f := storage.ResolvedCheckpointFilter{
		TwinID: "twin-a",
		Since:  time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Until:  time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
	}

	n, err := NewService(store, judge).CountReplayable(context.Background(), f)
	if err != nil {
		t.Fatalf("CountReplayable: %v", err)
	}
	if n != 2 {
		t.Errorf("CountReplayable = %d, want 2", n)
	}
	if store.gotFilter == nil {
		t.Fatal("store never saw a filter")
	}
	if got := *store.gotFilter; got.TwinID != f.TwinID || !got.Since.Equal(f.Since) || !got.Until.Equal(f.Until) {
		t.Errorf("store filter = %+v, want %+v", got, f)
	}
	// Counting the corpus is a read of storage, not a run of the judge.
	if c := judge.callCount(); c != 0 {
		t.Errorf("judge called %d times, want 0", c)
	}
}

func TestCountReplayable_EmptyCorpus(t *testing.T) {
	store, judge := fixture(fixtureCase{id: "a", choice: choiceApprove, reply: "PASS"})

	n, err := NewService(store, judge).CountReplayable(context.Background(), storage.ResolvedCheckpointFilter{})
	if err != nil {
		t.Fatalf("CountReplayable: %v", err)
	}
	if n != 0 {
		t.Errorf("CountReplayable = %d, want 0", n)
	}
}

// Replayable counts the corpus, not the scored set: a checkpoint with a world
// state is replayable whether or not it carries a label the judge can be
// scored against.
func TestCalibrate_ReportsReplayable(t *testing.T) {
	store, judge := fixture(
		fixtureCase{id: "scored", choice: choiceApprove, reply: "PASS"},
		fixtureCase{id: "unscored", choice: choiceReject, reply: "FAIL"},
		fixtureCase{id: "weird", choice: "weird", reply: "PASS"},
		fixtureCase{id: "nodecision", choice: choiceApprove, reply: "PASS"},
	)
	for i := range store.checkpoints {
		if store.checkpoints[i].ID == "nodecision" {
			store.checkpoints[i].Decision = nil
		}
	}
	withWorldState(store, "scored", "weird", "nodecision")

	rep := calibrate(t, store, judge, storage.ResolvedCheckpointFilter{})
	if rep.Skipped != 2 {
		t.Fatalf("Skipped = %d, want 2", rep.Skipped)
	}
	if rep.Replayable != 3 {
		t.Errorf("Replayable = %d, want 3", rep.Replayable)
	}
}
