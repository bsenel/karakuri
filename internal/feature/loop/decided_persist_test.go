package loop

import (
	"context"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
)

// EB-009 (docs/roadmap.md, Engineering Backlog). A loop that has its decision
// and is acting on it must be stored as no longer paused before the actions
// run. Otherwise a restart during the actions, which a delegated coding
// action makes likely since it is allowed 30 minutes, finds the row still
// paused, and ResumeStoredLoops waits for a decision that was already given.
// Nothing is pending for anyone to answer, so the objective stays blocked.

// decidedAndActing runs a loop to its first checkpoint, approves it, and
// returns once the approved plan's first action is in flight and held there.
func decidedAndActing(t *testing.T, env *stopEnv) (*serviceImpl, string, string) {
	t.Helper()
	// No action may be taken on the agent's own authority, so the first plan
	// escalates; the action then blocks until its context ends.
	svc, req := stopFixture(t, env, coreagent.AuthorityBounds{MaxAutonomousActions: 0})
	ctx := context.Background()

	run, err := svc.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	cpID := waitAtCheckpoint(t, svc, run.LoopID)
	if _, err := svc.Resume(ctx, run.LoopID, corecheckpoint.Decision{Choice: "approve", Approver: stopPrincipal}); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	env.waitStarted(t)
	t.Cleanup(func() { _, _ = svc.Stop(context.Background(), run.LoopID, stopPrincipal, "test cleanup") })
	return svc, run.LoopID, cpID
}

func TestADecidedLoopIsStoredAsNotPausedBeforeItActs(t *testing.T) {
	svc, loopID, _ := decidedAndActing(t, newStopEnv(true))

	if st := storedLoop(t, svc.store, loopID); st.Paused {
		t.Error("the stored loop still reads paused while its approved action runs; a restart now would wait for a decision that was already given")
	}
}

func TestARestartDuringTheApprovedActionsDoesNotWaitForTheDecisionAgain(t *testing.T) {
	svc, loopID, firstCP := decidedAndActing(t, newStopEnv(true))

	// A new process over the same store. Its environment does not block, so
	// a replayed loop gets as far as it can on its own.
	next := restarted(t, svc, newStopEnv(false))
	if err := next.ResumeStoredLoops(context.Background()); err != nil {
		t.Fatalf("ResumeStoredLoops: %v", err)
	}

	// The replay plans again and, at this agent's bounds, asks again: a new
	// checkpoint a person can see and answer. Waiting silently on the first
	// one, which is already decided, is the failure.
	deadline := time.Now().Add(5 * time.Second)
	for {
		next.mu.RLock()
		state := next.states[loopID]
		next.mu.RUnlock()
		if state != nil {
			state.mu.RLock()
			cpID := state.result.CheckpointID
			iteration := state.status.Iteration
			state.mu.RUnlock()
			if cpID != nil && *cpID != "" && *cpID != firstCP {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("after the restart the loop sits at iteration %d still holding checkpoint %v; it waits for the decision on %s, which was already given", iteration, cpID, firstCP)
			}
		} else if time.Now().After(deadline) {
			t.Fatal("the restart did not register the decided loop at all")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
