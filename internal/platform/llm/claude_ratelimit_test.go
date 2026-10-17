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

// failingModel is a langchaingo model whose every call fails with err.
type failingModel struct{ err error }

func (f failingModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	return nil, f.err
}

func (f failingModel) Call(context.Context, string, ...llms.CallOption) (string, error) {
	return "", f.err
}

// The shapes a 429 arrives in. langchaingo v0.1.14's anthropic client turns
// any non-200 into text holding the status code (anthropicclient.go:221-229),
// and anthropic.MapError (errors.go:61-80) turns that text into an *llms.Error
// with ErrCodeRateLimit. Neither keeps the Retry-After header.
func rateLimitShapes() map[string]error {
	raw := errors.New("API returned unexpected status code: 429: This request would exceed your organization's rate limit")
	return map[string]error{
		"status text":  raw,
		"mapped error": llms.NewError(llms.ErrCodeRateLimit, "anthropic", "Rate limit exceeded").WithCause(raw),
		"wrapped":      fmt.Errorf("anthropic: failed to create message: %w", raw),
	}
}

func TestClaudeCompleteClassifiesARateLimit(t *testing.T) {
	for name, in := range rateLimitShapes() {
		t.Run(name, func(t *testing.T) {
			c := &ClaudeProvider{model: failingModel{err: in}}

			_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}})

			if !errors.Is(err, coreerrors.ErrProviderRateLimited) {
				t.Fatalf("errors.Is(err, ErrProviderRateLimited) = false for %v", err)
			}
			if !strings.Contains(err.Error(), "claude") {
				t.Errorf("error %q does not name the provider", err)
			}
			var rl *coreerrors.ProviderRateLimitError
			if !errors.As(err, &rl) {
				t.Fatalf("errors.As to *ProviderRateLimitError failed for %v", err)
			}
			if rl.Provider != "claude" {
				t.Errorf("Provider = %q, want claude", rl.Provider)
			}
			// langchaingo drops the Retry-After header, so there is none to
			// carry; a guessed wait would be a false record.
			if rl.RetryAfter != 0 {
				t.Errorf("RetryAfter = %v, want 0: langchaingo's error holds no retry-after", rl.RetryAfter)
			}
		})
	}
}

func TestClaudeCompleteLeavesOtherErrorsUnclassified(t *testing.T) {
	for _, in := range []error{
		errors.New("API returned unexpected status code: 500: internal server error"),
		errors.New("API returned unexpected status code: 401: invalid x-api-key"),
		llms.NewError(llms.ErrCodeProviderUnavailable, "anthropic", "Anthropic service temporarily unavailable"),
		context.DeadlineExceeded,
	} {
		c := &ClaudeProvider{model: failingModel{err: in}}

		_, err := c.Complete(context.Background(), CompletionRequest{})

		if errors.Is(err, coreerrors.ErrProviderRateLimited) {
			t.Errorf("%v was classified as a rate limit", in)
		}
		if err != in {
			t.Errorf("got %v, want the provider's error returned unchanged (%v)", err, in)
		}
	}
}
