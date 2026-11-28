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

// Lifecycle signals the orchestrator sends a pack it called, as reserved tool
// names on the same CallTool path every call takes (Phase 39 step 4). The
// call's contract names what ended: the objective and the loop/pass for a pass,
// the objective for an objective. A signal is safe to repeat and delivery is
// best effort; a pack with nothing to release answers it and does nothing.
const (
	ToolPassEnded      capability.CapabilityID = "lifecycle.pass_ended"
	ToolObjectiveEnded capability.CapabilityID = "lifecycle.objective_ended"
)

// PackServer is a pack's tool server as the gateway reaches it (Phase 39 step
// 3). The result carries the trust of its text, set by the pack from what the
// payload holds; an error means the pack could not look.
type PackServer interface {
	ListTools(ctx context.Context) ([]capability.CapabilityID, error)
	CallTool(ctx context.Context, call CallContract, tool capability.CapabilityID, params map[string]any) (environment.ActionResult, error)
}
