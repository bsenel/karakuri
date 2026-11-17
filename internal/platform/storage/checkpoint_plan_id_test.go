package storage_test

import (
	"context"
	"testing"

	"github.com/bsenel/karakuri/internal/core/checkpoint"
)

// A checkpoint's plan identifier is what holds an approval to the plan the
// reviewer was shown (Phase 40 step 5): if it does not come back as it was
// saved, a restored loop cannot tell the plan it may run from any other.

func TestCheckpointPlanID_RoundTrips(t *testing.T) {
	s := newStore(t)
	const want = "plan-3f9a1c"
	err := s.SaveCheckpoint(context.Background(), checkpoint.Checkpoint{
		ID: "planned", ObjectiveID: "obj-plan", TwinID: "twin-a",
		Summary: "summary planned", Options: []string{"approve", "reject", "modify"},
		Actions: []checkpoint.Action{{CapabilityID: "test.act.ok", Params: map[string]any{"target": "shown.txt"}}},
		PlanID:  want,
		Status:  checkpoint.StatusPending,
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	if got := getCheckpoint(t, s, "planned").PlanID; got != want {
		t.Errorf("GetCheckpoint PlanID = %q, want %q", got, want)
	}
	listed, ok := pendingByID(t, s)["planned"]
	if !ok {
		t.Fatalf("checkpoint missing from the pending list")
	}
	if listed.PlanID != want {
		t.Errorf("ListPendingCheckpoints PlanID = %q, want %q", listed.PlanID, want)
	}
}

// A checkpoint that approves no plan, a budget pause or one written before the
// column existed, reads back with no plan rather than an invented one.
func TestCheckpointPlanID_EmptyWhenUnset(t *testing.T) {
	s := newStore(t)
	saveCheckpointWithWorldState(t, s, "no-plan", nil)

	if got := getCheckpoint(t, s, "no-plan").PlanID; got != "" {
		t.Errorf("GetCheckpoint PlanID = %q, want empty", got)
	}
}
