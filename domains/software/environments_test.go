package software

import (
	"context"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// TestNoopEnvActFailsHonestly verifies the noop env reports failure
// for any action — the historical "Success: true" lie masked missing
// adapters and let every Phase 13.5 dogfood objective fake-complete.
func TestNoopEnvActFailsHonestly(t *testing.T) {
	e := &noopEnv{id: environment.EnvironmentID("software.env.local")}

	result, err := e.Act(context.Background(), environment.Action{
		CapabilityID: capability.CapabilityID("fs.scaffold"),
	})
	if err != nil {
		t.Fatalf("Act should not return a Go error (failure is encoded in ActionResult), got: %v", err)
	}
	if result.Success {
		t.Errorf("noop env must report Success=false for unimplemented actions, got true")
	}
	if !strings.Contains(result.Error, "fs.scaffold") {
		t.Errorf("expected Error to name the capability, got %q", result.Error)
	}
	if !strings.Contains(result.Error, "no implementation") {
		t.Errorf("expected Error to explain the gap, got %q", result.Error)
	}
	if got, _ := result.StateDelta["status"].(string); got != "unimplemented" {
		t.Errorf("expected StateDelta status=unimplemented, got %v", result.StateDelta["status"])
	}
	if got, _ := result.StateDelta["env_id"].(string); got != "software.env.local" {
		t.Errorf("expected StateDelta env_id to identify the responding env, got %v", result.StateDelta["env_id"])
	}
}
