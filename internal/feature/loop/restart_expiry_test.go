package loop

// Phase 35 step 3: a checkpoint that lapsed while the server was down.
//
// What the code does today, read before writing these:
//   - ResumeStoredLoops (service.go) registers every non-completed row in
//     s.states before it returns. A PAUSED row is not replayed: it gets a
//     waiter goroutine blocked on decisionCh. On a decision the waiter puts
//     the decision back on the channel and calls runLoop from the top.
//   - ResumeCheckpoint (service.go) finds the waiting state by status.Paused
//     and result.CheckpointID, both of which ResumeStoredLoops restores from
//     the row, so ExpireDue -> Resolve -> ResumeCheckpoint reaches a loop
//     registered at boot. A row with no CheckpointID cannot be reached.
//   - The replay (runner.go) does not look at the channel first. It runs
//     observe and reason again, then stepDecide. Only if stepDecide escalates
//     does the runner read decisionCh, and by then stepDecide (decide.go) has
//     already written a new escalation row and called cpSvc.Create: a second
//     checkpoint, pending, with a fresh ExpiresAt, whose id replaces
//     result.CheckpointID. The reject waiting on the channel is then consumed,
//     recordCheckpointTerminal writes a rejection row with EscalationReason
//     "rejected_at_checkpoint", and finalizeLoop completes the loop. Nothing
//     resolves the second checkpoint. If the replayed plan does not escalate,
//     the reject is never read at all.
//   - No test in the repository reads "rejected_at_checkpoint" yet. The record
//     is the tool event recordCheckpointTerminal writes, so that is where
//     these tests read it, together with the stored loop state.
//   - Bootstrap ordering (internal/app/bootstrap.go, BootstrapServer): the
//     ResumeStoredLoops call comes before apiApp.Reconcile.Start(ctx) in one
//     straight-line function, and registration is synchronous, so the first
//     tick cannot run before the stored loops are registered. That ordering
//     is not tested here or anywhere: BootstrapServer takes a config path and
//     builds the whole server, with no seam between the two calls, so it
//     cannot be tested at that level without changing the implementation.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

const (
	restartLoopID       = "loop-restart-0001"
	restartCheckpointID = "cp-restart-0001"
	restartTTL          = 72 * time.Hour
)

var restartNow = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

// restartFixture is a server that has just booted with reconcile.checkpoint_ttl
// set, over a store the previous process left one paused loop in. The agent
// may take no action on its own, so the replayed plan escalates, which is what
// the same plan did before the restart.
func restartFixture(t *testing.T, expiresAt time.Time) (*serviceImpl, *featurecp.Service, storage.StorageAdapter, objective.Objective) {
	t.Helper()
	svc, _, obj := traceLoopFixture(t)
	svc.checkpointTTL = restartTTL
	store, cpSvc := svc.store, svc.cpSvc
	ctx := context.Background()

	err := store.SaveCheckpoint(ctx, corecheckpoint.Checkpoint{
		ID: restartCheckpointID, ObjectiveID: obj.ID,
		Reason:    "authority_exceeded",
		Summary:   "raised before the restart",
		Options:   []string{"approve", "reject", "modify"},
		Status:    corecheckpoint.StatusPending,
		CreatedAt: expiresAt.Add(-restartTTL),
		ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}

	req, err := json.Marshal(coreloop.Request{
		Objective: obj,
		Agent:     traceAgent(coreagent.AuthorityBounds{MaxAutonomousActions: 0}),
		MaxIter:   1,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	err = store.SaveLoopState(ctx, coreloop.State{
		LoopID:       restartLoopID,
		ObjectiveID:  obj.ID,
		Paused:       true,
		Completed:    false,
		LastStep:     coreloop.StepDecide,
		Status:       objective.StatusActive,
		CheckpointID: restartCheckpointID,
		RequestJSON:  string(req),
	})
	if err != nil {
		t.Fatalf("save loop state: %v", err)
	}
	return svc, cpSvc, store, obj
}

func objectiveEvents(t *testing.T, store storage.StorageAdapter, obj objective.Objective, kind string) []storage.ToolEvent {
	t.Helper()
	evs, err := store.ListToolEvents(context.Background(), storage.ToolEventFilter{ObjectiveID: string(obj.ID), Kind: kind})
	if err != nil {
		t.Fatalf("list %s events: %v", kind, err)
	}
	return evs
}

// terminalRejections are the rows recordCheckpointTerminal writes.
func terminalRejections(t *testing.T, store storage.StorageAdapter, obj objective.Objective) []storage.ToolEvent {
	t.Helper()
	var out []storage.ToolEvent
	for _, e := range objectiveEvents(t, store, obj, storage.ToolEventRejection) {
		if e.EscalationReason == "rejected_at_checkpoint" {
			out = append(out, e)
		}
	}
	return out
}

func pendingFor(t *testing.T, store storage.StorageAdapter, obj objective.Objective) []corecheckpoint.Checkpoint {
	t.Helper()
	all, err := store.ListPendingCheckpoints(context.Background(), "")
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	var out []corecheckpoint.Checkpoint
	for _, cp := range all {
		if cp.ObjectiveID == obj.ID {
			out = append(out, cp)
		}
	}
	return out
}

// The acceptance line: a checkpoint that lapsed while the server was down is
// expired by the first sweep after boot, and its loop ends as
// rejected_at_checkpoint rather than waiting again.
func TestRestartFirstSweepExpiresACheckpointThatLapsedWhileDown(t *testing.T) {
	svc, cpSvc, store, obj := restartFixture(t, restartNow.Add(-time.Hour))
	ctx := context.Background()

	if err := svc.ResumeStoredLoops(ctx); err != nil {
		t.Fatalf("ResumeStoredLoops: %v", err)
	}

	n, err := cpSvc.ExpireDue(ctx, restartNow)
	if err != nil {
		t.Fatalf("ExpireDue: %v", err)
	}
	if n != 1 {
		t.Fatalf("ExpireDue expired %d checkpoints, want 1", n)
	}

	// (a) The lapsed checkpoint is a rejection by system:timeout.
	cp, err := store.GetCheckpoint(ctx, restartCheckpointID)
	if err != nil {
		t.Fatalf("get checkpoint: %v", err)
	}
	if cp.Status != corecheckpoint.StatusResolved {
		t.Errorf("checkpoint status = %q, want resolved", cp.Status)
	}
	if cp.Decision == nil {
		t.Fatal("checkpoint carries no decision after the sweep")
	}
	if cp.Decision.Choice != "reject" || cp.Decision.Approver != "system:timeout" {
		t.Errorf("decision = %q by %q, want reject by system:timeout", cp.Decision.Choice, cp.Decision.Approver)
	}

	// (b) The loop the sweep reached ends, on the record, as
	// rejected_at_checkpoint.
	deadline := time.Now().Add(10 * time.Second)
	for {
		st, err := store.GetLoopState(ctx, restartLoopID)
		if err == nil && st.Completed && len(terminalRejections(t, store, obj)) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("loop did not end as rejected_at_checkpoint: stored state completed=%v paused=%v (err %v), %d terminal rejection rows",
				st.Completed, st.Paused, err, len(terminalRejections(t, store, obj)))
		}
		time.Sleep(10 * time.Millisecond)
	}
	terminal := terminalRejections(t, store, obj)
	if len(terminal) != 1 {
		t.Errorf("%d rejected_at_checkpoint rows, want 1", len(terminal))
	}
	for _, e := range terminal {
		if e.Approver != "system:timeout" {
			t.Errorf("terminal row approver = %q, want system:timeout", e.Approver)
		}
	}
	if got, err := store.GetObjective(ctx, obj.ID); err != nil {
		t.Errorf("get objective: %v", err)
	} else if got.Status != objective.StatusFailed {
		t.Errorf("objective status = %q, want %q", got.Status, objective.StatusFailed)
	}

	// (c) The replay did not raise a second checkpoint that waits again.
	if left := pendingFor(t, store, obj); len(left) != 0 {
		ids := make([]string, 0, len(left))
		for _, cp := range left {
			ids = append(ids, cp.ID)
		}
		t.Errorf("%d checkpoint(s) left pending for the objective after the expiry: %v; the replay raised a second one nobody will answer", len(left), ids)
	}

	// (d) Expiry never approves.
	if approvals := objectiveEvents(t, store, obj, storage.ToolEventApproval); len(approvals) != 0 {
		t.Errorf("%d approval row(s) recorded for an expired checkpoint, want none", len(approvals))
	}
}

// A checkpoint that still has time is left alone by the first sweep: pending,
// its loop registered and waiting, nothing replayed and nothing recorded.
func TestRestartFirstSweepLeavesACheckpointWithTimeLeft(t *testing.T) {
	svc, cpSvc, store, obj := restartFixture(t, restartNow.Add(time.Hour))
	ctx := context.Background()

	if err := svc.ResumeStoredLoops(ctx); err != nil {
		t.Fatalf("ResumeStoredLoops: %v", err)
	}

	n, err := cpSvc.ExpireDue(ctx, restartNow)
	if err != nil {
		t.Fatalf("ExpireDue: %v", err)
	}
	if n != 0 {
		t.Errorf("ExpireDue expired %d checkpoints, want 0", n)
	}

	// Long enough for a waiter that was wrongly woken to start a replay.
	time.Sleep(200 * time.Millisecond)

	cp, err := store.GetCheckpoint(ctx, restartCheckpointID)
	if err != nil {
		t.Fatalf("get checkpoint: %v", err)
	}
	if cp.Status != corecheckpoint.StatusPending || cp.Decision != nil {
		t.Errorf("checkpoint status = %q, decision = %v; want pending and undecided", cp.Status, cp.Decision)
	}

	svc.mu.RLock()
	state, ok := svc.states[restartLoopID]
	svc.mu.RUnlock()
	if !ok {
		t.Fatal("the paused loop is not registered after ResumeStoredLoops")
	}
	state.mu.RLock()
	paused, iteration := state.status.Paused, state.status.Iteration
	state.mu.RUnlock()
	if !paused || iteration != 0 {
		t.Errorf("loop paused = %v at iteration %d; want still paused at iteration 0", paused, iteration)
	}
	if len(state.decisionCh) != 0 {
		t.Error("a decision was delivered to a loop whose checkpoint has not lapsed")
	}

	st, err := store.GetLoopState(ctx, restartLoopID)
	if err != nil {
		t.Fatalf("get loop state: %v", err)
	}
	if st.Completed || !st.Paused {
		t.Errorf("stored loop completed = %v, paused = %v; want still paused", st.Completed, st.Paused)
	}

	if left := pendingFor(t, store, obj); len(left) != 1 || left[0].ID != restartCheckpointID {
		t.Errorf("pending checkpoints for the objective = %v, want only %s", left, restartCheckpointID)
	}
	for _, kind := range []string{storage.ToolEventRejection, storage.ToolEventApproval} {
		if evs := objectiveEvents(t, store, obj, kind); len(evs) != 0 {
			t.Errorf("%d %s row(s) recorded for a checkpoint with time left, want none", len(evs), kind)
		}
	}
}
