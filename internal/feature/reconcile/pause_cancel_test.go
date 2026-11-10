package reconcile

import (
	"context"
	"sync"
	"testing"
	"time"

	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 37 step 1: a pause cancels the pass in flight. These tests drive the
// pass through dispatch, where the cancel function is kept, with a loop whose
// actions block on the context the pass handed to Run.

// stopWait bounds every wait in these tests. Generous so a slow CI runner does
// not flake; a pass that has not ended by then was not stopped.
const stopWait = 5 * time.Second

// blockingLoop is a loop whose actions run until released or until the context
// the supervisor passed to Run is cancelled. On cancellation it writes the row
// the way a loop that honoured its context would: completed, failed, with the
// context's error. That row reads like a failure on purpose; the supervisor has
// to file the pass as stopped all the same.
type blockingLoop struct {
	stopRecorder

	store storage.StorageAdapter

	mu      sync.Mutex
	runs    int
	starts  int
	loopID  string
	release chan struct{}

	// completed and cut are what Stop answers the loop had run.
	completed []featureloop.StopAction
	cut       *featureloop.StopAction

	started   chan struct{} // closed when the first action starts
	cancelled chan struct{} // closed when the Run context is seen cancelled
	done      chan struct{} // closed when the action goroutine has exited
}

func newBlockingLoop(store storage.StorageAdapter) *blockingLoop {
	return &blockingLoop{
		store:     store,
		release:   make(chan struct{}),
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
		done:      make(chan struct{}),
	}
}

func (b *blockingLoop) Run(ctx context.Context, req coreloop.Request) (coreloop.Result, error) {
	b.mu.Lock()
	b.runs++
	first := b.runs == 1
	b.mu.Unlock()
	if !first {
		// Only one pass is expected in these tests. A second one means
		// Trigger or the tick worked on a paused objective.
		return coreloop.Result{}, context.Canceled
	}

	id := newOutcomeID()
	if err := b.store.SaveLoopState(ctx, coreloop.State{
		LoopID:      id,
		ObjectiveID: req.Objective.ID,
		Status:      objective.StatusActive,
	}); err != nil {
		return coreloop.Result{}, err
	}
	b.mu.Lock()
	b.loopID = id
	b.mu.Unlock()

	go b.act(ctx, id, req.Objective.ID)
	return coreloop.Result{LoopID: id, ObjectiveID: req.Objective.ID}, nil
}

// act starts actions one after the other. Each blocks until released or
// cancelled, and none starts once the context is done.
func (b *blockingLoop) act(ctx context.Context, loopID string, objID objective.ObjectiveID) {
	defer close(b.done)
	for {
		if ctx.Err() != nil {
			b.finalise(ctx, loopID, objID)
			return
		}
		b.mu.Lock()
		b.starts++
		if b.starts == 1 {
			close(b.started)
		}
		release := b.release
		b.mu.Unlock()

		select {
		case <-ctx.Done():
			b.finalise(ctx, loopID, objID)
			return
		case <-release:
		}
	}
}

func (b *blockingLoop) finalise(ctx context.Context, loopID string, objID objective.ObjectiveID) {
	close(b.cancelled)
	_ = b.store.SaveLoopState(context.WithoutCancel(ctx), coreloop.State{
		LoopID:      loopID,
		ObjectiveID: objID,
		Completed:   true,
		Status:      objective.StatusFailed,
		Outcome:     coreloop.RunErrored,
		Error:       ctx.Err().Error(),
	})
}

// releaseAction lets the action in flight end. The next one starts at once
// unless the context is already cancelled.
func (b *blockingLoop) releaseAction() {
	b.mu.Lock()
	defer b.mu.Unlock()
	close(b.release)
	b.release = make(chan struct{})
}

func (b *blockingLoop) actionStarts() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.starts
}

func (b *blockingLoop) runCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.runs
}

// Stop records the call and answers with the actions setStopActions gave,
// as the loop service answers with what the stopped loop had run.
func (b *blockingLoop) Stop(ctx context.Context, loopID, principal, reason string) (featureloop.StopResult, error) {
	res, err := b.stopRecorder.Stop(ctx, loopID, principal, reason)
	b.mu.Lock()
	res.Completed = append([]featureloop.StopAction(nil), b.completed...)
	res.Cut = b.cut
	b.mu.Unlock()
	return res, err
}

// setStopActions sets the completed and cut actions every later Stop answers.
func (b *blockingLoop) setStopActions(completed []featureloop.StopAction, cut *featureloop.StopAction) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.completed, b.cut = completed, cut
}

func (b *blockingLoop) id() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.loopID
}

// stoppable is a fixture with a pass running and blocked in its first action.
type stoppable struct {
	*fixture
	loop   *blockingLoop
	obj    objective.Objective
	before reconcile.State

	// shutdown cancels the context dispatch was given, as the server does
	// when it stops; it is not a pause.
	shutdown context.CancelFunc
}

// startBlockedPass declares an objective one failure short of the breaker, at
// a rung a failure would demote, and dispatches a pass that blocks in its
// first action. The cleanup cancels the dispatch context and waits for every
// goroutine, so a pass the code under test failed to stop does not outlive the
// test's database.
func startBlockedPass(t *testing.T) *stoppable {
	t.Helper()
	f := newFixture(t, Config{BreakerFailures: 3})
	f.use(t, map[string]string{"git": "aaa"})
	bl := newBlockingLoop(f.store)
	f.svc.loops = bl

	obj := f.declare(t, objective.Objective{
		Cadence: &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{
			Level:           objective.AutonomyActWithNotice,
			Ceiling:         objective.AutonomyAct,
			DemoteOnFailure: true,
		},
	})
	st := f.state(t, obj.ID)
	st.ConsecutiveFailures = 2
	if err := f.store.SaveReconcileState(context.Background(), st); err != nil {
		t.Fatalf("save state: %v", err)
	}
	before := f.state(t, obj.ID)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		waitFor(stopWait, func() bool { return !f.svc.isRunning(obj.ID) })
		select {
		case <-bl.done:
		case <-time.After(stopWait):
		}
	})
	f.svc.dispatch(ctx, obj.ID, reconcile.TriggerManual)

	select {
	case <-bl.started:
	case <-time.After(stopWait):
		t.Fatal("the pass never started its first action")
	}
	return &stoppable{fixture: f, loop: bl, obj: obj, before: before, shutdown: cancel}
}

func (s *stoppable) pause(t *testing.T) {
	t.Helper()
	if err := s.svc.Pause(context.Background(), s.obj.ID, "operator", "operator stopped it"); err != nil {
		t.Fatalf("pause: %v", err)
	}
}

// awaitEnded fails the test when the pass is still running after the bound.
func (s *stoppable) awaitEnded(t *testing.T) {
	t.Helper()
	if !waitFor(stopWait, func() bool { return !s.svc.isRunning(s.obj.ID) }) {
		t.Fatalf("the pass was still running %s after Pause returned", stopWait)
	}
}

func (s *Service) isRunning(id objective.ObjectiveID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running[id]
}

func waitFor(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return cond()
}

// The pass's context is cancelled by the pause, and the pass is recorded as
// ended with its own reason, which is not a failure.
func TestPauseCancelsThePassInFlight(t *testing.T) {
	s := startBlockedPass(t)
	s.pause(t)

	select {
	case <-s.loop.cancelled:
	case <-time.After(stopWait):
		t.Fatalf("the pass's context was not cancelled %s after Pause returned", stopWait)
	}
	s.awaitEnded(t)

	history, err := s.svc.History(context.Background(), s.obj.ID, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	var got *reconcile.Outcome
	for i := range history {
		if history[i].LoopID == s.loop.id() {
			got = &history[i]
		}
	}
	if got == nil {
		t.Fatalf("no outcome recorded for the stopped pass's loop %q; history has %d", s.loop.id(), len(history))
	}
	if got.Deferred != outcomeStopped {
		t.Errorf("stopped pass recorded reason %q, want %q", got.Deferred, outcomeStopped)
	}
	if got.Failed() {
		t.Errorf("stopped pass recorded as a failure: %q", got.Error)
	}
	if got.Converged || got.Escalated || got.Inconclusive {
		t.Errorf("stopped pass recorded converged=%v escalated=%v inconclusive=%v, want none",
			got.Converged, got.Escalated, got.Inconclusive)
	}
	if got.EndedAt.IsZero() {
		t.Error("stopped pass has no end time")
	}
	if st := s.state(t, s.obj.ID); st.LastOutcomeID != got.ID {
		t.Errorf("state's last outcome = %q, want the stopped pass %q", st.LastOutcomeID, got.ID)
	}
}

// The ending pass does not overwrite the pause: the state stays paused with
// the operator's reason, and a manual trigger is still refused.
func TestPauseInFlightLeavesTheObjectivePaused(t *testing.T) {
	s := startBlockedPass(t)
	s.pause(t)
	s.awaitEnded(t)

	st := s.state(t, s.obj.ID)
	if !st.Paused {
		t.Error("the objective is not paused after its pass was stopped")
	}
	if st.Phase != reconcile.PhasePaused {
		t.Errorf("phase = %q after the stopped pass ended, want %q", st.Phase, reconcile.PhasePaused)
	}
	if st.PausedReason != "operator stopped it" {
		t.Errorf("paused reason = %q, want the operator's", st.PausedReason)
	}
	if err := s.svc.Trigger(context.Background(), s.obj.ID); err == nil {
		t.Error("a manual trigger was accepted after the pause stopped the pass")
	}
	if waitFor(50*time.Millisecond, func() bool { return s.loop.runCount() > 1 }) {
		t.Errorf("a loop ran after the pause; %d runs", s.loop.runCount())
	}
}

// A stop is not a failure: the breaker's count and the autonomy level are what
// they were before the pass, and the breaker did not trip.
func TestPauseInFlightIsNotAFailure(t *testing.T) {
	s := startBlockedPass(t)
	s.pause(t)
	s.awaitEnded(t)

	st := s.state(t, s.obj.ID)
	if st.ConsecutiveFailures != s.before.ConsecutiveFailures {
		t.Errorf("consecutive failures = %d after a stop, want %d as before the pass",
			st.ConsecutiveFailures, s.before.ConsecutiveFailures)
	}
	if st.Autonomy != s.before.Autonomy {
		t.Errorf("autonomy = %q after a stop, want %q as before the pass", st.Autonomy, s.before.Autonomy)
	}
	if st.LastError != s.before.LastError {
		t.Errorf("last error = %q after a stop, want %q", st.LastError, s.before.LastError)
	}
	demotions, err := s.store.ListToolEvents(context.Background(), storage.ToolEventFilter{
		ObjectiveID: string(s.obj.ID),
		Kind:        storage.ToolEventDemotion,
	})
	if err != nil {
		t.Fatalf("list demotions: %v", err)
	}
	if len(demotions) != 0 {
		t.Errorf("a stop wrote %d demotion audit rows, want 0", len(demotions))
	}
	// One failure short of the breaker: a stop counted as a failure would
	// have replaced the operator's reason with the breaker's.
	if st.PausedReason != "operator stopped it" {
		t.Errorf("paused reason = %q, want the operator's; the breaker tripped on a stop", st.PausedReason)
	}
}

// No action of the pass starts once Pause has returned. The action in flight
// is released after Pause returns; a pass still holding a live context would
// start its next one.
func TestNoActionStartsAfterPauseReturns(t *testing.T) {
	s := startBlockedPass(t)
	if got := s.loop.actionStarts(); got != 1 {
		t.Fatalf("action starts before the pause = %d, want 1", got)
	}
	s.pause(t)
	s.loop.releaseAction()

	if waitFor(100*time.Millisecond, func() bool { return s.loop.actionStarts() > 1 }) {
		t.Errorf("%d actions started after Pause returned, want 0", s.loop.actionStarts()-1)
	}
	s.awaitEnded(t)
	if got := s.loop.actionStarts(); got != 1 {
		t.Errorf("action starts by the end of the pass = %d, want 1", got)
	}
}

// Pausing with no pass running behaves as it did before Phase 37: the state is
// paused with the reason, nothing else moves, and no outcome is recorded.
func TestPauseWithNoPassRunningBehavesAsBefore(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})
	before := f.state(t, obj.ID)

	if err := f.svc.Pause(context.Background(), obj.ID, "operator", "operator stopped it"); err != nil {
		t.Fatalf("pause: %v", err)
	}

	st := f.state(t, obj.ID)
	if !st.Paused || st.Phase != reconcile.PhasePaused || st.PausedReason != "operator stopped it" {
		t.Errorf("state after pause = paused %v, phase %q, reason %q", st.Paused, st.Phase, st.PausedReason)
	}
	if st.ConsecutiveFailures != before.ConsecutiveFailures || st.Autonomy != before.Autonomy {
		t.Errorf("pause moved counters: failures %d→%d, autonomy %q→%q",
			before.ConsecutiveFailures, st.ConsecutiveFailures, before.Autonomy, st.Autonomy)
	}
	history, err := f.svc.History(context.Background(), obj.ID, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("a pause with no pass running recorded %d outcomes, want 0", len(history))
	}
	if err := f.svc.Trigger(context.Background(), obj.ID); err == nil {
		t.Error("a manual trigger on a paused objective was accepted")
	}
	if f.loops.count() != 0 {
		t.Error("a paused objective was reconciled")
	}
	if err := f.svc.Pause(context.Background(), "no-such-objective", "operator", "x"); err == nil {
		t.Error("pausing an unknown objective succeeded")
	}
}
