package software

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/telemetry"
)

// errMCPServerUnimplemented is what the server answered before Phase 39 step
// 3 was built; the step's tests still name it to tell "not built" apart from
// "could not look".
var errMCPServerUnimplemented = errors.New("software: the pack's MCP server is not implemented")

// Bindings says which adapter instances a call's twin has bound, keyed by
// slot ("versioncontrol" and so on), as environment.BuildContext carries them.
type Bindings func(ctx context.Context, call domain.CallContract) map[string]string

// MCPServer serves the software pack's observe capabilities and its verifiers
// as MCP tools, inside the binary, behind the gateway (Phase 39 step 3).
//
// Each call builds the environment that serves the tool from the call's own
// twin bindings, the way the loop builds it, so a tool answers through the
// server what it answers in process. A verifier runs in the worktree the
// pack's keeper holds for the call's objective and pass (step 4); one that a
// call names a workdir for still runs there, until the loop stops naming one.
type MCPServer struct {
	pack       *Pack
	bindings   Bindings
	telemetry  telemetry.Reader
	workspaces *Workspaces
}

// MCPServer builds the pack's in-binary tool server over the bindings each
// call's twin holds.
func (p *Pack) MCPServer(bindings Bindings) *MCPServer {
	return &MCPServer{pack: p, bindings: bindings}
}

// WithTelemetry hands the server the read-only port the loop hands every
// environment on its BuildContext, so the platform telemetry verifiers read
// the same deployment through the server as in process.
func (s *MCPServer) WithTelemetry(r telemetry.Reader) *MCPServer {
	s.telemetry = r
	return s
}

// ListTools names the tools the server serves: every observe capability and
// every verifier the pack declares.
func (s *MCPServer) ListTools(context.Context) ([]capability.CapabilityID, error) {
	var served []capability.CapabilityID
	for _, c := range s.pack.Capabilities() {
		if strings.HasPrefix(string(c.ID), "software.observe.") || c.Verifiable {
			served = append(served, c.ID)
		}
	}
	return served, nil
}

// CallTool runs one served tool for the call the gateway sent.
//
// The result's trust is the environment's own, set from what the payload
// holds (AGENTS.md rule 9); a tool no environment of the pack runs, or whose
// environment cannot be built, is an error rather than an empty answer
// (rule 10).
func (s *MCPServer) CallTool(ctx context.Context, call domain.CallContract, tool capability.CapabilityID, params map[string]any) (environment.ActionResult, error) {
	if tool == domain.ToolPassEnded || tool == domain.ToolObjectiveEnded {
		return s.lifecycle(ctx, call, string(tool))
	}
	if !s.serves(ctx, tool) {
		return environment.ActionResult{}, fmt.Errorf("software: %s is not a tool this server serves", tool)
	}
	fac, ok := s.factoryFor(tool)
	if !ok {
		return environment.ActionResult{}, fmt.Errorf("software: no environment of the pack runs %s", tool)
	}
	var bound map[string]string
	if s.bindings != nil {
		bound = s.bindings(ctx, call)
	}
	env, err := fac.Build(environment.BuildContext{TwinID: call.TwinID, AdapterBindings: bound, Telemetry: s.telemetry})
	if err != nil {
		return environment.ActionResult{}, fmt.Errorf("software: build %s for %s: %w", fac.EnvID, tool, err)
	}
	return s.inWorkspace(ctx, call, fac.EnvID, params, func(p map[string]any) (environment.ActionResult, error) {
		return env.Act(ctx, environment.Action{CapabilityID: tool, Params: p})
	})
}

// serves reports whether tool is one ListTools names.
func (s *MCPServer) serves(ctx context.Context, tool capability.CapabilityID) bool {
	listed, _ := s.ListTools(ctx)
	for _, id := range listed {
		if id == tool {
			return true
		}
	}
	return false
}

// factoryFor is the pack environment whose Serves declaration names tool, the
// declaration the loop routes on.
func (s *MCPServer) factoryFor(tool capability.CapabilityID) (environment.Factory, bool) {
	for _, f := range s.pack.EnvironmentFactories() {
		for _, id := range f.Serves {
			if id == tool {
				return f, true
			}
		}
	}
	return environment.Factory{}, false
}
