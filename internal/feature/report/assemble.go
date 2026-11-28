// Package report turns what a twin's standing objectives did into something a
// person reads and acts on.
//
// The whole feature is a read: reconcile outcomes, the audit log, pending
// checkpoints and the cost ledger already record everything a digest says.
// Nothing here is written as work happens, so a digest can be regenerated for
// any past window and will say the same thing — which is also what makes it
// safe to send twice by mistake and dull to get wrong.
package report

import (
	"context"
	"fmt"
	"sort"
	"time"

	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/digest"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Assemble builds the digest for one twin over one window.
//
// Errors from individual sources are swallowed rather than failing the digest.
// A report that arrives without its cost line is worth far more than a report
// that does not arrive because the ledger was briefly unreachable — and the
// missing line is visible in the output, so nothing is silently claimed.
func (s *Service) Assemble(ctx context.Context, twinID string, since, until time.Time) (digest.Digest, error) {
	d := digest.Digest{TwinID: twinID, Since: since, Until: until}

	if t, err := s.store.GetTwin(ctx, twinID); err == nil {
		d.TwinName = t.Name
	}

	objectives, err := s.store.ListObjectives(ctx, storage.ObjectiveFilter{
		TwinID: twinID,
		Mode:   string(objective.ModeStanding),
	})
	if err != nil {
		return d, err
	}

	// Read once: the forge is asked one question per twin, not one per stream.
	// Skipped for a twin with no standing objective, which has nothing to list.
	var pulls []StreamPullRequest
	var pullsErr error
	if len(objectives) > 0 {
		pulls, pullsErr = s.readPullRequests(ctx, twinID)
	}

	titles := make(map[objective.ObjectiveID]string, len(objectives))
	for _, obj := range objectives {
		titles[obj.ID] = obj.Title
		sum := s.summarise(ctx, obj, since, until)
		var blind *digest.Block
		sum.WaitingPullRequests, blind = waitingFor(pulls, pullsErr, obj.ID, until)
		if blind != nil {
			sum.Blocks = append(sum.Blocks, *blind)
			sort.SliceStable(sum.Blocks, func(i, j int) bool { return sum.Blocks[i].Kind < sum.Blocks[j].Kind })
		}
		d.Objectives = append(d.Objectives, sum)
		d.AutonomyChanges = append(d.AutonomyChanges, s.autonomyChanges(ctx, obj, since)...)
		if x, ok := s.exhaustion(ctx, obj, since, until); ok {
			d.Exhausted = append(d.Exhausted, x)
		}
	}

	// Noisiest first. A reader skimming a morning brief should meet the
	// objective that failed four times before the one that quietly converged.
	sort.SliceStable(d.Objectives, func(i, j int) bool {
		return rank(d.Objectives[i]) > rank(d.Objectives[j])
	})
	sort.SliceStable(d.AutonomyChanges, func(i, j int) bool {
		return d.AutonomyChanges[i].At.After(d.AutonomyChanges[j].At)
	})
	// Most often stopped first: once at the end of a busy day is a budget
	// doing its job, and a dozen times is a ceiling set below what the cadence
	// asks for.
	sort.SliceStable(d.Exhausted, func(i, j int) bool {
		return d.Exhausted[i].Times > d.Exhausted[j].Times
	})

	d.Decisions = s.decisions(ctx, twinID, titles)
	d.Spend = s.spend(ctx, twinID, since, until)
	return d, nil
}

// rank orders objectives by how much they want attention. Failures first,
// then things waiting on a person, then anything that moved at all.
func rank(o digest.ObjectiveSummary) int {
	n := o.Failures*100 + o.Escalations*50 + o.DriftDetected*5 + o.Reconciles
	if o.Paused {
		n += 1000
	}
	return n
}

func (s *Service) summarise(ctx context.Context, obj objective.Objective, since, until time.Time) digest.ObjectiveSummary {
	sum := digest.ObjectiveSummary{ID: obj.ID, Title: obj.Title, Status: obj.Status}

	var stateBlockers []reconcile.Blocker
	var handovers []reconcile.Handover
	if st, err := s.store.GetReconcileState(ctx, obj.ID); err == nil {
		stateBlockers = st.Blockers
		handovers = st.Handovers
		sum.Autonomy = st.EffectiveAutonomy(obj.AutonomyDeclaration())
		sum.CriteriaMet = st.CriteriaMet
		sum.Paused = st.Paused
		sum.PausedWhy = st.PausedReason
		sum.LastError = st.LastError
	}

	// A generous cap rather than a page: a fifteen-minute sense cadence
	// produces ninety-six outcomes a day, and a digest that counted only the
	// first fifty would under-report the cheap passes — which are exactly the
	// number a reader is checking.
	outcomes, err := s.store.ListReconcileOutcomes(ctx, obj.ID, 2000)
	if err == nil {
		sum.Criteria, sum.Progress = s.criteria(ctx, obj, outcomes, since, until)
		for _, o := range outcomes {
			if o.StartedAt.Before(since) || o.StartedAt.After(until) {
				continue
			}
			if o.LoopID == "" {
				sum.Senses++
			} else {
				sum.Reconciles++
			}
			if o.Drift.Changed {
				sum.DriftDetected++
			}
			if o.Converged {
				sum.Converged++
			}
			if o.Escalated {
				sum.Escalations++
			}
			if o.Failed() {
				sum.Failures++
			}
			// A pass out of quota is filed as a deferral, so the history
			// floors its next pass; to a reader it is how the pass ended,
			// counted beside step 7's endings (Phase 40 step 8).
			ending := o.Ending
			if o.Deferred == waitingForQuotaOutcome {
				ending = waitingForQuotaOutcome
			}
			if ending != "" {
				if sum.Endings == nil {
					sum.Endings = map[string]int{}
				}
				sum.Endings[ending]++
			}
		}
	}

	// Actions come from the audit log rather than from the outcome: an
	// outcome says a loop ran, and the audit says what the loop touched.
	events, err := s.store.ListToolEvents(ctx, storage.ToolEventFilter{
		ObjectiveID:    string(obj.ID),
		Kind:           storage.ToolEventExecute,
		CreatedAtSince: &since,
		Limit:          1000,
	})
	if err == nil {
		for _, e := range events {
			if !e.CreatedAt.After(until) {
				sum.Actions++
			}
		}
	}
	sum.Refusals, sum.Blocks = s.refusalsAndBlocks(ctx, obj, since, until)
	sum.Blocks = withBlockers(sum.Blocks, stateBlockers, handovers, until)
	return sum
}

// refusalsAndBlocks reads the bounds refusals and the blocks of one stream
// for the window, oldest first.
//
// Refusals are the audit log's refusal rows in [since, until]. The blocks are
// what the stream is stuck on at the end of the window: the latest pass in the
// window deferred for quota or unable to see any environment, and the latest
// refusal of a single action (a row naming a capability, as the pushed-refs
// check writes; a refused plan names none) as a bounds violation.
func (s *Service) refusalsAndBlocks(ctx context.Context, obj objective.Objective, since, until time.Time) ([]digest.Refusal, []digest.Block) {
	var refusals []digest.Refusal
	var blocks []digest.Block

	events, err := s.store.ListToolEvents(ctx, storage.ToolEventFilter{
		ObjectiveID:    string(obj.ID),
		Kind:           storage.ToolEventRefusal,
		CreatedAtSince: &since,
		OldestFirst:    true,
		Limit:          1000,
	})
	if err != nil {
		// Blind, not quiet: no rows read is not no refusals (AGENTS.md rule 10).
		blocks = append(blocks, digest.Block{Kind: digest.BlockRemoteUnreadable, Reason: "audit log could not be read: " + err.Error(), At: until})
	} else {
		var violation *digest.Block
		for _, e := range events {
			if e.CreatedAt.After(until) {
				continue
			}
			refusals = append(refusals, digest.Refusal{At: e.CreatedAt, Capability: e.Capability, Reason: e.EscalationReason})
			if e.Capability != "" {
				violation = &digest.Block{Kind: digest.BlockBoundsViolation, Reason: e.EscalationReason, At: e.CreatedAt}
			}
		}
		if violation != nil {
			blocks = append(blocks, *violation)
		}
	}

	outcomes, err := s.store.ListReconcileOutcomes(ctx, obj.ID, 2000)
	if err != nil {
		blocks = append(blocks, digest.Block{Kind: digest.BlockRemoteUnreadable, Reason: "reconcile outcomes could not be read: " + err.Error(), At: until})
	} else {
		var latest *reconcile.Outcome
		for i := range outcomes {
			o := &outcomes[i]
			if o.StartedAt.Before(since) || o.StartedAt.After(until) {
				continue
			}
			if latest == nil || o.StartedAt.After(latest.StartedAt) {
				latest = o
			}
		}
		if latest != nil {
			if latest.Deferred == string(digest.BlockQuota) {
				blocks = append(blocks, digest.Block{Kind: digest.BlockQuota, Reason: latest.Deferred, At: latest.StartedAt, Until: latest.DeferredUntil})
			}
			if latest.Drift.Blind {
				blocks = append(blocks, digest.Block{Kind: digest.BlockRemoteUnreadable, Reason: "no environment could be read", At: latest.StartedAt})
			}
		}
	}

	sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].Kind < blocks[j].Kind })
	return refusals, blocks
}

// withBlockers adds what the stream's stored reconcile state says it is stuck
// on (Phase 40 step 14a), in a stable order: by kind, then subject. A blocker
// first seen after the window's end is not part of that window. A quota
// deferral already shown from the latest pass is not listed twice; otherwise
// it is shown as waiting for quota. Each shows how many diagnose-and-fix
// passes failed on it, and one whose attempts are spent and handed to the
// backlog reads "blocked on <diagnosis>, tried <what>" (Phase 40 step 14c).
func withBlockers(blocks []digest.Block, blockers []reconcile.Blocker, handovers []reconcile.Handover, until time.Time) []digest.Block {
	for _, b := range blockers {
		if b.FirstSeen.After(until) {
			continue
		}
		kind := digest.BlockKind(b.Kind)
		if b.Kind == reconcile.BlockerDeferredQuota {
			kind = digest.BlockQuota
			if hasKind(blocks, kind) {
				continue
			}
		}
		block := digest.Block{Kind: kind, Subject: b.Subject, Count: b.Count, Attempts: b.Attempts, At: b.LastSeen}
		if h, ok := spentHandover(handovers, b); ok {
			diagnosis := h.Diagnosis
			if diagnosis == "" {
				diagnosis = reconcile.LastDiagnosis(b.Tried)
			}
			tried := b.Tried
			if len(tried) == 0 {
				tried = h.Attempts
			}
			block.Reason = reconcile.BlockedOn(diagnosis, tried)
		}
		blocks = append(blocks, block)
	}
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].Kind != blocks[j].Kind {
			return blocks[i].Kind < blocks[j].Kind
		}
		return blocks[i].Subject < blocks[j].Subject
	})
	return blocks
}

// spentHandover is the handover that says the stream spent its attempts on
// this very blocker; a recurring kind, or another subject of it, is not.
func spentHandover(handovers []reconcile.Handover, b reconcile.Blocker) (reconcile.Handover, bool) {
	for _, h := range handovers {
		if h.Kind == b.Kind && h.Subject == b.Subject && h.Reason == reconcile.HandoverAttemptsSpent {
			return h, true
		}
	}
	return reconcile.Handover{}, false
}

func hasKind(blocks []digest.Block, kind digest.BlockKind) bool {
	for _, b := range blocks {
		if b.Kind == kind {
			return true
		}
	}
	return false
}

// criteria reads the criterion states the window's passes settled and returns
// the required criteria worth a line, and the optional weighted ones as
// progress.
//
// A pass's states are on the loop row its outcome points at, so this is a read
// like the rest of the digest. Sense-only outcomes settle nothing and are
// skipped rather than counted as a break in a run of passes. Outcomes arrive
// newest first, so passes[0] is the latest pass.
func (s *Service) criteria(ctx context.Context, obj objective.Objective, outcomes []reconcile.Outcome, since, until time.Time) ([]digest.CriterionLine, *digest.Progress) {
	if len(obj.SuccessCriteria) == 0 {
		return nil, nil
	}
	var passes []map[string]objective.Criterion
	for _, o := range outcomes {
		if o.LoopID == "" || o.StartedAt.Before(since) || o.StartedAt.After(until) {
			continue
		}
		st, err := s.store.GetLoopState(ctx, o.LoopID)
		if err != nil {
			continue
		}
		settled := make(map[string]objective.Criterion, len(st.Criteria))
		for _, c := range st.Criteria {
			settled[c.ID] = c
		}
		passes = append(passes, settled)
	}

	var lines []digest.CriterionLine
	var progress *digest.Progress
	for _, c := range obj.SuccessCriteria {
		if !c.IsRequired() {
			if c.Weight <= 0 {
				continue
			}
			if progress == nil {
				progress = &digest.Progress{}
			}
			progress.Of += c.Weight
			if len(passes) > 0 && passes[0][c.ID].IsMet() {
				progress.Earned += c.Weight
			}
			continue
		}
		if len(passes) == 0 {
			continue
		}
		state, reason := stateIn(passes[0], c.ID)
		n := 1
		for _, p := range passes[1:] {
			if st, _ := stateIn(p, c.ID); st != state {
				break
			}
			n++
		}
		// not_met is news at once. Inconclusive is news from the second pass:
		// once is a verifier that had nothing to read yet, twice in a row is a
		// criterion that will neither complete nor fail.
		if state == objective.CriterionNotMet || (state == objective.CriterionInconclusive && n > 1) {
			lines = append(lines, digest.CriterionLine{
				ID: c.ID, Description: c.Description, State: state, Reason: reason, Passes: n,
			})
		}
	}
	return lines, progress
}

// stateIn is what one pass settled about a criterion. A criterion the pass
// left without a state, or did not carry at all, was never run by its
// verifier: inconclusive: not_run, never met.
func stateIn(pass map[string]objective.Criterion, criterionID string) (objective.CriterionState, objective.InconclusiveReason) {
	c := pass[criterionID]
	if c.State == "" {
		return objective.CriterionInconclusive, objective.ReasonNotRun
	}
	return c.State, c.Reason
}

func (s *Service) autonomyChanges(ctx context.Context, obj objective.Objective, since time.Time) []digest.AutonomyChange {
	var out []digest.AutonomyChange
	for _, kind := range []string{storage.ToolEventPromotion, storage.ToolEventDemotion} {
		events, err := s.store.ListToolEvents(ctx, storage.ToolEventFilter{
			ObjectiveID:    string(obj.ID),
			Kind:           kind,
			CreatedAtSince: &since,
			Limit:          50,
		})
		if err != nil {
			continue
		}
		for _, e := range events {
			from, to, reason := decodeAutonomyPayload(e.PayloadJSON)
			out = append(out, digest.AutonomyChange{
				ObjectiveID:    obj.ID,
				ObjectiveTitle: obj.Title,
				From:           from,
				To:             to,
				Reason:         reason,
				At:             e.CreatedAt,
			})
		}
	}
	return out
}

// decisions lists what the reader owes an answer on.
//
// Oldest first, deliberately against the usual newest-first: the checkpoint
// that has been waiting three days is the one blocking work, and burying it
// under this morning's is how a queue grows.
func (s *Service) decisions(ctx context.Context, twinID string, titles map[objective.ObjectiveID]string) []digest.Decision {
	pending, err := s.store.ListPendingCheckpoints(ctx, twinID)
	if err != nil {
		return nil
	}
	out := make([]digest.Decision, 0, len(pending))
	for _, cp := range pending {
		out = append(out, digest.Decision{
			CheckpointID:   cp.ID,
			ObjectiveID:    cp.ObjectiveID,
			ObjectiveTitle: titles[cp.ObjectiveID],
			Reason:         cp.Reason,
			Summary:        cp.Summary,
			Options:        cp.Options,
			Proposed:       proposedCapabilities(cp.Actions),
			WaitingAt:      cp.CreatedAt,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].WaitingAt.Before(out[j].WaitingAt) })
	return out
}

func proposedCapabilities(actions []corecheckpoint.Action) []string {
	if len(actions) == 0 {
		return nil
	}
	out := make([]string, 0, len(actions))
	for _, a := range actions {
		out = append(out, a.CapabilityID)
	}
	return out
}

// spend reads the ledger for this twin over the window.
//
// An unpriced deployment reports Priced=false rather than a bare zero: units
// are counted whether or not a rate table exists, and a report that showed
// "$0.00" for work that happened would be claiming something untrue.
func (s *Service) spend(ctx context.Context, twinID string, since, until time.Time) digest.Spend {
	sp := digest.Spend{ByProvider: map[string]float64{}}
	buckets, err := s.costs(ctx, twinID, since, until)
	if err != nil {
		// The ledger could not be read. Priced stays false: "we could not
		// cost this" is nearer the truth than a confident $0.00.
		return sp
	}
	// Nothing happened in the window. That is a quiet week, not a missing
	// rate table, and a brief that says "nothing was costed" on a fully
	// priced deployment is simply wrong.
	sp.Priced = true

	var units float64
	for _, b := range buckets {
		provider := ""
		if len(b.Key) > 0 {
			provider = b.Key[0]
		}
		sp.Cost += b.Cost
		sp.ByProvider[provider] += b.Cost
		units += b.Units
	}

	// Priced asks whether a rate table exists, not whether anything was
	// spent. Units recorded with no cost against them is what says none
	// does — deriving it from the total instead made Priced a restatement
	// of Cost, so every genuinely quiet window was reported as unpriced.
	if units > 0 && sp.Cost == 0 {
		sp.Priced = false
	}
	return sp
}

// windowOf resolves the reporting window for a schedule at a moment.
func windowOf(sch digest.Schedule, now time.Time) (since, until time.Time) {
	return sch.Since(now).UTC(), now.UTC()
}

// exhaustion reports whether an objective stopped for want of money in the
// window, and what it was mid-way through when it did.
//
// Read from the outcomes the supervisor already writes: a budget deferral is
// recorded as an Outcome with Deferred set, deliberately not as a failure, so
// the circuit breaker never sees it and earned autonomy survives. That makes
// this a pure read like everything else in the digest — nothing new is written
// as work happens.
func (s *Service) exhaustion(ctx context.Context, obj objective.Objective, since, until time.Time) (digest.BudgetExhaustion, bool) {
	budget := obj.BudgetDeclaration()
	if !budget.Declared() {
		return digest.BudgetExhaustion{}, false
	}

	outcomes, err := s.store.ListReconcileOutcomes(ctx, obj.ID, 2000)
	if err != nil {
		return digest.BudgetExhaustion{}, false
	}

	x := digest.BudgetExhaustion{
		ObjectiveID:    obj.ID,
		ObjectiveTitle: obj.Title,
		Ceiling:        budget.Daily,
	}
	// Outcomes arrive newest first, so the first deferral seen is the most
	// recent — which is the one whose reset time has not yet passed and the
	// one worth reporting.
	var latest time.Time
	for _, o := range outcomes {
		if o.StartedAt.Before(since) || o.StartedAt.After(until) {
			continue
		}
		if o.Deferred != budgetExhaustedOutcome {
			continue
		}
		x.Times++
		if o.StartedAt.After(latest) {
			latest = o.StartedAt
			x.ResumesAt = o.DeferredUntil
			// What it was mid-way through, from the score it had reached. An
			// objective that ran out of money at 0.8 is a different message
			// from one that had nothing left to do.
			if o.CriteriaMet > 0 && o.CriteriaMet < 1 {
				x.InterruptedBy = fmt.Sprintf("%.0f%% of the way to its criteria", o.CriteriaMet*100)
			}
		}
	}
	if x.Times == 0 {
		return digest.BudgetExhaustion{}, false
	}
	return x, true
}

// budgetExhaustedOutcome is the Deferred marker the supervisor writes. Kept as
// a constant here rather than repeated as a literal, because a digest section
// that silently matches nothing is exactly the failure this phase is closing.
const budgetExhaustedOutcome = "budget_exhausted"

// waitingForQuotaOutcome is the Deferred marker the supervisor writes for a
// pass whose loop ended waiting_for_quota (Phase 40 step 8).
const waitingForQuotaOutcome = string(coreloop.RunWaitingForQuota)
