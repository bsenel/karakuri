package loop

import (
	"context"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
)

func TestExtractJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "raw JSON unchanged",
			in:   `{"actions":[],"confidence":0.8}`,
			want: `{"actions":[],"confidence":0.8}`,
		},
		{
			name: "raw JSON with whitespace",
			in:   "  \n  {\"actions\":[],\"confidence\":0.8}\n  ",
			want: `{"actions":[],"confidence":0.8}`,
		},
		{
			name: "fenced json language tag",
			in:   "```json\n{\"actions\":[],\"confidence\":0.8}\n```",
			want: `{"actions":[],"confidence":0.8}`,
		},
		{
			name: "fenced JSON uppercase language tag",
			in:   "```JSON\n{\"actions\":[],\"confidence\":0.8}\n```",
			want: `{"actions":[],"confidence":0.8}`,
		},
		{
			name: "fenced no language tag",
			in:   "```\n{\"actions\":[],\"confidence\":0.8}\n```",
			want: `{"actions":[],"confidence":0.8}`,
		},
		{
			name: "fenced with surrounding whitespace",
			in:   "\n\n  ```json\n  {\"actions\":[]}\n  ```\n\n",
			want: "{\"actions\":[]}",
		},
		{
			name: "fenced single-line (no newline after lang tag)",
			in:   "```json {\"actions\":[]} ```",
			want: `{"actions":[]}`,
		},
		{
			name: "prose-wrapped (no fence) — first object only",
			in:   "Here's my plan:\n\n{\"actions\":[],\"confidence\":0.8}\n\nLet me know.",
			want: `{"actions":[],"confidence":0.8}`,
		},
		{
			name: "prose-wrapped array",
			in:   "Here are the actions: [{\"capability\":\"x\"}] and that's it.",
			want: `[{"capability":"x"}]`,
		},
		{
			name: "malformed input returns trimmed input for downstream error reporting",
			in:   "  not json at all  ",
			want: "not json at all",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractJSON(c.in); got != c.want {
				t.Errorf("extractJSON(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// scriptedReasonContext builds the minimum stepContext needed for stepReason
// to run end-to-end with a scriptedAgent. Hub is required because stepReason
// emits step_started + step_completed events.
func scriptedReasonContext(t *testing.T, a coreagent.Agent) *stepContext {
	t.Helper()
	svc := &serviceImpl{hub: event.NewHub()}
	return &stepContext{
		agent: a,
		agentDef: coreagent.Definition{
			ID:                "test-agent",
			ReasoningStrategy: coreagent.ReasoningChainOfThought,
		},
		obj: objective.Objective{Title: "test objective"},
		svc: svc,
	}
}

func TestStepReason_ParsesFencedJSON(t *testing.T) {
	// The exact failure mode the Phase 14 dogfood surfaced: agent returns
	// ```json … ```; the parser used to fall through to the no-op
	// reason.plan wrapper. With extractJSON in place, the real plan lands.
	const fenced = "```json\n" +
		`{"actions":[{"capability":"git.branch","params":{"name":"feat/x"},"reason":"r","env_id":"local"}],` +
		`"confidence":0.78,"reasoning":"draft"}` + "\n```"

	agent := &scriptedAgent{scripted: []coreagent.Output{{Content: fenced, Confidence: 0.78}}}
	sc := scriptedReasonContext(t, agent)

	p := stepReason(context.Background(), sc, loop.WorldState{})

	if len(p.Actions) != 1 {
		t.Fatalf("expected 1 action from fenced JSON, got %d (fallback wrapper triggered?)", len(p.Actions))
	}
	if p.Actions[0].CapabilityID != "git.branch" {
		t.Errorf("expected capability=git.branch, got %q", p.Actions[0].CapabilityID)
	}
	if p.Confidence != 0.78 {
		t.Errorf("expected confidence=0.78 from parsed JSON, got %v (wrapper would set 0.7)", p.Confidence)
	}
	if p.Reasoning != "draft" {
		t.Errorf("expected reasoning=draft from parsed JSON, got %q", p.Reasoning)
	}
}

func TestStepReason_ParsesProseWrappedJSON(t *testing.T) {
	// Defensive: even when the agent ignores the no-fences instruction and
	// also forgets the fence, find the JSON object by brace-matching.
	const prose = "Here's the plan you asked for:\n\n" +
		`{"actions":[{"capability":"test","reason":"r"}],"confidence":0.6,"reasoning":"prose"}` +
		"\n\nLet me know if you want changes."

	agent := &scriptedAgent{scripted: []coreagent.Output{{Content: prose, Confidence: 0.6}}}
	sc := scriptedReasonContext(t, agent)

	p := stepReason(context.Background(), sc, loop.WorldState{})
	if len(p.Actions) != 1 || p.Actions[0].CapabilityID != "test" {
		t.Fatalf("expected 1 action capability=test from prose-wrapped JSON, got %+v", p.Actions)
	}
	if p.Confidence != 0.6 {
		t.Errorf("expected confidence=0.6, got %v", p.Confidence)
	}
}

func TestStepReason_TrulyMalformedFallsBackToWrapper(t *testing.T) {
	// Regression guard: keep the existing fallback for genuinely unparseable
	// output. The wrapper still records what the agent said so the audit
	// row preserves context.
	agent := &scriptedAgent{scripted: []coreagent.Output{{Content: "I cannot help with that.", Confidence: 0.4}}}
	sc := scriptedReasonContext(t, agent)

	p := stepReason(context.Background(), sc, loop.WorldState{})
	if len(p.Actions) != 1 || p.Actions[0].CapabilityID != "reason.plan" {
		t.Errorf("expected fallback wrapper with capability=reason.plan, got %+v", p.Actions)
	}
}
