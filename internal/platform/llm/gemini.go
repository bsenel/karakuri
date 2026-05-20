package llm

import (
	"context"
	"os"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"

	"github.com/bsenel/karakuri/internal/core/errors"
)

// GeminiProvider wraps Google's Gemini models via LangChain Go's googleai client.
// Auth: GOOGLE_API_KEY (preferred) or GOOGLE_AI_API_KEY env var. When neither is
// set, the provider degrades to mock responses so loops still complete in dev.
type GeminiProvider struct {
	model llms.Model
}

func NewGeminiProvider() *GeminiProvider {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_AI_API_KEY")
	}
	if apiKey == "" {
		return &GeminiProvider{}
	}
	model, err := googleai.New(context.Background(),
		googleai.WithAPIKey(apiKey),
		googleai.WithDefaultModel("gemini-1.5-pro"),
	)
	if err != nil {
		return &GeminiProvider{}
	}
	return &GeminiProvider{model: model}
}

func (g *GeminiProvider) Name() string { return "gemini" }

func (g *GeminiProvider) Available(_ context.Context) bool { return g.model != nil }

func (g *GeminiProvider) AsLLM() llms.Model { return g.model }

func (g *GeminiProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	if g.model == nil {
		return CompletionResponse{}, errors.ErrNotImplemented
	}
	prompt := req.SystemPrompt + "\n\n" + lastUserMessage(req.Messages)
	opts := []llms.CallOption{llms.WithTemperature(req.Temperature)}
	if req.MaxTokens > 0 {
		opts = append(opts, llms.WithMaxTokens(req.MaxTokens))
	}
	resp, err := llms.GenerateFromSinglePrompt(ctx, g.model, prompt, opts...)
	if err != nil {
		return CompletionResponse{}, err
	}
	return CompletionResponse{Content: resp, TokensUsed: len(resp) / 4}, nil
}

func (g *GeminiProvider) Stream(ctx context.Context, req CompletionRequest) (<-chan CompletionChunk, error) {
	ch := make(chan CompletionChunk, 1)
	go func() {
		defer close(ch)
		resp, err := g.Complete(ctx, req)
		if err != nil {
			ch <- CompletionChunk{Err: err}
			return
		}
		ch <- CompletionChunk{Content: resp.Content, Done: true}
	}()
	return ch, nil
}
