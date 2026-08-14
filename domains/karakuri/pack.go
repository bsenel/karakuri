// Package karakuri is the domain pack through which Karakuri improves itself.
//
// It is a domain pack like any other — the engine imports no knowledge of it,
// it registers through the same interface as software and agriculture, and it
// passes the same conformance suite. What is unusual is its subject: the
// environments it observes are this deployment's own telemetry, its own
// repository, and the state of the field it works in.
//
// The pack deliberately owns no way to change anything. Its capabilities
// analyse and draft; the acting is done by the software pack's capabilities,
// through a cross-domain objective. That split is not tidiness. A pack that
// could both decide what Karakuri should become and carry it out would be one
// component away from a system that rewrites its own bounds, and the reason to
// keep the writing capabilities in `software` is that they are the ones an
// operator already reviews.
package karakuri

import (
	"context"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/tools"
)

type Pack struct {
	tools *tools.Registry
}

func New() *Pack { return &Pack{} }

// NewWithTools builds the pack against the configured adapters, the way the
// software pack does. Without them the repository environment is a no-op that
// reports honestly rather than a stub that reports nothing.
func NewWithTools(reg *tools.Registry) *Pack { return &Pack{tools: reg} }

func (p *Pack) ID() string      { return "karakuri" }
func (p *Pack) Name() string    { return "Karakuri" }
func (p *Pack) Version() string { return "1.0.0" }
func (p *Pack) Description() string {
	return "Karakuri's own telemetry, repository and field as environments, so it can improve itself"
}

func (p *Pack) Init(_ context.Context, _ domain.Config) error { return nil }
func (p *Pack) Teardown(_ context.Context) error              { return nil }

func (p *Pack) Capabilities() []capability.Capability { return karakuriCapabilities() }

func (p *Pack) EnvironmentFactories() []environment.Factory {
	return karakuriEnvironmentFactories(p.tools)
}

func (p *Pack) AgentDefinitions() []agent.Definition { return karakuriAgentDefinitions() }

func (p *Pack) ObjectiveTemplates() []objective.Template { return karakuriObjectiveTemplates() }

func (p *Pack) PlannerHints() []domain.PlannerHint { return karakuriPlannerHints() }
