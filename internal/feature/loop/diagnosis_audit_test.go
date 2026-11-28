package loop

// Phase 40 step 14b, "the diagnosis is in the audit log before the first
// action". The loop writes one generic "plan" audit row for every plan, before
// stepAct runs the first action: the plan's reasoning and its actions. It
// learns nothing about blockers (AGENTS.md rule 8); a blocker pass's diagnosis
// is simply the reasoning the planner returned, and the row names the blocker
// the request carried because the request is part of what the plan answers.

import (
	"context"
	"strings"
	"testing"

	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// planRowKind is the kind of the generic row written for every plan.
const planRowKind = "plan"

const diagnosisText = "DIAGNOSIS: the merge conflicts in internal/feature/reconcile/run.go; main moved under the branch"

const diagnosisPlan = `{"actions":[` +
	`{"capability":"test.act.ok","env_id":"test.env.ok","params":{"n":1},"reason":"first"}` +
	`],"confidence":0.95,"reasoning":"` + diagnosisText + `"}`

// auditRowsOfRun runs one pass whose planner answers diagnosisPlan and returns
// the objective's audit rows, oldest first.
func auditRowsOfRun(t *testing.T, mutate func(*coreloop.Request)) []storage.ToolEvent {
	t.Helper()
	svc, req := quotaFixture(t, &quotaEnv{traceEnv: traceEnv{id: "test.env.ok"}}, 1)
	providers := llm.NewRegistry(nil)
	providers.Register(&traceProvider{reply: diagnosisPlan})
	svc.factory = platformagent.NewFactory(providers, svc.hub, svc.otel, nil)
	mutate(&req)
	run, err := svc.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitStoredCompleted(t, svc.store, run.LoopID)
	rows, err := svc.store.ListToolEvents(context.Background(), storage.ToolEventFilter{
		ObjectiveID: string(req.Objective.ID), OldestFirst: true,
	})
	if err != nil {
		t.Fatalf("list tool events: %v", err)
	}
	return rows
}

func indexOfKind(rows []storage.ToolEvent, kind string) int {
	for i, r := range rows {
		if r.Kind == kind {
			return i
		}
	}
	return -1
}

func TestABlockedPassWritesItsDiagnosisDownBeforeItsFirstAction(t *testing.T) {
	rows := auditRowsOfRun(t, func(r *coreloop.Request) {
		r.Unblock = &coreloop.Unblock{
			Kind: "unmergeable_pr", Subject: "karakuri/phase-40-step-14b", Count: 3,
			Remedy: "merge main into the branch",
		}
	})
	plan, exec := indexOfKind(rows, planRowKind), indexOfKind(rows, storage.ToolEventExecute)
	if exec < 0 {
		t.Fatalf("no action was audited; rows: %+v", rows)
	}
	if plan < 0 {
		t.Fatalf("no %q audit row was written for the plan; rows: %+v", planRowKind, rows)
	}
	if plan > exec {
		t.Errorf("the plan row is row %d, after the first action's row %d", plan, exec)
	}
	payload := rows[plan].PayloadJSON
	for _, want := range []string{diagnosisText, "unmergeable_pr", "karakuri/phase-40-step-14b"} {
		if !strings.Contains(payload, want) {
			t.Errorf("the plan row's payload does not hold %q: %s", want, payload)
		}
	}
}

// An ordinary pass gets the same row, and it names no blocker.
func TestAnOrdinaryPassWritesItsPlanDownBeforeItsFirstAction(t *testing.T) {
	rows := auditRowsOfRun(t, func(*coreloop.Request) {})
	plan, exec := indexOfKind(rows, planRowKind), indexOfKind(rows, storage.ToolEventExecute)
	if exec < 0 {
		t.Fatalf("no action was audited; rows: %+v", rows)
	}
	if plan < 0 {
		t.Fatalf("no %q audit row was written for the plan; rows: %+v", planRowKind, rows)
	}
	if plan > exec {
		t.Errorf("the plan row is row %d, after the first action's row %d", plan, exec)
	}
	if !strings.Contains(rows[plan].PayloadJSON, diagnosisText) {
		t.Errorf("the plan row's payload does not hold the reasoning: %s", rows[plan].PayloadJSON)
	}
	if strings.Contains(rows[plan].PayloadJSON, "unmergeable_pr") {
		t.Errorf("a pass with no blocker names one: %s", rows[plan].PayloadJSON)
	}
}
