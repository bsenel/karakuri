package loop

import (
	"context"
	"strings"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 40 step 3, slice B: a delegated action whose result carries a bounds
// violation (the software pack's push outside its declared refs) is written to
// the audit log as one bounds row of the kind step 2's refusal uses, naming the
// action and the ref, and the run's persisted state carries the violation as a
// typed field, so the supervisor can block the stream without reading Error.

const violatingCap = "software.act.delegate_to_cli"

// violatingEnv returns one action result, with or without a bounds violation.
type violatingEnv struct {
	provEnv
	violation *environment.BoundsViolation
}

func (e *violatingEnv) Act(context.Context, environment.Action) (environment.ActionResult, error) {
	if e.violation == nil {
		return environment.ActionResult{Success: true}, nil
	}
	return environment.ActionResult{
		Success:         false,
		Error:           "the run failed for a reason worded nothing like a violation",
		BoundsViolation: e.violation,
	}, nil
}

func pushViolation() *environment.BoundsViolation {
	return &environment.BoundsViolation{
		Reason: "bounds violation: main moved from aaa to bbb, outside the action's declared refs",
		Refs:   []string{"refs/heads/main"},
	}
}

func actFixture(t *testing.T, env *violatingEnv) *stepContext {
	t.Helper()
	env.id = "software.env.cli_agent"
	sc := decideFixture(t, coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions})
	sc.envs = []environment.Environment{env}
	return sc
}

func delegatePlan() plan {
	return plan{
		Confidence: 0.9,
		Actions:    []plannedAction{{CapabilityID: violatingCap, EnvID: "software.env.cli_agent"}},
	}
}

func TestAPushOutsideItsRefsIsOneBoundsRowNamingTheActionAndTheRef(t *testing.T) {
	sc := actFixture(t, &violatingEnv{violation: pushViolation()})

	stepAct(context.Background(), sc, delegatePlan())

	rows := boundsRows(t, sc)
	if len(rows) != 1 {
		t.Fatalf("bounds rows = %d, want exactly one for the push outside its refs", len(rows))
	}
	row := rows[0]
	if row.Kind != storage.ToolEventRefusal {
		t.Errorf("bounds row kind = %q, want %q, the kind step 2's bounds refusal uses", row.Kind, storage.ToolEventRefusal)
	}
	if row.Success {
		t.Error("the bounds row records a success")
	}
	if row.Capability != violatingCap {
		t.Errorf("bounds row capability = %q, want the action %q", row.Capability, violatingCap)
	}
	if !strings.Contains(row.EscalationReason, "main") {
		t.Errorf("bounds row reason %q does not name the ref that moved", row.EscalationReason)
	}
	if !strings.Contains(row.PayloadJSON, "refs/heads/main") {
		t.Errorf("bounds row payload %s does not name the ref refs/heads/main", row.PayloadJSON)
	}
}

func TestAPassWithNoViolationWritesNoBoundsRow(t *testing.T) {
	sc := actFixture(t, &violatingEnv{})

	stepAct(context.Background(), sc, delegatePlan())

	if rows := boundsRows(t, sc); len(rows) != 0 {
		t.Errorf("an action inside its refs wrote bounds rows: %+v", rows)
	}
}

// The violation reaches the loop's persisted row, which is all the supervisor
// reads, as the typed field and not as text.
func TestTheRunsStateCarriesTheViolation(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		env  *violatingEnv
		want bool
	}{
		{"a push outside its refs", &violatingEnv{violation: pushViolation()}, true},
		{"a pass with no violation", &violatingEnv{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc := actFixture(t, tc.env)
			stepAct(ctx, sc, delegatePlan())
			if err := sc.svc.persistState(ctx, sc.state, true); err != nil {
				t.Fatalf("persist: %v", err)
			}
			ls, err := sc.svc.store.GetLoopState(ctx, sc.loopID)
			if err != nil {
				t.Fatalf("get loop state: %v", err)
			}
			if !tc.want {
				if ls.BoundsViolation != nil {
					t.Errorf("a clean pass's state carries a violation: %+v", ls.BoundsViolation)
				}
				return
			}
			if ls.BoundsViolation == nil {
				t.Fatal("the run's state carries no bounds violation; the supervisor could only find it in Error's text")
			}
			if len(ls.BoundsViolation.Refs) != 1 || ls.BoundsViolation.Refs[0] != "refs/heads/main" {
				t.Errorf("violation refs = %v, want [refs/heads/main]", ls.BoundsViolation.Refs)
			}
		})
	}
}
