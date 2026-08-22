package conformance_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/conformance"
	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// routingPack is a minimal pack whose capabilities and Serves declarations the
// tests below vary. Everything else is filled in well enough to pass the rest
// of the suite, so a failure is unambiguously the routing check.
type routingPack struct {
	caps []capability.Capability
	envs []environment.Factory
}

func (p *routingPack) ID() string          { return "routing" }
func (p *routingPack) Name() string        { return "Routing" }
func (p *routingPack) Version() string     { return "0.1.0" }
func (p *routingPack) Description() string { return "fixture" }

func (p *routingPack) Capabilities() []capability.Capability       { return p.caps }
func (p *routingPack) EnvironmentFactories() []environment.Factory { return p.envs }
func (p *routingPack) AgentDefinitions() []agent.Definition        { return nil }
func (p *routingPack) ObjectiveTemplates() []objective.Template    { return nil }
func (p *routingPack) PlannerHints() []domain.PlannerHint          { return nil }

func (p *routingPack) Init(context.Context, domain.Config) error { return nil }
func (p *routingPack) Teardown(context.Context) error            { return nil }

func routingCap(id string, needsWorkspace bool) capability.Capability {
	return capability.Capability{
		ID:             capability.CapabilityID(id),
		Name:           id,
		Domain:         "routing",
		InputSchema:    capability.Schema{Type: "object"},
		OutputSchema:   capability.Schema{Type: "object"},
		NeedsWorkspace: needsWorkspace,
	}
}

// conformanceEnv exists only so a factory can build something non-nil; the
// routing check reads declarations, never behaviour.
type conformanceEnv struct{ id environment.EnvironmentID }

func (e *conformanceEnv) ID() environment.EnvironmentID { return e.id }
func (e *conformanceEnv) Domain() string                { return "routing" }

func (e *conformanceEnv) Observe(context.Context, environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{EnvID: e.id}, nil
}

func (e *conformanceEnv) Act(context.Context, environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{Success: true}, nil
}

func (e *conformanceEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return make(chan environment.EnvironmentEvent), nil
}

func (e *conformanceEnv) Snapshot(context.Context) (environment.EnvironmentSnapshot, error) {
	return environment.EnvironmentSnapshot{EnvID: e.id}, nil
}

func routingEnv(id string, serves ...capability.CapabilityID) environment.Factory {
	return environment.Factory{
		EnvID:  environment.EnvironmentID(id),
		Domain: "routing",
		Serves: serves,
		Build: func(environment.BuildContext) (environment.Environment, error) {
			return &conformanceEnv{id: environment.EnvironmentID(id)}, nil
		},
	}
}

// routingResult returns the capability_routing result, failing if the suite
// stopped running the check at all.
func routingResult(t *testing.T, p domain.Pack) conformance.Result {
	t.Helper()
	for _, r := range conformance.New().Run(context.Background(), p) {
		if r.Check == "capability_routing" {
			return r
		}
	}
	t.Fatal("the suite no longer runs capability_routing")
	return conformance.Result{}
}

// The check has to fail for each of the three ways a Serves declaration fails
// to be a route, or it is a check that passes everything.
func TestRoutingCheckCatchesBrokenDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pack     domain.Pack
		contains string
	}{
		{
			name: "serves a capability the pack does not declare",
			pack: &routingPack{
				caps: []capability.Capability{routingCap("routing.act.real", false)},
				envs: []environment.Factory{routingEnv("routing.env.a", "routing.act.typo")},
			},
			contains: "does not declare",
		},
		{
			name: "two environments claim one capability",
			pack: &routingPack{
				caps: []capability.Capability{routingCap("routing.act.contested", false)},
				envs: []environment.Factory{
					routingEnv("routing.env.a", "routing.act.contested"),
					routingEnv("routing.env.b", "routing.act.contested"),
				},
			},
			contains: "cannot choose between them",
		},
		{
			// The bug this phase exists for: write_code was given a git
			// worktree and had no environment behind it, so every plan that
			// used it created a branch and returned "unimplemented".
			name: "a capability that writes routes nowhere",
			pack: &routingPack{
				caps: []capability.Capability{routingCap("routing.act.write", true)},
				envs: []environment.Factory{routingEnv("routing.env.a")},
			},
			contains: "NeedsWorkspace",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := routingResult(t, tc.pack)
			if got.Passed {
				t.Fatalf("check passed on a pack that %s", tc.name)
			}
			if !strings.Contains(got.Message, tc.contains) {
				t.Errorf("message %q does not say what is wrong (want it to mention %q)", got.Message, tc.contains)
			}
		})
	}
}

// And it has to pass the shapes that are correct, or packs cannot ship. A
// capability nothing serves is the ordinary case — most are reasoned about or
// verified rather than executed — and a pack with no environments at all must
// not be failed for having no routes.
func TestRoutingCheckPassesHonestDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name string
		pack domain.Pack
	}{
		{
			name: "each served capability has exactly one environment",
			pack: &routingPack{
				caps: []capability.Capability{
					routingCap("routing.act.write", true),
					routingCap("routing.reason.think", false),
				},
				envs: []environment.Factory{routingEnv("routing.env.a", "routing.act.write")},
			},
		},
		{
			name: "a pack with no environments",
			pack: &routingPack{
				caps: []capability.Capability{routingCap("routing.reason.think", false)},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := routingResult(t, tc.pack); !got.Passed {
				t.Errorf("check failed a valid pack: %s", got.Message)
			}
		})
	}
}
