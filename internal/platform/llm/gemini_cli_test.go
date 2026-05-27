package llm

import (
	"context"
	"strings"
	"testing"
)

func TestGeminiProvider_RoutesThroughCLIWhenAPIKeyEmpty(t *testing.T) {
	// `gemini --prompt` emits plain text on stdout. Stub captures the
	// prompt argument and returns a fixed body so we can assert routing.
	_, dir := writeStubBinary(t, "gemini", `echo "gemini fallback response"`)

	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GOOGLE_AI_API_KEY", "")
	t.Setenv("PATH", dir)

	p := NewGeminiProvider()
	if !p.Available(context.Background()) {
		t.Fatalf("expected Available()=true with CLI stub on PATH")
	}
	if !p.UsingCLIFallback() {
		t.Fatalf("expected UsingCLIFallback()=true")
	}

	resp, err := p.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "gemini fallback response" {
		t.Errorf("expected stub output, got %q", resp.Content)
	}
	// Degraded path documents TokensUsed=0 — no usage data available from
	// plain-text CLI output.
	if resp.TokensUsed != 0 {
		t.Errorf("expected TokensUsed=0 from CLI fallback, got %d", resp.TokensUsed)
	}
}

func TestGeminiProvider_UnavailableWhenAPIKeyAndCLIBothMissing(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GOOGLE_AI_API_KEY", "")
	t.Setenv("PATH", "/dev/null")

	p := NewGeminiProvider()
	if p.Available(context.Background()) {
		t.Errorf("expected Available()=false when neither API key nor CLI present")
	}
	_, err := p.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Errorf("expected error from Complete() when no auth path wired")
	}
}

func TestGeminiProvider_CLIErrorSurfacesStderr(t *testing.T) {
	_, dir := writeStubBinary(t, "gemini", `echo "no project configured" 1>&2
exit 1
`)
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GOOGLE_AI_API_KEY", "")
	t.Setenv("PATH", dir)

	p := NewGeminiProvider()
	_, err := p.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatalf("expected error from failing CLI")
	}
	if !strings.Contains(err.Error(), "no project configured") {
		t.Errorf("expected stderr to be surfaced; got: %v", err)
	}
}
