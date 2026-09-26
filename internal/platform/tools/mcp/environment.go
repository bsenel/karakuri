package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// EnvIDFor is the environment one instance's tools execute in.
//
// One environment per instance rather than one for all of MCP, because routing,
// the audit log and /health all name an environment and "which server ran this"
// is the thing an operator asks first. It is also what lets Factory.Serves stay
// exact: each environment declares the tools its own server advertised.
func EnvIDFor(instance string) environment.EnvironmentID {
	return environment.EnvironmentID("mcp.env." + instance)
}

// Environment executes the tools one MCP instance discovered.
//
// It is an environment rather than an adapter behind somebody else's because
// there is no pack to put it behind: a discovered tool belongs to no domain an
// objective declares, and the twin that may reach it is decided by an adapter
// binding (ADR 006) rather than by the objective's subject matter.
type Environment struct {
	id   environment.EnvironmentID
	inst *Instance
}

// NewEnvironment wraps one instance. The instance has already discovered — an
// environment built over an unreachable one serves nothing and says so.
func NewEnvironment(inst *Instance) *Environment {
	return &Environment{id: EnvIDFor(inst.Name()), inst: inst}
}

var (
	_ environment.Environment = (*Environment)(nil)
	_ environment.ToolSource  = (*Environment)(nil)
)

func (e *Environment) ID() environment.EnvironmentID { return e.id }

// Domain is the reserved MCP domain, never a pack's. Conformance refuses a pack
// that claims it, which is what keeps a discovered tool out of pack grading
// without every per-pack check needing to know MCP exists.
func (e *Environment) Domain() string { return capability.MCPDomain }

// ProvidedCapabilities names what this environment executes for the twin it was
// built for — the reason step's catalog is the one reader (see ToolSource).
func (e *Environment) ProvidedCapabilities() []capability.CapabilityID {
	return e.inst.CapabilityIDs()
}

// Observe reports the server and what it advertises.
//
// The tool names and descriptions are the server's own text, which is why an
// observation that carries any of them is marked as somebody else's writing: a
// description reaches the planner's context, and that is Phase 27's surface
// arriving from a new direction. An instance advertising nothing — unreachable,
// or allowlisted down to nothing — carries nobody's prose and stays trusted, the
// same rule researchEnv applies to a search that found nothing.
func (e *Environment) Observe(_ context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	health := e.inst.Health(false)
	advertised := make([]map[string]any, 0, len(e.inst.Tools()))
	for _, t := range e.inst.Tools() {
		advertised = append(advertised, map[string]any{
			"capability":  string(capability.MCPCapabilityID(e.inst.Name(), t.Name)),
			"tool":        t.Name,
			"description": t.Description,
		})
	}

	trust := environment.TrustOperator
	if len(advertised) > 0 {
		trust = environment.TrustThirdParty
	}

	return environment.Observation{
		EnvID: e.id,
		State: map[string]any{
			"instance":         e.inst.Name(),
			"transport":        health.Transport,
			"state":            health.State,
			"server":           health.Server,
			"protocol_version": health.ProtocolVersion,
			"tools":            advertised,
			"filtered":         health.Filtered,
		},
		Timestamp: time.Now().UTC(),
		Trust:     trust,
	}, nil
}

// Act calls one discovered tool.
//
// Every result is marked as a third party's writing, without looking at what
// came back. The other environments decide per payload because they have a
// trusted half to distinguish — a commit SHA is not a PR title — and this one
// does not: everything an MCP server returns was written by the server. A
// refusal below is this repository's own sentence, and is marked the same way
// rather than left to the zero value, because "trusted unless something
// remembered to say otherwise" is the default ADR 021 accepts everywhere except
// here, where the whole environment is somebody else's.
func (e *Environment) Act(ctx context.Context, a environment.Action) (environment.ActionResult, error) {
	fail := func(format string, args ...any) (environment.ActionResult, error) {
		return environment.ActionResult{
			Success: false,
			Error:   fmt.Sprintf(format, args...),
			Trust:   environment.TrustThirdParty,
		}, nil
	}

	if !capability.IsMCPCapability(a.CapabilityID) {
		return fail("capability %q is not a discovered MCP tool", a.CapabilityID)
	}
	// The instance is in the ID, so an action for another tenant's server is
	// refused here rather than run against whichever instance happened to be
	// resolved. Two servers may both export read_file.
	if inst := capability.MCPInstanceOf(a.CapabilityID); inst != e.inst.Name() {
		return fail("capability %q belongs to MCP instance %q, and this environment serves %q",
			a.CapabilityID, inst, e.inst.Name())
	}
	tool := capability.MCPToolOf(a.CapabilityID)
	if tool == "" {
		return fail("capability %q names no tool", a.CapabilityID)
	}

	res, err := e.inst.Call(ctx, tool, a.Params)
	if err != nil {
		return fail("%s", err.Error())
	}

	// A tool that ran and failed is a result, not a transport error: the
	// distinction is the protocol's, and it is what tells the loop whether the
	// call is worth repeating.
	return environment.ActionResult{
		Success: !res.IsError,
		Error:   errorTextOf(res),
		Trust:   environment.TrustThirdParty,
		StateDelta: map[string]any{
			"capability": string(a.CapabilityID),
			"instance":   e.inst.Name(),
			"tool":       tool,
			"output":     res.Text(),
		},
	}, nil
}

func errorTextOf(res ToolResult) string {
	if !res.IsError {
		return ""
	}
	if text := res.Text(); text != "" {
		return text
	}
	return "the tool reported a failure and said nothing further"
}

// Subscribe has nothing to publish: this client sends requests and reads their
// replies, and a server's own notifications have no subscriber here.
func (e *Environment) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return nil, nil
}

// Snapshot deliberately carries no SHA.
//
// An empty SHA contributes nothing to the composite fingerprint a standing
// objective reconciles on, which is correct: a server restarting, or adding a
// tool this deployment does not allow, is not drift in the world the objective
// is converging.
func (e *Environment) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	obs, _ := e.Observe(ctx, environment.ObservationQuery{})
	return environment.EnvironmentSnapshot{
		EnvID:     e.id,
		State:     obs.State,
		Timestamp: obs.Timestamp,
	}, nil
}

// NewFactory is how an instance reaches the environment registry.
//
// Serves is the exact list of what this server advertised and the allowlist
// allowed, which is the routing decision ADR 022 settles: the registry
// reverse-indexes Serves once at Register and has no prefix matching, so
// discovery completes before registration rather than the index learning a
// namespace. An action for mcp.<instance>.<tool> then resolves the way every
// other capability does — through the index, not through the EnvID fallback.
//
// isDefault is the ADR 006 rule, unchanged: a twin naming no MCP instance gets
// the slot's default, and a twin naming one gets that one and no other. A twin
// bound elsewhere fails to build this environment, which leaves the tools of a
// server it may not reach out of its catalog entirely.
func NewFactory(inst *Instance, isDefault bool) environment.Factory {
	env := NewEnvironment(inst)
	return environment.Factory{
		EnvID:       env.ID(),
		Domain:      capability.MCPDomain,
		Description: describe(inst),
		Serves:      inst.CapabilityIDs(),
		Build: func(ctx environment.BuildContext) (environment.Environment, error) {
			bound := ctx.AdapterBindings[SlotName]
			if bound == "" && !isDefault {
				return nil, fmt.Errorf("twin is bound to no MCP instance and %q is not the default", inst.Name())
			}
			if bound != "" && bound != inst.Name() {
				return nil, fmt.Errorf("twin is bound to MCP instance %q, not %q", bound, inst.Name())
			}
			return env, nil
		},
	}
}

// describe names the server for the environment listing the planner reads. The
// instance name rather than the server's own: the instance is what an operator
// configured and what a twin binding names, and a server free to call itself
// anything is not an identifier this deployment should print as one.
func describe(inst *Instance) string {
	return fmt.Sprintf("MCP server %q (%s): %d tool(s) discovered and allowed",
		inst.Name(), inst.Transport(), len(inst.Tools()))
}
