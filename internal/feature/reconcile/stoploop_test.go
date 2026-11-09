package reconcile

import (
	"context"
	"errors"
	"testing"
	"time"

	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
)

// Phase 37 step 3, the owner's decision on #222: a stop must stop a
// watch-mode run too. A watch-mode loop's wait for the next event is its
// standing objective's, so stopping only the loop leaves the objective
// standing and the next tick starts a new pass. StopLoop stops the loop
// through the loop service and, when the loop belongs to a standing
// objective, pauses that objective with the stop's reason, which cancels the
// pass in flight as step 1 does.
//
// The fake loop runners here record Stop rather than store a stopped row (the
// loop service's own tests cover that row), so "the loop is stopped" reads as
// "the loop service was asked to stop it, by the stopper, with the reason".

// stopLoopBound is how long a stopped pass may take to be cancelled and
// filed. The acceptance line says 10 seconds.
const stopLoopBound = 10 * time.Second

// stopLoopReason is the reason every StopLoop here is given. It is the one
// assertFiledStopped expects the objective to be left paused with.
const stopLoopReason = "operator stopped it"

// stopLoopOf stops the blocked pass's loop through StopLoop as alice, and
// fails the test when StopLoop errs.
func stopLoopOf(t *testing.T, s *stoppable) LoopStopResult {
	t.Helper()
	loopID := s.loop.id()
	if loopID == "" {
		t.Fatal("the blocked pass has no loop ID")
	}
	res, err := s.svc.StopLoop(context.Background(), loopID, "alice", stopLoopReason)
	if err != nil {
		t.Fatalf("StopLoop of a running loop of a standing objective: %v", err)
	}
	return res
}

// (a) Stopping a loop of a standing objective in the middle of a pass stops
// the loop with the stopper and the reason, and pauses the objective with
// the reason. The pass is cancelled and filed as stopped, which is not a
// failure, and the answer names the objective as paused.
func TestStopLoopOfAStandingObjectivePausesIt(t *testing.T) {
	s := startBlockedPass(t)
	loopID := s.loop.id()
	res := stopLoopOf(t, s)

	select {
	case <-s.loop.cancelled:
	case <-time.After(stopLoopBound):
		t.Fatalf("the pass's context was not cancelled %s after StopLoop returned", stopLoopBound)
	}
	if !waitFor(stopLoopBound, func() bool { return !s.svc.isRunning(s.obj.ID) }) {
		t.Fatalf("the pass was still running %s after StopLoop returned", stopLoopBound)
	}

	// The pause's own stop of the pass's loop (finishStopped, as "reconcile")
	// may come too; the stopper's must be among the calls, and every call is
	// for this loop.
	calls := awaitStops(&s.loop.stopRecorder, 1)
	byStopper := false
	for _, c := range calls {
		if c.loopID != loopID {
			t.Errorf("Stop called for loop %q, want only the pass's loop %q", c.loopID, loopID)
		}
		if c.principal == "alice" && c.reason == stopLoopReason {
			byStopper = true
		}
	}
	if !byStopper {
		t.Errorf("the loop service was not asked to stop loop %q as alice with reason %q; calls: %+v",
			loopID, stopLoopReason, calls)
	}

	// Filed stopped, and the objective left paused with the stop's reason.
	assertFiledStopped(t, s.fixture, stoppedOutcome(t, s.svc, s.obj.ID, loopID), s.obj.ID)

	st := s.state(t, s.obj.ID)
	if st.ConsecutiveFailures != s.before.ConsecutiveFailures {
		t.Errorf("consecutive failures = %d after StopLoop, want %d as before the pass",
			st.ConsecutiveFailures, s.before.ConsecutiveFailures)
	}

	if res.LoopID != loopID {
		t.Errorf("result loop = %q, want %q", res.LoopID, loopID)
	}
	if res.Found != featureloop.StopFoundRunning {
		t.Errorf("result found = %q, want %q: the loop was mid-pass when the stop arrived",
			res.Found, featureloop.StopFoundRunning)
	}
	if res.ObjectiveID != s.obj.ID {
		t.Errorf("result objective = %q, want the loop's standing objective %q", res.ObjectiveID, s.obj.ID)
	}
	if !res.ObjectivePaused {
		t.Error("result says the objective was not paused")
	}
}

// (b) After the stop, the watch is over: a tick through the entry point the
// cadence uses starts no pass, even once the objective's cadence has come
// round again, and a manual trigger is refused.
func TestStopLoopLeavesNoPassForTheNextTick(t *testing.T) {
	s := startBlockedPass(t)
	stopLoopOf(t, s)
	if !waitFor(stopLoopBound, func() bool { return !s.svc.isRunning(s.obj.ID) }) {
		t.Fatalf("the pass was still running %s after StopLoop returned", stopLoopBound)
	}
	history, err := s.svc.History(context.Background(), s.obj.ID, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}

	// Past the objective's 1h cadence, so a standing objective that was not
	// paused would be due.
	s.clock = s.clock.Add(2 * time.Hour)
	s.svc.Tick(context.Background())

	if waitFor(stopSettle, func() bool { return s.loop.runCount() > 1 || s.svc.isRunning(s.obj.ID) }) {
		t.Errorf("a tick after StopLoop started a pass: %d loop runs, running %v",
			s.loop.runCount(), s.svc.isRunning(s.obj.ID))
	}
	waitFor(stopWait, func() bool { return !s.svc.isRunning(s.obj.ID) })
	after, err := s.svc.History(context.Background(), s.obj.ID, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(after) != len(history) {
		t.Errorf("a tick after StopLoop recorded %d new outcomes, want 0", len(after)-len(history))
	}
	if st := s.state(t, s.obj.ID); !st.Paused || st.PausedReason != stopLoopReason {
		t.Errorf("after the tick: paused %v, reason %q; want paused with %q", st.Paused, st.PausedReason, stopLoopReason)
	}

	if err := s.svc.Trigger(context.Background(), s.obj.ID); err == nil {
		t.Error("a manual trigger was accepted after StopLoop paused the objective")
	}
	if waitFor(stopSettle, func() bool { return s.loop.runCount() > 1 }) {
		t.Errorf("a loop ran after StopLoop; %d runs", s.loop.runCount())
	}
}

// (c) A loop of a one-shot objective has no watch to end: only the loop is
// stopped, no objective is paused (none has a control loop to pause), and
// the answer says so.
func TestStopLoopOfAOneShotLoopPausesNothing(t *testing.T) {
	f := newFixture(t, Config{})
	ctx := context.Background()
	obj := objective.Objective{ID: "obj-oneshot", Mode: objective.ModeOneshot, Domain: "test"}
	if err := f.store.SaveObjective(ctx, obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}
	if err := f.store.SaveLoopState(ctx, coreloop.State{
		LoopID:      "loop-oneshot",
		ObjectiveID: obj.ID,
		Status:      objective.StatusActive,
	}); err != nil {
		t.Fatalf("save loop state: %v", err)
	}

	res, err := f.svc.StopLoop(ctx, "loop-oneshot", "alice", "wrong branch")
	if err != nil {
		t.Fatalf("StopLoop of a one-shot loop: %v", err)
	}

	calls := f.loops.stopCalls()
	want := stopCall{loopID: "loop-oneshot", principal: "alice", reason: "wrong branch"}
	if len(calls) != 1 || calls[0] != want {
		t.Errorf("Stop calls = %+v, want exactly %+v", calls, want)
	}
	if res.LoopID != "loop-oneshot" || res.Found != featureloop.StopFoundRunning {
		t.Errorf("result = %+v, want the loop service's answer: loop-oneshot found running", res.StopResult)
	}
	if res.ObjectivePaused {
		t.Error("result says an objective was paused for a one-shot loop")
	}
	if res.ObjectiveID != obj.ID {
		t.Errorf("result objective = %q, want the loop's own %q, named though not paused", res.ObjectiveID, obj.ID)
	}
	if _, err := f.store.GetReconcileState(ctx, obj.ID); !errors.Is(err, coreerrors.ErrNotFound) {
		t.Errorf("a one-shot objective has a control-loop state after StopLoop (err %v); nothing should be paused", err)
	}
	if f.loops.count() != 0 {
		t.Errorf("StopLoop ran %d loops, want 0", f.loops.count())
	}
}
