package reconcile

// Phase 40 step 4, the supervisor's half: a stream whose loop a restart ended
// with interrupted_by_restart is not deferred, is due on the next tick, and
// the ending does not count toward the circuit breaker.
//
// What the code does today, read before writing this (run.go, 2026-11-16):
//   - A pass that escalated keeps the loop as ActiveLoopID and lets go. Every
//     later pass calls settleActiveLoop first; while the loop's row is not
//     completed the pass re-arms itself and returns, so the stream is deferred
//     for as long as the row stays open. That is the "deferred from then on"
//     a person had to repair by replacing the objective.
//   - Once the row reads completed the pass carries on, but only when it is
//     due: the escalation's finish() scheduled the next pass from the cadence,
//     so with an hourly cadence nothing runs until the hour is up, even though
//     the pass the restart cut off never finished.
//   - finish() counts a failure from outcome.Error; a row's Error and a failed
//     status are how a loop's ending reaches it.
//
// The loop service's half (ending the loop at boot) is specified in
// internal/feature/loop/interrupted_restart_test.go. This test starts from the
// row that half leaves.

import (
	"context"
	"testing"
	"time"

	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

const interruptedByRestart = coreloop.RunOutcome("interrupted_by_restart")

// endByRestart rewrites a loop's row the way the loop service ends it at boot:
// completed, not paused, its reason interrupted_by_restart, and no error, since
// a restart is not a failure of the loop.
func endByRestart(t *testing.T, f *fixture, loopID string) {
	t.Helper()
	ctx := context.Background()
	ls, err := f.store.GetLoopState(ctx, loopID)
	if err != nil {
		t.Fatalf("get loop state %s: %v", loopID, err)
	}
	ls.Completed = true
	ls.Paused = false
	ls.Status = objective.StatusStopped
	ls.Outcome = interruptedByRestart
	ls.Error = ""
	if err := f.store.SaveLoopState(ctx, ls); err != nil {
		t.Fatalf("save loop state: %v", err)
	}
}

func TestAStreamWhoseLoopARestartEndedRunsAFreshPassOnTheNextTick(t *testing.T) {
	const breaker = 3
	f := newFixture(t, Config{BreakerFailures: breaker})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})
	ctx := context.Background()

	// The pass before the restart escalated; its loop was then approved and
	// was acting when the process died, so the supervisor still holds it.
	f.loops.paused = true
	f.loops.checkpoint = "cp-before-restart"
	f.pass(t, obj.ID, reconcile.TriggerManual)
	st := f.state(t, obj.ID)
	interrupted := st.ActiveLoopID
	if interrupted == "" {
		t.Fatal("the escalated pass left no active loop; the fixture does not reach the case")
	}

	// Two failures already on the count, one short of the breaker, so a
	// restart counted as a failure would trip it.
	st.ConsecutiveFailures = breaker - 1
	if err := f.store.SaveReconcileState(ctx, st); err != nil {
		t.Fatalf("seed consecutive failures: %v", err)
	}

	// The restart: the loop service ends the loop at boot. A few minutes on,
	// well inside the hourly cadence and past any re-arm a waiting pass sets.
	endByRestart(t, f, interrupted)
	f.clock = f.clock.Add(6 * time.Minute)

	// The fresh pass escalates too, so its own ending moves no counter and
	// whatever ConsecutiveFailures reads afterwards is the restart's doing.
	f.loops.checkpoint = "cp-after-restart"
	before := f.loops.count()

	due, err := f.store.ListDueReconcileStates(ctx, f.svc.Holder(), f.clock, 10)
	if err != nil {
		t.Fatalf("ListDueReconcileStates: %v", err)
	}
	isDue := false
	for _, d := range due {
		if d.ObjectiveID == obj.ID {
			isDue = true
		}
	}
	if !isDue {
		next := "never"
		if s := f.state(t, obj.ID); s.NextDueAt != nil {
			next = s.NextDueAt.Sub(f.clock).String() + " from now"
		}
		t.Errorf("the stream is not due on the tick after its loop was ended by the restart (next due %s); the pass the restart cut off has to be run again, not left to the cadence", next)
	}

	f.svc.Tick(ctx)
	deadline := time.Now().Add(5 * time.Second)
	for f.loops.count() == before && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := f.loops.count(); got != before+1 {
		t.Fatalf("the next tick started %d loop(s), want 1 fresh pass for the stream whose loop the restart ended", got-before)
	}
	// Wait for the pass to be filed before reading what it left.
	for time.Now().Before(deadline) {
		if s := f.state(t, obj.ID); s.ActiveLoopID != "" && s.ActiveLoopID != interrupted && s.Phase == reconcile.PhaseWaiting {
			break
		}
		time.Sleep(time.Millisecond)
	}

	after := f.state(t, obj.ID)
	if after.ActiveLoopID == interrupted {
		t.Error("the supervisor still holds the loop the restart ended; the stream is deferred on it")
	}
	if after.ConsecutiveFailures != breaker-1 {
		t.Errorf("consecutive_failures = %d after a restart ended the loop, want %d unchanged", after.ConsecutiveFailures, breaker-1)
	}
	if after.Paused {
		t.Errorf("the stream was paused (%s); a restart is not a failure and must not trip the breaker", after.PausedReason)
	}
	pending, _ := f.store.ListPendingCheckpoints(ctx, "")
	for _, cp := range pending {
		if cp.ObjectiveID == obj.ID && cp.Reason == "reconcile_circuit_breaker" {
			t.Errorf("a circuit breaker checkpoint was raised: %s", cp.Summary)
		}
	}
	history, err := f.svc.History(ctx, obj.ID, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	for _, o := range history {
		if o.Failed() {
			t.Errorf("a pass was recorded as failed (%q); the restart's ending is not a failure", o.Error)
		}
		if o.Deferred != "" {
			t.Errorf("a pass was recorded as deferred (%q); a stream whose loop the restart ended is run again, not deferred", o.Deferred)
		}
	}
}
