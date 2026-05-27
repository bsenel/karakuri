package llm

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
)

// ClaudeProvider wraps Anthropic's Claude models via LangChain Go's anthropic
// client. Auth: ANTHROPIC_API_KEY env var. When unset, the provider falls
// back to invoking the `claude` CLI as a subprocess if it's on PATH —
// inherits whatever auth the CLI is configured with (OAuth via /login,
// env-var API key, apiKeyHelper). When neither path is wired, Available()
// returns false and Complete() returns a clear error.
type ClaudeProvider struct {
	model llms.Model
	cli   *claudeCLI // non-nil when the CLI fallback is wired
}

func NewClaudeProvider() (*ClaudeProvider, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey != "" {
		model, err := anthropic.New(
			anthropic.WithModel("claude-sonnet-4-6"),
			anthropic.WithToken(apiKey),
		)
		if err != nil {
			return nil, err
		}
		return &ClaudeProvider{model: model}, nil
	}
	// No API key — try the CLI fallback.
	if path, err := exec.LookPath("claude"); err == nil {
		return &ClaudeProvider{cli: newClaudeCLI(path)}, nil
	}
	// Neither path wired; Available() will report false.
	return &ClaudeProvider{}, nil
}

func (c *ClaudeProvider) Name() string { return "claude" }

func (c *ClaudeProvider) Available(_ context.Context) bool {
	return c.model != nil || c.cli != nil
}

// UsingCLIFallback reports whether the provider is routing through the
// `claude` CLI subprocess instead of the API. Bootstrap reads this to log
// the active mode at startup so operators understand the path their loop
// is taking.
func (c *ClaudeProvider) UsingCLIFallback() bool {
	return c.model == nil && c.cli != nil
}

// AsLLM returns the underlying LangChain Go model when available. The CLI
// fallback path returns nil — callers that strictly need a langchaingo
// model (the agent factory) should check Available() first and surface a
// clear error if no API path is wired.
func (c *ClaudeProvider) AsLLM() llms.Model { return c.model }

func (c *ClaudeProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	if c.model != nil {
		prompt := req.SystemPrompt + "\n\n" + lastUserMessage(req.Messages)
		resp, err := llms.GenerateFromSinglePrompt(ctx, c.model, prompt,
			llms.WithTemperature(req.Temperature),
			llms.WithMaxTokens(req.MaxTokens),
		)
		if err != nil {
			return CompletionResponse{}, err
		}
		return CompletionResponse{Content: resp, TokensUsed: len(resp) / 4}, nil
	}
	if c.cli != nil {
		return c.cli.complete(ctx, req)
	}
	return CompletionResponse{}, fmt.Errorf("claude provider: no API key (ANTHROPIC_API_KEY) and `claude` CLI not on PATH")
}

func (c *ClaudeProvider) Stream(ctx context.Context, req CompletionRequest) (<-chan CompletionChunk, error) {
	ch := make(chan CompletionChunk, 1)
	go func() {
		defer close(ch)
		resp, err := c.Complete(ctx, req)
		if err != nil {
			ch <- CompletionChunk{Err: err}
			return
		}
		ch <- CompletionChunk{Content: resp.Content, Done: true}
	}()
	return ch, nil
}

func lastUserMessage(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	return ""
}
