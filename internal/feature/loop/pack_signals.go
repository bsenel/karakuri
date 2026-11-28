package loop

import (
	"context"

	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// The orchestrator's lifecycle signals to the packs a run called (Phase 39
// step 4, ADR 028). The loop names what ended by its own identifiers; the
// gateway knows which packs it called and sends each its signal, best effort,
// so nothing here can fail a pass.

// signalPassEnded tells the packs this pass called that it ended. Repeat-safe:
// the gateway forgets a pass once it signalled it.
func (s *serviceImpl) signalPassEnded(ctx context.Context, sc *stepContext) {
	s.gate().PassEnded(ctx, domain.CallContract{
		TwinID:      sc.twinID,
		ObjectiveID: string(sc.obj.ID),
		LoopID:      sc.loopID,
		Pass:        sc.iteration,
	})
}

// signalObjectiveEnded tells the packs a one-shot objective called that it
// ended or was cancelled. A standing objective does not end with a run:
// reconcile runs its next pass.
func (s *serviceImpl) signalObjectiveEnded(ctx context.Context, obj objective.Objective) {
	if obj.ID == "" || obj.IsStanding() {
		return
	}
	s.gate().ObjectiveEnded(ctx, domain.CallContract{
		TwinID:      obj.TwinID,
		ObjectiveID: string(obj.ID),
	})
}

// signalRunEnd runs as the runner returns: the last pass ended, and the
// objective too when the run ended it or a stop cancelled it. A run that
// returned paused at a checkpoint has ended neither.
func (s *serviceImpl) signalRunEnd(ctx context.Context, sc *stepContext) {
	s.signalPassEnded(ctx, sc)
	sc.state.mu.RLock()
	over := sc.state.ended || sc.state.stoppedBy != ""
	sc.state.mu.RUnlock()
	if over {
		s.signalObjectiveEnded(ctx, sc.obj)
	}
}
