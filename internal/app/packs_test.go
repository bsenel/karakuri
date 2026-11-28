package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 39 step 2, part B: a pack is first-party because a configuration entry
// registered it, and its tools take their namespace from that entry (ADR 028
// section a).

type configuredPack struct {
	id        string
	caps      []capability.Capability
	templates []objective.Template
	agents    []agent.Definition
	hints     []domain.PlannerHint
}

func (p *configuredPack) ID() string                                    { return p.id }
func (p *configuredPack) Name() string                                  { return p.id }
func (p *configuredPack) Version() string                               { return "v0" }
func (p *configuredPack) Description() string                           { return "" }
func (p *configuredPack) Capabilities() []capability.Capability         { return p.caps }
func (p *configuredPack) EnvironmentFactories() []environment.Factory   { return nil }
func (p *configuredPack) AgentDefinitions() []agent.Definition          { return p.agents }
func (p *configuredPack) ObjectiveTemplates() []objective.Template      { return p.templates }
func (p *configuredPack) PlannerHints() []domain.PlannerHint            { return p.hints }
func (p *configuredPack) Init(_ context.Context, _ domain.Config) error { return nil }
func (p *configuredPack) Teardown(_ context.Context) error              { return nil }

func packWithTools(id string, toolIDs ...capability.CapabilityID) *configuredPack {
	p := &configuredPack{
		id:        id,
		templates: []objective.Template{{ID: id + ".tmpl", Domain: id}},
		agents:    []agent.Definition{{ID: agent.AgentID(id + ".agent"), Domain: id}},
		hints:     []domain.PlannerHint{{Condition: id + " work", Guidance: "observe first"}},
	}
	for i, tid := range toolIDs {
		p.caps = append(p.caps, capability.Capability{
			ID: tid, Domain: id, Verifiable: i == 0,
			InputSchema: capability.Schema{Type: "object"}, OutputSchema: capability.Schema{Type: "object"},
		})
	}
	return p
}

func registeredIDs(r *domain.Registry) []string {
	var out []string
	for _, c := range r.Capabilities() {
		out = append(out, string(c.ID))
	}
	sort.Strings(out)
	return out
}

func activeIDs(packs []domain.Pack) []string {
	var out []string
	for _, p := range packs {
		out = append(out, p.ID())
	}
	sort.Strings(out)
	return out
}

// loadDomains reads the domains: section the way the server does, from a
// configuration file, so the YAML key is part of what is tested.
func loadDomains(t *testing.T, yaml string) []config.DomainConfig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "karakuri.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg.Domains
}

// Only what an enabled entry names is registered. A compiled-in pack with no
// entry, or with a disabled one, contributes nothing to the registries, and an
// entry naming a pack that does not exist registers nothing in its place.
func TestOnlyAPackAnEnabledEntryNamesIsRegistered(t *testing.T) {
	entries := loadDomains(t, `
domains:
  - id: trivial
    enabled: true
  - id: dormant
    enabled: false
  - id: ghost
    enabled: true
`)
	packs := []domain.Pack{
		packWithTools("trivial", "trivial.verify.ok", "trivial.observe.state"),
		packWithTools("dormant", "dormant.verify.ok"),
		packWithTools("unlisted", "unlisted.verify.ok"),
	}
	domReg := domain.NewRegistry()

	manifests, active := registerConfiguredPacks(context.Background(), entries, packs, domReg)

	if got, want := activeIDs(active), []string{"trivial"}; !reflect.DeepEqual(got, want) {
		t.Errorf("active packs = %v, want %v", got, want)
	}
	if got, want := registeredIDs(manifests), []string{"trivial.observe.state", "trivial.verify.ok"}; !reflect.DeepEqual(got, want) {
		t.Errorf("registered tools = %v, want %v", got, want)
	}
	if got := manifests.ObjectiveTemplates(); len(got) != 1 || got[0].ID != "trivial.tmpl" {
		t.Errorf("registered templates = %+v, want only trivial.tmpl", got)
	}
}

// What is registered is the pack's manifest: its tools, templates, agent
// definitions and hints, each exactly once.
func TestAConfiguredPackIsRegisteredFromItsManifest(t *testing.T) {
	entries := loadDomains(t, `
domains:
  - id: trivial
    enabled: true
`)
	pack := packWithTools("trivial", "trivial.verify.ok", "trivial.observe.state")
	manifests, _ := registerConfiguredPacks(context.Background(), entries, []domain.Pack{pack}, domain.NewRegistry())

	want := domain.ManifestOf(pack)
	if got := manifests.ObjectiveTemplates(); !reflect.DeepEqual(got, want.ObjectiveTemplates) {
		t.Errorf("templates = %+v, want the manifest's %+v", got, want.ObjectiveTemplates)
	}
	if got := manifests.AgentDefinitions(); !reflect.DeepEqual(got, want.AgentDefinitions) {
		t.Errorf("agent definitions = %+v, want the manifest's %+v", got, want.AgentDefinitions)
	}
	if got := manifests.PlannerHints(); !reflect.DeepEqual(got, want.PlannerHints) {
		t.Errorf("planner hints = %+v, want the manifest's %+v", got, want.PlannerHints)
	}
	if got, wantIDs := registeredIDs(manifests), []string{"trivial.observe.state", "trivial.verify.ok"}; !reflect.DeepEqual(got, wantIDs) {
		t.Errorf("registered tools = %v, want %v", got, wantIDs)
	}
}

// The entry's pack ID is the namespace. A pack the operator enabled as rogue
// that names its tools software.… is refused, so enabling it cannot put
// anything under the software pack's ID.
func TestAConfiguredPackCannotRegisterToolsOutsideItsEntrysNamespace(t *testing.T) {
	entries := loadDomains(t, `
domains:
  - id: trivial
    enabled: true
  - id: rogue
    enabled: true
`)
	packs := []domain.Pack{
		packWithTools("trivial", "trivial.verify.ok"),
		packWithTools("rogue", "rogue.verify.ok", "software.observe.fetch_prs"),
	}
	manifests, active := registerConfiguredPacks(context.Background(), entries, packs, domain.NewRegistry())

	if got, want := activeIDs(active), []string{"trivial"}; !reflect.DeepEqual(got, want) {
		t.Errorf("active packs = %v, want %v: rogue claims the software namespace", got, want)
	}
	if got, want := registeredIDs(manifests), []string{"trivial.verify.ok"}; !reflect.DeepEqual(got, want) {
		t.Errorf("registered tools = %v, want %v", got, want)
	}
}

// The pack's disabled registration still reaches domReg, as bootstrap does
// today, so `List` keeps reporting a pack the operator turned off.
func TestAnUnconfiguredPackIsStillListedAsDisabled(t *testing.T) {
	entries := loadDomains(t, `
domains:
  - id: trivial
    enabled: true
`)
	domReg := domain.NewRegistry()
	registerConfiguredPacks(context.Background(), entries, []domain.Pack{
		packWithTools("trivial", "trivial.verify.ok"),
		packWithTools("dormant", "dormant.verify.ok"),
	}, domReg)

	var listed []string
	for _, p := range domReg.List() {
		listed = append(listed, p.ID())
	}
	sort.Strings(listed)
	if want := []string{"dormant", "trivial"}; !reflect.DeepEqual(listed, want) {
		t.Errorf("domReg lists %v, want %v", listed, want)
	}
}
