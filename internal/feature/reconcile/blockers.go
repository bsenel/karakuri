package reconcile

import (
	"time"

	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// noProgressPasses is how many passes that had work, with no new commit on the
// stream's branch, make the stream blocked with no_progress (Phase 40 step
// 14a). No objective or template field declares it yet, so it is a constant.
const noProgressPasses = 3

// branchEnvironment is the environment whose fingerprint is the stream's
// branch tip.
const branchEnvironment = "git"

// noteBlockers records on the stream's state what a finished pass raised or
// cleared (Phase 40 step 14a). It reads only signals the pass already
// produced, calls no model, and touches no counter that decides whether a pass
// runs: it detects, counts and shows.
//
// unmergeable_pr is not raised here: nothing reads a pull request's
// mergeability yet (roadmap step 6a), so no signal reaches reconcile.
func (s *Service) noteBlockers(st *reconcile.State, outcome reconcile.Outcome, final coreloop.State, fp *reconcile.Fingerprint) {
	at := outcome.EndedAt
	if at.IsZero() {
		at = s.now()
	}
	obj := string(outcome.ObjectiveID)

	// A deferral is shown and changes nothing else.
	if outcome.Deferred == "budget_exhausted" || outcome.Deferred == "provider_rate_limited" {
		raiseBlocker(st, reconcile.BlockerDeferredQuota, obj, at)
		return
	}
	// A look, a pause or a question to a person settled nothing.
	if outcome.LoopID == "" || outcome.Deferred != "" || outcome.Escalated {
		return
	}

	raised := map[blockerKey]bool{}
	raise := func(kind reconcile.BlockerKind, subject string) {
		raised[blockerKey{kind, subject}] = true
		raiseBlocker(st, kind, subject, at)
	}

	if final.Outcome == coreloop.RunErrored {
		raise(reconcile.BlockerPlanError, obj)
	} else {
		for _, c := range final.Criteria {
			if c.IsRequired() && c.State == objective.CriterionNotMet {
				raise(reconcile.BlockerVerifyFailed, c.ID)
			}
		}
	}
	if final.PlanRefusedReason != "" {
		raise(reconcile.BlockerPlanRefused, obj)
	}
	if v := final.BoundsViolation; v != nil && len(v.Refs) > 0 {
		raise(reconcile.BlockerPushedRefsViolation, v.Refs[0])
	}

	// Progress on a subject removes its blocker: a criterion now met, or a
	// signal that did not come back on a pass that ran to its end.
	met := map[string]bool{}
	for _, c := range final.Criteria {
		if c.State == objective.CriterionMet {
			met[c.ID] = true
		}
	}
	kept := st.Blockers[:0]
	for _, b := range st.Blockers {
		switch b.Kind {
		case reconcile.BlockerVerifyFailed:
			if met[b.Subject] || outcome.Converged {
				continue
			}
		case reconcile.BlockerPlanError, reconcile.BlockerPlanRefused, reconcile.BlockerPushedRefsViolation, reconcile.BlockerDeferredQuota:
			if !raised[blockerKey{b.Kind, b.Subject}] {
				continue
			}
		}
		kept = append(kept, b)
	}
	st.Blockers = kept

	noteNoProgress(st, outcome, fp, at)
}

// noteNoProgress counts passes with work that left the branch's tip where the
// previous one found it. A converged pass is work done, and a stream whose
// environment cannot see its branch is not counted: blind is not quiet.
func noteNoProgress(st *reconcile.State, outcome reconcile.Outcome, fp *reconcile.Fingerprint, at time.Time) {
	obj := string(outcome.ObjectiveID)
	sha := ""
	if fp != nil {
		sha = fp.Environments[branchEnvironment]
	}
	if sha == "" {
		return
	}
	moved := st.BranchSHA != "" && st.BranchSHA != sha
	st.BranchSHA = sha
	if moved || outcome.Converged {
		st.StalledPasses = 0
		dropBlocker(st, reconcile.BlockerNoProgress, obj)
		return
	}
	st.StalledPasses++
	if st.StalledPasses >= noProgressPasses {
		raiseBlocker(st, reconcile.BlockerNoProgress, obj, at)
	}
}

type blockerKey struct {
	kind    reconcile.BlockerKind
	subject string
}

// raiseBlocker counts a blocker seen again, or adds it.
func raiseBlocker(st *reconcile.State, kind reconcile.BlockerKind, subject string, at time.Time) {
	for i := range st.Blockers {
		if b := &st.Blockers[i]; b.Kind == kind && b.Subject == subject {
			b.Count++
			b.LastSeen = at
			return
		}
	}
	st.Blockers = append(st.Blockers, reconcile.Blocker{
		Kind: kind, Subject: subject, Count: 1, FirstSeen: at, LastSeen: at,
	})
}

func dropBlocker(st *reconcile.State, kind reconcile.BlockerKind, subject string) {
	kept := st.Blockers[:0]
	for _, b := range st.Blockers {
		if b.Kind != kind || b.Subject != subject {
			kept = append(kept, b)
		}
	}
	st.Blockers = kept
}
