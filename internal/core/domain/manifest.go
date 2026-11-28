package domain

import (
	"errors"

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

// errManifestNotImplemented is compile scaffolding for Phase 39 step 2; the
// tests in manifest_test.go describe the behaviour that replaces it.
var errManifestNotImplemented = errors.New("pack manifest registration: not implemented")

// ManifestOf builds a manifest from what a compiled-in pack returns today.
func ManifestOf(_ Pack) Manifest { return Manifest{} }

// RegisterManifest registers a pack from its tools and its manifest.
func (r *Registry) RegisterManifest(_ string, _ []capability.Capability, _ Manifest) error {
	return errManifestNotImplemented
}

// Capabilities returns the tools of every pack registered from a manifest.
func (r *Registry) Capabilities() []capability.Capability { return nil }

// ObjectiveTemplates returns the templates of every pack registered from a
// manifest.
func (r *Registry) ObjectiveTemplates() []objective.Template { return nil }

// AgentDefinitions returns the agent definitions of every pack registered from
// a manifest.
func (r *Registry) AgentDefinitions() []agent.Definition { return nil }
