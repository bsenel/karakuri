package llm

import (
	"context"

	"github.com/tmc/langchaingo/llms"
)

type Message struct {
	Role    string
	Content string
}

type ToolSchema struct {
	Name        string
	Description string
}

type ToolCall struct {
	Name string
	Args map[string]any
}

type CompletionRequest struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolSchema
	Temperature  float64
	MaxTokens    int
}

type CompletionResponse struct {
	Content    string
	ToolCalls  []ToolCall
	TokensUsed int
}

type CompletionChunk struct {
	Content string
	Done    bool
	Err     error
}

type ProviderAdapter interface {
	Name() string
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
	Stream(ctx context.Context, req CompletionRequest) (<-chan CompletionChunk, error)
	Available(ctx context.Context) bool
	// AsLLM returns the underlying LangChain Go model; only consumed within internal/platform/agent.
	AsLLM() llms.Model
}
