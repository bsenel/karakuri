package eval

import (
	"testing"

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

// No checkpoint carries a world state until an escalation records one.
func TestCalibrate_ReportsNoReplayableWithoutWorldState(t *testing.T) {
	store, judge := fixture(fixtureCase{id: "a", choice: choiceApprove, reply: "PASS"})

	rep := calibrate(t, store, judge, storage.ResolvedCheckpointFilter{})
	if rep.Replayable != 0 {
		t.Errorf("Replayable = %d, want 0", rep.Replayable)
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
