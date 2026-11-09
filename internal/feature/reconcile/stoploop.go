package reconcile

import (
	"context"
	"errors"
	"fmt"

	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	"github.com/bsenel/karakuri/internal/core/objective"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
)

// LoopStopResult is what StopLoop found and did (Phase 37 step 3): the loop
// service's answer, and the objective the loop belongs to, if any, with
// whether the stop paused it. Embedded so the wire shape keeps loop_id and
// found and adds the objective's two fields.
type LoopStopResult struct {
	featureloop.StopResult
	ObjectiveID     objective.ObjectiveID `json:"objective_id,omitempty"`
	ObjectivePaused bool                  `json:"objective_paused"`
}

// StopLoop stops a loop on behalf of principal, for reason. When the loop
// belongs to a standing objective that is not paused, it also pauses that
// objective with the reason, so the watch ends and no later tick starts a new
// pass (Phase 37 step 3).
//
// The pause comes first. Stored paused, the objective is not due to any tick
// and Trigger refuses it, so nothing starts a pass between the pause and the
// stop; stopping first would leave that window open. The pause also cancels a
// pass in flight, as an operator's pause does, and that pass's loop may be
// stopped by finishStopped before the stop below arrives, or read as ended
// because its objective is now paused. Either way the stop ended a live watch,
// so a loop service answer of "ended" is reported as "running" when this call
// paused the objective.
//
// A pause that fails does not hold back the stop the caller asked for; the
// loop is stopped and the error says the objective was not paused.
func (s *Service) StopLoop(ctx context.Context, loopID, principal, reason string) (LoopStopResult, error) {
	var res LoopStopResult
	var pauseErr error
	st, err := s.store.GetLoopState(ctx, loopID)
	switch {
	case err == nil:
		res.ObjectiveID = st.ObjectiveID
		if st.ObjectiveID != "" {
			res.ObjectivePaused, pauseErr = s.pauseForStop(ctx, st.ObjectiveID, principal, reason)
		}
	case !errors.Is(err, coreerrors.ErrNotFound):
		return res, fmt.Errorf("read loop %q: %w", loopID, err)
	}

	stopped, err := s.loops.Stop(ctx, loopID, principal, reason)
	res.StopResult = stopped
	if err != nil {
		return res, err
	}
	if res.ObjectivePaused && res.Found == featureloop.StopFoundEnded {
		res.Found = featureloop.StopFoundRunning
	}
	if pauseErr != nil {
		return res, fmt.Errorf("loop %q stopped, but its objective %q was not paused: %w",
			loopID, res.ObjectiveID, pauseErr)
	}
	return res, nil
}

// pauseForStop pauses a standing objective for a loop stop and reports
// whether it did. An objective with no control-loop state (a one-shot one)
// has nothing to pause, and one already paused keeps the reason it was paused
// with.
func (s *Service) pauseForStop(ctx context.Context, id objective.ObjectiveID, principal, reason string) (bool, error) {
	st, err := s.store.GetReconcileState(ctx, id)
	if errors.Is(err, coreerrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if st.Paused {
		return false, nil
	}
	if err := s.Pause(ctx, id, principal, reason); err != nil {
		return false, err
	}
	return true, nil
}
