package reconcile

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
)

// Phase 37 step 4: stop everything. One StopAll call stops every running loop
// as step 3 does and pauses every standing objective as step 1 does, for the
// runs the caller may stop. The permission is a predicate the API layer
// injects (loop:stop on each loop, objective:pause on each objective), so this
// package holds no authorization code. A loop of a standing objective is
// reached by pausing that objective, and is reported once, as the objective.

// rowStopper is the blocking loop with a Stop that stores the row stopped, as
// the loop service does, so a second StopAll finds nothing left running.
// Each call is still recorded, by the stopper and with the reason.
type rowStopper struct {
	*blockingLoop
}

func (r *rowStopper) Stop(ctx context.Context, loopID, principal, reason string) (featureloop.StopResult, error) {
	_, _ = r.stopRecorder.Stop(ctx, loopID, principal, reason)
	res := featureloop.StopResult{LoopID: loopID}
	st, err := r.store.GetLoopState(ctx, loopID)
	if errors.Is(err, coreerrors.ErrNotFound) {
		res.Found = featureloop.StopFoundUnknown
		return res, nil
	}
	if err != nil {
		return res, err
	}
	if st.Completed {
		res.Found = featureloop.StopFoundEnded
		return res, nil
	}
	st.Completed = true
	st.Status = objective.StatusStopped
	st.Outcome = coreloop.RunStopped
	st.StoppedBy, st.StopReason = principal, reason
	if err := r.store.SaveLoopState(ctx, st); err != nil {
		return res, err
	}
	res.Found = featureloop.StopFoundRunning
	return res, nil
}

// stopAllWorld is the acceptance line's deployment: two running one-shot
// loops, and two standing objectives, obj-1 mid-pass in a blocked action and
// obj-2 idle between passes.
type stopAllWorld struct {
	*stoppable
	stops   *rowStopper
	idle    objective.Objective
	oneShot []string // the one-shot loop IDs, sorted
}

func startStopAllWorld(t *testing.T) *stopAllWorld {
	t.Helper()
	s := startBlockedPass(t)
	rs := &rowStopper{blockingLoop: s.loop}
	s.svc.loops = rs

	idle := s.declare(t, objective.Objective{ID: "obj-2", Cadence: &objective.Cadence{Every: "1h"}})

	ctx := context.Background()
	loops := []string{"loop-oneshot-a", "loop-oneshot-b"}
	for i, loopID := range loops {
		obj := objective.Objective{
			ID:     objective.ObjectiveID("obj-oneshot-" + string(rune('a'+i))),
			Mode:   objective.ModeOneshot,
			Domain: "test",
		}
		if err := s.store.SaveObjective(ctx, obj); err != nil {
			t.Fatalf("save one-shot objective: %v", err)
		}
		if err := s.store.SaveLoopState(ctx, coreloop.State{
			LoopID:      loopID,
			ObjectiveID: obj.ID,
			Status:      objective.StatusActive,
		}); err != nil {
			t.Fatalf("save one-shot loop: %v", err)
		}
	}
	return &stopAllWorld{stoppable: s, stops: rs, idle: idle, oneShot: loops}
}

func allowAll(string, string) bool { return true }

// stopAll calls StopAll as alice with stopLoopReason, failing the test on an
// error.
func (w *stopAllWorld) stopAll(t *testing.T, mayStop func(kind, id string) bool) StopAllResult {
	t.Helper()
	res, err := w.svc.StopAll(context.Background(), "alice", stopLoopReason, mayStop)
	if err != nil {
		t.Fatalf("StopAll: %v", err)
	}
	return res
}

// awaitPassEnded waits, within the acceptance bound, for the blocked pass's
// context to be cancelled, the pass to end and the loop's goroutine to exit.
func (w *stopAllWorld) awaitPassEnded(t *testing.T) {
	t.Helper()
	select {
	case <-w.loop.cancelled:
	case <-time.After(stopLoopBound):
		t.Fatalf("the pass's context was not cancelled %s after StopAll returned", stopLoopBound)
	}
	if !waitFor(stopLoopBound, func() bool { return !w.svc.isRunning(w.obj.ID) }) {
		t.Fatalf("the pass was still running %s after StopAll returned", stopLoopBound)
	}
	select {
	case <-w.loop.done:
	case <-time.After(stopLoopBound):
		t.Fatalf("the pass's loop had not exited %s after StopAll returned", stopLoopBound)
	}
}

func stoppedLoopIDs(res StopAllResult) []string {
	ids := make([]string, 0, len(res.LoopsStopped))
	for _, l := range res.LoopsStopped {
		ids = append(ids, l.LoopID)
	}
	sort.Strings(ids)
	return ids
}

func pausedByID(res StopAllResult) map[objective.ObjectiveID]PausedObjective {
	m := make(map[objective.ObjectiveID]PausedObjective, len(res.ObjectivesPaused))
	for _, p := range res.ObjectivesPaused {
		m[p.ObjectiveID] = p
	}
	return m
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// (a) One call leaves both one-shot loops stopped, by the caller and with the
// reason, and both standing objectives paused with the reason. The pass in
// flight is cancelled and filed stopped within the bound, which is not a
// failure. The pass's loop is covered by its objective: it is not listed as a
// stopped loop as well, and its ending is not an error.
func TestStopAllStopsEveryLoopAndPausesEveryObjective(t *testing.T) {
	w := startStopAllWorld(t)
	passLoop := w.loop.id()
	res := w.stopAll(t, allowAll)
	w.awaitPassEnded(t)

	if got := stoppedLoopIDs(res); !equalStrings(got, w.oneShot) {
		t.Errorf("loops stopped = %v, want exactly the one-shot loops %v (the pass's loop %q is reported as its objective)",
			got, w.oneShot, passLoop)
	}
	for _, l := range res.LoopsStopped {
		if l.Found != featureloop.StopFoundRunning {
			t.Errorf("loop %q reported %q, want %q", l.LoopID, l.Found, featureloop.StopFoundRunning)
		}
	}

	calls := awaitStops(&w.loop.stopRecorder, len(w.oneShot))
	for _, loopID := range w.oneShot {
		byCaller := false
		for _, c := range calls {
			if c.loopID == loopID && c.principal == "alice" && c.reason == stopLoopReason {
				byCaller = true
			}
		}
		if !byCaller {
			t.Errorf("loop %q was not stopped as alice with reason %q; calls: %+v", loopID, stopLoopReason, calls)
		}
	}
	for _, c := range calls {
		if c.loopID != passLoop && c.loopID != w.oneShot[0] && c.loopID != w.oneShot[1] {
			t.Errorf("Stop called for loop %q, which is not running", c.loopID)
		}
	}

	paused := pausedByID(res)
	if len(res.ObjectivesPaused) != 2 || len(paused) != 2 {
		t.Errorf("objectives paused = %+v, want obj-1 and obj-2 once each", res.ObjectivesPaused)
	}
	if p, ok := paused[w.obj.ID]; !ok || !p.PassCancelled {
		t.Errorf("objective %q: paused %v, pass cancelled %v; want paused with its pass cancelled", w.obj.ID, ok, p.PassCancelled)
	}
	if p, ok := paused[w.idle.ID]; !ok || p.PassCancelled {
		t.Errorf("objective %q: paused %v, pass cancelled %v; want paused with no pass to cancel", w.idle.ID, ok, p.PassCancelled)
	}
	if len(res.NotReached) != 0 {
		t.Errorf("not reached = %+v, want none: every run was permitted", res.NotReached)
	}

	assertFiledStopped(t, w.fixture, stoppedOutcome(t, w.svc, w.obj.ID, passLoop), w.obj.ID)
	if st := w.state(t, w.obj.ID); st.ConsecutiveFailures != w.before.ConsecutiveFailures {
		t.Errorf("consecutive failures = %d after StopAll, want %d as before the pass",
			st.ConsecutiveFailures, w.before.ConsecutiveFailures)
	}
	if st := w.state(t, w.idle.ID); !st.Paused || st.PausedReason != stopLoopReason {
		t.Errorf("objective %q after StopAll: paused %v, reason %q; want paused with %q",
			w.idle.ID, st.Paused, st.PausedReason, stopLoopReason)
	}
}

// (b) A second call right after changes nothing and says so: nothing stopped,
// nothing paused, nothing missed, no error, no Stop call, and the reasons the
// first call paused with are kept.
func TestStopAllTwiceChangesNothingTheSecondTime(t *testing.T) {
	w := startStopAllWorld(t)
	w.stopAll(t, allowAll)
	w.awaitPassEnded(t)
	before := len(awaitStops(&w.loop.stopRecorder, len(w.oneShot)))

	res, err := w.svc.StopAll(context.Background(), "bob", "again", allowAll)
	if err != nil {
		t.Fatalf("second StopAll: %v", err)
	}
	if len(res.LoopsStopped) != 0 {
		t.Errorf("second StopAll stopped %+v, want nothing", res.LoopsStopped)
	}
	if len(res.ObjectivesPaused) != 0 {
		t.Errorf("second StopAll paused %+v, want nothing", res.ObjectivesPaused)
	}
	if len(res.NotReached) != 0 {
		t.Errorf("second StopAll did not reach %+v, want nothing: already stopped is not unreached", res.NotReached)
	}
	if got := len(awaitStops(&w.loop.stopRecorder, before)); got != before {
		t.Errorf("second StopAll made %d Stop calls, want 0", got-before)
	}
	for _, id := range []objective.ObjectiveID{w.obj.ID, w.idle.ID} {
		if st := w.state(t, id); !st.Paused || st.PausedReason != stopLoopReason {
			t.Errorf("objective %q after the second StopAll: paused %v, reason %q; want the first call's %q",
				id, st.Paused, st.PausedReason, stopLoopReason)
		}
	}
}

// (c) A caller allowed only obj-2 pauses obj-2 only. It stops no loop, leaves
// obj-1's pass running, and names every other run as not permitted: both
// one-shot loops and obj-1, whose pass's loop is covered by obj-1's entry and
// not listed again. The answer does not claim everything was stopped.
func TestStopAllWithANarrowScopeNamesTheRestNotReached(t *testing.T) {
	w := startStopAllWorld(t)
	onlyIdle := func(kind, id string) bool {
		return kind == StopAllKindObjective && id == string(w.idle.ID)
	}
	res := w.stopAll(t, onlyIdle)

	if len(res.LoopsStopped) != 0 {
		t.Errorf("loops stopped = %+v, want none: no loop was permitted", res.LoopsStopped)
	}
	if len(res.ObjectivesPaused) != 1 || res.ObjectivesPaused[0].ObjectiveID != w.idle.ID {
		t.Errorf("objectives paused = %+v, want only %q", res.ObjectivesPaused, w.idle.ID)
	}
	if st := w.state(t, w.idle.ID); !st.Paused || st.PausedReason != stopLoopReason {
		t.Errorf("objective %q: paused %v, reason %q; want paused with %q", w.idle.ID, st.Paused, st.PausedReason, stopLoopReason)
	}

	if st := w.state(t, w.obj.ID); st.Paused {
		t.Errorf("objective %q was paused (reason %q) though the caller may not pause it", w.obj.ID, st.PausedReason)
	}
	if waitFor(stopSettle, func() bool { return !w.svc.isRunning(w.obj.ID) }) {
		t.Errorf("objective %q's pass ended though the caller may not stop it", w.obj.ID)
	}
	if calls := w.loop.stopCalls(); len(calls) != 0 {
		t.Errorf("Stop calls = %+v, want none", calls)
	}

	want := map[NotReachedRun]bool{
		{Kind: StopAllKindLoop, ID: w.oneShot[0], Reason: StopAllNotPermitted}:          true,
		{Kind: StopAllKindLoop, ID: w.oneShot[1], Reason: StopAllNotPermitted}:          true,
		{Kind: StopAllKindObjective, ID: string(w.obj.ID), Reason: StopAllNotPermitted}: true,
	}
	if len(res.NotReached) != len(want) {
		t.Errorf("not reached = %+v, want exactly %d entries: both one-shot loops and %q", res.NotReached, len(want), w.obj.ID)
	}
	for _, n := range res.NotReached {
		if !want[n] {
			t.Errorf("unexpected not-reached entry %+v", n)
		}
		delete(want, n)
	}
	for n := range want {
		t.Errorf("not reached is missing %+v", n)
	}
	if len(res.NotReached) == 0 {
		t.Error("a narrow caller's answer names nothing as not reached, which claims everything was stopped")
	}
}

// A standing objective whose loop waits at a checkpoint has no pass in flight:
// the pass let go once the plan escalated, and the objective records the loop
// as its ActiveLoopID. Pausing it cancels nothing, so StopAll has to stop that
// loop itself, as step 3 does, or a later approval lets it act on an objective
// that reads paused. It is listed as a stopped loop, and the objective as
// paused with no pass cancelled.
func TestStopAllStopsALoopWaitingAtACheckpoint(t *testing.T) {
	w := startStopAllWorld(t)
	ctx := context.Background()

	waiting := w.declare(t, objective.Objective{ID: "obj-waiting", Cadence: &objective.Cadence{Every: "1h"}})
	const waitingLoop = "loop-at-checkpoint"
	if err := w.store.SaveLoopState(ctx, coreloop.State{
		LoopID:       waitingLoop,
		ObjectiveID:  waiting.ID,
		Status:       objective.StatusActive,
		Paused:       true,
		CheckpointID: "cp-waiting",
	}); err != nil {
		t.Fatalf("save waiting loop: %v", err)
	}
	st := w.state(t, waiting.ID)
	st.ActiveLoopID = waitingLoop
	st.Phase = reconcile.PhaseWaiting
	if err := w.store.SaveReconcileState(ctx, st); err != nil {
		t.Fatalf("save waiting state: %v", err)
	}

	res := w.stopAll(t, allowAll)
	w.awaitPassEnded(t)

	want := append([]string{waitingLoop}, w.oneShot...)
	sort.Strings(want)
	if got := stoppedLoopIDs(res); !equalStrings(got, want) {
		t.Errorf("loops stopped = %v, want %v: the loop waiting at a checkpoint must be stopped", got, want)
	}
	if p, ok := pausedByID(res)[waiting.ID]; !ok || p.PassCancelled {
		t.Errorf("objective %q: paused %v, pass cancelled %v; want paused with no pass to cancel", waiting.ID, ok, p.PassCancelled)
	}
	ls, err := w.store.GetLoopState(ctx, waitingLoop)
	if err != nil {
		t.Fatalf("read waiting loop: %v", err)
	}
	if !ls.Completed || ls.Status != objective.StatusStopped || ls.StoppedBy != "alice" || ls.StopReason != stopLoopReason {
		t.Errorf("waiting loop after StopAll: completed %v, status %q, stopped by %q, reason %q; want stopped by alice with %q",
			ls.Completed, ls.Status, ls.StoppedBy, ls.StopReason, stopLoopReason)
	}
}

// (d) Objectives stay paused afterwards: past both cadences, a tick starts no
// pass and records no outcome, and a manual trigger is refused for each.
func TestStopAllLeavesObjectivesPaused(t *testing.T) {
	w := startStopAllWorld(t)
	w.stopAll(t, allowAll)
	w.awaitPassEnded(t)

	ctx := context.Background()
	ids := []objective.ObjectiveID{w.obj.ID, w.idle.ID}
	histories := map[objective.ObjectiveID]int{}
	for _, id := range ids {
		h, err := w.svc.History(ctx, id, 10)
		if err != nil {
			t.Fatalf("history of %q: %v", id, err)
		}
		histories[id] = len(h)
	}

	w.clock = w.clock.Add(2 * time.Hour)
	w.svc.Tick(ctx)
	if waitFor(stopSettle, func() bool {
		return w.loop.runCount() > 1 || w.svc.isRunning(w.obj.ID) || w.svc.isRunning(w.idle.ID)
	}) {
		t.Errorf("a tick after StopAll started a pass: %d loop runs", w.loop.runCount())
	}
	waitFor(stopWait, func() bool { return !w.svc.isRunning(w.obj.ID) && !w.svc.isRunning(w.idle.ID) })

	for _, id := range ids {
		h, err := w.svc.History(ctx, id, 10)
		if err != nil {
			t.Fatalf("history of %q: %v", id, err)
		}
		if len(h) != histories[id] {
			t.Errorf("a tick after StopAll recorded %d new outcomes for %q, want 0", len(h)-histories[id], id)
		}
		if st := w.state(t, id); !st.Paused || st.PausedReason != stopLoopReason {
			t.Errorf("objective %q after the tick: paused %v, reason %q; want paused with %q",
				id, st.Paused, st.PausedReason, stopLoopReason)
		}
		if err := w.svc.Trigger(ctx, id); err == nil {
			t.Errorf("a manual trigger of %q was accepted after StopAll paused it", id)
		}
	}
	if waitFor(stopSettle, func() bool { return w.loop.runCount() > 1 }) {
		t.Errorf("a loop ran after StopAll; %d runs", w.loop.runCount())
	}
}
