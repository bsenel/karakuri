package agent_test

import (
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// Phase 40 step 1: bounds can say what an action may hold. The names below are
// deliberately not a pack's: the core compares strings with patterns and must
// not know that "tools" is a coding agent's allow-list or "target" a branch.
const (
	capDelegate capability.CapabilityID = "test.act.delegate"
	capOther    capability.CapabilityID = "test.act.other"
)

func toolsLimit() agent.ActionLimit {
	return agent.ActionLimit{Capability: capDelegate, Parameter: "tools", Patterns: []string{"Read", "Bash(git:*)"}}
}

func targetLimit() agent.ActionLimit {
	return agent.ActionLimit{Capability: capDelegate, Parameter: "target", Patterns: []string{"stream/a-*"}}
}

func delegate(params map[string]any) environment.Action {
	return environment.Action{CapabilityID: capDelegate, Params: params}
}

func TestCheckActionLimits(t *testing.T) {
	cases := []struct {
		name    string
		limits  []agent.ActionLimit
		actions []environment.Action
		// want is the violations expected, compared on action, parameter,
		// value and limit; nil means the plan is inside its limits.
		want []agent.ActionLimitViolation
	}{
		{
			name:    "no limits declared: nothing is a violation",
			limits:  nil,
			actions: []environment.Action{delegate(map[string]any{"tools": []any{"Bash", "WebFetch"}}), delegate(nil)},
		},
		{
			name:    "a string value matching a pattern passes",
			limits:  []agent.ActionLimit{targetLimit()},
			actions: []environment.Action{delegate(map[string]any{"target": "stream/a-2026-11-15"})},
		},
		{
			name:    "a string value matching no pattern is a violation",
			limits:  []agent.ActionLimit{targetLimit()},
			actions: []environment.Action{delegate(map[string]any{"target": "main"})},
			want:    []agent.ActionLimitViolation{{Action: capDelegate, Parameter: "target", Value: "main", Limit: targetLimit()}},
		},
		{
			// path.Match's * does not cross a separator, so a pattern for one
			// level of names does not admit a nested one.
			name:    "a star does not cross a slash",
			limits:  []agent.ActionLimit{targetLimit()},
			actions: []environment.Action{delegate(map[string]any{"target": "stream/a-1/nested"})},
			want:    []agent.ActionLimitViolation{{Action: capDelegate, Parameter: "target", Value: "stream/a-1/nested", Limit: targetLimit()}},
		},
		{
			name:    "a []string value whose every element matches passes",
			limits:  []agent.ActionLimit{toolsLimit()},
			actions: []environment.Action{delegate(map[string]any{"tools": []string{"Read", "Bash(git:status)"}})},
		},
		{
			// A plan decoded from JSON carries lists as []any, which is the
			// shape the loop actually hands the bounds.
			name:    "a []any value whose every element matches passes",
			limits:  []agent.ActionLimit{toolsLimit()},
			actions: []environment.Action{delegate(map[string]any{"tools": []any{"Read", "Bash(git:log)"}})},
		},
		{
			// The case the phase exists for: the unrestricted shell is not
			// the restricted one, and one bad element is enough.
			name:    "a list with the unrestricted Bash is a violation",
			limits:  []agent.ActionLimit{toolsLimit()},
			actions: []environment.Action{delegate(map[string]any{"tools": []any{"Read", "Bash"}})},
			want:    []agent.ActionLimitViolation{{Action: capDelegate, Parameter: "tools", Value: "Bash", Limit: toolsLimit()}},
		},
		{
			// Bounds fail toward refusing: an action that leaves out a limited
			// parameter has not shown it is inside the limit.
			name:    "a missing parameter under a declared limit is a violation",
			limits:  []agent.ActionLimit{toolsLimit()},
			actions: []environment.Action{delegate(map[string]any{"prompt": "do the work"})},
			want:    []agent.ActionLimitViolation{{Action: capDelegate, Parameter: "tools", Value: "", Limit: toolsLimit()}},
		},
		{
			name:    "nil params under a declared limit is a violation",
			limits:  []agent.ActionLimit{toolsLimit()},
			actions: []environment.Action{delegate(nil)},
			want:    []agent.ActionLimitViolation{{Action: capDelegate, Parameter: "tools", Value: "", Limit: toolsLimit()}},
		},
		{
			name:    "a limit on one capability does not touch another",
			limits:  []agent.ActionLimit{toolsLimit()},
			actions: []environment.Action{{CapabilityID: capOther, Params: map[string]any{"tools": []any{"Bash"}}}},
		},
		{
			// Two limits on one parameter must both hold, which is what lets
			// a template narrow an agent's bounds and never widen them.
			name: "two limits on one parameter must both hold",
			limits: []agent.ActionLimit{
				{Capability: capDelegate, Parameter: "target", Patterns: []string{"stream/*"}},
				targetLimit(),
			},
			actions: []environment.Action{delegate(map[string]any{"target": "stream/b-1"})},
			want:    []agent.ActionLimitViolation{{Action: capDelegate, Parameter: "target", Value: "stream/b-1", Limit: targetLimit()}},
		},
		{
			name:    "a list element that is not a string is a violation",
			limits:  []agent.ActionLimit{toolsLimit()},
			actions: []environment.Action{delegate(map[string]any{"tools": []any{"Read", 3}})},
			want:    []agent.ActionLimitViolation{{Action: capDelegate, Parameter: "tools", Value: "3", Limit: toolsLimit()}},
		},
		{
			// A pattern path.Match cannot parse matches nothing rather than
			// everything.
			name:    "a malformed pattern matches nothing",
			limits:  []agent.ActionLimit{{Capability: capDelegate, Parameter: "target", Patterns: []string{"["}}},
			actions: []environment.Action{delegate(map[string]any{"target": "["})},
			want: []agent.ActionLimitViolation{{Action: capDelegate, Parameter: "target", Value: "[",
				Limit: agent.ActionLimit{Capability: capDelegate, Parameter: "target", Patterns: []string{"["}}}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := agent.AuthorityBounds{ActionLimits: tc.limits}.CheckActionLimits(tc.actions)
			if len(got) != len(tc.want) {
				t.Fatalf("violations = %+v, want %+v", got, tc.want)
			}
			for i := range tc.want {
				if !sameViolation(got[i], tc.want[i]) {
					t.Errorf("violation %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func sameViolation(a, b agent.ActionLimitViolation) bool {
	return a.Action == b.Action && a.Parameter == b.Parameter && a.Value == b.Value &&
		a.Limit.Capability == b.Limit.Capability && a.Limit.Parameter == b.Limit.Parameter &&
		strings.Join(a.Limit.Patterns, "\x00") == strings.Join(b.Limit.Patterns, "\x00")
}

// The violation reaches the operator through the verdict Decide already
// returns: an escalation whose reason names the action, the parameter, the
// offending value and the limit's patterns.
func TestDecideActionsReportsALimitViolationThroughTheVerdict(t *testing.T) {
	bounds := agent.AuthorityBounds{MaxAutonomousActions: agent.UnlimitedActions, ActionLimits: []agent.ActionLimit{toolsLimit()}}

	v := bounds.DecideActions(1.0, 0, []environment.Action{delegate(map[string]any{"tools": []any{"Read", "Bash"}})}, nil, agent.Evidence{})

	if !v.Escalate {
		t.Fatal("an action outside its limit was let through")
	}
	for _, want := range []string{string(capDelegate), "tools", `"Bash"`, "Bash(git:*)"} {
		if !strings.Contains(v.Reason, want) {
			t.Errorf("reason %q does not name %s", v.Reason, want)
		}
	}
	if v.Allowed != 1 {
		t.Errorf("Allowed = %d, want 1: an escalated plan is shown to the reviewer intact", v.Allowed)
	}
}

// With no limits declared, DecideActions is Decide: the same verdict for the
// same plan, whatever the actions' parameters hold.
func TestDecideActionsWithoutLimitsIsDecide(t *testing.T) {
	for _, bounds := range []agent.AuthorityBounds{
		{MaxAutonomousActions: agent.UnlimitedActions},
		{MaxAutonomousActions: 0},
		{MaxAutonomousActions: 1},
		{MaxAutonomousActions: agent.UnlimitedActions, RequiresApprovalFor: []capability.CapabilityID{capOther}},
		{MaxAutonomousActions: agent.UnlimitedActions, ConfidenceThreshold: 0.9},
	} {
		actions := []environment.Action{
			delegate(map[string]any{"tools": []any{"Bash"}}),
			{CapabilityID: capOther},
		}
		got := bounds.DecideActions(0.5, bounds.ConfidenceThreshold, actions, nil, agent.Evidence{})
		want := bounds.Decide(0.5, bounds.ConfidenceThreshold, []capability.CapabilityID{capDelegate, capOther}, nil, agent.Evidence{})
		if got != want {
			t.Errorf("bounds %+v: DecideActions = %+v, Decide = %+v", bounds, got, want)
		}
	}
}
