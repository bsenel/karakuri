package llm

import (
	"context"
	"errors"
	"strings"
	"testing"

	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
)

// The gemini CLI prints plain text, so a rate limit is only what it writes to
// stderr before a non-zero exit.
func TestGeminiCLICompleteClassifiesARateLimit(t *testing.T) {
	for name, body := range map[string]string{
		"stderr 429": `echo "[API Error: got status: 429 Too Many Requests]" 1>&2
exit 1
`,
		"stderr quota": `echo "Quota exceeded for quota metric 'Generate Content API requests per minute'" 1>&2
exit 1
`,
		"stderr rate limit": `echo "rate limit exceeded" 1>&2
exit 1
`,
	} {
		t.Run(name, func(t *testing.T) {
			_, dir := writeStubBinary(t, "gemini", body)
			t.Setenv("GOOGLE_API_KEY", "")
			t.Setenv("GOOGLE_AI_API_KEY", "")
			t.Setenv("PATH", dir)
			p := NewGeminiProvider()

			_, err := p.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}})

			if !errors.Is(err, coreerrors.ErrProviderRateLimited) {
				t.Fatalf("errors.Is(err, ErrProviderRateLimited) = false for %v", err)
			}
			var rl *coreerrors.ProviderRateLimitError
			if !errors.As(err, &rl) {
				t.Fatalf("errors.As to *ProviderRateLimitError failed for %v", err)
			}
			if rl.Provider != "gemini" {
				t.Errorf("Provider = %q, want gemini", rl.Provider)
			}
		})
	}
}

func TestGeminiCLICompleteLeavesOtherErrorsUnclassified(t *testing.T) {
	_, dir := writeStubBinary(t, "gemini", `echo "no project configured" 1>&2
exit 1
`)
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GOOGLE_AI_API_KEY", "")
	t.Setenv("PATH", dir)
	p := NewGeminiProvider()

	_, err := p.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}})

	if err == nil {
		t.Fatalf("expected error from failing CLI")
	}
	if errors.Is(err, coreerrors.ErrProviderRateLimited) {
		t.Errorf("%v was classified as a rate limit", err)
	}
	if !strings.Contains(err.Error(), "no project configured") {
		t.Errorf("error %q does not carry the CLI's stderr", err)
	}
}
