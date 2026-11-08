package reconcile

import (
	"context"
	"errors"

	"github.com/bsenel/karakuri/internal/core/objective"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
)

// LoopStopResult is what StopLoop found and did (Phase 37 step 3): the loop
// service's answer, and the standing objective the loop belongs to, if any,
// with whether the stop paused it. Embedded so the wire shape keeps loop_id
// and found and adds the objective's two fields.
type LoopStopResult struct {
	featureloop.StopResult
	ObjectiveID     objective.ObjectiveID `json:"objective_id,omitempty"`
	ObjectivePaused bool                  `json:"objective_paused"`
}

// StopLoop stops a loop on behalf of principal, for reason. When the loop
// belongs to a standing objective it also pauses that objective, so the watch
// ends and no later tick starts a new pass (Phase 37 step 3).
//
// Stub: specified by stoploop_test.go, implemented in the next step.
func (s *Service) StopLoop(ctx context.Context, loopID, principal, reason string) (LoopStopResult, error) {
	return LoopStopResult{}, errors.New("reconcile: StopLoop is unimplemented")
}
