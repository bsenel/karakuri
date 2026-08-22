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
	"github.com/bsenel/karakuri/internal/core/objective"
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

	titles := make(map[objective.ObjectiveID]string, len(objectives))
	for _, obj := range objectives {
		titles[obj.ID] = obj.Title
		d.Objectives = append(d.Objectives, s.summarise(ctx, obj, since, until))
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

	if st, err := s.store.GetReconcileState(ctx, obj.ID); err == nil {
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
	return sum
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
