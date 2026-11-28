package software

import (
	"context"
	"errors"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// errMCPServerUnimplemented is what the server answers until Phase 39 step 3
// is built.
var errMCPServerUnimplemented = errors.New("software: the pack's MCP server is not implemented")

// Bindings says which adapter instances a call's twin has bound, keyed by
// slot ("versioncontrol" and so on), as environment.BuildContext carries them.
type Bindings func(ctx context.Context, call domain.CallContract) map[string]string

// MCPServer serves the software pack's observe capabilities and its verifiers
// as MCP tools, inside the binary, behind the gateway (Phase 39 step 3).
type MCPServer struct {
	pack     *Pack
	bindings Bindings
}

// MCPServer builds the pack's in-binary tool server over the bindings each
// call's twin holds.
func (p *Pack) MCPServer(bindings Bindings) *MCPServer {
	return &MCPServer{pack: p, bindings: bindings}
}

// ListTools names the tools the server serves.
func (s *MCPServer) ListTools(context.Context) ([]capability.CapabilityID, error) {
	return nil, errMCPServerUnimplemented
}

// CallTool runs one served tool for the call the gateway sent.
func (s *MCPServer) CallTool(context.Context, domain.CallContract, capability.CapabilityID, map[string]any) (environment.ActionResult, error) {
	return environment.ActionResult{}, errMCPServerUnimplemented
}
