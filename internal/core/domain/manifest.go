package domain

import (
	"fmt"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Manifest is what a pack ships outside MCP (ADR 028 Decision 5): the tools it
// nominates as criterion verifiers, its snapshot tool, its objective, agent and
// stream templates, and its routing hints. Nothing else; anything not here or
// in the pack's tool list is a pack internal.
//
// The manifest nominates verifiers and grants nothing: the orchestrator still
// decides which verifiers it trusts (ADR 028 section a).
type Manifest struct {
	Verifiers          []capability.CapabilityID
	SnapshotTool       capability.CapabilityID
	ObjectiveTemplates []objective.Template
	AgentDefinitions   []agent.Definition
	PlannerHints       []PlannerHint
}

// ManifestOf builds a manifest from what a compiled-in pack returns today. Its
// verifiers are the pack's verifiable capabilities. SnapshotTool stays empty:
// a compiled-in pack snapshots through its environments, not through a tool.
func ManifestOf(p Pack) Manifest {
	var verifiers []capability.CapabilityID
	for _, c := range p.Capabilities() {
		if c.Verifiable {
			verifiers = append(verifiers, c.ID)
		}
	}
	return Manifest{
		Verifiers:          verifiers,
		ObjectiveTemplates: p.ObjectiveTemplates(),
		AgentDefinitions:   p.AgentDefinitions(),
		PlannerHints:       p.PlannerHints(),
	}
}

// RegisterManifest registers a pack from its tools and its manifest. A tool,
// verifier or snapshot tool under the reserved `mcp.` namespace is refused,
// and a refused manifest registers nothing (ADR 028 section a, ADR 022).
func (r *Registry) RegisterManifest(packID string, tools []capability.Capability, m Manifest) error {
	if packID == "" {
		return fmt.Errorf("domain pack ID must not be empty")
	}
	for _, c := range tools {
		if capability.IsMCPCapability(c.ID) {
			return fmt.Errorf("domain %q: tool %q is in the reserved mcp. namespace", packID, c.ID)
		}
	}
	for _, id := range m.Verifiers {
		if capability.IsMCPCapability(id) {
			return fmt.Errorf("domain %q: verifier %q is in the reserved mcp. namespace", packID, id)
		}
	}
	if capability.IsMCPCapability(m.SnapshotTool) {
		return fmt.Errorf("domain %q: snapshot tool %q is in the reserved mcp. namespace", packID, m.SnapshotTool)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.manifests[packID] {
		return fmt.Errorf("domain %q already registered from a manifest", packID)
	}
	r.manifests[packID] = true
	r.tools = append(r.tools, tools...)
	r.templates = append(r.templates, m.ObjectiveTemplates...)
	r.agents = append(r.agents, m.AgentDefinitions...)
	r.hints = append(r.hints, m.PlannerHints...)
	return nil
}

// Capabilities returns the tools of every pack registered from a manifest.
func (r *Registry) Capabilities() []capability.Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]capability.Capability(nil), r.tools...)
}

// ObjectiveTemplates returns the templates of every pack registered from a
// manifest.
func (r *Registry) ObjectiveTemplates() []objective.Template {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]objective.Template(nil), r.templates...)
}

// AgentDefinitions returns the agent definitions of every pack registered from
// a manifest.
func (r *Registry) AgentDefinitions() []agent.Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]agent.Definition(nil), r.agents...)
}
