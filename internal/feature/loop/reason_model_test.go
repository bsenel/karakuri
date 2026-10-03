package loop

import (
	"context"
	"errors"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/loop"
)

// failingAgent is a model call that never comes back.
type failingAgent struct{}

func (failingAgent) Run(context.Context, coreagent.Input) (coreagent.Output, error) {
	return coreagent.Output{}, errors.New("provider unavailable")
}
func (failingAgent) Stream(context.Context, coreagent.Input) (<-chan coreagent.OutputChunk, error) {
	return nil, nil
}

// The plan remembers who drafted it, so a decision made on it can say which
// model it came from.
func TestPlanKeepsTheProviderAndModelThatProducedIt(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{
		{Content: goodPlan, Confidence: 0.8, Provider: "anthropic", Model: "model-a"},
	}}

	p := stepReason(context.Background(), scriptedReasonContext(t, agent), loop.WorldState{})

	if p.provider != "anthropic" || p.model != "model-a" {
		t.Errorf("provider/model = %q/%q, want anthropic/model-a", p.provider, p.model)
	}
}

// When the retry is what parsed, the retry's call is the one that produced
// the plan. A fallback provider may have served it.
func TestPlanFromARetryNamesTheCallThatParsed(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{
		{Content: "prose, not a plan", Provider: "anthropic", Model: "model-a"},
		{Content: goodPlan, Confidence: 0.8, Provider: "fallback", Model: "model-b"},
	}}

	p := stepReason(context.Background(), scriptedReasonContext(t, agent), loop.WorldState{})

	if p.provider != "fallback" || p.model != "model-b" {
		t.Errorf("provider/model = %q/%q, want fallback/model-b", p.provider, p.model)
	}
}

// A Reflexion revision replaces the draft, so it is the revising call that is
// named.
func TestRevisedPlanNamesTheCallThatRevisedIt(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{
		{Content: goodPlan, Confidence: 0.8, Provider: "anthropic", Model: "model-a"},
		{Content: "the weakest assumption is the test command", Provider: "anthropic", Model: "model-a"},
		{Content: goodPlan, Confidence: 0.9, Provider: "fallback", Model: "model-b"},
	}}
	sc := scriptedReasonContext(t, agent)
	sc.agentDef.ReasoningStrategy = coreagent.ReasoningReflexion

	p := stepReason(context.Background(), sc, loop.WorldState{})

	if p.provider != "fallback" || p.model != "model-b" {
		t.Errorf("provider/model = %q/%q, want fallback/model-b", p.provider, p.model)
	}
}

// The placeholder built when the call fails was produced by no model. It
// says so by saying nothing, never by guessing a name.
func TestPlaceholderPlanAfterAFailedCallNamesNoModel(t *testing.T) {
	p := stepReason(context.Background(), scriptedReasonContext(t, failingAgent{}), loop.WorldState{})

	if len(p.Actions) != 1 || p.Actions[0].CapabilityID != "reason.plan" {
		t.Fatalf("expected the failure placeholder, got %+v", p.Actions)
	}
	if p.provider != "" || p.model != "" {
		t.Errorf("provider/model = %q/%q, want both empty", p.provider, p.model)
	}
}
