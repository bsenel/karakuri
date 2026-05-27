package llm

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
)

// GeminiProvider wraps Google's Gemini models via LangChain Go's googleai
// client. Auth: GOOGLE_API_KEY (preferred) or GOOGLE_AI_API_KEY env var.
// When unset, the provider falls back to invoking the `gemini` CLI as a
// subprocess if it's on PATH — inherits the CLI's auth (gcloud
// application-default credentials, etc.). When neither path is wired,
// Available() returns false and Complete() returns a clear error.
type GeminiProvider struct {
	model llms.Model
	cli   *geminiCLI // non-nil when the CLI fallback is wired
}

func NewGeminiProvider() *GeminiProvider {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_AI_API_KEY")
	}
	if apiKey != "" {
		model, err := googleai.New(context.Background(),
			googleai.WithAPIKey(apiKey),
			googleai.WithDefaultModel("gemini-1.5-pro"),
		)
		if err == nil {
			return &GeminiProvider{model: model}
		}
	}
	// No API key (or API construction failed) — try the CLI fallback.
	if path, err := exec.LookPath("gemini"); err == nil {
		return &GeminiProvider{cli: newGeminiCLI(path)}
	}
	return &GeminiProvider{}
}

func (g *GeminiProvider) Name() string { return "gemini" }

func (g *GeminiProvider) Available(_ context.Context) bool {
	return g.model != nil || g.cli != nil
}

// UsingCLIFallback reports whether the provider is routing through the
// `gemini` CLI subprocess instead of the API. Bootstrap reads this at
// startup to log the active mode.
func (g *GeminiProvider) UsingCLIFallback() bool {
	return g.model == nil && g.cli != nil
}

func (g *GeminiProvider) AsLLM() llms.Model { return g.model }

func (g *GeminiProvider) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	if g.model != nil {
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
	if g.cli != nil {
		return g.cli.complete(ctx, req)
	}
	return CompletionResponse{}, fmt.Errorf("gemini provider: no API key (GOOGLE_API_KEY / GOOGLE_AI_API_KEY) and `gemini` CLI not on PATH")
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
