package loop

// Phase 40 step 7, the loop's half: every ending the phase introduces is a
// named outcome in the stored run, apart from failure.
//
// What the code does today, read before writing these (2026-11-17):
//   - stepDecide (decide.go, branch 3a) carries out a refusal: one bounds row,
//     no checkpoint, no pause, and no action into act. The iteration then runs
//     verify and learn over nothing, and the loop goes on to its next
//     iteration, which plans and is refused again. Nothing ends the run on the
//     refusal, so it runs until MaxIter and finalizeLoop names it
//     budget_spent, with status failed because no criterion was met.
//   - finalizeLoop (runner.go) is the only place that names an outcome for a
//     run that ran: errored when it ended on an error, converged when its
//     criteria were met, budget_spent otherwise. It writes the run_end audit
//     row (recordRunEnd), which is the record of how a run ended that outlives
//     the process (Phase 38 step 1).
//   - endedByRestart (service.go, step 4) stores the loop row with the outcome
//     interrupted_by_restart and writes no run_end row, so the audit log has no
//     record that the run ended at all.
//
// Not written here, on purpose:
//   - Waiting for quota, the third ending step 7 names, is step 8's ending
//     (Out of quota is a state), which is not delivered. Its test belongs with
//     it.
//   - Phase 39's "pass ended" signal does not exist yet, so no test asserts it
//     is sent.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// refusingRunBounds refuse the scripted plan: its one test.act.ok action
// carries no branch, and a limit that names the parameter refuses an action
// that leaves it out (agent.CheckActionLimits). Every other bound lets the
// plan run, so the refusal is the limits' doing alone.
func refusingRunBounds() coreagent.AuthorityBounds {
	b := unlimited
	b.ActionLimits = []coreagent.ActionLimit{{Capability: "test.act.ok", Parameter: "branch", Patterns: []string{"karakuri/*"}}}
	b.RefuseOutsideLimits = true
	return b
}

// runEndOutcomes returns the outcome each run_end audit row for loopID names.
func runEndOutcomes(t *testing.T, store storage.StorageAdapter, obj objective.ObjectiveID, loopID string) []string {
	t.Helper()
	rows, err := store.ListToolEvents(context.Background(), storage.ToolEventFilter{ObjectiveID: string(obj), Kind: storage.ToolEventRunEnd})
	if err != nil {
		t.Fatalf("list run_end rows: %v", err)
	}
	var out []string
	for _, r := range rows {
		var p struct {
			LoopID  string `json:"loop_id"`
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal([]byte(r.PayloadJSON), &p); err != nil {
			t.Fatalf("decode run_end payload %q: %v", r.PayloadJSON, err)
		}
		if p.LoopID == loopID {
			out = append(out, p.Outcome)
		}
	}
	return out
}

// (a) A run whose plan its bounds refuse, at act_with_notice, ends with the
// outcome refused_by_bounds in its stored row and its run_end row. It is not
// errored: nothing failed, the bounds did what they were declared to do. And
// it carries no error text, which is the field reconcile reads a failure from.
func TestARunRefusedByItsBoundsEndsRefusedByBounds(t *testing.T) {
	env := newStopEnv(false)
	svc, req := stopFixture(t, env, refusingRunBounds())
	req.AutonomyRung = objective.AutonomyActWithNotice
	ctx := context.Background()

	run, err := svc.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	st := waitStoredCompleted(t, svc.store, run.LoopID)

	if n := env.startCount(); n != 0 {
		t.Fatalf("%d action(s) started under bounds that refuse the plan; the fixture does not reach the refusal", n)
	}
	if st.Outcome != coreloop.RunRefusedByBounds {
		t.Errorf("stored outcome = %q after the bounds refused the plan, want %q: a refusal is an ending of its own, not a run that spent its iterations", st.Outcome, coreloop.RunRefusedByBounds)
	}
	if st.Outcome == coreloop.RunErrored {
		t.Error("a run refused by its bounds is stored as errored; nothing in it failed")
	}
	if st.Error != "" {
		t.Errorf("stored error = %q; reconcile reads this field as the run's failure", st.Error)
	}
	got := runEndOutcomes(t, svc.store, req.Objective.ID, run.LoopID)
	if len(got) != 1 || got[0] != string(coreloop.RunRefusedByBounds) {
		t.Errorf("run_end rows for the refused run name %v, want one naming %q", got, coreloop.RunRefusedByBounds)
	}
}

// (b) A run a restart ended keeps interrupted_by_restart in its stored run.
// Step 4's tests already pin the loop row; this pins the run_end row, the
// record of how a run ended that the audit log keeps, which endedByRestart
// does not write today.
func TestARunARestartEndedIsStoredAsInterruptedByRestart(t *testing.T) {
	env := newStopEnv(false)
	svc, req := stopFixture(t, env, unlimited)
	obj := standing(t, svc, req.Objective)
	storeInterrupted(t, svc, obj, false, "")
	t.Cleanup(func() { _, _ = svc.Stop(context.Background(), interruptedLoopID, stopPrincipal, "test cleanup") })

	if err := svc.ResumeStoredLoops(context.Background()); err != nil {
		t.Fatalf("ResumeStoredLoops: %v", err)
	}
	time.Sleep(stopSettle)

	if st := storedLoop(t, svc.store, interruptedLoopID); st.Outcome != coreloop.RunInterruptedByRestart {
		t.Fatalf("stored outcome = %q; step 4 ends this loop with %q, and this test starts from there", st.Outcome, coreloop.RunInterruptedByRestart)
	}
	got := runEndOutcomes(t, svc.store, obj.ID, interruptedLoopID)
	if len(got) != 1 || got[0] != string(coreloop.RunInterruptedByRestart) {
		t.Errorf("run_end rows for the loop the restart ended name %v, want one naming %q: the audit log has no record the run ended", got, coreloop.RunInterruptedByRestart)
	}
}
