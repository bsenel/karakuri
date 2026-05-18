package llm

import (
	"context"
	"os"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
)

type ClaudeProvider struct {
	model llms.Model
}

func NewClaudeProvider() (*ClaudeProvider, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return &ClaudeProvider{}, nil
	}
	model, err := anthropic.New(
		anthropic.WithModel("claude-sonnet-4-20250514"),
		anthropic.WithToken(apiKey),
	)
	if err != nil {
		return nil, err
	}
	return &ClaudeProvider{model: model}, nil
}

func (c *ClaudeProvider) Name() string { return "claude" }

func (c *ClaudeProvider) Available(_ context.Context) bool {
	return true // mock mode when ANTHROPIC_API_KEY unset
}

func (c *ClaudeProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	if c.model == nil {
		return CompletionResponse{Content: mockResponse(req), TokensUsed: 100}, nil
	}
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

func (c *ClaudeProvider) Model() llms.Model { return c.model }

func lastUserMessage(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	return ""
}

func mockResponse(req CompletionRequest) string {
	return "Generated content for: " + lastUserMessage(req.Messages)
}
