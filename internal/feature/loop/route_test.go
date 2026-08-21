package loop

import (
	"context"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// routeEnv is an environment that records nothing and answers everything. The
// tests here are about which environment is picked, never about what it does.
type routeEnv struct{ id environment.EnvironmentID }

func (e *routeEnv) ID() environment.EnvironmentID { return e.id }
func (e *routeEnv) Domain() string                { return "test" }

func (e *routeEnv) Observe(context.Context, environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{EnvID: e.id, Timestamp: time.Now().UTC()}, nil
}

func (e *routeEnv) Act(context.Context, environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{Success: true}, nil
}

func (e *routeEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return make(chan environment.EnvironmentEvent), nil
}

func (e *routeEnv) Snapshot(context.Context) (environment.EnvironmentSnapshot, error) {
	return environment.EnvironmentSnapshot{EnvID: e.id}, nil
}

// routeRegistry builds a registry from id → served capabilities, and the
// matching built environments in the order given.
func routeRegistry(t *testing.T, decls ...struct {
	id     environment.EnvironmentID
	serves []capability.CapabilityID
}) (*environment.Registry, []environment.Environment) {
	t.Helper()
	reg := environment.NewRegistry()
	envs := make([]environment.Environment, 0, len(decls))
	for _, d := range decls {
		id := d.id
		if err := reg.Register(environment.Factory{
			EnvID:  id,
			Domain: "test",
			Serves: d.serves,
			Build: func(environment.BuildContext) (environment.Environment, error) {
				return &routeEnv{id: id}, nil
			},
		}); err != nil {
			t.Fatalf("register %q: %v", id, err)
		}
		envs = append(envs, &routeEnv{id: id})
	}
	return reg, envs
}

type envDecl = struct {
	id     environment.EnvironmentID
	serves []capability.CapabilityID
}

// The regression this phase exists for. A plan that writes code without naming
// the CLI environment used to reach whatever env_id it guessed — in the live
// run that produced this fix, noopEnv, which returned "unimplemented" after a
// worktree had already been created. The pack knows which environment runs
// write_code; the planner does not, and is no longer asked.
func TestRoutingPrefersTheDeclaredEnvOverThePlansGuess(t *testing.T) {
	reg, envs := routeRegistry(t,
		envDecl{id: "test.env.noop", serves: nil},
		envDecl{id: "test.env.cli", serves: []capability.CapabilityID{"test.act.write_code"}},
	)
	svc := &serviceImpl{envReg: reg}

	got, routedBy := svc.resolveEnv(envs, plannedAction{
		CapabilityID: "test.act.write_code",
		EnvID:        "test.env.noop", // the model's wrong guess
	})
	if got == nil {
		t.Fatal("write_code did not route anywhere")
	}
	if got.ID() != "test.env.cli" {
		t.Errorf("routed to %q, want test.env.cli", got.ID())
	}
	if routedBy != "capability" {
		t.Errorf("routed_by = %q, want capability", routedBy)
	}
}

// An unclaimed capability still routes by env_id. Most capabilities are not
// served by any environment, and the plan's preference is the only signal
// available for the ones that are executed ad hoc.
func TestRoutingFallsBackToEnvIDWhenNothingClaimsTheCapability(t *testing.T) {
	reg, envs := routeRegistry(t,
		envDecl{id: "test.env.a", serves: nil},
		envDecl{id: "test.env.b", serves: nil},
	)
	svc := &serviceImpl{envReg: reg}

	got, routedBy := svc.resolveEnv(envs, plannedAction{
		CapabilityID: "test.act.unclaimed",
		EnvID:        "test.env.b",
	})
	if got == nil {
		t.Fatal("no environment resolved")
	}
	if got.ID() != "test.env.b" {
		t.Errorf("routed to %q, want test.env.b", got.ID())
	}
	if routedBy != "env_id" {
		t.Errorf("routed_by = %q, want env_id", routedBy)
	}
}

// Two environments claiming one capability is a pack bug, caught by
// conformance. Until it is fixed, routing must not resolve it by picking:
// map iteration order deciding which environment writes files is the silent
// misrouting this whole change removes.
func TestAmbiguousRoutingDefersToThePlanRatherThanGuessing(t *testing.T) {
	reg, envs := routeRegistry(t,
		envDecl{id: "test.env.a", serves: []capability.CapabilityID{"test.act.contested"}},
		envDecl{id: "test.env.b", serves: []capability.CapabilityID{"test.act.contested"}},
	)
	svc := &serviceImpl{envReg: reg}

	got, routedBy := svc.resolveEnv(envs, plannedAction{
		CapabilityID: "test.act.contested",
		EnvID:        "test.env.b",
	})
	if got == nil {
		t.Fatal("no environment resolved")
	}
	if got.ID() != "test.env.b" {
		t.Errorf("routed to %q, want the plan's choice test.env.b", got.ID())
	}
	if routedBy != "env_id" {
		t.Errorf("routed_by = %q, want env_id", routedBy)
	}
}

// A route to an environment the twin has not enabled is not a route. The
// registry holds every pack's declarations; only the environments this loop
// actually built can run anything.
func TestRoutingIgnoresEnvironmentsThisLoopDidNotBuild(t *testing.T) {
	reg, _ := routeRegistry(t,
		envDecl{id: "test.env.cli", serves: []capability.CapabilityID{"test.act.write_code"}},
	)
	svc := &serviceImpl{envReg: reg}

	// The loop built a different environment entirely.
	built := []environment.Environment{&routeEnv{id: "test.env.other"}}

	got, routedBy := svc.resolveEnv(built, plannedAction{CapabilityID: "test.act.write_code"})
	if got != nil {
		t.Errorf("routed to %q, want no route: the serving env was not built", got.ID())
	}
	if routedBy != "unrouted" {
		t.Errorf("routed_by = %q, want unrouted", routedBy)
	}
}

// The pre-existing rules still hold: one environment and no env_id is
// unambiguous, and an env_id matching nothing fails honestly rather than
// falling through to envs[0].
func TestRoutingKeepsTheSoleEnvAndUnroutedRules(t *testing.T) {
	reg, envs := routeRegistry(t, envDecl{id: "test.env.only", serves: nil})
	svc := &serviceImpl{envReg: reg}

	got, routedBy := svc.resolveEnv(envs, plannedAction{CapabilityID: "test.act.anything"})
	if got == nil || got.ID() != "test.env.only" {
		t.Errorf("sole environment was not used")
	}
	if routedBy != "sole_env" {
		t.Errorf("routed_by = %q, want sole_env", routedBy)
	}

	got, routedBy = svc.resolveEnv(envs, plannedAction{
		CapabilityID: "test.act.anything",
		EnvID:        "test.env.invented",
	})
	if got != nil {
		t.Errorf("an env_id matching nothing routed to %q", got.ID())
	}
	if routedBy != "unrouted" {
		t.Errorf("routed_by = %q, want unrouted", routedBy)
	}
}

// A service with no registry — every loop test that predates this change, and
// any embedding that does not wire one — routes exactly as it did before.
func TestRoutingWithoutARegistryBehavesAsBefore(t *testing.T) {
	svc := &serviceImpl{}
	envs := []environment.Environment{
		&routeEnv{id: "test.env.a"},
		&routeEnv{id: "test.env.b"},
	}

	got, routedBy := svc.resolveEnv(envs, plannedAction{
		CapabilityID: "test.act.write_code",
		EnvID:        "test.env.a",
	})
	if got == nil || got.ID() != "test.env.a" {
		t.Errorf("env_id routing broke without a registry")
	}
	if routedBy != "env_id" {
		t.Errorf("routed_by = %q, want env_id", routedBy)
	}
}
