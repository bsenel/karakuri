package loop

import (
	"context"
	"strings"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
)

func ptr[T any](v T) *T { return &v }

func newModifyContext(a coreagent.Agent) *stepContext {
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

func TestTrimRemovedActions_DropsOneOccurrence(t *testing.T) {
	in := plan{Actions: []plannedAction{
		{CapabilityID: "code.write"},
		{CapabilityID: "code.write"},
		{CapabilityID: "test.run"},
	}}
	out := trimRemovedActions(in, &corecheckpoint.Modifications{
		RemovedActions: []string{"code.write"},
	})
	if len(out.Actions) != 2 {
		t.Fatalf("expected 2 actions after dropping one code.write, got %d", len(out.Actions))
	}
	if out.Actions[0].CapabilityID != "code.write" || out.Actions[1].CapabilityID != "test.run" {
		t.Errorf("expected second code.write preserved + test.run kept, got %+v", out.Actions)
	}
}

func TestTrimRemovedActions_DropsAllWhenListed(t *testing.T) {
	in := plan{Actions: []plannedAction{
		{CapabilityID: "code.write"},
		{CapabilityID: "code.write"},
		{CapabilityID: "test.run"},
	}}
	out := trimRemovedActions(in, &corecheckpoint.Modifications{
		RemovedActions: []string{"code.write", "code.write"},
	})
	if len(out.Actions) != 1 || out.Actions[0].CapabilityID != "test.run" {
		t.Fatalf("expected only test.run after dropping both code.write, got %+v", out.Actions)
	}
}

func TestTrimRemovedActions_NilModsIsNoop(t *testing.T) {
	in := plan{Actions: []plannedAction{{CapabilityID: "x"}, {CapabilityID: "y"}}}
	out := trimRemovedActions(in, nil)
	if len(out.Actions) != 2 {
		t.Errorf("nil mods must not modify the plan")
	}
	out2 := trimRemovedActions(in, &corecheckpoint.Modifications{})
	if len(out2.Actions) != 2 {
		t.Errorf("empty mods must not modify the plan")
	}
}

func TestApplyConfidenceFloor(t *testing.T) {
	cases := []struct {
		name     string
		conf     float64
		floor    *float64
		expected float64
	}{
		{"nil floor is noop", 0.5, nil, 0.5},
		{"floor raises", 0.5, ptr(0.8), 0.8},
		{"floor below confidence keeps higher", 0.9, ptr(0.7), 0.9},
		{"floor equals confidence", 0.7, ptr(0.7), 0.7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var mods *corecheckpoint.Modifications
			if c.floor != nil {
				mods = &corecheckpoint.Modifications{RevisedConfidence: c.floor}
			}
			out := applyConfidenceFloor(plan{Confidence: c.conf}, mods)
			if out.Confidence != c.expected {
				t.Errorf("expected confidence=%v, got %v", c.expected, out.Confidence)
			}
		})
	}
}

func TestStepReasonRevise_AppliesNoteAsCritique(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{
		{Content: `{"actions":[{"capability":"scaffold","reason":"ok"}],"confidence":0.85,"reasoning":"narrowed"}`, Confidence: 0.85},
	}}
	sc := newModifyContext(agent)

	draft := plan{
		Actions:    []plannedAction{{CapabilityID: "code.write"}, {CapabilityID: "scaffold"}},
		Confidence: 0.6,
	}
	decision := corecheckpoint.Decision{
		Choice: "modify",
		Note:   "scaffold only this iteration",
	}
	revised, applied := stepReasonRevise(context.Background(), sc, draft, decision)
	if !applied {
		t.Fatalf("expected revise to be applied when note is set")
	}
	if len(revised.Actions) != 1 || revised.Actions[0].CapabilityID != "scaffold" {
		t.Errorf("expected revised plan to drop code.write and keep scaffold, got %+v", revised.Actions)
	}
	// Critique input must have made it into the prompt.
	if len(agent.tasksSeen) == 0 || !strings.Contains(agent.tasksSeen[0], "scaffold only this iteration") {
		t.Errorf("expected operator note in revise prompt, got tasks=%v", agent.tasksSeen)
	}
}

func TestStepReasonRevise_AppliesConstraintsAsCritique(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{
		{Content: `{"actions":[{"capability":"scaffold"}],"confidence":0.8,"reasoning":"r"}`, Confidence: 0.8},
	}}
	sc := newModifyContext(agent)

	draft := plan{Actions: []plannedAction{{CapabilityID: "x"}}, Confidence: 0.5}
	decision := corecheckpoint.Decision{
		Choice: "modify",
		Modifications: &corecheckpoint.Modifications{
			AddedConstraints: []string{"no destructive ops", "stay inside /tmp"},
		},
	}
	_, applied := stepReasonRevise(context.Background(), sc, draft, decision)
	if !applied {
		t.Fatal("expected revise to be applied when constraints are set")
	}
	if !strings.Contains(agent.tasksSeen[0], "no destructive ops") || !strings.Contains(agent.tasksSeen[0], "stay inside /tmp") {
		t.Errorf("expected both constraints in revise prompt, got %q", agent.tasksSeen[0])
	}
}

func TestStepReasonRevise_NoFeedbackSkipsAgent(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{
		{Content: `{"actions":[{"capability":"never_called"}],"confidence":1.0}`, Confidence: 1.0},
	}}
	sc := newModifyContext(agent)

	draft := plan{Actions: []plannedAction{{CapabilityID: "kept"}}, Confidence: 0.4}
	revised, applied := stepReasonRevise(context.Background(), sc, draft, corecheckpoint.Decision{Choice: "modify"})
	if applied {
		t.Errorf("expected applied=false when no note + no constraints")
	}
	if agent.calls != 0 {
		t.Errorf("agent must not be called when no feedback provided, got %d calls", agent.calls)
	}
	if len(revised.Actions) != 1 || revised.Actions[0].CapabilityID != "kept" {
		t.Errorf("expected trimmed draft preserved, got %+v", revised.Actions)
	}
}

func TestStepReasonRevise_FallsBackOnUnparseableRevision(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{
		{Content: "not json at all"},
	}}
	sc := newModifyContext(agent)
	draft := plan{Actions: []plannedAction{{CapabilityID: "kept"}}, Confidence: 0.5}
	decision := corecheckpoint.Decision{Choice: "modify", Note: "rework"}
	revised, applied := stepReasonRevise(context.Background(), sc, draft, decision)
	if applied {
		t.Errorf("expected applied=false when revision is unparseable")
	}
	if len(revised.Actions) != 1 || revised.Actions[0].CapabilityID != "kept" {
		t.Errorf("expected draft preserved, got %+v", revised.Actions)
	}
}

func TestStepReasonRevise_FallsBackOnEmptyActions(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{
		{Content: `{"actions":[],"confidence":0.9}`},
	}}
	sc := newModifyContext(agent)
	draft := plan{Actions: []plannedAction{{CapabilityID: "kept"}}, Confidence: 0.5}
	decision := corecheckpoint.Decision{Choice: "modify", Note: "rework"}
	revised, applied := stepReasonRevise(context.Background(), sc, draft, decision)
	if applied {
		t.Errorf("expected applied=false when revision has no actions")
	}
	if revised.Actions[0].CapabilityID != "kept" {
		t.Errorf("expected draft preserved when revision is empty")
	}
}

func TestStepReasonRevise_HonorsConfidenceFloor(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{
		{Content: `{"actions":[{"capability":"x"}],"confidence":0.3,"reasoning":"low"}`, Confidence: 0.3},
	}}
	sc := newModifyContext(agent)
	draft := plan{Actions: []plannedAction{{CapabilityID: "x"}}, Confidence: 0.2}
	decision := corecheckpoint.Decision{
		Choice: "modify",
		Note:   "go ahead but conservatively",
		Modifications: &corecheckpoint.Modifications{
			RevisedConfidence: ptr(0.75),
		},
	}
	revised, applied := stepReasonRevise(context.Background(), sc, draft, decision)
	if !applied {
		t.Fatalf("expected revise to be applied")
	}
	if revised.Confidence != 0.75 {
		t.Errorf("expected confidence floored to 0.75, got %v", revised.Confidence)
	}
}

func TestStepReasonRevise_NonModifyChoiceIsNoop(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{
		{Content: "should not be called"},
	}}
	sc := newModifyContext(agent)
	draft := plan{Actions: []plannedAction{{CapabilityID: "kept"}}}
	for _, choice := range []string{"approve", "reject", ""} {
		t.Run("choice="+choice, func(t *testing.T) {
			_, applied := stepReasonRevise(context.Background(), sc, draft, corecheckpoint.Decision{Choice: choice, Note: "x"})
			if applied {
				t.Errorf("expected applied=false for choice=%q", choice)
			}
		})
	}
	if agent.calls != 0 {
		t.Errorf("agent must not be called for non-modify choices, got %d calls", agent.calls)
	}
}
