package llm

import (
	"context"
	"errors"
	"strings"
	"testing"

	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
)

// The CLI fallback sees a rate limit only as what the subprocess prints: text
// on stderr with a non-zero exit, or a JSON reply with is_error set. Those are
// the stubs below.
func TestClaudeCLICompleteClassifiesARateLimit(t *testing.T) {
	for name, body := range map[string]string{
		"stderr 429": `echo 'API Error: 429 {"type":"error","error":{"type":"rate_limit_error","message":"Number of requests has exceeded your rate limit"}}' 1>&2
exit 1
`,
		"stderr usage limit": `echo "Claude AI usage limit reached" 1>&2
exit 1
`,
		"is_error reply": `printf '{"is_error":true,"error":"rate limit exceeded, please try again later"}\n'`,
	} {
		t.Run(name, func(t *testing.T) {
			_, dir := writeStubBinary(t, "claude", body)
			t.Setenv("ANTHROPIC_API_KEY", "")
			t.Setenv("PATH", dir)
			p, err := NewClaudeProvider()
			if err != nil {
				t.Fatalf("constructor: %v", err)
			}

			_, err = p.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}})

			if !errors.Is(err, coreerrors.ErrProviderRateLimited) {
				t.Fatalf("errors.Is(err, ErrProviderRateLimited) = false for %v", err)
			}
			var rl *coreerrors.ProviderRateLimitError
			if !errors.As(err, &rl) {
				t.Fatalf("errors.As to *ProviderRateLimitError failed for %v", err)
			}
			if rl.Provider != "claude" {
				t.Errorf("Provider = %q, want claude", rl.Provider)
			}
		})
	}
}

func TestClaudeCLICompleteLeavesOtherErrorsUnclassified(t *testing.T) {
	for name, tc := range map[string]struct{ body, want string }{
		"stderr auth": {body: `echo "auth required" 1>&2
exit 1
`, want: "auth required"},
		"is_error reply": {body: `printf '{"is_error":true,"error":"prompt is too long"}\n'`, want: "prompt is too long"},
	} {
		t.Run(name, func(t *testing.T) {
			_, dir := writeStubBinary(t, "claude", tc.body)
			t.Setenv("ANTHROPIC_API_KEY", "")
			t.Setenv("PATH", dir)
			p, err := NewClaudeProvider()
			if err != nil {
				t.Fatalf("constructor: %v", err)
			}

			_, err = p.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}})

			if err == nil {
				t.Fatalf("expected error from failing CLI")
			}
			if errors.Is(err, coreerrors.ErrProviderRateLimited) {
				t.Errorf("%v was classified as a rate limit", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not carry the CLI's message %q", err, tc.want)
			}
		})
	}
}
