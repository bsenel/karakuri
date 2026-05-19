package healthcare

import (
	"context"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Pack is a stub for the healthcare domain pack. Full implementation in v3+.
type Pack struct{}

func New() *Pack { return &Pack{} }

func (p *Pack) ID() string          { return "healthcare" }
func (p *Pack) Name() string        { return "uhealthcare (stub)" }
func (p *Pack) Version() string     { return "0.0.1-stub" }
func (p *Pack) Description() string { return "Stub domain pack for healthcare; full implementation in v3+" }

func (p *Pack) Init(_ context.Context, _ domain.Config) error { return nil }
func (p *Pack) Teardown(_ context.Context) error              { return nil }

func (p *Pack) Capabilities() []capability.Capability          { return nil }
func (p *Pack) EnvironmentFactories() []environment.Factory    { return nil }
func (p *Pack) AgentDefinitions() []agent.Definition           { return nil }
func (p *Pack) ObjectiveTemplates() []objective.Template       { return nil }
func (p *Pack) PlannerHints() []domain.PlannerHint             { return nil }
