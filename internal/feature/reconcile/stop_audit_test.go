package reconcile

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 37 step 5, slice 1: every pause and resume leaves one audit row of
// kind storage.ToolEventStop. Its payload names the control, the principal,
// the reason and the objective; ToolEvent.ObjectiveID is the objective and
// ToolEvent.Approver the principal. A pause that cancels a pass also says so
// and names the pass's loop, and the internal loop stop reconcile issues for
// that pass, as principal "reconcile", leaves no row of its own.

// stopEvent is one stop row with its payload decoded.
type stopEvent struct {
	row     storage.ToolEvent
	payload map[string]any
}

// stopEvents lists every stop row in the store, oldest first, across all
// objectives, so a row written under the wrong objective is still seen.
func stopEvents(t *testing.T, store storage.StorageAdapter) []stopEvent {
	t.Helper()
	rows, err := store.ListToolEvents(context.Background(), storage.ToolEventFilter{
		Kind:        storage.ToolEventStop,
		OldestFirst: true,
	})
	if err != nil {
		t.Fatalf("list stop events: %v", err)
	}
	out := make([]stopEvent, 0, len(rows))
	for _, r := range rows {
		ev := stopEvent{row: r, payload: map[string]any{}}
		if err := json.Unmarshal([]byte(r.PayloadJSON), &ev.payload); err != nil {
			t.Fatalf("stop event %q has a payload that is not JSON: %q: %v", r.ID, r.PayloadJSON, err)
		}
		out = append(out, ev)
	}
	return out
}

// eventsWithControl keeps the events whose payload control is control.
func eventsWithControl(evs []stopEvent, control string) []stopEvent {
	var out []stopEvent
	for _, ev := range evs {
		if ev.payload["control"] == control {
			out = append(out, ev)
		}
	}
	return out
}

// onlyStopEvent fails unless evs holds exactly one event, and returns it.
func onlyStopEvent(t *testing.T, evs []stopEvent, what string) stopEvent {
	t.Helper()
	if len(evs) != 1 {
		t.Fatalf("%s left %d stop events, want exactly 1: %+v", what, len(evs), evs)
	}
	return evs[0]
}

// assertStopEvent checks the row and payload fields every stop event carries.
func assertStopEvent(t *testing.T, ev stopEvent, control, principal, reason string, objID objective.ObjectiveID) {
	t.Helper()
	if ev.row.Kind != storage.ToolEventStop {
		t.Errorf("kind = %q, want %q", ev.row.Kind, storage.ToolEventStop)
	}
	if ev.row.ObjectiveID != string(objID) {
		t.Errorf("row objective = %q, want %q", ev.row.ObjectiveID, objID)
	}
	if ev.row.Approver != principal {
		t.Errorf("row approver = %q, want the principal %q", ev.row.Approver, principal)
	}
	if got := ev.payload["control"]; got != control {
		t.Errorf("payload control = %v, want %q", got, control)
	}
	if got := ev.payload["principal"]; got != principal {
		t.Errorf("payload principal = %v, want %q", got, principal)
	}
	if reason != "" {
		if got := ev.payload["reason"]; got != reason {
			t.Errorf("payload reason = %v, want %q", got, reason)
		}
	}
	if got := ev.payload["objective_id"]; got != string(objID) {
		t.Errorf("payload objective_id = %v, want %q", got, objID)
	}
}

// declareIdle declares a standing objective with no pass running.
func declareIdle(t *testing.T) (*fixture, objective.Objective) {
	t.Helper()
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})
	return f, obj
}

// (a) A pause of an idle standing objective leaves one event with the
// principal, the reason and the objective, and no pass cancelled.
func TestPauseOfAnIdleObjectiveLeavesOneStopEvent(t *testing.T) {
	f, obj := declareIdle(t)

	if err := f.svc.Pause(context.Background(), obj.ID, "alice", "wrong branch"); err != nil {
		t.Fatalf("pause: %v", err)
	}

	ev := onlyStopEvent(t, stopEvents(t, f.store), "a pause of an idle objective")
	assertStopEvent(t, ev, "pause", "alice", "wrong branch", obj.ID)
	if got, ok := ev.payload["pass_cancelled"]; !ok || got != false {
		t.Errorf("payload pass_cancelled = %v (present %v), want false", got, ok)
	}
	if got, ok := ev.payload["loop_id"]; ok && got != "" {
		t.Errorf("payload loop_id = %v, want none for an idle objective", got)
	}
	if st := f.state(t, obj.ID); !st.Paused {
		t.Error("the objective is not paused after the pause that was recorded")
	}
}

// A pause that fails changes nothing, so it records nothing.
func TestPauseOfAnUnknownObjectiveLeavesNoStopEvent(t *testing.T) {
	f, _ := declareIdle(t)

	if err := f.svc.Pause(context.Background(), "no-such-objective", "alice", "x"); err == nil {
		t.Fatal("pausing an unknown objective succeeded")
	}
	if evs := stopEvents(t, f.store); len(evs) != 0 {
		t.Errorf("a failed pause left %d stop events, want 0: %+v", len(evs), evs)
	}
}

// (b) A pause that cancels a pass in flight leaves one event saying so, with
// the pass's loop, and the loop stop reconcile issues for that pass as
// principal "reconcile" leaves none of its own.
func TestPauseInFlightLeavesOneStopEventNamingTheLoop(t *testing.T) {
	s := startBlockedPass(t)
	loopID := s.loop.id()
	if loopID == "" {
		t.Fatal("the blocked pass has no loop ID")
	}

	if err := s.svc.Pause(context.Background(), s.obj.ID, "alice", "operator stopped it"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	s.awaitEnded(t)
	awaitStops(&s.loop.stopRecorder, 1)
	// The row may be written once the pass has ended and its loop ID is
	// settled; wait for it, then a little longer for a second one.
	waitFor(stopWait, func() bool { return len(stopEvents(t, s.store)) >= 1 })
	waitFor(stopSettle, func() bool { return len(stopEvents(t, s.store)) > 1 })

	evs := stopEvents(t, s.store)
	for _, ev := range evs {
		if ev.row.Approver == "reconcile" || ev.payload["principal"] == "reconcile" {
			t.Errorf("the internal loop stop left its own event: %+v", ev)
		}
	}
	ev := onlyStopEvent(t, evs, "a pause that cancelled a pass")
	assertStopEvent(t, ev, "pause", "alice", "operator stopped it", s.obj.ID)
	if got := ev.payload["pass_cancelled"]; got != true {
		t.Errorf("payload pass_cancelled = %v, want true", got)
	}
	if got := ev.payload["loop_id"]; got != loopID {
		t.Errorf("payload loop_id = %v, want the cancelled pass's loop %q", got, loopID)
	}
}

// (c) A resume leaves one event with the principal and the objective, beside
// the pause's own.
func TestResumeLeavesOneStopEvent(t *testing.T) {
	f, obj := declareIdle(t)
	if err := f.svc.Pause(context.Background(), obj.ID, "alice", "wrong branch"); err != nil {
		t.Fatalf("pause: %v", err)
	}

	if err := f.svc.Resume(context.Background(), obj.ID, "bob"); err != nil {
		t.Fatalf("resume: %v", err)
	}

	evs := stopEvents(t, f.store)
	if len(evs) != 2 {
		t.Fatalf("a pause then a resume left %d stop events, want 2: %+v", len(evs), evs)
	}
	onlyStopEvent(t, eventsWithControl(evs, "pause"), "the pause")
	ev := onlyStopEvent(t, eventsWithControl(evs, "resume"), "the resume")
	assertStopEvent(t, ev, "resume", "bob", "", obj.ID)
	if st := f.state(t, obj.ID); st.Paused {
		t.Error("the objective is still paused after the resume that was recorded")
	}
}

// A resume that fails changes nothing, so it records nothing.
//
// Resume does not refuse an objective that is not paused today; it resumes it
// all the same. That case is left to the implementation and not specified
// here; only a resume that fails is.
func TestResumeOfAnUnknownObjectiveLeavesNoStopEvent(t *testing.T) {
	f, _ := declareIdle(t)

	if err := f.svc.Resume(context.Background(), "no-such-objective", "bob"); err == nil {
		t.Fatal("resuming an unknown objective succeeded")
	}
	if evs := stopEvents(t, f.store); len(evs) != 0 {
		t.Errorf("a failed resume left %d stop events, want 0: %+v", len(evs), evs)
	}
}
