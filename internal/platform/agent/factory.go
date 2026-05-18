package agent

import (
	"context"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
)

type Factory struct {
	providers *llm.Registry
	events    *event.Hub
	otel      *observability.OTel
}

func NewFactory(providers *llm.Registry, events *event.Hub, otel *observability.OTel) *Factory {
	return &Factory{providers: providers, events: events, otel: otel}
}

func (f *Factory) New(ctx context.Context, input coreagent.Input) (coreagent.Agent, error) {
	providerName := input.Provider
	if providerName == "" {
		providerName = "claude"
	}
	provider, ok := f.providers.Get(providerName)
	if !ok {
		return nil, context.Canceled
	}
	return &langchainAgent{
		provider: provider, input: input, events: f.events, otel: f.otel, sessionSHA: "",
	}, nil
}

func (f *Factory) NewWithSession(ctx context.Context, sessionSHA string, input coreagent.Input) (coreagent.Agent, error) {
	a, err := f.New(ctx, input)
	if err != nil {
		return nil, err
	}
	if la, ok := a.(*langchainAgent); ok {
		la.sessionSHA = sessionSHA
	}
	return a, nil
}

type langchainAgent struct {
	provider   llm.ProviderAdapter
	input      coreagent.Input
	events     *event.Hub
	otel       *observability.OTel
	sessionSHA string
}

func (a *langchainAgent) Run(ctx context.Context, input coreagent.Input) (coreagent.Output, error) {
	start := time.Now()
	_ = a.events.Publish(ctx, event.Event{
		Type: event.AgentStarted, SessionSHA: a.sessionSHA,
		Payload: map[string]any{"role": input.Role, "provider": a.provider.Name()},
		Timestamp: time.Now().UTC(),
	})
	msgs := append(input.Memory, coreagent.Message{Role: "user", Content: input.UserPrompt})
	resp, err := a.provider.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: input.SystemPrompt,
		Messages:     toLLMMessages(msgs),
		Temperature:  input.Temperature,
		MaxTokens:    4096,
	})
	if err != nil {
		return coreagent.Output{}, err
	}
	a.otel.IncAgentInvocation(input.Role)
	a.otel.ObserveAgentLatency(input.Role, time.Since(start))
	a.otel.RecordTokens(input.Role, resp.TokensUsed)
	return coreagent.Output{Content: resp.Content, ToolCalls: toCoreToolCalls(resp.ToolCalls), TokensUsed: resp.TokensUsed}, nil
}

func (a *langchainAgent) Stream(ctx context.Context, input coreagent.Input) (<-chan coreagent.OutputChunk, error) {
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

func toLLMMessages(msgs []coreagent.Message) []llm.Message {
	out := make([]llm.Message, len(msgs))
	for i, m := range msgs {
		out[i] = llm.Message{Role: m.Role, Content: m.Content}
	}
	return out
}

func toCoreToolCalls(calls []llm.ToolCall) []coreagent.ToolCall {
	out := make([]coreagent.ToolCall, len(calls))
	for i, c := range calls {
		out[i] = coreagent.ToolCall{Name: c.Name, Args: c.Args}
	}
	return out
}
