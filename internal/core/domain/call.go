package domain

import (
	"context"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// CallContract is what the gateway sends a pack's server with every call (ADR
// 028 section b): the orchestrator's own identifiers for the call and the
// authority bounds in force. A pack keys whatever it keeps on these and on
// nothing it defined itself.
type CallContract struct {
	TenantID    string
	TwinID      string
	ObjectiveID string
	LoopID      string
	Pass        int
	ActionID    string
	Bounds      agent.AuthorityBounds
}

// PackServer is a pack's tool server as the gateway reaches it (Phase 39 step
// 3). The result carries the trust of its text, set by the pack from what the
// payload holds; an error means the pack could not look.
type PackServer interface {
	ListTools(ctx context.Context) ([]capability.CapabilityID, error)
	CallTool(ctx context.Context, call CallContract, tool capability.CapabilityID, params map[string]any) (environment.ActionResult, error)
}
