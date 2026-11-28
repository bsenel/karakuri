package domain

import (
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
)

// Phase 39 step 2, part B: the namespace of a registered pack's tools is the
// pack ID of the configuration entry that registered it (ADR 028 section a).
// A tool, verifier or snapshot tool the pack names under any other prefix is
// the pack claiming somebody else's namespace, and registration refuses it
// whole.
func TestRegisterManifestRefusesAToolOutsideTheEntrysNamespace(t *testing.T) {
	foreign := capability.CapabilityID("software.observe.fetch_prs")
	for name, build := range map[string]func() ([]capability.Capability, Manifest){
		"as a capability": func() ([]capability.Capability, Manifest) {
			tools := append(cleanManifestTools(), capability.Capability{
				ID: foreign, Domain: "trivial",
				InputSchema: capability.Schema{Type: "object"}, OutputSchema: capability.Schema{Type: "object"},
			})
			return tools, cleanManifest()
		},
		"as a verifier": func() ([]capability.Capability, Manifest) {
			m := cleanManifest()
			m.Verifiers = append(m.Verifiers, foreign)
			return cleanManifestTools(), m
		},
		"as the snapshot tool": func() ([]capability.Capability, Manifest) {
			m := cleanManifest()
			m.SnapshotTool = foreign
			return cleanManifestTools(), m
		},
		// The ID prefix is not enough: a tool under trivial. that says it
		// belongs to domain software is still claiming the software pack.
		"as a capability's domain": func() ([]capability.Capability, Manifest) {
			tools := append(cleanManifestTools(), capability.Capability{
				ID: "trivial.observe.more", Domain: "software",
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
				t.Fatal("registered a manifest naming a tool outside the entry's namespace trivial.; want a refusal")
			}
			if !strings.Contains(err.Error(), "trivial") {
				t.Errorf("err = %q, want it to name the entry's pack ID", err)
			}
			if got := r.Capabilities(); len(got) != 0 {
				t.Errorf("a refused manifest left %d capabilities registered", len(got))
			}
		})
	}
}

// A prefix that only starts like the pack ID is another namespace: trivial2.x
// is not under trivial.
func TestRegisterManifestReadsTheNamespaceAsAWholeSegment(t *testing.T) {
	tools := append(cleanManifestTools(), capability.Capability{
		ID: "trivial2.observe", Domain: "trivial",
		InputSchema: capability.Schema{Type: "object"}, OutputSchema: capability.Schema{Type: "object"},
	})
	if err := NewRegistry().RegisterManifest("trivial", tools, cleanManifest()); err == nil {
		t.Fatal("registered trivial2.observe under pack trivial; want a refusal")
	}
}
