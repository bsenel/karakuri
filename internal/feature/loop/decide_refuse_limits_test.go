package loop

import (
	"context"
	"strings"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 40 step 2: under bounds that refuse outside their limits (the
// supervisor sets RefuseOutsideLimits at the rungs that act), a plan outside
// them is refused, not escalated. The iteration ends with one bounds refusal
// in the audit log and no checkpoint, because an escalation would hand the
// mechanical review back to a person, and that is the gate the phase removes.

func refusingBounds(base coreagent.AuthorityBounds) coreagent.AuthorityBounds {
	base.ActionLimits = []coreagent.ActionLimit{limitedTools()}
	base.RefuseOutsideLimits = true
	return base
}

func outsidePlan() plan {
	return limitedPlan(
		plannedAction{CapabilityID: unlimitedCap},
		plannedAction{CapabilityID: limitedCap, Params: map[string]any{"allowed_tools": []any{"Read", "Bash(gh pr merge:*)"}}},
	)
}

func boundsRows(t *testing.T, sc *stepContext) []storage.ToolEvent {
	t.Helper()
	rows, err := sc.svc.store.ListToolEvents(context.Background(), storage.ToolEventFilter{ObjectiveID: string(sc.obj.ID)})
	if err != nil {
		t.Fatalf("list tool events: %v", err)
	}
	var out []storage.ToolEvent
	for _, r := range rows {
		if r.BoundsViolation {
			out = append(out, r)
		}
	}
	return out
}

func TestDecideRefusesAPlanOutsideItsLimits(t *testing.T) {
	cases := []struct {
		name   string
		bounds coreagent.AuthorityBounds
	}{
		{"bounds that would let the plan run", refusingBounds(limitedBounds())},
		// The strategist and the maintainer declare MaxAutonomousActions 0:
		// their plans escalate on that bound alone. Outside its limits a plan
		// is still refused, or the stream's mechanical check would reach a
		// person through the other door.
		{"bounds that would escalate the plan anyway", refusingBounds(coreagent.AuthorityBounds{MaxAutonomousActions: 0, ConfidenceThreshold: 0.5})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			sc := decideFixture(t, tc.bounds)

			got, paused := stepDecide(ctx, sc, outsidePlan(), nil)

			if paused {
				t.Error("a refused plan paused the loop, so it waits for a decision no checkpoint asks for")
			}
			if len(got.Actions) != 0 {
				t.Errorf("a refused plan still carries %d actions into act", len(got.Actions))
			}
			if sc.state.result.CheckpointID != nil && *sc.state.result.CheckpointID != "" {
				t.Errorf("a refused plan raised checkpoint %s", *sc.state.result.CheckpointID)
			}
			pending, err := sc.svc.store.ListPendingCheckpoints(ctx, sc.twinID)
			if err != nil {
				t.Fatalf("list checkpoints: %v", err)
			}
			if len(pending) != 0 {
				t.Errorf("a refused plan left %d pending checkpoints", len(pending))
			}

			rows := boundsRows(t, sc)
			if len(rows) != 1 {
				t.Fatalf("bounds rows = %d, want exactly one refusal", len(rows))
			}
			if rows[0].Kind == storage.ToolEventEscalation {
				t.Error("the refusal is recorded as an escalation; nobody was asked")
			}
			for _, want := range []string{limitedCap, "allowed_tools", "Bash(gh pr merge:*)"} {
				if !strings.Contains(rows[0].EscalationReason, want) {
					t.Errorf("the refusal's reason %q does not name %s", rows[0].EscalationReason, want)
				}
			}
			if reasons := escalationReasons(t, sc); len(reasons) != 0 {
				t.Errorf("a refused plan wrote escalation rows: %v", reasons)
			}
		})
	}
}

// Inside its limits, a plan under refusing bounds runs exactly as it would
// without them: no row, no pause, every action carried on.
func TestDecideRunsAPlanInsideItsLimitsUnderRefusingBounds(t *testing.T) {
	sc := decideFixture(t, refusingBounds(limitedBounds()))
	p := limitedPlan(plannedAction{CapabilityID: limitedCap, Params: map[string]any{"allowed_tools": []any{"Read", "Bash(git:status)"}}})

	got, paused := stepDecide(context.Background(), sc, p, nil)

	if paused || len(got.Actions) != 1 {
		t.Errorf("paused=%v actions=%d, want the plan to run", paused, len(got.Actions))
	}
	if rows := boundsRows(t, sc); len(rows) != 0 {
		t.Errorf("a plan inside its limits wrote bounds rows: %+v", rows)
	}
}

// No second gate: the refusal decide records is the one the bounds policy
// returns, and the policy says refuse, not escalate.
func TestARefusalIsTheBoundsPolicysVerdict(t *testing.T) {
	bounds := refusingBounds(limitedBounds())
	p := outsidePlan()
	actions := make([]environment.Action, 0, len(p.Actions))
	for _, a := range p.Actions {
		actions = append(actions, environment.Action{CapabilityID: capability.CapabilityID(a.CapabilityID), Params: a.Params})
	}
	verdict := bounds.DecideActions(p.Confidence, 0, actions, nil, coreagent.Evidence{})
	if !verdict.Refuse || verdict.Escalate {
		t.Fatalf("verdict = %+v, want a refusal and no escalation", verdict)
	}

	sc := decideFixture(t, bounds)
	stepDecide(context.Background(), sc, p, nil)
	rows := boundsRows(t, sc)
	if len(rows) != 1 {
		t.Fatalf("bounds rows = %d, want one", len(rows))
	}
	if rows[0].EscalationReason != verdict.Reason {
		t.Errorf("decide recorded %q, the bounds policy says %q", rows[0].EscalationReason, verdict.Reason)
	}
}

// Without RefuseOutsideLimits nothing changes from step 1: a violation
// escalates with a checkpoint, which is what the propose rung relies on.
func TestWithoutRefusalAViolationStillEscalates(t *testing.T) {
	sc := decideFixture(t, limitedBounds(limitedTools()))
	if _, paused := stepDecide(context.Background(), sc, outsidePlan(), nil); !paused {
		t.Fatal("a violation under bounds that do not refuse ran without stopping")
	}
	if sc.state.result.CheckpointID == nil || *sc.state.result.CheckpointID == "" {
		t.Error("a violation under bounds that do not refuse raised no checkpoint")
	}
}
