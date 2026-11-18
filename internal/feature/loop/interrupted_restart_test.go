package loop

// Phase 40 step 4: an interrupted loop is ended and its pass re-run.
//
// What the code does today, read before writing these (service.go,
// ResumeStoredLoops, line 555 as read on 2026-11-16):
//   - Every row ListActiveLoopStates returns (completed = false) is
//     registered. A row that is not paused is replayed at once: runLoop from
//     the top, observe, reason, decide and act, whatever objective it belongs
//     to. A paused row gets a waiter on its decision channel, whether or not
//     its checkpoint is still pending.
//   - For a standing objective that replay is a second lifecycle beside the
//     one reconcile runs: the supervisor's next pass starts its own loop, and
//     a paused row whose checkpoint was already answered waits on a channel
//     nothing will write to (the 2026-10-24 rejections and EB-009).
//
// The step chooses ending over resuming: a standing objective's loop with no
// pending checkpoint is ended with the reason interrupted_by_restart, not as
// a failure, and the supervisor's next tick runs a fresh pass from the branch
// tip. A loop waiting at a pending checkpoint, and any loop of a one-shot
// objective, are restored exactly as before.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// interruptedByRestart is the reason the step names. Where the code keeps it
// is the code slice's choice; the stored row's outcome is where reconcile and
// the loop's own status already read how a run ended, as Phase 37 keeps
// stopped there.
const interruptedByRestart = coreloop.RunOutcome("interrupted_by_restart")

const interruptedLoopID = "loop-interrupted-0001"

// standing turns the fixture's objective into a standing one, in the store and
// in the request the stored row carries, the way reconcile's passes run them.
func standing(t *testing.T, svc *serviceImpl, obj objective.Objective) objective.Objective {
	t.Helper()
	obj.Mode = objective.ModeStanding
	obj.Cadence = &objective.Cadence{Every: "1h"}
	obj.Autonomy = &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct}
	if err := svc.store.SaveObjective(context.Background(), obj); err != nil {
		t.Fatalf("save standing objective: %v", err)
	}
	return obj
}

// storeInterrupted writes the row a process leaves when it dies in the middle
// of an iteration: not completed, and, unless paused is set, not paused.
func storeInterrupted(t *testing.T, svc *serviceImpl, obj objective.Objective, paused bool, cpID string) {
	t.Helper()
	req, err := json.Marshal(coreloop.Request{Objective: obj, Agent: traceAgent(unlimited), MaxIter: 5})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	step := coreloop.StepAct
	if paused {
		step = coreloop.StepDecide
	}
	if err := svc.store.SaveLoopState(context.Background(), coreloop.State{
		LoopID:       interruptedLoopID,
		ObjectiveID:  obj.ID,
		Iteration:    1,
		Paused:       paused,
		Completed:    false,
		LastStep:     step,
		Status:       objective.StatusActive,
		CheckpointID: cpID,
		RequestJSON:  string(req),
	}); err != nil {
		t.Fatalf("save loop state: %v", err)
	}
}

func storeCheckpoint(t *testing.T, svc *serviceImpl, obj objective.Objective, id string, status corecheckpoint.Status, decision *corecheckpoint.Decision) {
	t.Helper()
	if err := svc.store.SaveCheckpoint(context.Background(), corecheckpoint.Checkpoint{
		ID:          id,
		ObjectiveID: obj.ID,
		Reason:      "authority_exceeded",
		Summary:     "raised before the restart",
		Options:     []string{"approve", "reject", "modify"},
		Status:      status,
		Decision:    decision,
		CreatedAt:   time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}
}

// assertEndedByRestart is what the ended row must say: ended, so no later
// restart lists it; ended for the restart, as its own reason; and not a
// failure, so reconcile's breaker does not count it.
func assertEndedByRestart(t *testing.T, st coreloop.State) {
	t.Helper()
	if !st.Completed {
		t.Errorf("stored loop completed = false; a standing objective's interrupted loop must be ended at boot, not left for a replay")
	}
	if st.Paused {
		t.Error("stored loop still reads paused; an ended loop waits for nothing")
	}
	if st.Outcome != interruptedByRestart {
		t.Errorf("stored outcome = %q, want %q", st.Outcome, interruptedByRestart)
	}
	if st.Status == objective.StatusFailed || st.Outcome == coreloop.RunErrored {
		t.Errorf("stored status = %q, outcome = %q; a restart is not a failure of the loop", st.Status, st.Outcome)
	}
	if st.Status == objective.StatusActive || st.Status == "" {
		t.Errorf("stored status = %q; an ended loop is not active", st.Status)
	}
	if st.Error != "" {
		t.Errorf("stored error = %q; reconcile reads this field as the run's failure and counts it toward the breaker", st.Error)
	}
}

// (1) The acceptance line, at the loop's seam: a standing objective's loop
// that was acting when the process died is ended with interrupted_by_restart,
// and no action of it starts again.
func TestARestartEndsAStandingObjectivesInterruptedLoop(t *testing.T) {
	env := newStopEnv(false)
	svc, req := stopFixture(t, env, unlimited)
	obj := standing(t, svc, req.Objective)
	storeInterrupted(t, svc, obj, false, "")
	ctx := context.Background()
	// Until the step lands the replay runs on past the test; stop it before
	// the store is removed.
	t.Cleanup(func() { _, _ = svc.Stop(context.Background(), interruptedLoopID, stopPrincipal, "test cleanup") })

	if err := svc.ResumeStoredLoops(ctx); err != nil {
		t.Fatalf("ResumeStoredLoops: %v", err)
	}
	time.Sleep(stopSettle)

	if n := env.startCount(); n != 0 {
		t.Errorf("the restart replayed the interrupted loop and started %d action(s); a standing objective's loop is ended and its pass re-run by reconcile, not resumed", n)
	}
	assertEndedByRestart(t, storedLoop(t, svc.store, interruptedLoopID))

	active, err := svc.store.ListActiveLoopStates(ctx)
	if err != nil {
		t.Fatalf("ListActiveLoopStates: %v", err)
	}
	for _, st := range active {
		if st.LoopID == interruptedLoopID {
			t.Error("ListActiveLoopStates still lists the ended loop; the next restart would find it again")
		}
	}
	if got, err := svc.store.GetObjective(ctx, obj.ID); err != nil {
		t.Errorf("get objective: %v", err)
	} else if got.Status == objective.StatusFailed {
		t.Errorf("objective status = %q after a restart ended its loop; a restart is not a failure", got.Status)
	}
}

// (1b) The 2026-10-24 shape: a standing objective's loop stored paused at
// decide whose checkpoint was already answered. Nothing is pending for anyone
// to answer, so it is ended the same way rather than given a waiter.
func TestARestartEndsAStandingLoopPausedOnAnAnsweredCheckpoint(t *testing.T) {
	env := newStopEnv(false)
	svc, req := stopFixture(t, env, unlimited)
	obj := standing(t, svc, req.Objective)
	const answered = "cp-answered-0001"
	storeCheckpoint(t, svc, obj, answered, corecheckpoint.StatusResolved,
		&corecheckpoint.Decision{Choice: "approve", Approver: "user:ada"})
	storeInterrupted(t, svc, obj, true, answered)
	t.Cleanup(func() { _, _ = svc.Stop(context.Background(), interruptedLoopID, stopPrincipal, "test cleanup") })

	if err := svc.ResumeStoredLoops(context.Background()); err != nil {
		t.Fatalf("ResumeStoredLoops: %v", err)
	}
	time.Sleep(stopSettle)

	if n := env.startCount(); n != 0 {
		t.Errorf("%d action(s) started for a loop the restart should have ended", n)
	}
	assertEndedByRestart(t, storedLoop(t, svc.store, interruptedLoopID))
}

// (2) A standing objective's loop waiting at a checkpoint that is still
// pending is restored exactly as today: registered, paused, nothing replayed,
// its row untouched. The person's answer is what moves it.
func TestARestartStillRestoresAStandingLoopAtAPendingCheckpoint(t *testing.T) {
	env := newStopEnv(false)
	svc, req := stopFixture(t, env, unlimited)
	obj := standing(t, svc, req.Objective)
	const pending = "cp-pending-0001"
	storeCheckpoint(t, svc, obj, pending, corecheckpoint.StatusPending, nil)
	storeInterrupted(t, svc, obj, true, pending)
	ctx := context.Background()
	before := storedLoop(t, svc.store, interruptedLoopID)

	if err := svc.ResumeStoredLoops(ctx); err != nil {
		t.Fatalf("ResumeStoredLoops: %v", err)
	}
	time.Sleep(stopSettle)

	svc.mu.RLock()
	state, ok := svc.states[interruptedLoopID]
	svc.mu.RUnlock()
	if !ok {
		t.Fatal("the loop waiting at a pending checkpoint is not registered; resolving the checkpoint cannot reach it")
	}
	state.mu.RLock()
	paused, cpID := state.status.Paused, state.result.CheckpointID
	state.mu.RUnlock()
	if !paused || cpID == nil || *cpID != pending {
		t.Errorf("registered loop paused = %v, checkpoint = %v; want paused at %s", paused, cpID, pending)
	}
	if n := env.startCount(); n != 0 {
		t.Errorf("%d action(s) started for a loop waiting on a person", n)
	}
	assertRowUnchanged(t, before, storedLoop(t, svc.store, interruptedLoopID), "after the restart")
}

// (3) A one-shot objective has no supervisor to re-run its pass, so its loop
// is still resumed as today: the replay acts, and the row does not read as
// ended by the restart.
func TestARestartStillResumesAOneShotObjectivesLoop(t *testing.T) {
	env := newStopEnv(false)
	svc, req := stopFixture(t, env, unlimited)
	obj := req.Objective // as the fixture saved it: not standing
	if obj.IsStanding() {
		t.Fatal("the fixture's objective is standing; this test needs a one-shot one")
	}
	storeInterrupted(t, svc, obj, false, "")

	if err := svc.ResumeStoredLoops(context.Background()); err != nil {
		t.Fatalf("ResumeStoredLoops: %v", err)
	}
	env.waitStarted(t)

	if st := storedLoop(t, svc.store, interruptedLoopID); st.Outcome == interruptedByRestart {
		t.Errorf("a one-shot objective's loop was ended with %q; only a standing objective's pass is re-run by a supervisor", st.Outcome)
	}
	t.Cleanup(func() { _, _ = svc.Stop(context.Background(), interruptedLoopID, stopPrincipal, "test cleanup") })
}
