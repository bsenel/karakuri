package domain

import (
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 39 step 2: a pack whose manifest or tool list names anything under the
// `mcp.` namespace fails registration, the direction checkReservedNamespace
// already checks (ADR 028 section a, ADR 022).

func cleanManifestTools() []capability.Capability {
	return []capability.Capability{
		{ID: "trivial.observe", Domain: "trivial", InputSchema: capability.Schema{Type: "object"}, OutputSchema: capability.Schema{Type: "object"}},
		{ID: "trivial.verify", Domain: "trivial", Verifiable: true, InputSchema: capability.Schema{Type: "object"}, OutputSchema: capability.Schema{Type: "object"}},
	}
}

func cleanManifest() Manifest {
	return Manifest{
		Verifiers:          []capability.CapabilityID{"trivial.verify"},
		SnapshotTool:       "trivial.observe",
		ObjectiveTemplates: []objective.Template{{ID: "trivial.tmpl", Domain: "trivial"}},
		PlannerHints:       []PlannerHint{{Condition: "always", Guidance: "observe first"}},
	}
}

// A manifest with nothing under `mcp.` registers; the control for the refusals
// below, so they cannot pass because registration refuses everything.
func TestRegisterManifestAcceptsAManifestOutsideTheReservedNamespace(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterManifest("trivial", cleanManifestTools(), cleanManifest()); err != nil {
		t.Fatalf("register a clean manifest: %v", err)
	}
}

func TestRegisterManifestRefusesAToolUnderTheMCPNamespace(t *testing.T) {
	discovered := capability.MCPCapabilityID("github", "merge_check")
	if !capability.IsMCPCapability(discovered) {
		t.Fatalf("test setup: %q is not in the reserved namespace", discovered)
	}
	for name, build := range map[string]func() ([]capability.Capability, Manifest){
		"as a verifier": func() ([]capability.Capability, Manifest) {
			m := cleanManifest()
			m.Verifiers = append(m.Verifiers, discovered)
			return cleanManifestTools(), m
		},
		"as the snapshot tool": func() ([]capability.Capability, Manifest) {
			m := cleanManifest()
			m.SnapshotTool = discovered
			return cleanManifestTools(), m
		},
		"as a capability": func() ([]capability.Capability, Manifest) {
			tools := append(cleanManifestTools(), capability.Capability{
				ID: discovered, Domain: "trivial",
				InputSchema: capability.Schema{Type: "object"}, OutputSchema: capability.Schema{Type: "object"},
			})
			return tools, cleanManifest()
		},
	} {
		t.Run(name, func(t *testing.T) {
			tools, m := build()
			r := NewRegistry()
			err := r.RegisterManifest("trivial", tools, m)
			if err == nil {
				t.Fatalf("registered a manifest naming %q; want a refusal", discovered)
			}
			if !strings.Contains(err.Error(), string(discovered)) {
				t.Errorf("err = %q, want it to name the tool %q", err, discovered)
			}
			if got := r.Capabilities(); len(got) != 0 {
				t.Errorf("a refused manifest left %d capabilities registered", len(got))
			}
			if got := r.ObjectiveTemplates(); len(got) != 0 {
				t.Errorf("a refused manifest left %d objective templates registered", len(got))
			}
			if got := r.PlannerHints(); len(got) != 0 {
				t.Errorf("a refused manifest left %d planner hints registered", len(got))
			}
		})
	}
}
