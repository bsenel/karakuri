package reconcile

// Phase 40 step 9a: the pass that follows a cut-off one is told what the
// cut-off pass left, so it plans only what is left and works on the same
// branch.

import (
	"context"
	"errors"
	"log/slog"

	"github.com/bsenel/karakuri/internal/core/environment"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// resumeLookback is how many recorded passes are read to find the last one
// whose loop ran.
const resumeLookback = 10

// resumeFor is the brief the next pass is given, or nil after an ordinary
// ending. The last pass that ran a loop decides: one that ended
// waiting_for_quota or interrupted_by_restart was cut off.
func (s *Service) resumeFor(ctx context.Context, obj objective.Objective) *coreloop.Resume {
	history, err := s.store.ListReconcileOutcomes(ctx, obj.ID, resumeLookback)
	if err != nil {
		slog.Warn("reconcile: could not read the last pass to see whether it was cut off",
			"objective", obj.ID, "error", err)
		return nil
	}
	for _, last := range history {
		if last.LoopID == "" {
			continue
		}
		// A restart's cut-off record shares its loop, and often its start time,
		// with the escalated pass's own record, so the loop decides rather than
		// whichever of the two sorts first.
		cutOff := false
		for _, o := range history {
			if o.LoopID == last.LoopID &&
				(o.Deferred == outcomeWaitingForQuota || o.Ending == string(coreloop.RunInterruptedByRestart)) {
				cutOff = true
			}
		}
		if !cutOff {
			return nil
		}
		o := last
		brief := &coreloop.Resume{}
		if s.branchCommits != nil {
			brief.Branch, brief.Commits, err = s.branchCommits(ctx, obj)
			if err != nil {
				brief.Commits = nil
				brief.CommitsUnreadable = err.Error()
			}
		} else {
			brief.CommitsUnreadable = "no reader of the stream's branch is configured"
		}
		if ls, lerr := s.store.GetLoopState(ctx, o.LoopID); lerr == nil {
			for _, c := range ls.Criteria {
				if c.State != objective.CriterionMet {
					brief.Unsettled = append(brief.Unsettled, c.ID)
				}
			}
		}
		return brief
	}
	return nil
}

// streamBranch reads the stream's branch through the first of the objective's
// environments that can (environment.BranchReader). None able is an error,
// not an empty branch.
func (s *Service) streamBranch(ctx context.Context, obj objective.Objective) (string, []string, error) {
	var errs []error
	for _, env := range s.envsFor(ctx, obj) {
		r, ok := env.(environment.BranchReader)
		if !ok {
			continue
		}
		branch, commits, err := r.StreamBranch(ctx, obj.TemplateID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		return branch, commits, nil
	}
	if len(errs) > 0 {
		return "", nil, errors.Join(errs...)
	}
	return "", nil, errors.New("no environment of the objective can read the stream's branch")
}
