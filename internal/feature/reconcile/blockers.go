package reconcile

import (
	"context"
	"log/slog"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// unblockBrief is the blocker the stream's next pass is asked to diagnose and
// fix (Phase 40 step 14b): the one with the highest count, then the oldest
// first seen, then kind, then subject, so every pass chooses alike. Quota is
// waiting, not something a pass can fix, and is never chosen. The remedy is
// the objective's template's text for that kind.
func unblockBrief(blockers []reconcile.Blocker, tmpl objective.Template) *coreloop.Unblock {
	var best *reconcile.Blocker
	for i := range blockers {
		b := &blockers[i]
		if b.Kind == reconcile.BlockerDeferredQuota {
			continue
		}
		if best == nil || blockerBefore(*b, *best) {
			best = b
		}
	}
	if best == nil {
		return nil
	}
	return &coreloop.Unblock{
		Kind: string(best.Kind), Subject: best.Subject, Count: best.Count,
		FirstSeen: best.FirstSeen, LastSeen: best.LastSeen,
		Remedy: tmpl.BlockerRemedies[string(best.Kind)],
	}
}

// maxRecalledRemedies bounds how many stored remedies of one kind a pass is
// shown; the newest come first.
const maxRecalledRemedies = 3

// keepConfirmedRemedy stores the remedy a diagnose-and-fix pass applied as one
// semantic memory of the stream's agent, with the blocker's kind as its domain
// (Phase 40 step 14b). Only a confirmed remedy is kept: the pass named one and
// the blocker it was given is gone from the state after this pass's reading.
// A failed write is logged and costs the pass nothing.
func (s *Service) keepConfirmedRemedy(ctx context.Context, st *reconcile.State, agentID coreagent.AgentID, brief *coreloop.Unblock, remedy string) {
	if brief == nil || remedy == "" {
		return
	}
	for _, b := range st.Blockers {
		if string(b.Kind) == brief.Kind && b.Subject == brief.Subject {
			return
		}
	}
	err := s.store.SaveMemorySemantic(ctx, memory.Entry{
		ID: "remedy-" + newOutcomeID(), AgentID: agentID, Tier: string(memory.TierSemantic),
		Domain: brief.Kind, Content: remedy, CreatedAt: s.now(),
	})
	if err != nil {
		slog.Warn("reconcile: storing a confirmed remedy failed", "kind", brief.Kind, "error", err)
	}
}

// recalledRemedies are the remedies the agent stored for blockers of kind,
// newest first. A store that cannot answer recalls nothing.
func (s *Service) recalledRemedies(ctx context.Context, agentID coreagent.AgentID, kind string) []string {
	entries, err := s.store.QuerySemantic(ctx, memory.Query{AgentID: agentID, Domain: kind, TopK: maxRecalledRemedies})
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.Domain == kind && e.Content != "" {
			out = append(out, e.Content)
		}
	}
	return out
}

func blockerBefore(a, b reconcile.Blocker) bool {
	switch {
	case a.Count != b.Count:
		return a.Count > b.Count
	case !a.FirstSeen.Equal(b.FirstSeen):
		return a.FirstSeen.Before(b.FirstSeen)
	case a.Kind != b.Kind:
		return a.Kind < b.Kind
	}
	return a.Subject < b.Subject
}

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
func (s *Service) noteBlockers(st *reconcile.State, outcome reconcile.Outcome, final coreloop.State, fp *reconcile.Fingerprint, given *coreloop.Unblock) {
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

	// A sensed fingerprint lists the stream's unmergeable branches; one it no
	// longer lists is repaired, or its pull request is gone.
	if fp != nil {
		for _, branch := range fp.Unmergeable {
			raise(reconcile.BlockerUnmergeablePR, branch)
		}
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
		case reconcile.BlockerPlanError, reconcile.BlockerPlanRefused, reconcile.BlockerPushedRefsViolation, reconcile.BlockerDeferredQuota, reconcile.BlockerUnmergeablePR:
			if !raised[blockerKey{b.Kind, b.Subject}] {
				continue
			}
		}
		kept = append(kept, b)
	}
	st.Blockers = kept

	// The signal was read again above: a blocker the pass was given that is
	// still raised (a refused plan included) is one more failed attempt. One
	// that cleared is gone, and its attempts with it.
	if given != nil {
		for i := range st.Blockers {
			if b := &st.Blockers[i]; string(b.Kind) == given.Kind && b.Subject == given.Subject {
				b.Attempts++
			}
		}
	}

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
