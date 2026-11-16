package checkpoint

// Phase 40 step 5, part B: a decision on a superseded checkpoint authorises
// nothing.
//
// What the code does today, read before writing these:
//   - record refuses a checkpoint whose stored Status is resolved, with a
//     plain fmt.Errorf and no coreerrors sentinel; every other status falls
//     through to ResolveCheckpoint, the audit row and, from Resolve, the
//     resumer. corecheckpoint.StatusSuperseded exists only as a name.
//
// A superseded checkpoint is refused the way a resolved one is, so these ask
// for an error and pin no sentinel: the already-resolved refusal carries none
// either. The refusal must come before anything is recorded or delivered.

import (
	"context"
	"sync"
	"testing"
	"time"

	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/event"
)

// recordingResumer counts every decision handed to it, so a test can say a
// refused decision reached no loop.
type recordingResumer struct {
	mu    sync.Mutex
	calls []string
}

func (r *recordingResumer) ResumeCheckpoint(_ context.Context, checkpointID string, _ corecheckpoint.Decision) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, checkpointID)
	return true, nil
}

func (r *recordingResumer) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

// seedSuperseded writes a checkpoint already closed as superseded by a newer
// one, the way the loop is to leave it, straight into the store.
func seedSuperseded(t *testing.T, store *fakeStore) corecheckpoint.Checkpoint {
	t.Helper()
	closed := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	cp := corecheckpoint.Checkpoint{
		ID: "cp-old", ObjectiveID: "obj", TwinID: "twin",
		Summary:    "the older plan",
		Options:    []string{"approve", "reject", "modify"},
		PlanID:     "sha256:old",
		Status:     corecheckpoint.StatusSuperseded,
		CreatedAt:  closed.Add(-time.Hour),
		ResolvedAt: &closed,
	}
	if err := store.SaveCheckpoint(context.Background(), cp); err != nil {
		t.Fatalf("seed superseded: %v", err)
	}
	return cp
}

func TestSupersededCheckpointRefusesEveryDecision(t *testing.T) {
	choices := []string{corecheckpoint.ChoiceApprove, corecheckpoint.ChoiceReject, corecheckpoint.ChoiceModify}
	doors := []struct {
		name string
		call func(*Service, string, corecheckpoint.Decision) error
	}{
		{"Resolve", func(s *Service, id string, d corecheckpoint.Decision) error { return s.Resolve(context.Background(), id, d) }},
		{"Record", func(s *Service, id string, d corecheckpoint.Decision) error { return s.Record(context.Background(), id, d) }},
	}
	for _, door := range doors {
		for _, choice := range choices {
			t.Run(door.name+"/"+choice, func(t *testing.T) {
				store := newFakeStore()
				resumer := &recordingResumer{}
				svc := NewService(store, event.NewHub())
				svc.SetResumer(resumer)
				cp := seedSuperseded(t, store)

				err := door.call(svc, cp.ID, corecheckpoint.Decision{Choice: choice, Approver: "user:ada"})

				if err == nil {
					t.Errorf("%s(%s) on a superseded checkpoint = nil, want an error: a decision on it authorises nothing", door.name, choice)
				}
				if n := resumer.count(); n != 0 {
					t.Errorf("%s(%s) delivered the decision to %d loop(s); a superseded checkpoint delivers nothing", door.name, choice, n)
				}
				if len(store.events) != 0 {
					t.Errorf("%s(%s) wrote %d audit row(s); a refused decision records nothing", door.name, choice, len(store.events))
				}
				got, _ := store.GetCheckpoint(context.Background(), cp.ID)
				if got.Status != corecheckpoint.StatusSuperseded {
					t.Errorf("%s(%s) left the checkpoint %q, want it still %q", door.name, choice, got.Status, corecheckpoint.StatusSuperseded)
				}
				if got.Decision != nil {
					t.Errorf("%s(%s) stored decision %+v on a superseded checkpoint, want none", door.name, choice, *got.Decision)
				}
			})
		}
	}
}
