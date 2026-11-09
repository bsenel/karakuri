package reconcile

import (
	"context"
	"errors"
	"fmt"

	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
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

// StopAll stops every running loop and pauses every standing objective that
// mayStop allows, on behalf of principal, for reason (Phase 37 step 4).
// mayStop is asked with StopAllKindLoop and a loop ID, or StopAllKindObjective
// and an objective ID; the API layer answers it from the caller's permissions.
//
// Objectives come first, through Pause, so each pass in flight is cancelled as
// step 1 does and no tick starts a new pass while the loops are being stopped.
// A loop of a standing objective that was not paused when StopAll read it is
// reported once, as that objective (paused, or not reached), and is neither
// stopped nor listed again: the pause ends its pass. Every other running loop
// is stopped through the loop service, as step 3 does.
//
// An objective already paused, or a loop that has already ended, is not
// listed, so a second call reports that nothing changed. A run a store or Stop
// error held back is listed as not reached with that error's text. A listing
// that fails returns its error along with what was done before it.
func (s *Service) StopAll(ctx context.Context, principal, reason string, mayStop func(kind, id string) bool) (StopAllResult, error) {
	res := StopAllResult{
		LoopsStopped:     []featureloop.StopResult{},
		ObjectivesPaused: []PausedObjective{},
		NotReached:       []NotReachedRun{},
	}
	notReached := func(kind, id, why string) {
		res.NotReached = append(res.NotReached, NotReachedRun{Kind: kind, ID: id, Reason: why})
	}

	ids, err := s.store.ListReconcileStateIDs(ctx)
	if err != nil {
		return res, fmt.Errorf("list standing objectives: %w", err)
	}
	// covered holds the objectives this call answers for, so their loops are
	// not reported a second time as loops.
	covered := make(map[objective.ObjectiveID]bool, len(ids))
	for _, id := range ids {
		st, err := s.store.GetReconcileState(ctx, id)
		if errors.Is(err, coreerrors.ErrNotFound) {
			// Forgotten since the listing: no longer standing.
			continue
		}
		if err == nil && st.Paused {
			continue
		}
		covered[id] = true
		switch {
		case err != nil:
			notReached(StopAllKindObjective, string(id), err.Error())
		case !mayStop(StopAllKindObjective, string(id)):
			notReached(StopAllKindObjective, string(id), StopAllNotPermitted)
		default:
			cancelled, err := s.pause(ctx, id, principal, reason)
			if err != nil {
				notReached(StopAllKindObjective, string(id), err.Error())
				continue
			}
			res.ObjectivesPaused = append(res.ObjectivesPaused, PausedObjective{ObjectiveID: id, PassCancelled: cancelled})
		}
	}

	loops, err := s.store.ListActiveLoopStates(ctx)
	if err != nil {
		return res, fmt.Errorf("list running loops: %w", err)
	}
	for _, l := range loops {
		if l.ObjectiveID != "" && covered[l.ObjectiveID] {
			continue
		}
		if !mayStop(StopAllKindLoop, l.LoopID) {
			notReached(StopAllKindLoop, l.LoopID, StopAllNotPermitted)
			continue
		}
		stopped, err := s.loops.Stop(ctx, l.LoopID, principal, reason)
		if err != nil {
			notReached(StopAllKindLoop, l.LoopID, err.Error())
			continue
		}
		// A loop that ended between the listing and the stop changed nothing.
		if stopped.Found == featureloop.StopFoundRunning {
			res.LoopsStopped = append(res.LoopsStopped, stopped)
		}
	}
	return res, nil
}
