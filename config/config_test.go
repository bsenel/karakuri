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

// YAML decodes an inline list into []any and a map into map[string]any, so
// both shapes are read; a non-string entry is dropped rather than stringified.
func TestOptStringsAndOptStringMap(t *testing.T) {
	inst := InstanceConfig{Options: map[string]any{
		"allowed_tools": []any{"read_file", 3, "list_directory"},
		"typed":         []string{"a"},
		"env":           map[string]any{"NODE_ENV": "production", "PORT": 8080},
		"headers":       map[string]string{"X-Tenant": "acme"},
	}}

	if got := inst.OptStrings("allowed_tools"); len(got) != 2 || got[0] != "read_file" || got[1] != "list_directory" {
		t.Errorf("OptStrings(allowed_tools) = %v", got)
	}
	if got := inst.OptStrings("typed"); len(got) != 1 || got[0] != "a" {
		t.Errorf("OptStrings(typed) = %v", got)
	}
	if got := inst.OptStrings("missing"); got != nil {
		t.Errorf("OptStrings(missing) = %v, want nil", got)
	}
	if got := inst.OptStringMap("env"); len(got) != 1 || got["NODE_ENV"] != "production" {
		t.Errorf("OptStringMap(env) = %v", got)
	}
	if got := inst.OptStringMap("headers"); got["X-Tenant"] != "acme" {
		t.Errorf("OptStringMap(headers) = %v", got)
	}
	if got := inst.OptStringMap("missing"); got != nil {
		t.Errorf("OptStringMap(missing) = %v, want nil", got)
	}
}
