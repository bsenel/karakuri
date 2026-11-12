package reconcile

import (
	"context"
	"sort"
	"testing"

	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 37 step 5, slice 2: "A stop-all leaves one event for the call and one
// per run it touched."
//
// The call's row is a storage.ToolEventStop with control "stop_all", the
// caller's principal and reason, no objective, and what the call did, in the
// shape StopAllResult has on the wire: "loops_stopped" (objects with
// "loop_id"), "objectives_paused" (objects with "objective_id" and
// "pass_cancelled") and "not_reached" (objects with "kind", "id" and
// "reason"). Each list is present, and empty when nothing was in it.
//
// Every run the call touched leaves its own row, as a pause or a loop stop
// leaves one on its own: a "pause" row per objective paused and a "loop_stop"
// row per loop stopped. A loop of a standing objective is touched through its
// objective, so it leaves no loop_stop row of its own. Each of those rows has
// the caller's principal and reason and "stop_all_id", the ID of the call's
// row, so a reader can find every run one stop-all touched.

// stopAllEvents waits for want stop rows, then a moment more for any extra,
// and returns them all: the mid-pass objective's pause row is written by the
// pass's goroutine after StopAll has returned. The polls count rows without
// failing on a busy database, since a pass still running writes beside them.
func stopAllEvents(t *testing.T, w *stopAllWorld, want int) []stopEvent {
	t.Helper()
	count := func() int {
		rows, err := w.store.ListToolEvents(context.Background(), storage.ToolEventFilter{Kind: storage.ToolEventStop})
		if err != nil {
			return -1
		}
		return len(rows)
	}
	waitFor(stopWait, func() bool { return count() >= want })
	waitFor(stopSettle, func() bool { return count() > want })
	var evs []stopEvent
	waitFor(stopWait, func() bool {
		if count() < 0 {
			return false
		}
		evs = stopEvents(t, w.store)
		return true
	})
	if evs == nil {
		evs = stopEvents(t, w.store)
	}
	return evs
}

// payloadList reads a list of objects from a stop row's payload, failing when
// the key is missing or is not a list.
func payloadList(t *testing.T, ev stopEvent, key string) []map[string]any {
	t.Helper()
	raw, ok := ev.payload[key]
	if !ok {
		t.Fatalf("stop_all payload has no %s: %v", key, ev.payload)
	}
	list, ok := raw.([]any)
	if !ok {
		t.Fatalf("stop_all payload %s = %#v, want a list", key, raw)
	}
	out := make([]map[string]any, 0, len(list))
	for _, v := range list {
		out = append(out, payloadAction(t, v, "an entry of "+key))
	}
	return out
}

// payloadIDs reads field from each object of the payload list key, sorted.
func payloadIDs(t *testing.T, ev stopEvent, key, field string) []string {
	t.Helper()
	var ids []string
	for _, e := range payloadList(t, ev, key) {
		id, _ := e[field].(string)
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// assertCallEvent checks the call row's own fields: control stop_all, the
// caller and the reason, and no objective, because the call is not one run's.
func assertCallEvent(t *testing.T, ev stopEvent, principal, reason string) {
	t.Helper()
	if ev.row.Kind != storage.ToolEventStop {
		t.Errorf("stop_all row kind = %q, want %q", ev.row.Kind, storage.ToolEventStop)
	}
	if ev.row.Approver != principal || ev.payload["principal"] != principal {
		t.Errorf("stop_all row approver = %q, payload principal = %v; want the caller %q",
			ev.row.Approver, ev.payload["principal"], principal)
	}
	if got := ev.payload["reason"]; got != reason {
		t.Errorf("stop_all payload reason = %v, want %q", got, reason)
	}
	if got, ok := ev.payload["objective_id"]; ev.row.ObjectiveID != "" || (ok && got != "") {
		t.Errorf("stop_all row objective = %q, payload objective_id = %v; want none, the call is not one run's",
			ev.row.ObjectiveID, got)
	}
	if ev.row.EscalationReason != reason {
		t.Errorf("stop_all row escalation reason = %q, want the call's reason %q", ev.row.EscalationReason, reason)
	}
	if got, ok := ev.payload["stop_all_id"]; ok {
		t.Errorf("stop_all row carries stop_all_id %v; the call's row is the one the others point to", got)
	}
}

// assertTiedToCall checks a run's row has the call's principal and reason and
// points at the call's row.
func assertTiedToCall(t *testing.T, ev stopEvent, call stopEvent, control string, objID objective.ObjectiveID) {
	t.Helper()
	assertStopEvent(t, ev, control, "alice", stopLoopReason, objID)
	if got := ev.payload["stop_all_id"]; got != call.row.ID {
		t.Errorf("%s row for %q: stop_all_id = %v, want the call's row ID %q", control, objID, got, call.row.ID)
	}
}

// One StopAll over two running one-shot loops and two standing objectives,
// one of them mid-pass, leaves five rows: the call's, a pause per objective
// and a loop_stop per one-shot loop. The mid-pass objective's own loop leaves
// none of its own.
func TestStopAllLeavesOneEventForTheCallAndOnePerRun(t *testing.T) {
	w := startStopAllWorld(t)
	passLoop := w.loop.id()
	w.stopAll(t, allowAll)
	w.awaitPassEnded(t)

	evs := stopAllEvents(t, w, 5)
	calls := eventsWithControl(evs, "stop_all")
	if len(calls) == 0 {
		t.Fatalf("StopAll left no stop_all row for the call; rows: %+v", evs)
	}
	call := onlyStopEvent(t, calls, "the stop_all call")
	assertCallEvent(t, call, "alice", stopLoopReason)

	if got := payloadIDs(t, call, "loops_stopped", "loop_id"); !equalStrings(got, w.oneShot) {
		t.Errorf("stop_all row loops_stopped = %v, want the one-shot loops %v", got, w.oneShot)
	}
	paused := payloadList(t, call, "objectives_paused")
	cancelled := map[string]any{}
	for _, p := range paused {
		id, _ := p["objective_id"].(string)
		cancelled[id] = p["pass_cancelled"]
	}
	if len(paused) != 2 || len(cancelled) != 2 {
		t.Errorf("stop_all row objectives_paused = %v, want %q and %q once each", paused, w.obj.ID, w.idle.ID)
	}
	if got, ok := cancelled[string(w.obj.ID)]; !ok || got != true {
		t.Errorf("stop_all row: %q pass_cancelled = %v (listed %v), want true", w.obj.ID, got, ok)
	}
	if got, ok := cancelled[string(w.idle.ID)]; !ok || got != false {
		t.Errorf("stop_all row: %q pass_cancelled = %v (listed %v), want false", w.idle.ID, got, ok)
	}
	if nr := payloadList(t, call, "not_reached"); len(nr) != 0 {
		t.Errorf("stop_all row not_reached = %v, want none: every run was permitted", nr)
	}

	pauses := eventsWithControl(evs, "pause")
	byObjective := map[string]stopEvent{}
	for _, ev := range pauses {
		byObjective[ev.row.ObjectiveID] = ev
	}
	if len(pauses) != 2 || len(byObjective) != 2 {
		t.Errorf("StopAll left %d pause rows, want one each for %q and %q: %+v", len(pauses), w.obj.ID, w.idle.ID, pauses)
	}
	for _, id := range []objective.ObjectiveID{w.obj.ID, w.idle.ID} {
		ev, ok := byObjective[string(id)]
		if !ok {
			t.Errorf("StopAll left no pause row for %q", id)
			continue
		}
		assertTiedToCall(t, ev, call, "pause", id)
	}
	if ev, ok := byObjective[string(w.obj.ID)]; ok {
		if got := ev.payload["pass_cancelled"]; got != true {
			t.Errorf("pause row for %q: pass_cancelled = %v, want true", w.obj.ID, got)
		}
		if got := ev.payload["loop_id"]; got != passLoop {
			t.Errorf("pause row for %q: loop_id = %v, want the cancelled pass's loop %q", w.obj.ID, got, passLoop)
		}
	}

	stops := eventsWithControl(evs, "loop_stop")
	byLoop := map[string]stopEvent{}
	for _, ev := range stops {
		id, _ := ev.payload["loop_id"].(string)
		byLoop[id] = ev
		if id == passLoop {
			t.Errorf("the mid-pass objective's loop %q left a loop_stop row of its own; its pause row covers it: %+v", passLoop, ev)
		}
	}
	if len(stops) != len(w.oneShot) || len(byLoop) != len(w.oneShot) {
		t.Errorf("StopAll left %d loop_stop rows, want one per one-shot loop %v: %+v", len(stops), w.oneShot, stops)
	}
	for i, loopID := range w.oneShot {
		ev, ok := byLoop[loopID]
		if !ok {
			t.Errorf("StopAll left no loop_stop row for %q", loopID)
			continue
		}
		assertTiedToCall(t, ev, call, "loop_stop", objective.ObjectiveID("obj-oneshot-"+string(rune('a'+i))))
	}

	for _, ev := range evs {
		if ev.row.Approver == stopPrincipal || ev.payload["principal"] == stopPrincipal {
			t.Errorf("the internal stop of the pass's loop left a row of its own: %+v", ev)
		}
	}
	if len(evs) != 5 {
		t.Errorf("StopAll left %d stop rows, want 5 (the call, two pauses, two loop stops): %+v", len(evs), evs)
	}
}

// A second StopAll that changes nothing leaves the call's row only, and that
// row says nothing was stopped, paused or missed.
func TestStopAllThatChangesNothingLeavesOnlyTheCallEvent(t *testing.T) {
	w := startStopAllWorld(t)
	w.stopAll(t, allowAll)
	w.awaitPassEnded(t)
	first := stopAllEvents(t, w, 5)
	seen := make(map[string]bool, len(first))
	for _, ev := range first {
		seen[ev.row.ID] = true
	}

	if _, err := w.svc.StopAll(context.Background(), "bob", "again", allowAll); err != nil {
		t.Fatalf("second StopAll: %v", err)
	}
	all := stopAllEvents(t, w, len(first)+1)
	var added []stopEvent
	for _, ev := range all {
		if !seen[ev.row.ID] {
			added = append(added, ev)
		}
	}

	call := onlyStopEvent(t, added, "a second StopAll that changed nothing")
	assertCallEvent(t, call, "bob", "again")
	for _, key := range []string{"loops_stopped", "objectives_paused", "not_reached"} {
		if list := payloadList(t, call, key); len(list) != 0 {
			t.Errorf("second stop_all row %s = %v, want none: the call changed nothing", key, list)
		}
	}
}

// A caller allowed only obj-2 leaves the call's row, naming obj-2 paused and
// every other run not reached, and one pause row, for obj-2, tied to it. The
// runs it could not reach leave no row: it did not touch them.
func TestStopAllWithANarrowScopeRecordsWhatItDidNotReach(t *testing.T) {
	w := startStopAllWorld(t)
	onlyIdle := func(kind, id string) bool {
		return kind == StopAllKindObjective && id == string(w.idle.ID)
	}
	w.stopAll(t, onlyIdle)

	evs := stopAllEvents(t, w, 2)
	calls := eventsWithControl(evs, "stop_all")
	if len(calls) == 0 {
		t.Fatalf("StopAll left no stop_all row for the call; rows: %+v", evs)
	}
	call := onlyStopEvent(t, calls, "the stop_all call")
	assertCallEvent(t, call, "alice", stopLoopReason)

	if list := payloadList(t, call, "loops_stopped"); len(list) != 0 {
		t.Errorf("stop_all row loops_stopped = %v, want none: no loop was permitted", list)
	}
	if got := payloadIDs(t, call, "objectives_paused", "objective_id"); !equalStrings(got, []string{string(w.idle.ID)}) {
		t.Errorf("stop_all row objectives_paused = %v, want only %q", got, w.idle.ID)
	}
	want := map[NotReachedRun]bool{
		{Kind: StopAllKindLoop, ID: w.oneShot[0], Reason: StopAllNotPermitted}:          true,
		{Kind: StopAllKindLoop, ID: w.oneShot[1], Reason: StopAllNotPermitted}:          true,
		{Kind: StopAllKindObjective, ID: string(w.obj.ID), Reason: StopAllNotPermitted}: true,
	}
	nr := payloadList(t, call, "not_reached")
	if len(nr) != len(want) {
		t.Errorf("stop_all row not_reached = %v, want exactly %d entries: both one-shot loops and %q", nr, len(want), w.obj.ID)
	}
	for _, e := range nr {
		kind, _ := e["kind"].(string)
		id, _ := e["id"].(string)
		reason, _ := e["reason"].(string)
		n := NotReachedRun{Kind: kind, ID: id, Reason: reason}
		if !want[n] {
			t.Errorf("unexpected not_reached entry %v", e)
		}
		delete(want, n)
	}
	for n := range want {
		t.Errorf("stop_all row not_reached is missing %+v", n)
	}

	pause := onlyStopEvent(t, eventsWithControl(evs, "pause"), "a StopAll allowed only obj-2")
	assertTiedToCall(t, pause, call, "pause", w.idle.ID)
	if stops := eventsWithControl(evs, "loop_stop"); len(stops) != 0 {
		t.Errorf("StopAll left %d loop_stop rows though it stopped no loop: %+v", len(stops), stops)
	}
	if len(evs) != 2 {
		t.Errorf("StopAll left %d stop rows, want 2 (the call and obj-2's pause): %+v", len(evs), evs)
	}
}
