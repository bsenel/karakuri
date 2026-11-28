package software

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 39 step 2: the software pack's manifest is written from what
// domain.Pack returns today, and registering the pack from it yields the same
// capability, template, agent and hint IDs as registering it the current way
// (internal/app/bootstrap.go). Compared against the current path, never a
// hard-coded list, so the test does not drift as the pack grows.

func sortedCapabilityIDs(cs []capability.Capability) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, string(c.ID))
	}
	slices.Sort(out)
	return out
}

func sortedTemplateIDs(ts []objective.Template) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.ID)
	}
	slices.Sort(out)
	return out
}

func sortedAgentIDs(ds []agent.Definition) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, string(d.ID))
	}
	slices.Sort(out)
	return out
}

// A planner hint has no ID; its condition, guidance and priority are what the
// planner reads, so the three together identify it.
func sortedHints(hs []domain.PlannerHint) []string {
	out := make([]string, 0, len(hs))
	for _, h := range hs {
		out = append(out, fmt.Sprintf("%d|%s|%s", h.Priority, h.Condition, h.Guidance))
	}
	slices.Sort(out)
	return out
}

func TestSoftwareManifestIsBuiltFromWhatThePackReturns(t *testing.T) {
	p := New()
	m := domain.ManifestOf(p)

	var wantVerifiers []string
	for _, c := range p.Capabilities() {
		if c.Verifiable {
			wantVerifiers = append(wantVerifiers, string(c.ID))
		}
	}
	slices.Sort(wantVerifiers)
	if len(wantVerifiers) == 0 {
		t.Fatal("test setup: the software pack declares no verifiable capability")
	}
	gotVerifiers := make([]string, 0, len(m.Verifiers))
	for _, id := range m.Verifiers {
		gotVerifiers = append(gotVerifiers, string(id))
	}
	slices.Sort(gotVerifiers)
	if !slices.Equal(gotVerifiers, wantVerifiers) {
		t.Errorf("manifest verifiers = %v, want the pack's verifiable capabilities %v", gotVerifiers, wantVerifiers)
	}
	if got, want := sortedTemplateIDs(m.ObjectiveTemplates), sortedTemplateIDs(p.ObjectiveTemplates()); !slices.Equal(got, want) {
		t.Errorf("manifest objective templates = %v, want %v", got, want)
	}
	if got, want := sortedAgentIDs(m.AgentDefinitions), sortedAgentIDs(p.AgentDefinitions()); !slices.Equal(got, want) {
		t.Errorf("manifest agent definitions = %v, want %v", got, want)
	}
	if got, want := sortedHints(m.PlannerHints), sortedHints(p.PlannerHints()); !slices.Equal(got, want) {
		t.Errorf("manifest planner hints = %v, want %v", got, want)
	}
}

func TestSoftwarePackRegisteredFromItsManifestMatchesTheCurrentPath(t *testing.T) {
	ctx := context.Background()
	p := New()

	// The current path, as bootstrap does it: register the Go value, then read
	// its capabilities and templates off it.
	current := domain.NewRegistry()
	if err := current.Register(ctx, p, domain.Config{}); err != nil {
		t.Fatalf("register the current way: %v", err)
	}
	wantCaps := sortedCapabilityIDs(p.Capabilities())
	wantTemplates := sortedTemplateIDs(p.ObjectiveTemplates())
	wantAgents := sortedAgentIDs(p.AgentDefinitions())
	wantHints := sortedHints(current.PlannerHints())
	if len(wantCaps) == 0 || len(wantTemplates) == 0 || len(wantAgents) == 0 || len(wantHints) == 0 {
		t.Fatalf("test setup: the current path yields an empty list (caps %d, templates %d, agents %d, hints %d)",
			len(wantCaps), len(wantTemplates), len(wantAgents), len(wantHints))
	}

	fromManifest := domain.NewRegistry()
	if err := fromManifest.RegisterManifest(p.ID(), p.Capabilities(), domain.ManifestOf(p)); err != nil {
		t.Fatalf("register from the manifest: %v", err)
	}
	if got := sortedCapabilityIDs(fromManifest.Capabilities()); !slices.Equal(got, wantCaps) {
		t.Errorf("capability IDs from the manifest = %v, want %v", got, wantCaps)
	}
	if got := sortedTemplateIDs(fromManifest.ObjectiveTemplates()); !slices.Equal(got, wantTemplates) {
		t.Errorf("objective template IDs from the manifest = %v, want %v", got, wantTemplates)
	}
	if got := sortedAgentIDs(fromManifest.AgentDefinitions()); !slices.Equal(got, wantAgents) {
		t.Errorf("agent definition IDs from the manifest = %v, want %v", got, wantAgents)
	}
	if got := sortedHints(fromManifest.PlannerHints()); !slices.Equal(got, wantHints) {
		t.Errorf("planner hints from the manifest = %v, want %v", got, wantHints)
	}
}
