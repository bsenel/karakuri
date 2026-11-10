package loop

import (
	"context"
	"errors"
	"fmt"

	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// StopFound is what a stop found the loop to be when it arrived.
type StopFound string

const (
	// StopFoundRunning is a loop that had not ended, including one waiting
	// at a checkpoint; the stop ended it.
	StopFoundRunning StopFound = "running"
	// StopFoundEnded is a loop that had already ended; the stop changed
	// nothing.
	StopFoundEnded StopFound = "ended"
	// StopFoundUnknown is a loop ID no process and no stored row knows.
	StopFoundUnknown StopFound = "unknown"
)

// StopResult is what Stop found and did (Phase 37 step 3).
type StopResult struct {
	LoopID string    `json:"loop_id"`
	Found  StopFound `json:"found"`

	// Completed are the actions the loop had finished when the stop arrived,
	// and Cut the one in flight that the stop cut, nil when none was (Phase
	// 37 step 5). They are for the stop's audit record, not the wire.
	Completed []StopAction `json:"-"`
	Cut       *StopAction  `json:"-"`
}

// StopAction is one action a stopped loop ran, as the stop's audit record
// names it (Phase 37 step 5).
type StopAction struct {
	CapabilityID string `json:"capability"`
	EnvID        string `json:"env_id,omitempty"`
	// Ended says, for the cut action, whether it is known to have ended. A
	// stop cancels the action and returns without waiting for it, so it is
	// StopActionEndedUnknown unless the loop saw it end.
	Ended StopActionEnded `json:"ended,omitempty"`
	// Workdir and Branch are what a delegated action may have left half
	// done: the worktree it ran in and the branch it wrote to.
	Workdir string `json:"workdir,omitempty"`
	Branch  string `json:"branch,omitempty"`
}

// StopActionEnded is whether a cut action is known to have ended.
type StopActionEnded string

const (
	// StopActionEndedYes is a cut action the loop saw end.
	StopActionEndedYes StopActionEnded = "ended"
	// StopActionEndedUnknown is a cut action not known to have ended.
	StopActionEndedUnknown StopActionEnded = "unknown"
)

// Stop ends a loop on behalf of principal, for reason (Phase 37 step 3).
//
// The order is the point. The stopped state is stored first, completed so
// that ListActiveLoopStates leaves it out, and only then is the loop's context
// cancelled: a restart landing between the two finds a stopped row and replays
// nothing. A checkpoint the loop was waiting at is closed as rejected by the
// principal, so a later decision on it is refused and authorises nothing.
//
// The stop comes from outside the loop; it does not change how the loop
// decides to end (AGENTS.md rule 8). A loop that has already ended is left as
// it is and reported so, except a watch: a loop of a standing objective that
// is not paused, whose wait for the next event is the objective's, not the
// loop's. Its row is stored stopped and it is reported running, so the caller
// that pauses the objective knows the watch was live.
func (s *serviceImpl) Stop(ctx context.Context, loopID, principal, reason string) (StopResult, error) {
	res := StopResult{LoopID: loopID}

	s.mu.RLock()
	state := s.states[loopID]
	cancel := s.cancels[loopID]
	s.mu.RUnlock()
	if state == nil {
		return s.stopStored(ctx, loopID, principal, reason)
	}

	state.mu.RLock()
	ended, objID := state.ended, state.status.ObjectiveID
	state.mu.RUnlock()
	watch := ended && s.watching(ctx, objID)
	if ended && !watch {
		res.Found = StopFoundEnded
		return res, nil
	}

	cpID, took := state.markStopped(principal, reason, watch)
	if !took {
		res.Found = StopFoundEnded
		return res, nil
	}
	res.Found = StopFoundRunning

	// Stored before the cancel. When the write fails the loop is cancelled
	// all the same, since it was asked to stop, and the error says the stop
	// is not durable: a restart could replay the row it left.
	persistErr := s.persistState(ctx, state, true)
	s.closeStopped(ctx, objID, cpID, principal, reason)
	if cancel != nil {
		cancel(errLoopStopped)
	}
	if persistErr != nil {
		return res, fmt.Errorf("loop %q stopped, but its stopped state was not stored: %w", loopID, persistErr)
	}
	return res, nil
}

// stopStored stops a loop no runner in this process holds, from its stored
// row alone.
func (s *serviceImpl) stopStored(ctx context.Context, loopID, principal, reason string) (StopResult, error) {
	res := StopResult{LoopID: loopID}
	st, err := s.store.GetLoopState(ctx, loopID)
	if errors.Is(err, coreerrors.ErrNotFound) {
		res.Found = StopFoundUnknown
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("read loop %q: %w", loopID, err)
	}
	if st.Outcome == loop.RunStopped || (st.Completed && !s.watching(ctx, st.ObjectiveID)) {
		res.Found = StopFoundEnded
		return res, nil
	}

	cpID := ""
	if st.Paused {
		cpID = st.CheckpointID
	}
	st.Completed = true
	st.Paused = false
	st.Status = objective.StatusStopped
	st.Outcome = loop.RunStopped
	st.Error = ""
	st.StoppedBy, st.StopReason = principal, reason
	if err := s.store.SaveLoopState(ctx, st); err != nil {
		return res, fmt.Errorf("store loop %q stopped: %w", loopID, err)
	}
	s.closeStopped(ctx, st.ObjectiveID, cpID, principal, reason)
	res.Found = StopFoundRunning
	return res, nil
}

// closeStopped closes what a stopped loop leaves open: the checkpoint it was
// waiting at, rejected by the principal who stopped it, and a one-shot
// objective's status. A standing objective's row is reconcile's.
//
// The rejection is recorded as closed by a stop rather than as a person's
// judgement of the plan, so it does not lower the escalation approval rate.
func (s *serviceImpl) closeStopped(ctx context.Context, objID objective.ObjectiveID, cpID, principal, reason string) {
	if cpID != "" && s.cpSvc != nil {
		_ = s.cpSvc.RecordClosedByStop(ctx, cpID, corecheckpoint.Decision{
			Choice:   "reject",
			Approver: principal,
			Note:     "The loop was stopped: " + reason,
		})
	}
	if objID == "" {
		return
	}
	if obj, err := s.store.GetObjective(ctx, objID); err == nil && !obj.IsStanding() {
		_ = s.store.UpdateObjectiveStatus(ctx, objID, objective.StatusStopped)
	}
}

// watching reports whether a loop of this objective is part of a live watch:
// the objective is standing and not paused, so its wait for the next event
// goes on after the loop's pass has ended.
func (s *serviceImpl) watching(ctx context.Context, objID objective.ObjectiveID) bool {
	obj, err := s.store.GetObjective(ctx, objID)
	if err != nil || !obj.IsStanding() {
		return false
	}
	rs, err := s.store.GetReconcileState(ctx, objID)
	if err != nil {
		// Declared standing and never yet reconciled is still a watch.
		return errors.Is(err, coreerrors.ErrNotFound)
	}
	return !rs.Paused
}
