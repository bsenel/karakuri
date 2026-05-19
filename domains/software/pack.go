// Package software implements the Karakuri Software Development domain pack.
package software

import (
	"context"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

type Pack struct{}

func New() *Pack { return &Pack{} }

func (p *Pack) ID() string          { return "software" }
func (p *Pack) Name() string        { return "Software Development" }
func (p *Pack) Version() string     { return "1.0.0" }
func (p *Pack) Description() string { return "Capabilities, environments, and agents for autonomous software development" }

func (p *Pack) Init(_ context.Context, _ domain.Config) error { return nil }

func (p *Pack) Teardown(_ context.Context) error { return nil }

func (p *Pack) Capabilities() []capability.Capability {
	return softwareCapabilities()
}

func (p *Pack) EnvironmentFactories() []environment.Factory {
	return softwareEnvironmentFactories()
}

func (p *Pack) AgentDefinitions() []agent.Definition {
	return softwareAgentDefinitions()
}

func (p *Pack) ObjectiveTemplates() []objective.Template {
	return softwareObjectiveTemplates()
}

func (p *Pack) PlannerHints() []domain.PlannerHint {
	return softwarePlannerHints()
}
