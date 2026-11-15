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

// Phase 40 step 1: the decide step checks every action of a plan against the
// bounds' action limits, inside the bounds check it already runs (AGENTS.md
// rule 8, ADR 015). Every bound below other than the limits is set so it cannot
// escalate on its own (an unlimited cap, a zero threshold, no approval set, no
// third-party evidence), so a pause can only be the limits' doing.
const (
	limitedCap   = "test.act.delegate"
	unlimitedCap = "test.act.other"
)

func limitedTools() coreagent.ActionLimit {
	return coreagent.ActionLimit{Capability: limitedCap, Parameter: "allowed_tools", Patterns: []string{"Read", "Bash(git:*)"}}
}

func limitedBounds(limits ...coreagent.ActionLimit) coreagent.AuthorityBounds {
	return coreagent.AuthorityBounds{
		MaxAutonomousActions: coreagent.UnlimitedActions,
		ConfidenceThreshold:  0,
		ActionLimits:         limits,
	}
}

func limitedPlan(actions ...plannedAction) plan {
	return plan{Confidence: 0.99, Actions: actions}
}

func TestDecideChecksActionLimits(t *testing.T) {
	cases := []struct {
		name   string
		bounds coreagent.AuthorityBounds
		plan   plan
		// violation is empty when the plan must run as today; otherwise the
		// words the escalation reason must hold: the action's capability, the
		// parameter, the offending value and the limit's patterns.
		violation []string
	}{
		{
			name:   "(a) no limits declared: an unrestricted shell runs as today",
			bounds: limitedBounds(),
			plan:   limitedPlan(plannedAction{CapabilityID: limitedCap, Params: map[string]any{"allowed_tools": []any{"Read", "Bash"}}}),
		},
		{
			name:   "(a) no limits declared: an action without the parameter runs as today",
			bounds: limitedBounds(),
			plan:   limitedPlan(plannedAction{CapabilityID: limitedCap}),
		},
		{
			name: "(b) a string parameter outside its limit is a violation",
			bounds: limitedBounds(coreagent.ActionLimit{
				Capability: limitedCap, Parameter: "branch", Patterns: []string{"stream/a-*"},
			}),
			plan:      limitedPlan(plannedAction{CapabilityID: limitedCap, Params: map[string]any{"branch": "main"}}),
			violation: []string{limitedCap, "branch", `"main"`, "stream/a-*"},
		},
		{
			name:   "(c) an action inside its limits passes",
			bounds: limitedBounds(limitedTools()),
			plan:   limitedPlan(plannedAction{CapabilityID: limitedCap, Params: map[string]any{"allowed_tools": []any{"Read", "Bash(git:status)"}}}),
		},
		{
			name:      "(d) a list holding the unrestricted Bash is a violation",
			bounds:    limitedBounds(limitedTools()),
			plan:      limitedPlan(plannedAction{CapabilityID: limitedCap, Params: map[string]any{"allowed_tools": []any{"Read", "Bash"}}}),
			violation: []string{limitedCap, "allowed_tools", `"Bash"`, "Read", "Bash(git:*)"},
		},
		{
			name:      "(e) a missing parameter under a declared limit is a violation",
			bounds:    limitedBounds(limitedTools()),
			plan:      limitedPlan(plannedAction{CapabilityID: limitedCap, Params: map[string]any{"prompt": "do the work"}}),
			violation: []string{limitedCap, "allowed_tools", "Bash(git:*)"},
		},
		{
			name:   "(f) a limit on one capability does not touch another",
			bounds: limitedBounds(limitedTools()),
			plan:   limitedPlan(plannedAction{CapabilityID: unlimitedCap, Params: map[string]any{"allowed_tools": []any{"Bash"}}}),
		},
		{
			name:   "(f) the violating action is found among others",
			bounds: limitedBounds(limitedTools()),
			plan: limitedPlan(
				plannedAction{CapabilityID: unlimitedCap},
				plannedAction{CapabilityID: limitedCap, Params: map[string]any{"allowed_tools": []any{"Bash"}}},
			),
			violation: []string{limitedCap, "allowed_tools", `"Bash"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			sc := decideFixture(t, tc.bounds)

			got, paused := stepDecide(ctx, sc, tc.plan, nil)

			if len(got.Actions) != len(tc.plan.Actions) {
				t.Errorf("plan carries %d actions after decide, want all %d", len(got.Actions), len(tc.plan.Actions))
			}
			reasons := escalationReasons(t, sc)

			if len(tc.violation) == 0 {
				if paused {
					t.Errorf("a plan inside its limits paused: %v", reasons)
				}
				if len(reasons) != 0 {
					t.Errorf("a plan inside its limits wrote escalation rows: %v", reasons)
				}
				return
			}

			if !paused {
				t.Fatal("a plan outside its action limits ran without stopping")
			}
			if len(reasons) != 1 {
				t.Fatalf("escalation rows = %v, want exactly one", reasons)
			}
			for _, want := range tc.violation {
				if !strings.Contains(reasons[0], want) {
					t.Errorf("reason %q does not name %s", reasons[0], want)
				}
			}
		})
	}
}

// (g) No second gate. The reason the decide step records is the reason the
// bounds policy itself returns for the same plan: the limits are checked where
// every other bound is, and the audit row and checkpoint carry that verdict.
func TestActionLimitsAreCheckedInTheExistingBoundsCheck(t *testing.T) {
	ctx := context.Background()
	bounds := limitedBounds(limitedTools())
	p := limitedPlan(plannedAction{CapabilityID: limitedCap, Params: map[string]any{"allowed_tools": []any{"Read", "Bash"}}})

	actions := make([]environment.Action, 0, len(p.Actions))
	for _, a := range p.Actions {
		actions = append(actions, environment.Action{CapabilityID: capability.CapabilityID(a.CapabilityID), Params: a.Params})
	}
	verdict := bounds.DecideActions(p.Confidence, 0, actions, nil, coreagent.Evidence{})
	if !verdict.Escalate {
		t.Fatal("the bounds policy let an action outside its limit through")
	}

	sc := decideFixture(t, bounds)
	if _, paused := stepDecide(ctx, sc, p, nil); !paused {
		t.Fatal("the decide step let an action outside its limit through")
	}

	rows, err := sc.svc.store.ListToolEvents(ctx, storage.ToolEventFilter{ObjectiveID: string(sc.obj.ID)})
	if err != nil {
		t.Fatalf("list tool events: %v", err)
	}
	var bound []storage.ToolEvent
	for _, r := range rows {
		if r.BoundsViolation {
			bound = append(bound, r)
		}
	}
	if len(bound) != 1 {
		t.Fatalf("bounds rows = %d, want one: a second gate would write its own", len(bound))
	}
	if bound[0].EscalationReason != verdict.Reason {
		t.Errorf("decide recorded %q, the bounds policy says %q: the limit was checked somewhere other than the bounds check",
			bound[0].EscalationReason, verdict.Reason)
	}
	if sc.state.result.CheckpointID == nil || *sc.state.result.CheckpointID == "" {
		t.Error("the violation raised no checkpoint, so decide did not report it the way it reports every bound today")
	}
}

func escalationReasons(t *testing.T, sc *stepContext) []string {
	t.Helper()
	rows, err := sc.svc.store.ListToolEvents(context.Background(), storage.ToolEventFilter{ObjectiveID: string(sc.obj.ID)})
	if err != nil {
		t.Fatalf("list tool events: %v", err)
	}
	var reasons []string
	for _, r := range rows {
		if r.Kind == storage.ToolEventEscalation {
			reasons = append(reasons, r.EscalationReason)
		}
	}
	return reasons
}
