package agent

import "context"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ToolCall struct {
	Name   string         `json:"name"`
	Args   map[string]any `json:"args,omitempty"`
	Result string         `json:"result,omitempty"`
}

type Input struct {
	Role         string    `json:"role"`
	SystemPrompt string    `json:"system_prompt"`
	UserPrompt   string    `json:"user_prompt"`
	Tools        []string  `json:"tools,omitempty"`
	Memory       []Message `json:"memory,omitempty"`
	Temperature  float64   `json:"temperature"`
	WorkDir      string    `json:"work_dir,omitempty"`
	Provider     string    `json:"provider,omitempty"`
}

type Output struct {
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	TokensUsed int        `json:"tokens_used"`
}

type OutputChunk struct {
	Content  string
	ToolCall *ToolCall
	Done     bool
	Err      error
}

type Agent interface {
	Run(ctx context.Context, input Input) (Output, error)
	Stream(ctx context.Context, input Input) (<-chan OutputChunk, error)
}

type AgentFactory interface {
	New(ctx context.Context, input Input) (Agent, error)
}
