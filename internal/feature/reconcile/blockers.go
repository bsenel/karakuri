package reconcile

import (
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// noProgressPasses is how many passes that had work, with no new commit on the
// stream's branch, make the stream blocked with no_progress (Phase 40 step
// 14a). No objective or template field declares it yet, so it is a constant.
const noProgressPasses = 3

// noteBlockers will record the blockers a finished pass raised or cleared on
// the stream's state. Scaffold only: step 14a's code slice implements it.
func (s *Service) noteBlockers(st *reconcile.State, outcome reconcile.Outcome, final coreloop.State) {
}
