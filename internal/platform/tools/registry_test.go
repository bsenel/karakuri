package tools

import (
	"testing"

	"github.com/bsenel/karakuri/config"
)

func TestNewRegistry_EmptySlots(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.VC.Resolve(""); ok {
		t.Errorf("empty VC slot should not resolve")
	}
	if _, ok := r.Email.Resolve("anything"); ok {
		t.Errorf("empty Email slot should not resolve")
	}
}

func TestSlotInstances_ResolveDefault(t *testing.T) {
	cfg := config.SlotConfig{
		Default: "acme",
		Instances: map[string]config.InstanceConfig{
			"acme":     {Type: "github", Options: map[string]any{"token": "ghp_a", "repo": "acme/api"}},
			"personal": {Type: "github", Options: map[string]any{"token": "ghp_p", "repo": "bsenel/x"}},
		},
	}
	slot := buildVCSlot(cfg)

	// Empty name → default
	def, ok := slot.Resolve("")
	if !ok || def.Name() != "github" {
		t.Errorf("expected default github, got ok=%t name=%v", ok, def)
	}
	// Named → specific
	personal, ok := slot.Resolve("personal")
	if !ok || personal.Name() != "github" {
		t.Errorf("expected personal github, got ok=%t name=%v", ok, personal)
	}
	// Unknown → false
	if _, ok := slot.Resolve("nonexistent"); ok {
		t.Errorf("unknown instance should not resolve")
	}
}

func TestNewRegistryFromConfig_MultiInstance(t *testing.T) {
	cfg := config.ToolsConfig{
		VersionControl: config.SlotConfig{
			Default: "acme",
			Instances: map[string]config.InstanceConfig{
				"acme":     {Type: "github", Options: map[string]any{"token": "ghp_a", "repo": "acme/api"}},
				"personal": {Type: "github", Options: map[string]any{"token": "ghp_p", "repo": "bsenel/x"}},
			},
		},
		Email: config.SlotConfig{
			Default: "acme_outlook",
			Instances: map[string]config.InstanceConfig{
				"acme_outlook":   {Type: "outlook", Options: map[string]any{"oauth_token": "eyJ", "from_address": "bot@acme.com"}},
				"personal_gmail": {Type: "gmail", Options: map[string]any{"oauth_token": "ya29", "from_address": "me@x.com"}},
			},
		},
	}
	r := NewRegistryFromConfig(cfg)

	// Both VC instances are present.
	if _, ok := r.VC.Resolve("acme"); !ok {
		t.Errorf("expected acme VC instance present")
	}
	if _, ok := r.VC.Resolve("personal"); !ok {
		t.Errorf("expected personal VC instance present")
	}
	// Email: gmail and outlook coexist.
	em1, ok1 := r.Email.Resolve("acme_outlook")
	em2, ok2 := r.Email.Resolve("personal_gmail")
	if !ok1 || em1.Name() != "outlook" {
		t.Errorf("expected acme_outlook → outlook, got ok=%t name=%v", ok1, em1)
	}
	if !ok2 || em2.Name() != "gmail" {
		t.Errorf("expected personal_gmail → gmail, got ok=%t name=%v", ok2, em2)
	}
}

func TestRegistryStatus_ListsAllInstances(t *testing.T) {
	cfg := config.ToolsConfig{
		VersionControl: config.SlotConfig{
			Default: "acme",
			Instances: map[string]config.InstanceConfig{
				"acme":     {Type: "github", Options: map[string]any{"token": "ghp_a"}},
				"personal": {Type: "github", Options: map[string]any{"token": "ghp_p"}},
			},
		},
		Email: config.SlotConfig{
			Instances: map[string]config.InstanceConfig{
				"corp": {Type: "smtp", Options: map[string]any{"host": "smtp.acme.com", "username": "bot", "password": "x", "port": 587}},
			},
		},
	}
	r := NewRegistryFromConfig(cfg)
	status := r.Status()

	vcCount, emailCount := 0, 0
	hasAcmeDefault := false
	for _, s := range status {
		if s.Slot == "versioncontrol" {
			vcCount++
			if s.Instance == "acme" && s.IsDefault {
				hasAcmeDefault = true
			}
		}
		if s.Slot == "email" {
			emailCount++
		}
	}
	if vcCount != 2 {
		t.Errorf("expected 2 versioncontrol rows, got %d", vcCount)
	}
	if emailCount != 1 {
		t.Errorf("expected 1 email row, got %d", emailCount)
	}
	if !hasAcmeDefault {
		t.Errorf("expected acme to be marked as default")
	}
}

func TestEmptySlotShowsNoopInStatus(t *testing.T) {
	r := NewRegistryFromConfig(config.ToolsConfig{})
	status := r.Status()
	for _, s := range status {
		if s.Slot == "versioncontrol" {
			if s.Instance != "<noop>" || s.Active {
				t.Errorf("empty VC slot should show <noop>+inactive, got %+v", s)
			}
		}
	}
}

func TestUnknownInstanceType_LoggedAndSkipped(t *testing.T) {
	cfg := config.SlotConfig{
		Default: "x",
		Instances: map[string]config.InstanceConfig{
			"x": {Type: "weird_provider", Options: map[string]any{}},
		},
	}
	slot := buildVCSlot(cfg)
	if _, ok := slot.Resolve("x"); ok {
		t.Errorf("unknown type should not produce an adapter")
	}
}

func TestNewRegistryFromConfig_CLIAgents(t *testing.T) {
	cfg := config.ToolsConfig{
		CLIAgents: config.SlotConfig{
			Default: "acme_claude",
			Instances: map[string]config.InstanceConfig{
				"acme_claude":  {Type: "claude_code"},
				"acme_cursor":  {Type: "cursor_cli"},
				"acme_gemini":  {Type: "gemini_cli"},
				"acme_copilot": {Type: "copilot_cli"},
			},
		},
	}
	r := NewRegistryFromConfig(cfg)
	want := map[string]string{
		"acme_claude":  "claude_code",
		"acme_cursor":  "cursor_cli",
		"acme_gemini":  "gemini_cli",
		"acme_copilot": "copilot_cli",
	}
	for name, expected := range want {
		a, ok := r.CLIAgents.Resolve(name)
		if !ok {
			t.Errorf("expected instance %s to resolve", name)
			continue
		}
		if a.Name() != expected {
			t.Errorf("instance %s: expected name %s, got %s", name, expected, a.Name())
		}
	}
	// Default resolves to claude_code.
	def, ok := r.CLIAgents.Resolve("")
	if !ok || def.Name() != "claude_code" {
		t.Errorf("expected default = claude_code, got ok=%t name=%v", ok, def)
	}
}

func TestCLIAgentStatusShape(t *testing.T) {
	cfg := config.ToolsConfig{
		CLIAgents: config.SlotConfig{
			Default: "primary",
			Instances: map[string]config.InstanceConfig{
				"primary": {Type: "claude_code"},
			},
		},
	}
	r := NewRegistryFromConfig(cfg)
	found := false
	for _, s := range r.Status() {
		if s.Slot == "cli_agents" && s.Instance == "primary" && s.Type == "claude_code" && s.IsDefault {
			found = true
		}
	}
	if !found {
		t.Errorf("cli_agents primary instance not surfaced in Status()")
	}
}

func TestInstanceOptString_AndOptInt(t *testing.T) {
	inst := config.InstanceConfig{
		Options: map[string]any{
			"host":  "smtp.example.com",
			"port":  587,
			"other": 1.5,
		},
	}
	if got := inst.OptString("host"); got != "smtp.example.com" {
		t.Errorf("OptString host: got %q", got)
	}
	if got := inst.OptString("missing"); got != "" {
		t.Errorf("OptString missing: expected empty, got %q", got)
	}
	if got := inst.OptInt("port"); got != 587 {
		t.Errorf("OptInt port: got %d", got)
	}
	if got := inst.OptInt("missing"); got != 0 {
		t.Errorf("OptInt missing: expected 0, got %d", got)
	}
}
