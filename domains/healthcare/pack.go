// Package healthcare implements the Karakuri Healthcare domain pack.
//
// Healthcare is the second non-software production pack (alongside Agriculture)
// and exercises the platform's safety properties at full strength: every act
// capability that touches a treatment plan escalates to a human checkpoint,
// authority bounds prevent runaway action execution, and clinical_review is
// the mandatory final verifier on diagnosis_support objectives.
package healthcare

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

func (p *Pack) ID() string      { return "healthcare" }
func (p *Pack) Name() string    { return "Healthcare" }
func (p *Pack) Version() string { return "1.0.0" }
func (p *Pack) Description() string {
	return "Capabilities, environments, and agents for clinical decision support with strict authority bounds"
}

func (p *Pack) Init(_ context.Context, _ domain.Config) error { return nil }
func (p *Pack) Teardown(_ context.Context) error              { return nil }

func (p *Pack) Capabilities() []capability.Capability {
	return healthcareCapabilities()
}

func (p *Pack) EnvironmentFactories() []environment.Factory {
	return healthcareEnvironmentFactories()
}

func (p *Pack) AgentDefinitions() []agent.Definition {
	return healthcareAgentDefinitions()
}

func (p *Pack) ObjectiveTemplates() []objective.Template {
	return healthcareObjectiveTemplates()
}

func (p *Pack) PlannerHints() []domain.PlannerHint {
	return healthcarePlannerHints()
}
