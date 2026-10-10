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

// loadYAML writes body to a temp file and loads it. The token is set so Load
// does not shell out to gh.
func loadYAML(t *testing.T, body string) *Config {
	t.Helper()
	t.Setenv("GITHUB_TOKEN", "preexisting-token")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

// Config sets no default for the audit retention: the floor is declared in
// internal/feature/audit and resolved at startup.
func TestLoad_AuditRetentionAbsentIsZero(t *testing.T) {
	cfg := loadYAML(t, "executor: local\n")
	if got := cfg.Audit.Retention; got.FloorDays != 0 || got.Days != 0 {
		t.Fatalf("Audit.Retention = %+v, want zero values", got)
	}
}

func TestLoad_AuditRetentionParsed(t *testing.T) {
	cfg := loadYAML(t, "audit:\n  retention:\n    floor_days: 365\n    days: 400\n")
	if got := cfg.Audit.Retention; got.FloorDays != 365 || got.Days != 400 {
		t.Fatalf("Audit.Retention = %+v, want FloorDays 365, Days 400", got)
	}
}

// Config does not validate: a value below the floor loads as written, and the
// refusal is internal/app's.
func TestLoad_AuditRetentionBelowFloorIsNotRejected(t *testing.T) {
	cfg := loadYAML(t, "audit:\n  retention:\n    floor_days: 100\n")
	if got := cfg.Audit.Retention; got.FloorDays != 100 || got.Days != 0 {
		t.Fatalf("Audit.Retention = %+v, want FloorDays 100, Days 0", got)
	}
	cfg = loadYAML(t, "audit:\n  retention:\n    days: 30\n")
	if got := cfg.Audit.Retention; got.FloorDays != 0 || got.Days != 30 {
		t.Fatalf("Audit.Retention = %+v, want FloorDays 0, Days 30", got)
	}
}

// A secret under tools.observability is referenced by env var name, like every
// other slot's.
func TestLoad_ObservabilityInstanceEnvRefResolved(t *testing.T) {
	t.Setenv("ACME_OBS_API_KEY", "obs-secret")
	cfg := loadYAML(t, "tools:\n  observability:\n    default: acme\n    instances:\n      acme:\n        type: some_backend\n        api_key_env: ACME_OBS_API_KEY\n")
	inst, ok := cfg.Tools.Observability.Instances["acme"]
	if !ok {
		t.Fatalf("Tools.Observability.Instances = %+v, want an acme instance", cfg.Tools.Observability.Instances)
	}
	if got := inst.OptString("api_key"); got != "obs-secret" {
		t.Fatalf("api_key = %q, want %q", got, "obs-secret")
	}
}

// default.yaml ships the tools.observability slot empty; the top-level
// observability section is the telemetry exporters and is a different thing.
func TestLoad_DefaultYAMLObservabilitySlotEmpty(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "preexisting-token")
	cfg, err := Load("default.yaml")
	if err != nil {
		t.Fatalf("Load default.yaml: %v", err)
	}
	got := cfg.Tools.Observability
	if got.Default != "" || len(got.Instances) != 0 {
		t.Fatalf("Tools.Observability = %+v, want default \"\" and no instances", got)
	}
	if len(cfg.Observability.Exporters) == 0 {
		t.Fatalf("Observability.Exporters is empty, want the telemetry exporters untouched")
	}
}

// Checkpoint expiry (Phase 35) is off until an operator sets it: neither the
// built-in defaults nor the shipped default.yaml may give a checkpoint a time
// after which it is rejected.
func TestDefault_CheckpointTTLIsOff(t *testing.T) {
	if got := Default().Reconcile.CheckpointTTL; got != "" {
		t.Errorf("Default().Reconcile.CheckpointTTL = %q, want empty (off)", got)
	}
	cfg, err := Load("default.yaml")
	if err != nil {
		t.Fatalf("Load default.yaml: %v", err)
	}
	if got := cfg.Reconcile.CheckpointTTL; got != "" {
		t.Errorf("default.yaml reconcile.checkpoint_ttl = %q, want unset (off)", got)
	}
}
