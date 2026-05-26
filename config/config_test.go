package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEnsureGitHubToken_KeepsExistingValue(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "preexisting-token")
	ensureGitHubToken()
	if got := os.Getenv("GITHUB_TOKEN"); got != "preexisting-token" {
		t.Errorf("expected env var to remain 'preexisting-token', got %q", got)
	}
}

func TestEnsureGitHubToken_NoopWhenGhMissing(t *testing.T) {
	// Hide `gh` from PATH so exec.Command("gh", ...) fails. The function must
	// swallow that and leave GITHUB_TOKEN untouched — the github tool adapter
	// will surface the missing token at its own startup check, not here.
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("PATH", "/dev/null") // empty PATH
	ensureGitHubToken()
	if got := os.Getenv("GITHUB_TOKEN"); got != "" {
		t.Errorf("expected env var to stay empty when gh is unavailable, got %q", got)
	}
}

func TestEnsureGitHubToken_PicksUpFromStubGh(t *testing.T) {
	// Write a stub `gh` executable that prints a fixed token, then put its
	// dir at the front of PATH. The function should invoke it and populate
	// GITHUB_TOKEN with the trimmed output.
	dir := t.TempDir()
	stub := filepath.Join(dir, "gh")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\necho '   stubbed-gh-token   '\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	// Sanity: confirm the stub is executable in this shell.
	if _, err := exec.LookPath(stub); err != nil {
		t.Skipf("stub not executable in this environment: %v", err)
	}

	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("PATH", dir)

	ensureGitHubToken()
	if got := os.Getenv("GITHUB_TOKEN"); got != "stubbed-gh-token" {
		t.Errorf("expected token from stub gh (trimmed), got %q", got)
	}
}
