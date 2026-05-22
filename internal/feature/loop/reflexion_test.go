package loop

import (
	"context"
	"strings"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// scriptedAgent returns predetermined responses in sequence. Tests of the
// reflexion pass need an agent that returns "critique" on call 1 and
// "revised plan JSON" on call 2.
type scriptedAgent struct {
	calls    int
	scripted []coreagent.Output
	tasksSeen []string
}

func (s *scriptedAgent) Run(_ context.Context, in coreagent.Input) (coreagent.Output, error) {
	s.tasksSeen = append(s.tasksSeen, in.Task)
	if s.calls >= len(s.scripted) {
		s.calls++
		return coreagent.Output{}, nil
	}
	out := s.scripted[s.calls]
	s.calls++
	return out, nil
}
func (s *scriptedAgent) Stream(_ context.Context, _ coreagent.Input) (<-chan coreagent.OutputChunk, error) {
	return nil, nil
}

func newReflexionContext(a coreagent.Agent) *stepContext {
	return &stepContext{
		agent: a,
		agentDef: coreagent.Definition{
			ID:                "test-agent",
			ReasoningStrategy: coreagent.ReasoningReflexion,
		},
		obj: objective.Objective{Title: "test objective"},
	}
}

func TestReflexionPass_AppliesRevision(t *testing.T) {
	agent := &scriptedAgent{
		scripted: []coreagent.Output{
			{Content: "The draft misses an integration test step.", Confidence: 0.9},
			{Content: `{"actions":[{"capability":"run_tests","reason":"integration coverage"}],"confidence":0.85,"reasoning":"refined"}`},
		},
	}
	sc := newReflexionContext(agent)

	draft := plan{
		Actions:    []plannedAction{{CapabilityID: "lint", Reason: "syntax"}},
		Confidence: 0.5,
	}

	revised, critique, ok := reflexionPass(context.Background(), sc, draft)
	if !ok {
		t.Fatalf("expected reflexion to succeed, got ok=false")
	}
	if revised.Actions[0].CapabilityID != "run_tests" {
		t.Errorf("expected revised plan to use run_tests, got %s", revised.Actions[0].CapabilityID)
	}
	if !strings.Contains(critique, "integration test") {
		t.Errorf("expected critique to be returned, got %q", critique)
	}
	if agent.calls != 2 {
		t.Errorf("expected exactly 2 agent calls (critique + revise), got %d", agent.calls)
	}
}

func TestReflexionPass_FallsBackOnUnparseableRevision(t *testing.T) {
	agent := &scriptedAgent{
		scripted: []coreagent.Output{
			{Content: "Missing tests."},
			{Content: "this is not json at all"},
		},
	}
	sc := newReflexionContext(agent)
	draft := plan{
		Actions:    []plannedAction{{CapabilityID: "lint"}},
		Confidence: 0.5,
	}
	revised, _, ok := reflexionPass(context.Background(), sc, draft)
	if ok {
		t.Errorf("expected ok=false when revision is not parseable JSON")
	}
	if len(revised.Actions) != 1 || revised.Actions[0].CapabilityID != "lint" {
		t.Errorf("expected draft to be preserved, got %+v", revised)
	}
}

func TestReflexionPass_EmptyCritiqueAborts(t *testing.T) {
	agent := &scriptedAgent{
		scripted: []coreagent.Output{
			{Content: ""},
		},
	}
	sc := newReflexionContext(agent)
	draft := plan{Actions: []plannedAction{{CapabilityID: "lint"}}}
	_, _, ok := reflexionPass(context.Background(), sc, draft)
	if ok {
		t.Errorf("expected ok=false when critique is empty")
	}
	if agent.calls != 1 {
		t.Errorf("expected only 1 agent call (no revision attempted), got %d", agent.calls)
	}
}

func TestReflexionPass_EmptyActionsRevertsToDraft(t *testing.T) {
	agent := &scriptedAgent{
		scripted: []coreagent.Output{
			{Content: "Has problems"},
			{Content: `{"actions":[],"confidence":0.6}`},
		},
	}
	sc := newReflexionContext(agent)
	draft := plan{Actions: []plannedAction{{CapabilityID: "lint"}}, Confidence: 0.5}
	revised, _, ok := reflexionPass(context.Background(), sc, draft)
	if ok {
		t.Errorf("expected ok=false when revision has no actions")
	}
	if revised.Actions[0].CapabilityID != "lint" {
		t.Errorf("expected draft preserved when revision is empty")
	}
}
