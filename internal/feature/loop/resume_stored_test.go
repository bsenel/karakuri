package loop

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// saveState writes the row ResumeStoredLoops reads.
func saveState(t *testing.T, store storage.StorageAdapter, loopID string, paused bool) {
	t.Helper()
	req, _ := json.Marshal(coreloop.Request{Objective: objective.Objective{ID: "obj-1"}, MaxIter: 1})
	err := store.SaveLoopState(context.Background(), coreloop.State{
		LoopID:      loopID,
		ObjectiveID: "obj-1",
		Paused:      paused,
		Completed:   false,
		Status:      "running",
		RequestJSON: string(req),
	})
	if err != nil {
		t.Fatalf("save loop state: %v", err)
	}
}

// A paused loop is a loop waiting on a question nobody has answered. Replaying
// it costs a reason step and produces a second checkpoint asking the same
// thing, and on a deployment holding a dozen of them that is what a restart
// costs before anyone has typed anything.
func TestResumeStoredLoopsDoesNotReplayAPausedLoop(t *testing.T) {
	svc, _, store := newResumeFixture(t)
	saveState(t, store, "paused-1", true)

	if err := svc.ResumeStoredLoops(context.Background()); err != nil {
		t.Fatalf("ResumeStoredLoops: %v", err)
	}

	// Registered, so Resume() can still find it...
	svc.mu.RLock()
	state, ok := svc.states["paused-1"]
	svc.mu.RUnlock()
	if !ok {
		t.Fatal("a paused loop must still be registered, or resolving its checkpoint cannot reach it")
	}

	// ...but nothing has run. A replay would have advanced the iteration and
	// written a fresh checkpoint.
	time.Sleep(150 * time.Millisecond)
	state.mu.RLock()
	iteration := state.status.Iteration
	state.mu.RUnlock()
	if iteration != 0 {
		t.Errorf("paused loop advanced to iteration %d; it must not run until a decision arrives", iteration)
	}
}

// The waiter must hand the decision back, so the replayed runner consumes it
// at its own first escalation. Otherwise resolving a checkpoint costs the
// operator a second approval for the same question.
func TestResumeStoredLoopsGivesTheDecisionBackToTheRunner(t *testing.T) {
	svc, _, store := newResumeFixture(t)
	saveState(t, store, "paused-2", true)

	if err := svc.ResumeStoredLoops(context.Background()); err != nil {
		t.Fatalf("ResumeStoredLoops: %v", err)
	}
	svc.mu.RLock()
	state := svc.states["paused-2"]
	svc.mu.RUnlock()

	state.decisionCh <- corecheckpoint.Decision{Choice: "approve"}

	// The waiter wakes, puts the decision back, and starts the runner. The
	// decision must still be readable.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("the operator's decision was consumed and never returned to the channel")
		default:
		}
		select {
		case d := <-state.decisionCh:
			if d.Choice != "approve" {
				t.Fatalf("decision came back as %q, want approve", d.Choice)
			}
			return
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// The bound exists so a deployment's whole backlog cannot open at once. The
// budget gate is per iteration, so simultaneous starts all read the allowance
// before any of them has spent against it.
func TestResumeConcurrencyIsBounded(t *testing.T) {
	if maxResumeConcurrency <= 0 {
		t.Fatal("maxResumeConcurrency must bound the replay")
	}
	if maxResumeConcurrency > 8 {
		t.Errorf("maxResumeConcurrency = %d is not a bound worth having", maxResumeConcurrency)
	}
}
