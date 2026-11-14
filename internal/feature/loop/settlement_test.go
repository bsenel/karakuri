package loop

import (
	"context"
	"encoding/json"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 38 step 1: the record of a verify step, and of the run's end, says for
// each criterion how it was settled, so "the required check ran and failed"
// and "the required check never ran" are no longer the same entry. Nothing
// here changes a state, a score or whether the loop completes.

// settledEntry is what the record says about one criterion, read off JSON so
// the test does not fix the Go type the list is carried in.
type settledEntry struct {
	ID         string                   `json:"id"`
	Met        bool                     `json:"met"`
	State      objective.CriterionState `json:"state"`
	Settlement objective.Settlement     `json:"settlement"`
}

// settledEntries decodes a recorded list of criteria, whatever type holds it.
func settledEntries(t *testing.T, v any) map[string]settledEntry {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal recorded criteria: %v", err)
	}
	var list []settledEntry
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("recorded criteria are not a list of {id, met, state, settlement}: %v (%s)", err, raw)
	}
	out := make(map[string]settledEntry, len(list))
	for _, e := range list {
		out[e.ID] = e
	}
	return out
}

// wantSettled is one acceptance-case criterion as the record should read it.
type wantSettled struct {
	met        bool
	state      objective.CriterionState
	settlement objective.Settlement
}

func checkSettled(t *testing.T, where string, got map[string]settledEntry, want map[string]wantSettled) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: %d criteria recorded, want %d: %+v", where, len(got), len(want), got)
	}
	for id, w := range want {
		g, ok := got[id]
		if !ok {
			t.Errorf("%s: no entry for criterion %q in %+v", where, id, got)
			continue
		}
		if g.Met != w.met || g.State != w.state {
			t.Errorf("%s: %s met=%v state=%q, want met=%v state=%q (Phase 38 changes no judgement)",
				where, id, g.Met, g.State, w.met, w.state)
		}
		if g.Settlement != w.settlement {
			t.Errorf("%s: %s settlement=%q, want %q", where, id, g.Settlement, w.settlement)
		}
	}
}

// settlementCriteria is the roadmap's acceptance case: three required criteria
// (one whose verifier runs and fails, one whose declared verifier never runs,
// one with no verifier that the agent judges) and a fourth whose verifier is
// reserved to a third party. ranVerifier is the capability the pass runs.
func settlementCriteria(ranVerifier string) []objective.Criterion {
	return []objective.Criterion{
		{ID: "tested", Description: "the tests pass", Verifier: capability.CapabilityID(ranVerifier)},
		{ID: "reviewed", Description: "a reviewer approved", Verifier: "test.verify.review"},
		{ID: "explained", Description: "the change is explained"},
		{ID: "external", Description: "the external check is green", Verifier: "mcp.thirdparty.check"},
	}
}

// The verify step's record names each criterion with how it was settled, and
// the states, met values and score are what they were before Phase 38.
func TestStepVerifyRecordsHowEachCriterionWasSettled(t *testing.T) {
	hub := event.NewHub()
	judge := &scriptedAgent{scripted: []coreagent.Output{{Content: "PASS"}}}
	sc := &stepContext{
		svc:   &serviceImpl{hub: hub},
		agent: judge,
		obj: objective.Objective{
			ID:              "obj-settlement",
			SuccessCriteria: settlementCriteria("test.verify.tests"),
		},
	}
	log := collectEvents(hub, sc.obj.ID)

	score, allMet := stepVerify(context.Background(), sc, []actionOutcome{
		// Ran, and reported failure. test.verify.review never runs.
		{CapabilityID: "test.verify.tests", Result: environment.ActionResult{Success: false, Error: "2 tests failed"}},
	})
	events := log.stop()

	// Today's judgement, unchanged: only the judged criterion is met, one of
	// four equal weights.
	if allMet {
		t.Error("completed with required criteria unmet")
	}
	if score != 0.25 {
		t.Errorf("weighted score = %v, want 0.25 as before Phase 38", score)
	}
	if judge.calls != 1 {
		t.Errorf("the judge was asked %d times, want once, for the criterion with no verifier", judge.calls)
	}

	want := map[string]wantSettled{
		"tested":    {false, objective.CriterionNotMet, objective.SettledVerifierRan},
		"reviewed":  {false, objective.CriterionInconclusive, objective.SettledVerifierNeverRan},
		"explained": {true, objective.CriterionMet, objective.SettledNoVerifier},
		"external":  {false, objective.CriterionNotMet, objective.SettledReservedVerifier},
	}
	reviewed := criterionByID(t, sc.obj.SuccessCriteria, "reviewed")
	if reviewed.Reason != objective.ReasonNotRun {
		t.Errorf("reviewed reason = %q, want %q as before Phase 38", reviewed.Reason, objective.ReasonNotRun)
	}

	payloads := verifyCompleted(events)
	if len(payloads) != 1 {
		t.Fatalf("verify completed %d times, want once", len(payloads))
	}
	if n, _ := payloads[0]["criteria_met_count"].(int); n != 1 {
		t.Errorf("criteria_met_count = %v, want 1 as before Phase 38", payloads[0]["criteria_met_count"])
	}
	if s, _ := payloads[0]["weighted_score"].(float64); s != 0.25 {
		t.Errorf("payload weighted_score = %v, want 0.25", payloads[0]["weighted_score"])
	}
	checkSettled(t, "verify step-completed payload", settledEntries(t, payloads[0]["criteria"]), want)
}

// The run's end carries the same list: on the objective's terminal event and
// on an audit row in tool_events, which is what the evidence export reads
// (step 2). The hub is not stored, so the event alone would not outlive the
// process. The loop's termination and its verdict are unchanged.
func TestFinalisationRecordsHowEachCriterionWasSettled(t *testing.T) {
	svc, store, obj := settleLoopFixture(t,
		&settleEnv{id: "test.env.ok", result: environment.ActionResult{Success: false, Error: "2 tests failed"}},
		settlementCriteria("test.act.ok"))
	def := traceAgent(coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions})
	log := collectEvents(svc.hub, obj.ID)

	runSync(svc, "loop-settlement-0001", loop.Request{Objective: obj, Agent: def, MaxIter: 1}, nil)
	events := log.stop()

	// Today's verdict, unchanged. The judge in this fixture answers with the
	// plan it was scripted with, which reads as FAIL, so nothing is met.
	want := map[string]wantSettled{
		"tested":    {false, objective.CriterionNotMet, objective.SettledVerifierRan},
		"reviewed":  {false, objective.CriterionInconclusive, objective.SettledVerifierNeverRan},
		"explained": {false, objective.CriterionNotMet, objective.SettledNoVerifier},
		"external":  {false, objective.CriterionNotMet, objective.SettledReservedVerifier},
	}

	st := finalLoopState(t, store, "loop-settlement-0001")
	if st.Status != objective.StatusFailed || st.Outcome != loop.RunBudgetSpent {
		t.Errorf("loop status=%q outcome=%q, want %q/%q as before Phase 38",
			st.Status, st.Outcome, objective.StatusFailed, loop.RunBudgetSpent)
	}
	if st.CriteriaMet != 0 {
		t.Errorf("loop criteria_met score = %v, want 0 as before Phase 38", st.CriteriaMet)
	}
	checkSettled(t, "stored loop state", settledEntries(t, st.Criteria), want)

	// The objective's terminal event.
	var terminal *event.Event
	for i := range events {
		if events[i].Type == event.TypeObjectiveFailed || events[i].Type == event.TypeObjectiveCompleted {
			terminal = &events[i]
		}
	}
	if terminal == nil {
		t.Fatalf("no terminal objective event among %d published", len(events))
	}
	if terminal.Type != event.TypeObjectiveFailed {
		t.Errorf("terminal event = %q, want %q", terminal.Type, event.TypeObjectiveFailed)
	}
	if met, _ := terminal.Payload["criteria_met"].(bool); met {
		t.Error("terminal event says criteria_met = true")
	}
	if terminal.Payload["criteria"] == nil {
		t.Errorf("terminal event carries no criteria list: %v", terminal.Payload)
	} else {
		checkSettled(t, "terminal objective event", settledEntries(t, terminal.Payload["criteria"]), want)
	}

	// The audit row written on the finalise path: one row for this loop that
	// carries the criteria list.
	rows, err := store.ListToolEvents(context.Background(), storage.ToolEventFilter{ObjectiveID: string(obj.ID)})
	if err != nil {
		t.Fatalf("list tool events: %v", err)
	}
	var found []map[string]any
	for _, r := range rows {
		var p map[string]any
		if json.Unmarshal([]byte(r.PayloadJSON), &p) != nil {
			continue
		}
		if _, ok := p["criteria"]; ok && p["loop_id"] == "loop-settlement-0001" {
			found = append(found, p)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d tool_events rows carry this loop's criteria, want 1 written at finalisation (rows: %d)", len(found), len(rows))
	}
	checkSettled(t, "finalisation audit row", settledEntries(t, found[0]["criteria"]), want)
}
