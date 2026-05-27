package llm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeStubBinary writes a sh script to a temp dir and returns its path.
// Mirrors the pattern from config/config_test.go (PR #19): isolates the
// test from whatever `claude` binary the host machine may have installed.
func writeStubBinary(t *testing.T, name, body string) (binPath, dir string) {
	t.Helper()
	dir = t.TempDir()
	binPath = filepath.Join(dir, name)
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	if _, err := exec.LookPath(binPath); err != nil {
		t.Skipf("stub not executable in this environment: %v", err)
	}
	return binPath, dir
}

func TestClaudeProvider_RoutesThroughCLIWhenAPIKeyEmpty(t *testing.T) {
	// Stub emits the canonical `claude --print --output-format json` shape.
	// Uses printf (shell builtin) instead of cat so it works with the
	// restricted PATH the tests set up.
	_, dir := writeStubBinary(t, "claude",
		`printf '{"session_id":"sess-1","result":"hello from stub","usage":{"input_tokens":12,"output_tokens":7}}\n'`)

	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("PATH", dir)

	p, err := NewClaudeProvider()
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	if !p.Available(context.Background()) {
		t.Fatalf("expected Available()=true with CLI stub on PATH")
	}
	if !p.UsingCLIFallback() {
		t.Fatalf("expected UsingCLIFallback()=true (no API key, CLI present)")
	}

	resp, err := p.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "hello from stub" {
		t.Errorf("expected content from stub, got %q", resp.Content)
	}
	if resp.TokensUsed != 19 { // 12 + 7
		t.Errorf("expected TokensUsed=19 (sum of usage), got %d", resp.TokensUsed)
	}
}

func TestClaudeProvider_UnavailableWhenAPIKeyAndCLIBothMissing(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("PATH", "/dev/null") // no `claude` resolvable

	p, err := NewClaudeProvider()
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	if p.Available(context.Background()) {
		t.Errorf("expected Available()=false when neither path is wired")
	}
	if p.UsingCLIFallback() {
		t.Errorf("expected UsingCLIFallback()=false when CLI is missing")
	}

	_, err = p.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Errorf("expected error from Complete() when no auth path wired")
	}
}

func TestClaudeProvider_CLIErrorSurfacesStderr(t *testing.T) {
	// Stub exits non-zero with a stderr message — the provider should wrap
	// it so the caller sees what went wrong, not a bare "exit status 1".
	_, dir := writeStubBinary(t, "claude", `echo "auth required" 1>&2
exit 1
`)
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("PATH", dir)

	p, err := NewClaudeProvider()
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	_, err = p.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatalf("expected error from failing CLI")
	}
	if !strings.Contains(err.Error(), "auth required") {
		t.Errorf("expected stderr to be surfaced; got: %v", err)
	}
}

func TestClaudeProvider_RecoversFromBannerBeforeJSON(t *testing.T) {
	// First-run banners can prepend non-JSON lines; the parser should fall
	// back to scanning the last JSON-looking line.
	_, dir := writeStubBinary(t, "claude",
		`printf 'Welcome to Claude Code! Logging in...\n{"session_id":"sess-2","result":"after banner","usage":{"input_tokens":3,"output_tokens":2}}\n'`)
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("PATH", dir)

	p, _ := NewClaudeProvider()
	resp, err := p.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "after banner" {
		t.Errorf("expected banner skip + JSON parse, got %q", resp.Content)
	}
}

func TestClaudeProvider_APIKeySetSkipsCLIFallback(t *testing.T) {
	// When ANTHROPIC_API_KEY is non-empty, the CLI fallback must not be
	// constructed even if `claude` is on PATH — API path always wins.
	_, dir := writeStubBinary(t, "claude", `exit 99`) // would fail if invoked
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")
	t.Setenv("PATH", dir)

	p, err := NewClaudeProvider()
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	if p.UsingCLIFallback() {
		t.Errorf("expected UsingCLIFallback()=false when API key is set")
	}
	if p.AsLLM() == nil {
		t.Errorf("expected langchaingo model to be constructed when API key is set")
	}
}
