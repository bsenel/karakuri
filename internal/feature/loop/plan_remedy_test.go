package loop

// Phase 40 step 14b, review Blocker 2 on pull request #284: the planner names
// the remedy it applied as a field of the plan it returns, and the loop copies
// it onto the run's state, where reconcile reads it to keep a confirmed remedy.
// Nothing here sets State.Remedy by hand: the planner's answer is the only
// source.

import (
	"context"
	"testing"

	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/llm"
)

const plannedRemedy = "merge main into the branch, keep both sides of run.go, push"

const remedyPlan = `{"actions":[` +
	`{"capability":"test.act.ok","env_id":"test.env.ok","params":{"n":1},"reason":"first"}` +
	`],"confidence":0.95,"reasoning":"main moved under the branch","remedy":"` + plannedRemedy + `"}`

// stateOfRunPlanning runs one pass whose planner answers reply and returns
// the run's stored state once it has ended.
func stateOfRunPlanning(t *testing.T, reply string) coreloop.State {
	t.Helper()
	svc, req := quotaFixture(t, &quotaEnv{traceEnv: traceEnv{id: "test.env.ok"}}, 1)
	providers := llm.NewRegistry(nil)
	providers.Register(&traceProvider{reply: reply})
	svc.factory = platformagent.NewFactory(providers, svc.hub, svc.otel, nil)
	req.Unblock = &coreloop.Unblock{Kind: "unmergeable_pr", Subject: "karakuri/phase-40-step-14b", Count: 2}
	run, err := svc.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return waitStoredCompleted(t, svc.store, run.LoopID)
}

func TestTheRemedyThePlannerNamesIsOnTheRunsState(t *testing.T) {
	st := stateOfRunPlanning(t, remedyPlan)
	if st.Remedy != plannedRemedy {
		t.Fatalf("State.Remedy = %q, want %q: the remedy the planner named in its plan", st.Remedy, plannedRemedy)
	}
}

// A plan that names no remedy leaves the state's empty: the loop invents none.
func TestAPlanWithNoRemedyLeavesTheRunsRemedyEmpty(t *testing.T) {
	st := stateOfRunPlanning(t, diagnosisPlan)
	if st.Remedy != "" {
		t.Fatalf("State.Remedy = %q, want empty for a plan that named none", st.Remedy)
	}
}

// The plan's field is decoded from the planner's JSON as "remedy".
func TestParsePlanReadsTheRemedy(t *testing.T) {
	p, err := parsePlan(remedyPlan)
	if err != nil {
		t.Fatalf("parsePlan: %v", err)
	}
	if p.Remedy != plannedRemedy {
		t.Fatalf("plan.Remedy = %q, want %q", p.Remedy, plannedRemedy)
	}
}
