package loop

import (
	"context"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// manyAgentPack stands in for a pack that has grown: the agent an objective
// wants is not the one declared first.
type manyAgentPack struct{ id string }

func (p *manyAgentPack) ID() string          { return p.id }
func (p *manyAgentPack) Name() string        { return p.id }
func (p *manyAgentPack) Version() string     { return "1.0.0" }
func (p *manyAgentPack) Description() string { return "" }

func (p *manyAgentPack) Init(context.Context, domain.Config) error { return nil }
func (p *manyAgentPack) Teardown(context.Context) error            { return nil }

func (p *manyAgentPack) Capabilities() []capability.Capability       { return nil }
func (p *manyAgentPack) EnvironmentFactories() []environment.Factory { return nil }
func (p *manyAgentPack) ObjectiveTemplates() []objective.Template    { return nil }
func (p *manyAgentPack) PlannerHints() []domain.PlannerHint          { return nil }
func (p *manyAgentPack) AgentDefinitions() []coreagent.Definition {
	return []coreagent.Definition{
		{ID: "first", Domain: p.id, Authority: coreagent.AuthorityBounds{MaxAutonomousActions: 5}},
		{ID: "wanted", Domain: p.id, Authority: coreagent.AuthorityBounds{MaxAutonomousActions: 0}},
	}
}

func registryWith(t *testing.T, p domain.Pack) *domain.Registry {
	t.Helper()
	reg := domain.NewRegistry()
	if err := reg.Register(context.Background(), p, domain.Config{}); err != nil {
		t.Fatalf("register pack: %v", err)
	}
	return reg
}

// An objective that names an agent gets that agent.
//
// Template.SuggestedAgents was declared and read by nothing: an objective
// created from a template kept no reference back to it, so selection fell
// through to "the first agent the domain declares". That was correct while
// packs had two agents and silently wrong once one had nine — the
// self-improvement template ran under the strategist, and the test guarding
// the maintainer's bounds guarded an agent that never ran.
func TestSelectAgentHonoursTheObjectiveAgent(t *testing.T) {
	reg := registryWith(t, &manyAgentPack{id: "d"})

	got := SelectAgent(reg, objective.Objective{ID: "o1", Domain: "d", AgentID: "wanted"}, coreagent.Definition{})
	if got.ID != "wanted" {
		t.Errorf("selected %q, want the agent the objective named", got.ID)
	}
	// And the bounds that come with it, which is the point: authority is read
	// off whichever definition this returns.
	if got.Authority.MaxAutonomousActions != 0 {
		t.Errorf("selected agent carries MaxAutonomousActions = %d, not the named agent's bounds",
			got.Authority.MaxAutonomousActions)
	}
}

// Naming nothing keeps the old behaviour, so every objective written before
// this behaves exactly as it did.
func TestSelectAgentFallsBackToTheFirstDeclared(t *testing.T) {
	reg := registryWith(t, &manyAgentPack{id: "d"})
	if got := SelectAgent(reg, objective.Objective{Domain: "d"}, coreagent.Definition{}); got.ID != "first" {
		t.Errorf("selected %q, want the domain's first declared agent", got.ID)
	}
}

// An explicit definition still wins over the objective's own field: the
// supervisor passes one when it has already resolved authority.
func TestExplicitAgentStillWins(t *testing.T) {
	reg := registryWith(t, &manyAgentPack{id: "d"})
	explicit := coreagent.Definition{ID: "explicit"}
	if got := SelectAgent(reg, objective.Objective{Domain: "d", AgentID: "wanted"}, explicit); got.ID != "explicit" {
		t.Errorf("selected %q, want the explicitly passed definition", got.ID)
	}
}

// A name no pack declares falls back rather than failing the objective — but
// it must not silently run something else with no trace, so the fallback is
// logged. Here we only assert it does not wedge or pick nothing.
func TestUnknownAgentNameFallsBack(t *testing.T) {
	reg := registryWith(t, &manyAgentPack{id: "d"})
	got := SelectAgent(reg, objective.Objective{Domain: "d", AgentID: "typo"}, coreagent.Definition{})
	if got.ID != "first" {
		t.Errorf("selected %q, want the fallback after an unresolvable name", got.ID)
	}
}
