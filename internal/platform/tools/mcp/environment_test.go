package mcp

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

func connectedEnv(t *testing.T) (*Instance, *Environment) {
	t.Helper()
	inst := NewInstance(context.Background(), "acme_files", stdioConfig("read_file", "fail_tool"))
	t.Cleanup(func() { _ = inst.Close() })
	if inst.State() != StateConnected {
		t.Fatalf("fake server did not connect: %s", inst.Health(false).Error)
	}
	return inst, NewEnvironment(inst)
}

// Untrusted by construction: every result is somebody else's writing, whether
// the tool succeeded, failed, or was refused before it ran.
func TestEveryActResultIsThirdParty(t *testing.T) {
	_, env := connectedEnv(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name    string
		id      capability.CapabilityID
		success bool
		errText string
	}{
		{"success", capability.MCPCapabilityID("acme_files", "read_file"), true, ""},
		{"tool failed", capability.MCPCapabilityID("acme_files", "fail_tool"), false, "the tool broke"},
		{"another instance", capability.MCPCapabilityID("other_files", "read_file"), false, "other_files"},
		{"not MCP", "software.act.run_tests", false, "not a discovered MCP tool"},
		{"off the allowlist", capability.MCPCapabilityID("acme_files", "delete_repo"), false, "allowlist"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := env.Act(ctx, environment.Action{CapabilityID: tc.id, Params: map[string]any{"path": "a"}})
			if err != nil {
				t.Fatalf("Act returned a Go error: %v", err)
			}
			if res.Trust != environment.TrustThirdParty {
				t.Errorf("trust = %q, want third party", res.Trust)
			}
			if res.Success != tc.success {
				t.Errorf("success = %v, want %v (error %q)", res.Success, tc.success, res.Error)
			}
			if tc.errText != "" && !strings.Contains(res.Error, tc.errText) {
				t.Errorf("error = %q, want it to mention %q", res.Error, tc.errText)
			}
		})
	}
}

func TestActOutputCarriesTheToolsText(t *testing.T) {
	_, env := connectedEnv(t)
	res, _ := env.Act(context.Background(), environment.Action{
		CapabilityID: capability.MCPCapabilityID("acme_files", "read_file"),
		Params:       map[string]any{"path": "go.mod"},
	})
	if res.StateDelta["output"] != "contents of go.mod" || res.StateDelta["tool"] != "read_file" {
		t.Errorf("state delta = %v", res.StateDelta)
	}
}

// Advertised descriptions reach the planner, so an observation carrying them is
// third party; one carrying none stays trusted.
func TestObserveMarksAdvertisedToolsAsThirdParty(t *testing.T) {
	_, env := connectedEnv(t)
	obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if obs.Trust != environment.TrustThirdParty {
		t.Errorf("trust = %q with tools advertised, want third party", obs.Trust)
	}

	empty := NewEnvironment(NewInstance(context.Background(), "nothing", stdioConfig()))
	t.Cleanup(func() { _ = empty.inst.Close() })
	obs, _ = empty.Observe(context.Background(), environment.ObservationQuery{})
	if obs.Trust != environment.TrustOperator {
		t.Errorf("trust = %q with nothing advertised, want operator", obs.Trust)
	}
}

func TestEnvironmentIsAToolSourceInTheReservedDomain(t *testing.T) {
	inst, env := connectedEnv(t)
	if env.ID() != "mcp.env.acme_files" || env.Domain() != capability.MCPDomain {
		t.Errorf("env %q in %q", env.ID(), env.Domain())
	}
	var src environment.ToolSource = env
	if !reflect.DeepEqual(src.ProvidedCapabilities(), inst.CapabilityIDs()) {
		t.Errorf("provided = %v, want %v", src.ProvidedCapabilities(), inst.CapabilityIDs())
	}
}

// Serves is exactly what was discovered and allowed, and the build follows the
// ADR 006 binding rule: a twin reaches its own instance, or the default when it
// names none, and nothing else.
func TestFactoryServesDiscoveredToolsAndBuildsOnlyForBoundTwins(t *testing.T) {
	inst, _ := connectedEnv(t)

	for _, tc := range []struct {
		name      string
		isDefault bool
		bound     string
		builds    bool
	}{
		{"default, unbound twin", true, "", true},
		{"not default, unbound twin", false, "", false},
		{"bound to this instance", false, "acme_files", true},
		{"bound to another instance", true, "other_files", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := NewFactory(inst, tc.isDefault)
			if f.Domain != capability.MCPDomain || f.EnvID != EnvIDFor("acme_files") {
				t.Errorf("factory %q in %q", f.EnvID, f.Domain)
			}
			if !reflect.DeepEqual(f.Serves, inst.CapabilityIDs()) {
				t.Errorf("serves = %v, want %v", f.Serves, inst.CapabilityIDs())
			}
			bindings := map[string]string{}
			if tc.bound != "" {
				bindings[SlotName] = tc.bound
			}
			env, err := f.Build(environment.BuildContext{AdapterBindings: bindings})
			if built := err == nil && env != nil; built != tc.builds {
				t.Errorf("built = %v (err %v), want %v", built, err, tc.builds)
			}
		})
	}
}
