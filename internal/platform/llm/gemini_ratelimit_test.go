package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"

	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
)

// The shapes a 429 arrives in from the Gemini API. The genai SDK reports it as
// googleapi text holding the status, and googleai.MapError (errors.go:22-26)
// turns text holding "429", "rate limit" or "quota exceeded" into an
// *llms.Error with ErrCodeRateLimit. Neither keeps a Retry-After value.
func geminiRateLimitShapes() map[string]error {
	raw := errors.New("googleapi: Error 429: Resource has been exhausted (e.g. check quota).")
	return map[string]error{
		"status text":  raw,
		"mapped error": llms.NewError(llms.ErrCodeRateLimit, "googleai", "Rate limit exceeded").WithCause(raw),
		"wrapped":      fmt.Errorf("googleai: generate content: %w", raw),
	}
}

func TestGeminiCompleteClassifiesARateLimit(t *testing.T) {
	for name, in := range geminiRateLimitShapes() {
		t.Run(name, func(t *testing.T) {
			g := &GeminiProvider{model: failingModel{err: in}}

			_, err := g.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}})

			if !errors.Is(err, coreerrors.ErrProviderRateLimited) {
				t.Fatalf("errors.Is(err, ErrProviderRateLimited) = false for %v", err)
			}
			if !strings.Contains(err.Error(), "gemini") {
				t.Errorf("error %q does not name the provider", err)
			}
			var rl *coreerrors.ProviderRateLimitError
			if !errors.As(err, &rl) {
				t.Fatalf("errors.As to *ProviderRateLimitError failed for %v", err)
			}
			if rl.Provider != "gemini" {
				t.Errorf("Provider = %q, want gemini", rl.Provider)
			}
			if rl.RetryAfter != 0 {
				t.Errorf("RetryAfter = %v, want 0: langchaingo's error holds no retry-after", rl.RetryAfter)
			}
		})
	}
}

func TestGeminiCompleteLeavesOtherErrorsUnclassified(t *testing.T) {
	for _, in := range []error{
		errors.New("googleapi: Error 500: Internal error encountered."),
		errors.New("googleapi: Error 403: API key not valid. Please pass a valid API key."),
		llms.NewError(llms.ErrCodeProviderUnavailable, "googleai", "Google AI service temporarily unavailable"),
		context.DeadlineExceeded,
	} {
		g := &GeminiProvider{model: failingModel{err: in}}

		_, err := g.Complete(context.Background(), CompletionRequest{})

		if errors.Is(err, coreerrors.ErrProviderRateLimited) {
			t.Errorf("%v was classified as a rate limit", in)
		}
		if err != in {
			t.Errorf("got %v, want the provider's error returned unchanged (%v)", err, in)
		}
	}
}
