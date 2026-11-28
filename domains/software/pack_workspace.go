package software

import (
	"context"
	"errors"
	"time"

	"github.com/bsenel/karakuri/internal/core/domain"
)

// errWorkspacesUnimplemented is what the pack's worktree keeper answers until
// Phase 39 step 4 slice A is built.
var errWorkspacesUnimplemented = errors.New("software: the pack's worktrees are not implemented")

// Workspaces is the software pack's own keeper of worktrees (Phase 39 step 4).
// It keys a worktree on the identifiers every call carries, the objective and
// the loop/pass, so actions of one pass share their files and nothing else
// does; the orchestrator never sees a path.
type Workspaces struct{}

// NewWorkspaces keeps worktrees of the repository at repoRoot, removing any
// older than maxAge by the clock now when no signal released it.
func NewWorkspaces(repoRoot string, maxAge time.Duration, now func() time.Time) *Workspaces {
	return &Workspaces{}
}

// For is the worktree of the call's objective and pass, made on first use.
// The path stays inside the pack.
func (w *Workspaces) For(ctx context.Context, call domain.CallContract) (string, error) {
	return "", errWorkspacesUnimplemented
}

// PassEnded removes the worktree of the call's objective and pass.
func (w *Workspaces) PassEnded(ctx context.Context, call domain.CallContract) error {
	return errWorkspacesUnimplemented
}

// ObjectiveEnded removes every worktree of the call's objective.
func (w *Workspaces) ObjectiveEnded(ctx context.Context, call domain.CallContract) error {
	return errWorkspacesUnimplemented
}

// Expire removes every worktree older than the age bound.
func (w *Workspaces) Expire(ctx context.Context) error {
	return errWorkspacesUnimplemented
}

// WithWorkspaces gives the server the pack's worktree keeper, so a verifier
// runs in the worktree of the call's objective and pass.
func (s *MCPServer) WithWorkspaces(w *Workspaces) *MCPServer {
	return s
}
