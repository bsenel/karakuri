// Package agent implements AgentFactory using LangChain Go.
// All LangChain Go imports are confined to this package and internal/platform/llm.
package agent

import (
	"context"
	"encoding/json"
	"fmt"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
)

// Factory implements coreagent.Factory.
type Factory struct {
	providers *llm.Registry
	hub       *event.Hub
	otel      *observability.OTel
}

func NewFactory(providers *llm.Registry, hub *event.Hub, otel *observability.OTel) *Factory {
	return &Factory{providers: providers, hub: hub, otel: otel}
}

func (f *Factory) New(ctx context.Context, def coreagent.Definition) (coreagent.Agent, error) {
	providerName := def.LLMHints.PreferredProvider
	if providerName == "" {
		providerName = "claude"
	}
	provider, ok := f.providers.Get(providerName)
	if !ok {
		return nil, fmt.Errorf("provider %q not available", providerName)
	}
	return &karakuriAgent{
		def:      def,
		provider: provider,
		hub:      f.hub,
		otel:     f.otel,
	}, nil
}

type karakuriAgent struct {
	def      coreagent.Definition
	provider llm.ProviderAdapter
	hub      *event.Hub
	otel     *observability.OTel
}

func (a *karakuriAgent) Run(ctx context.Context, input coreagent.Input) (coreagent.Output, error) {
	systemPrompt := buildSystemPrompt(a.def, input)
	userPrompt := buildUserPrompt(input)

	resp, err := a.provider.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: systemPrompt,
		Messages:     []llm.Message{{Role: "user", Content: userPrompt}},
		Temperature:  a.def.LLMHints.TemperatureMax,
		MaxTokens:    8192,
	})
	if err != nil {
		return coreagent.Output{}, err
	}

	a.hub.Publish(ctx, event.Event{
		Type:    event.TypeMemoryLearned,
		Payload: map[string]any{"agent_id": string(a.def.ID), "tokens": resp.TokensUsed},
	})

	return coreagent.Output{
		Content:    resp.Content,
		Confidence: 0.85,
		TokensUsed: resp.TokensUsed,
		Reasoning:  resp.Content,
	}, nil
}

func (a *karakuriAgent) Stream(ctx context.Context, input coreagent.Input) (<-chan coreagent.OutputChunk, error) {
	ch := make(chan coreagent.OutputChunk, 8)
	go func() {
		defer close(ch)
		out, err := a.Run(ctx, input)
		if err != nil {
			ch <- coreagent.OutputChunk{Err: err}
			return
		}
		ch <- coreagent.OutputChunk{Content: out.Content, Done: true}
	}()
	return ch, nil
}

func buildSystemPrompt(def coreagent.Definition, input coreagent.Input) string {
	return fmt.Sprintf(
		"You are %s, an autonomous agent. Your reasoning strategy is %s. "+
			"You operate in the %s domain. "+
			"Complete the assigned task and produce structured output.",
		def.Name, string(def.ReasoningStrategy), def.Domain,
	)
}

func buildUserPrompt(input coreagent.Input) string {
	objJSON, _ := json.Marshal(input.Objective)
	memSummary := ""
	if len(input.Memory) > 0 {
		memSummary = fmt.Sprintf("You have %d prior memory entries relevant to this objective.", len(input.Memory))
	}
	return fmt.Sprintf("Objective: %s\n\nTask: %s\n\n%s",
		string(objJSON), input.Task, memSummary)
}
