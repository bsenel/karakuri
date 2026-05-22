package conformance

import (
	"context"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// fakePack is a minimal domain.Pack implementation used only by these tests.
type fakePack struct {
	id     string
	caps   []capability.Capability
	envs   []environment.Factory
	agents []agent.Definition
}

func (p *fakePack) ID() string          { return p.id }
func (p *fakePack) Name() string        { return p.id }
func (p *fakePack) Version() string     { return "v0" }
func (p *fakePack) Description() string { return "" }
func (p *fakePack) Capabilities() []capability.Capability { return p.caps }
func (p *fakePack) EnvironmentFactories() []environment.Factory { return p.envs }
func (p *fakePack) AgentDefinitions() []agent.Definition { return p.agents }
func (p *fakePack) ObjectiveTemplates() []objective.Template { return nil }
func (p *fakePack) PlannerHints() []domain.PlannerHint { return nil }
func (p *fakePack) Init(_ context.Context, _ domain.Config) error { return nil }
func (p *fakePack) Teardown(_ context.Context) error { return nil }

func TestCheckCrossPackCollisions_NoCollision(t *testing.T) {
	a := &fakePack{
		id: "alpha",
		caps: []capability.Capability{
			{ID: "alpha.lint", InputSchema: capability.Schema{Type: "object"}, OutputSchema: capability.Schema{Type: "object"}},
		},
	}
	b := &fakePack{
		id: "beta",
		caps: []capability.Capability{
			{ID: "beta.test", InputSchema: capability.Schema{Type: "object"}, OutputSchema: capability.Schema{Type: "object"}},
		},
	}
	results := CheckCrossPackCollisions(a, b)
	for _, r := range results {
		if !r.Passed {
			t.Errorf("expected all checks to pass, %s failed: %s", r.Check, r.Message)
		}
	}
}

func TestCheckCrossPackCollisions_CapabilityCollision(t *testing.T) {
	a := &fakePack{
		id: "alpha",
		caps: []capability.Capability{{ID: "shared.run"}},
	}
	b := &fakePack{
		id: "beta",
		caps: []capability.Capability{{ID: "shared.run"}},
	}
	results := CheckCrossPackCollisions(a, b)
	var got Result
	for _, r := range results {
		if r.Check == "cross_pack_capability_collision" {
			got = r
			break
		}
	}
	if got.Passed {
		t.Fatalf("expected capability collision to be flagged")
	}
	if !strings.Contains(got.Message, "shared.run") || !strings.Contains(got.Message, "alpha") || !strings.Contains(got.Message, "beta") {
		t.Errorf("error message missing context: %s", got.Message)
	}
}

func TestCheckCrossPackCollisions_EnvCollision(t *testing.T) {
	a := &fakePack{id: "alpha", envs: []environment.Factory{{EnvID: "shared-env"}}}
	b := &fakePack{id: "beta", envs: []environment.Factory{{EnvID: "shared-env"}}}
	results := CheckCrossPackCollisions(a, b)
	for _, r := range results {
		if r.Check == "cross_pack_environment_collision" && r.Passed {
			t.Errorf("expected env collision to fail; got: %s", r.Message)
		}
	}
}

func TestCheckCrossPackCollisions_AgentCollision(t *testing.T) {
	a := &fakePack{id: "alpha", agents: []agent.Definition{{ID: "specialist"}}}
	b := &fakePack{id: "beta", agents: []agent.Definition{{ID: "specialist"}}}
	results := CheckCrossPackCollisions(a, b)
	for _, r := range results {
		if r.Check == "cross_pack_agent_collision" && r.Passed {
			t.Errorf("expected agent collision to fail; got: %s", r.Message)
		}
	}
}

func TestCheckCrossPackCollisions_LessThanTwoPacks(t *testing.T) {
	results := CheckCrossPackCollisions(&fakePack{id: "alpha"})
	if len(results) != 1 || !results[0].Passed {
		t.Errorf("expected single passing no-op result, got %+v", results)
	}
}
