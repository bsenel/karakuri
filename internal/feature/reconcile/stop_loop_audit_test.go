package reconcile

import (
	"context"
	"testing"

	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
)

// Phase 37 step 5, slice 2: a loop stop leaves one audit row of kind
// storage.ToolEventStop, as a pause does, and the row says what the run had
// done: the actions it completed, the one the stop cut, whether that one is
// known to have ended, and what it may have left half done.
//
// The loop service knows what a loop ran, so its StopResult carries the
// completed and cut actions; reconcile writes every stop row through
// recordStop, so the audit write stays in one place. The fakes here answer
// Stop with the actions a loop service would, and the loop package's own
// tests hold the real service to filling them in.
//
// The payload names the actions under "completed_actions" (a list) and
// "cut_action" (an object, absent when no action was in flight), each with
// "capability" and, where it has them, "env_id", "workdir" and "branch"; the
// cut one has "ended", which is "ended" or "unknown".

// scriptedStopper is a loop runner whose Stop answers with a set result. It
// runs nothing; these tests stop loops whose rows they wrote themselves.
type scriptedStopper struct {
	stopRecorder
	answer featureloop.StopResult
}

func (s *scriptedStopper) Run(context.Context, coreloop.Request) (coreloop.Result, error) {
	return coreloop.Result{}, context.Canceled
}

func (s *scriptedStopper) Stop(ctx context.Context, loopID, principal, reason string) (featureloop.StopResult, error) {
	_, _ = s.stopRecorder.Stop(ctx, loopID, principal, reason)
	res := s.answer
	res.LoopID = loopID
	return res, nil
}

// The actions every test here has the stopped loop report: one read that
// completed, and a delegated coding run in flight in its own worktree.
var (
	completedRead = featureloop.StopAction{CapabilityID: "read_file", EnvID: "software"}
	cutDelegation = featureloop.StopAction{
		CapabilityID: "delegate_to_cli",
		EnvID:        "software",
		Ended:        featureloop.StopActionEndedUnknown,
		Workdir:      "/tmp/karakuri/worktrees/obj-oneshot-1",
		Branch:       "karakuri/obj-oneshot-1",
	}
)

// oneShotLoop stores a one-shot objective with an active loop and has the
// loop service answer Stop with answer.
func oneShotLoop(t *testing.T, answer featureloop.StopResult) (*fixture, *scriptedStopper, objective.Objective, string) {
	t.Helper()
	f := newFixture(t, Config{})
	ctx := context.Background()
	obj := objective.Objective{ID: "obj-oneshot", Mode: objective.ModeOneshot, Domain: "test"}
	if err := f.store.SaveObjective(ctx, obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}
	const loopID = "loop-oneshot"
	if err := f.store.SaveLoopState(ctx, coreloop.State{
		LoopID:      loopID,
		ObjectiveID: obj.ID,
		Status:      objective.StatusActive,
	}); err != nil {
		t.Fatalf("save loop state: %v", err)
	}
	stopper := &scriptedStopper{answer: answer}
	f.svc.loops = stopper
	return f, stopper, obj, loopID
}

// payloadAction reads one action object of a stop row's payload.
func payloadAction(t *testing.T, v any, what string) map[string]any {
	t.Helper()
	a, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want an object naming the action", what, v)
	}
	return a
}

// assertCompleted checks the payload names exactly the completed actions.
func assertCompleted(t *testing.T, ev stopEvent, want ...featureloop.StopAction) {
	t.Helper()
	raw, ok := ev.payload["completed_actions"]
	if !ok {
		t.Fatalf("payload has no completed_actions: %v", ev.payload)
	}
	list, ok := raw.([]any)
	if !ok {
		t.Fatalf("payload completed_actions = %#v, want a list", raw)
	}
	if len(list) != len(want) {
		t.Fatalf("payload completed_actions has %d actions, want %d: %v", len(list), len(want), list)
	}
	for i, w := range want {
		a := payloadAction(t, list[i], "a completed action")
		if a["capability"] != w.CapabilityID {
			t.Errorf("completed action %d capability = %v, want %q", i, a["capability"], w.CapabilityID)
		}
		if w.EnvID != "" && a["env_id"] != w.EnvID {
			t.Errorf("completed action %d env_id = %v, want %q", i, a["env_id"], w.EnvID)
		}
	}
}

// assertCut checks the payload names the cut action, says it is not known
// to have ended, and names the workdir and branch it may have left half done.
func assertCut(t *testing.T, ev stopEvent, want featureloop.StopAction) {
	t.Helper()
	raw, ok := ev.payload["cut_action"]
	if !ok {
		t.Fatalf("payload has no cut_action: %v", ev.payload)
	}
	a := payloadAction(t, raw, "payload cut_action")
	if a["capability"] != want.CapabilityID {
		t.Errorf("cut action capability = %v, want %q", a["capability"], want.CapabilityID)
	}
	if a["ended"] != string(featureloop.StopActionEndedUnknown) {
		t.Errorf("cut action ended = %v, want %q: a stop does not wait for the action, so it cannot know it ended",
			a["ended"], featureloop.StopActionEndedUnknown)
	}
	if want.Workdir != "" && a["workdir"] != want.Workdir {
		t.Errorf("cut action workdir = %v, want %q, what it may have left half done", a["workdir"], want.Workdir)
	}
	if want.Branch != "" && a["branch"] != want.Branch {
		t.Errorf("cut action branch = %v, want %q, what it may have left half done", a["branch"], want.Branch)
	}
}

// (a) StopLoop of a running one-shot loop leaves one loop_stop row with the
// stopper, the reason, the loop and the objective.
func TestStopLoopOfAOneShotLoopLeavesOneStopEvent(t *testing.T) {
	f, _, obj, loopID := oneShotLoop(t, featureloop.StopResult{
		Found:     featureloop.StopFoundRunning,
		Completed: []featureloop.StopAction{completedRead},
		Cut:       &cutDelegation,
	})

	if _, err := f.svc.StopLoop(context.Background(), loopID, "alice", "wrong branch"); err != nil {
		t.Fatalf("StopLoop: %v", err)
	}

	ev := onlyStopEvent(t, stopEvents(t, f.store), "StopLoop of a one-shot loop")
	assertStopEvent(t, ev, "loop_stop", "alice", "wrong branch", obj.ID)
	if ev.row.EscalationReason != "wrong branch" {
		t.Errorf("row escalation reason = %q, want the stop's reason %q", ev.row.EscalationReason, "wrong branch")
	}
	if got := ev.payload["loop_id"]; got != loopID {
		t.Errorf("payload loop_id = %v, want %q", got, loopID)
	}
}

// (b) The row names the actions the loop completed and the one the stop cut,
// with whether it is known to have ended and its workdir and branch.
func TestStopLoopEventNamesTheCompletedAndCutActions(t *testing.T) {
	f, _, _, loopID := oneShotLoop(t, featureloop.StopResult{
		Found:     featureloop.StopFoundRunning,
		Completed: []featureloop.StopAction{completedRead},
		Cut:       &cutDelegation,
	})

	if _, err := f.svc.StopLoop(context.Background(), loopID, "alice", "wrong branch"); err != nil {
		t.Fatalf("StopLoop: %v", err)
	}

	ev := onlyStopEvent(t, stopEvents(t, f.store), "StopLoop of a loop with an action in flight")
	assertCompleted(t, ev, completedRead)
	assertCut(t, ev, cutDelegation)
}

// (b) A cut action the loop service does not say ended is recorded as
// "unknown", never as ended: the row must not claim more than is known.
func TestStopLoopEventSaysUnknownWhenTheCutActionsEndIsNotKnown(t *testing.T) {
	notSaid := cutDelegation
	notSaid.Ended = ""
	f, _, _, loopID := oneShotLoop(t, featureloop.StopResult{
		Found: featureloop.StopFoundRunning,
		Cut:   &notSaid,
	})

	if _, err := f.svc.StopLoop(context.Background(), loopID, "alice", "wrong branch"); err != nil {
		t.Fatalf("StopLoop: %v", err)
	}

	ev := onlyStopEvent(t, stopEvents(t, f.store), "StopLoop of a loop with an action in flight")
	assertCut(t, ev, notSaid)
	assertCompleted(t, ev)
}

// (b) A loop stopped between actions, at a checkpoint say, cut nothing: the
// row lists what completed and names no cut action.
func TestStopLoopEventNamesNoCutActionWhenNoneWasInFlight(t *testing.T) {
	f, _, _, loopID := oneShotLoop(t, featureloop.StopResult{
		Found:     featureloop.StopFoundRunning,
		Completed: []featureloop.StopAction{completedRead},
	})

	if _, err := f.svc.StopLoop(context.Background(), loopID, "alice", "wrong branch"); err != nil {
		t.Fatalf("StopLoop: %v", err)
	}

	ev := onlyStopEvent(t, stopEvents(t, f.store), "StopLoop of a loop between actions")
	assertCompleted(t, ev, completedRead)
	if got, ok := ev.payload["cut_action"]; ok && got != nil {
		t.Errorf("payload cut_action = %v, want none: no action was in flight", got)
	}
}

// (c) Stopping a loop that has already ended changes nothing, so it records
// nothing; nor does stopping a loop nobody knows.
func TestStopLoopOfAnEndedOrUnknownLoopLeavesNoStopEvent(t *testing.T) {
	for _, found := range []featureloop.StopFound{featureloop.StopFoundEnded, featureloop.StopFoundUnknown} {
		t.Run(string(found), func(t *testing.T) {
			f, _, obj, loopID := oneShotLoop(t, featureloop.StopResult{Found: found})

			res, err := f.svc.StopLoop(context.Background(), loopID, "alice", "wrong branch")
			if err != nil {
				t.Fatalf("StopLoop: %v", err)
			}
			if res.Found != found {
				t.Errorf("result found = %q, want %q", res.Found, found)
			}
			if evs := stopEvents(t, f.store); len(evs) != 0 {
				t.Errorf("StopLoop of a loop found %s left %d stop events, want 0: %+v", found, len(evs), evs)
			}
			if res.ObjectivePaused {
				t.Errorf("StopLoop of a loop found %s paused objective %q", found, obj.ID)
			}
		})
	}
}

// (d) StopLoop of a watch-mode loop pauses its objective and stops the loop:
// one row for the pause, one for the loop stop, and none for the internal
// stop reconcile makes of the cancelled pass's loop.
func TestStopLoopOfAWatchLeavesOnePauseAndOneLoopStopEvent(t *testing.T) {
	s := startBlockedPass(t)
	loopID := s.loop.id()
	stopLoopOf(t, s)
	s.awaitEnded(t)
	awaitStops(&s.loop.stopRecorder, 2)
	waitFor(stopWait, func() bool { return len(stopEvents(t, s.store)) >= 2 })
	waitFor(stopSettle, func() bool { return len(stopEvents(t, s.store)) > 2 })

	evs := stopEvents(t, s.store)
	for _, ev := range evs {
		if ev.row.Approver == stopPrincipal || ev.payload["principal"] == stopPrincipal {
			t.Errorf("the internal loop stop left its own event: %+v", ev)
		}
	}
	if len(evs) != 2 {
		t.Fatalf("StopLoop of a watch left %d stop events, want 2 (the pause and the loop stop): %+v", len(evs), evs)
	}
	pause := onlyStopEvent(t, eventsWithControl(evs, "pause"), "the pause StopLoop made")
	assertStopEvent(t, pause, "pause", "alice", stopLoopReason, s.obj.ID)
	if got := pause.payload["loop_id"]; got != loopID {
		t.Errorf("pause payload loop_id = %v, want the cancelled pass's loop %q", got, loopID)
	}
	stop := onlyStopEvent(t, eventsWithControl(evs, "loop_stop"), "the loop stop")
	assertStopEvent(t, stop, "loop_stop", "alice", stopLoopReason, s.obj.ID)
	if got := stop.payload["loop_id"]; got != loopID {
		t.Errorf("loop stop payload loop_id = %v, want %q", got, loopID)
	}
}

// (e) A pause that cancels a pass leaves one row, not a second for the
// internal "reconcile" stop of the pass's loop, and that row names the loop's
// completed and cut actions, from what the internal stop was told.
func TestPauseInFlightEventNamesTheCancelledLoopsActions(t *testing.T) {
	s := startBlockedPass(t)
	s.loop.setStopActions([]featureloop.StopAction{completedRead}, &cutDelegation)

	if err := s.svc.Pause(context.Background(), s.obj.ID, "alice", "operator stopped it"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	s.awaitEnded(t)
	awaitStops(&s.loop.stopRecorder, 1)
	waitFor(stopWait, func() bool { return len(stopEvents(t, s.store)) >= 1 })
	waitFor(stopSettle, func() bool { return len(stopEvents(t, s.store)) > 1 })

	ev := onlyStopEvent(t, stopEvents(t, s.store), "a pause that cancelled a pass")
	assertStopEvent(t, ev, "pause", "alice", "operator stopped it", s.obj.ID)
	assertCompleted(t, ev, completedRead)
	assertCut(t, ev, cutDelegation)
}
