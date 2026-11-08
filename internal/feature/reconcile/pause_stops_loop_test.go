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
)

// Phase 37 step 3: a pause that cancels a pass also stops the pass's loop.
// Cancelling the pass's context is not enough on its own, because the loop
// service runs each loop on a context only its own Stop cancels.

// stopCall is one call a fake loop runner's Stop received.
type stopCall struct {
	loopID, principal, reason string
}

// stopRecorder records the Stop calls of a fake LoopRunner. Embedded in every
// fake so each one satisfies the interface the same way.
type stopRecorder struct {
	stopMu sync.Mutex
	stops  []stopCall
}

func (r *stopRecorder) Stop(_ context.Context, loopID, principal, reason string) (featureloop.StopResult, error) {
	r.stopMu.Lock()
	defer r.stopMu.Unlock()
	r.stops = append(r.stops, stopCall{loopID: loopID, principal: principal, reason: reason})
	return featureloop.StopResult{LoopID: loopID, Found: featureloop.StopFoundRunning}, nil
}

func (r *stopRecorder) stopCalls() []stopCall {
	r.stopMu.Lock()
	defer r.stopMu.Unlock()
	return append([]stopCall(nil), r.stops...)
}

// stopSettle is how long a test watches for a Stop call that should not come,
// or for a second one after the first.
const stopSettle = 100 * time.Millisecond

// awaitStops waits until at least n Stop calls are recorded, then a little
// longer so a duplicate has a chance to show up.
func awaitStops(r *stopRecorder, n int) []stopCall {
	waitFor(stopWait, func() bool { return len(r.stopCalls()) >= n })
	waitFor(stopSettle, func() bool { return len(r.stopCalls()) > n })
	return r.stopCalls()
}

// stoppedOutcome returns the outcome recorded for the pass whose loop is
// loopID, failing the test when there is none.
func stoppedOutcome(t *testing.T, svc *Service, objID objective.ObjectiveID, loopID string) reconcile.Outcome {
	t.Helper()
	history, err := svc.History(context.Background(), objID, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	for _, o := range history {
		if o.LoopID == loopID {
			return o
		}
	}
	t.Fatalf("no outcome recorded for the stopped pass's loop %q; history has %d", loopID, len(history))
	return reconcile.Outcome{}
}

// assertFiledStopped checks the pass was filed as step 1 files a stop: not a
// failure, and the objective left paused with the operator's reason.
func assertFiledStopped(t *testing.T, f *fixture, got reconcile.Outcome, objID objective.ObjectiveID) {
	t.Helper()
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
	st := f.state(t, objID)
	if st.LastOutcomeID != got.ID {
		t.Errorf("state's last outcome = %q, want the stopped pass %q", st.LastOutcomeID, got.ID)
	}
	if !st.Paused || st.Phase != reconcile.PhasePaused || st.PausedReason != "operator stopped it" {
		t.Errorf("state after the stopped pass = paused %v, phase %q, reason %q; want paused with the operator's reason",
			st.Paused, st.Phase, st.PausedReason)
	}
}

// A pause of a pass whose loop is running stops that loop, once, with the
// pass's loop ID and the pause's reason; the pass is filed as stopped all the
// same.
func TestPauseStopsTheLoopOfThePassItCancels(t *testing.T) {
	s := startBlockedPass(t)
	loopID := s.loop.id()
	if loopID == "" {
		t.Fatal("the blocked pass has no loop ID")
	}
	s.pause(t)
	s.awaitEnded(t)

	calls := awaitStops(&s.loop.stopRecorder, 1)
	if len(calls) != 1 {
		t.Fatalf("Stop called %d times after the pause, want 1: %+v", len(calls), calls)
	}
	if calls[0].loopID != loopID {
		t.Errorf("Stop called with loop %q, want the pass's loop %q", calls[0].loopID, loopID)
	}
	if calls[0].reason != "operator stopped it" {
		t.Errorf("Stop called with reason %q, want the pause's %q", calls[0].reason, "operator stopped it")
	}

	assertFiledStopped(t, s.fixture, stoppedOutcome(t, s.svc, s.obj.ID, loopID), s.obj.ID)
}

// A pass cancelled by shutdown leaves its loop alone: the loop service's own
// shutdown and restart handle it, and a stop would end a loop the restart is
// meant to resume.
func TestShutdownDoesNotStopThePassLoop(t *testing.T) {
	s := startBlockedPass(t)
	s.shutdown()

	select {
	case <-s.loop.cancelled:
	case <-time.After(stopWait):
		t.Fatalf("the pass's context was not cancelled %s after shutdown", stopWait)
	}
	s.awaitEnded(t)

	if calls := awaitStops(&s.loop.stopRecorder, 0); len(calls) != 0 {
		t.Errorf("shutdown called Stop %d times, want 0: %+v", len(calls), calls)
	}
}

// gatedLoop is a loop runner whose Run does not return until the context the
// pass handed it is cancelled, so a pause lands while Run is still running.
// With an ID it then returns one, as the real Run does: the loop service
// starts the loop on its own context and does not look at the caller's.
// Without one it returns the context's error and no loop.
type gatedLoop struct {
	stopRecorder

	fixture *fixture
	withID  bool

	mu      sync.Mutex
	loopID  string
	entered chan struct{}
}

func (g *gatedLoop) Run(ctx context.Context, req coreloop.Request) (coreloop.Result, error) {
	close(g.entered)
	<-ctx.Done()
	if !g.withID {
		return coreloop.Result{}, ctx.Err()
	}
	id := newOutcomeID()
	if err := g.fixture.store.SaveLoopState(context.WithoutCancel(ctx), coreloop.State{
		LoopID:      id,
		ObjectiveID: req.Objective.ID,
		Status:      objective.StatusActive,
	}); err != nil {
		return coreloop.Result{}, err
	}
	g.mu.Lock()
	g.loopID = id
	g.mu.Unlock()
	return coreloop.Result{LoopID: id, ObjectiveID: req.Objective.ID}, nil
}

func (g *gatedLoop) id() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.loopID
}

// pauseDuringRun dispatches a pass, pauses it while its Run has not returned,
// and waits for the pass to end.
func pauseDuringRun(t *testing.T, withID bool) (*fixture, *gatedLoop, objective.Objective) {
	t.Helper()
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	gl := &gatedLoop{fixture: f, withID: withID, entered: make(chan struct{})}
	f.svc.loops = gl

	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		waitFor(stopWait, func() bool { return !f.svc.isRunning(obj.ID) })
	})
	f.svc.dispatch(ctx, obj.ID, reconcile.TriggerManual)

	select {
	case <-gl.entered:
	case <-time.After(stopWait):
		t.Fatal("the pass never called Run")
	}
	if err := f.svc.Pause(context.Background(), obj.ID, "operator stopped it"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if !waitFor(stopWait, func() bool { return !f.svc.isRunning(obj.ID) }) {
		t.Fatalf("the pass was still running %s after Pause returned", stopWait)
	}
	return f, gl, obj
}

// A pause that lands while Run is still running cannot know the loop's ID yet.
// When Run then returns one, the loop it started is stopped all the same.
func TestPauseBeforeRunReturnsStopsTheLoopRunStarted(t *testing.T) {
	f, gl, obj := pauseDuringRun(t, true)
	loopID := gl.id()
	if loopID == "" {
		t.Fatal("Run returned no loop ID")
	}

	calls := awaitStops(&gl.stopRecorder, 1)
	if len(calls) != 1 {
		t.Fatalf("Stop called %d times, want 1 for the loop Run started: %+v", len(calls), calls)
	}
	if calls[0].loopID != loopID {
		t.Errorf("Stop called with loop %q, want the loop Run returned %q", calls[0].loopID, loopID)
	}
	if calls[0].reason != "operator stopped it" {
		t.Errorf("Stop called with reason %q, want the pause's %q", calls[0].reason, "operator stopped it")
	}

	assertFiledStopped(t, f, stoppedOutcome(t, f.svc, obj.ID, loopID), obj.ID)
}

// When Run returns no loop, there is nothing to stop: Stop is never called
// with an empty loop ID.
func TestPauseBeforeRunReturnsNoLoopCallsNoStop(t *testing.T) {
	f, gl, obj := pauseDuringRun(t, false)

	for _, c := range awaitStops(&gl.stopRecorder, 0) {
		if c.loopID == "" {
			t.Errorf("Stop called with an empty loop ID: %+v", c)
		}
	}
	assertFiledStopped(t, f, stoppedOutcome(t, f.svc, obj.ID, ""), obj.ID)
}
