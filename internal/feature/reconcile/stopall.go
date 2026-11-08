package reconcile

import (
	"context"
	"errors"

	"github.com/bsenel/karakuri/internal/core/objective"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
)

// The kinds of run StopAll reaches, as its permission predicate and its
// NotReached entries name them.
const (
	StopAllKindLoop      = "loop"
	StopAllKindObjective = "objective"
)

// StopAllNotPermitted is the NotReached reason for a run the caller's
// predicate did not allow.
const StopAllNotPermitted = "not permitted"

// PausedObjective is one standing objective StopAll paused, and whether the
// pause cancelled a pass in flight.
type PausedObjective struct {
	ObjectiveID   objective.ObjectiveID `json:"objective_id"`
	PassCancelled bool                  `json:"pass_cancelled"`
}

// NotReachedRun is a run StopAll did not stop or pause, and why.
type NotReachedRun struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// StopAllResult is what StopAll did (Phase 37 step 4): the loops it stopped,
// the standing objectives it paused, and every run it could not reach, so a
// caller whose scope is narrower than the deployment is never told
// "everything" when it was not.
type StopAllResult struct {
	LoopsStopped     []featureloop.StopResult `json:"loops_stopped"`
	ObjectivesPaused []PausedObjective        `json:"objectives_paused"`
	NotReached       []NotReachedRun          `json:"not_reached"`
}

// errStopAllUnimplemented is returned until the use case lands.
var errStopAllUnimplemented = errors.New("stop-all: unimplemented")

// StopAll stops every running loop and pauses every standing objective that
// mayStop allows, on behalf of principal, for reason (Phase 37 step 4).
// mayStop is asked with StopAllKindLoop and a loop ID, or StopAllKindObjective
// and an objective ID; the API layer answers it from the caller's permissions.
func (s *Service) StopAll(ctx context.Context, principal, reason string, mayStop func(kind, id string) bool) (StopAllResult, error) {
	return StopAllResult{}, errStopAllUnimplemented
}
