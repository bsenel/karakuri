package reconcile

import (
	"context"
	"log/slog"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/event"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	"github.com/bsenel/karakuri/internal/platform/storage"
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
// runs: it detects, counts and shows. It returns the blockers it read cleared.
func (s *Service) noteBlockers(st *reconcile.State, outcome reconcile.Outcome, final coreloop.State, fp *reconcile.Fingerprint) []reconcile.Clear {
	at := outcome.EndedAt
	if at.IsZero() {
		at = s.now()
	}
	obj := string(outcome.ObjectiveID)

	// A deferral is shown and changes nothing else.
	if outcome.Deferred == "budget_exhausted" || outcome.Deferred == "provider_rate_limited" || outcome.Deferred == outcomeWaitingForQuota {
		raiseBlocker(st, reconcile.BlockerDeferredQuota, obj, at)
		return nil
	}
	// A look, a pause or a question to a person settled nothing.
	if outcome.LoopID == "" || outcome.Deferred != "" || outcome.Escalated {
		return nil
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
	var cleared []reconcile.Clear
	kept := st.Blockers[:0]
	for _, b := range st.Blockers {
		switch b.Kind {
		case reconcile.BlockerVerifyFailed:
			if met[b.Subject] || outcome.Converged {
				cleared = append(cleared, reconcile.Clear{Kind: b.Kind, Subject: b.Subject, At: at})
				continue
			}
		case reconcile.BlockerPlanError, reconcile.BlockerPlanRefused, reconcile.BlockerPushedRefsViolation, reconcile.BlockerDeferredQuota, reconcile.BlockerUnmergeablePR:
			if !raised[blockerKey{b.Kind, b.Subject}] {
				cleared = append(cleared, reconcile.Clear{Kind: b.Kind, Subject: b.Subject, At: at})
				continue
			}
		}
		kept = append(kept, b)
	}
	st.Blockers = kept

	if c, ok := noteNoProgress(st, outcome, fp, at); ok {
		cleared = append(cleared, c)
	}
	return cleared
}

// noteNoProgress counts passes with work that left the branch's tip where the
// previous one found it. A converged pass is work done, and a stream whose
// environment cannot see its branch is not counted: blind is not quiet. It
// returns the no_progress blocker it cleared, if it cleared one.
func noteNoProgress(st *reconcile.State, outcome reconcile.Outcome, fp *reconcile.Fingerprint, at time.Time) (reconcile.Clear, bool) {
	obj := string(outcome.ObjectiveID)
	sha := ""
	if fp != nil {
		sha = fp.Environments[branchEnvironment]
	}
	if sha == "" {
		return reconcile.Clear{}, false
	}
	moved := st.BranchSHA != "" && st.BranchSHA != sha
	st.BranchSHA = sha
	if moved || outcome.Converged {
		st.StalledPasses = 0
		if dropBlocker(st, reconcile.BlockerNoProgress, obj) {
			return reconcile.Clear{Kind: reconcile.BlockerNoProgress, Subject: obj, At: at}, true
		}
		return reconcile.Clear{}, false
	}
	st.StalledPasses++
	if st.StalledPasses >= noProgressPasses {
		raiseBlocker(st, reconcile.BlockerNoProgress, obj, at)
	}
	return reconcile.Clear{}, false
}

// blockerLimits are the template's declared attempts and recurrence window
// (Phase 40 step 14c), or the defaults where it declares none. A window that
// does not parse is the default too: the template's tests are where a bad
// duration is caught, and a stream must not lose its limit to one.
func blockerLimits(tmpl objective.Template) (int, time.Duration) {
	attempts := tmpl.BlockerAttempts
	if attempts <= 0 {
		attempts = objective.DefaultBlockerAttempts
	}
	window, err := time.ParseDuration(tmpl.BlockerRecurrenceWindow)
	if tmpl.BlockerRecurrenceWindow == "" || err != nil || window <= 0 {
		window, _ = time.ParseDuration(objective.DefaultBlockerRecurrenceWindow)
	}
	return attempts, window
}

// auditRowsOf are the IDs of the audit rows the stream wrote while the pass
// ran, oldest first. A store that cannot answer leaves the attempt without
// them rather than failing the pass.
func (s *Service) auditRowsOf(ctx context.Context, outcome reconcile.Outcome) []string {
	since, before := outcome.StartedAt, outcome.EndedAt.Add(time.Nanosecond)
	events, err := s.store.ListToolEvents(ctx, storage.ToolEventFilter{
		ObjectiveID: string(outcome.ObjectiveID), CreatedAtSince: &since, CreatedAtBefore: &before, OldestFirst: true,
	})
	if err != nil {
		slog.Warn("reconcile: reading an attempt's audit rows failed", "objective", outcome.ObjectiveID, "error", err)
		return nil
	}
	ids := make([]string, 0, len(events))
	for _, e := range events {
		ids = append(ids, e.ID)
	}
	return ids
}

// noteAttempt counts the pass as a failed attempt on the blocker it was given
// when the signal, read again by noteBlockers, is still raised (a refused plan
// included), and records what the pass found and tried (Phase 40 steps 14b and
// 14c). A blocker that cleared is gone, and its attempts with it. It returns
// the blocker attempted, nil when the pass was given none or it cleared.
func (s *Service) noteAttempt(ctx context.Context, st *reconcile.State, outcome reconcile.Outcome, final coreloop.State, given *coreloop.Unblock) *reconcile.Blocker {
	if given == nil {
		return nil
	}
	for i := range st.Blockers {
		b := &st.Blockers[i]
		if string(b.Kind) != given.Kind || b.Subject != given.Subject {
			continue
		}
		remedy := final.Remedy
		if remedy == "" {
			remedy = given.Remedy
		}
		b.Attempts++
		b.Tried = append(b.Tried, reconcile.Attempt{
			LoopID: outcome.LoopID, At: outcome.EndedAt, Diagnosis: final.Diagnosis, Remedy: remedy,
			AuditRows: s.auditRowsOf(ctx, outcome),
		})
		return b
	}
	return nil
}

// handoverOf is the stream's handover of kind, nil when it has none: one per
// kind, so later occurrences add evidence to it.
func handoverOf(st *reconcile.State, kind reconcile.BlockerKind) *reconcile.Handover {
	for i := range st.Handovers {
		if st.Handovers[i].Kind == kind {
			return &st.Handovers[i]
		}
	}
	return nil
}

// handOverSpent hands a blocker whose attempts are spent to the backlog, or
// adds its attempts to the kind's handover already listed.
func handOverSpent(st *reconcile.State, b reconcile.Blocker, at time.Time) {
	h := handoverOf(st, b.Kind)
	if h == nil {
		st.Handovers = append(st.Handovers, reconcile.Handover{Kind: b.Kind})
		h = &st.Handovers[len(st.Handovers)-1]
	}
	h.Subject = b.Subject
	h.Reason = reconcile.HandoverAttemptsSpent
	if d := reconcile.LastDiagnosis(b.Tried); d != "" {
		h.Diagnosis = d
	}
	h.Attempts = append(h.Attempts, b.Tried...)
	h.At = at
}

// noteRecurrence hands a blocker kind read cleared for the second time inside
// the window to the backlog, and adds each later clear of a kind already
// handed over to its handover as evidence (Phase 40 step 14c). The stream keeps
// running either way. Quota is waiting, not a blocker a fix removes, and is
// never handed over.
func noteRecurrence(st *reconcile.State, cleared []reconcile.Clear, window time.Duration, at time.Time) {
	kept := st.Clears[:0]
	for _, c := range st.Clears {
		if at.Sub(c.At) <= window {
			kept = append(kept, c)
		}
	}
	st.Clears = kept

	for _, c := range cleared {
		if c.Kind == reconcile.BlockerDeferredQuota {
			continue
		}
		if h := handoverOf(st, c.Kind); h != nil {
			h.Cleared = append(h.Cleared, c.At)
			continue
		}
		st.Clears = append(st.Clears, c)
		var times []time.Time
		for _, prior := range st.Clears {
			if prior.Kind == c.Kind {
				times = append(times, prior.At)
			}
		}
		if len(times) < 2 {
			continue
		}
		st.Handovers = append(st.Handovers, reconcile.Handover{
			Kind: c.Kind, Subject: c.Subject, Reason: reconcile.HandoverRecurring, Cleared: times, At: at,
		})
		rest := st.Clears[:0]
		for _, prior := range st.Clears {
			if prior.Kind != c.Kind {
				rest = append(rest, prior)
			}
		}
		st.Clears = rest
	}
}

// stopOnBlocker takes a stream that spent its attempts on a blocker out of
// rotation (Phase 40 step 14c): the objective is blocked and the state paused
// with "blocked on <diagnosis>, tried <what>", so no further pass is spent on
// it until the owner resumes it. The breaker is not involved: the stream ends
// with a diagnosis, not a breaker checkpoint that has none.
func (s *Service) stopOnBlocker(ctx context.Context, st *reconcile.State, obj objective.Objective, b reconcile.Blocker) {
	pausedAt := s.now()
	st.Paused = true
	st.PausedReason = reconcile.BlockedOn(reconcile.LastDiagnosis(b.Tried), b.Tried)
	st.PausedBy, st.PausedAt = "", &pausedAt
	st.Phase = reconcile.PhasePaused
	st.NextDueAt = nil
	_ = s.store.UpdateObjectiveStatus(ctx, obj.ID, objective.StatusBlocked)
	s.publish(ctx, event.TypeObjectiveSuspended, obj, map[string]any{
		"reason":   "blocker_attempts_spent",
		"summary":  st.PausedReason,
		"kind":     string(b.Kind),
		"subject":  b.Subject,
		"attempts": b.Attempts,
	})
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

// dropBlocker removes a blocker and says whether it was there.
func dropBlocker(st *reconcile.State, kind reconcile.BlockerKind, subject string) bool {
	dropped := false
	kept := st.Blockers[:0]
	for _, b := range st.Blockers {
		if b.Kind != kind || b.Subject != subject {
			kept = append(kept, b)
		} else {
			dropped = true
		}
	}
	st.Blockers = kept
	return dropped
}
