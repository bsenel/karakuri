package domain

import (
	"context"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

type Config map[string]any

type Pack interface {
	ID() string
	Name() string
	Version() string
	Description() string

	Capabilities() []capability.Capability
	EnvironmentFactories() []environment.Factory
	AgentDefinitions() []agent.Definition
	ObjectiveTemplates() []objective.Template
	PlannerHints() []PlannerHint

	Init(ctx context.Context, cfg Config) error
	Teardown(ctx context.Context) error
}

type PlannerHint struct {
	Condition string
	Guidance  string
	Priority  int
}
