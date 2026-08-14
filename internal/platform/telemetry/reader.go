// Package telemetry answers what this deployment has been doing, by reading
// the records it already keeps.
//
// It implements the read-only port in internal/core/telemetry so the karakuri
// domain pack can treat Karakuri's own behaviour as an environment. Like the
// digest, it accumulates nothing: every number here is a query over
// objectives, reconcile outcomes, the audit log and the cost ledger, so a
// snapshot for a past window is reproducible and no hot path pays for it.
package telemetry

import (
	"context"
	"sort"
	"time"

	coretelemetry "github.com/bsenel/karakuri/internal/core/telemetry"

	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota"
	"github.com/bsenel/karakuri/quota/cost"
)

// staleDecisionAfter is how long a pending checkpoint waits before it counts
// as a bottleneck rather than as a queue.
//
// A day, because the most common way an autonomous system stops being
// autonomous is a queue of questions nobody is reading — and a question that
// has gone unanswered for a working day is no longer waiting, it is stuck.
const staleDecisionAfter = 24 * time.Hour

type Reader struct {
	store storage.StorageAdapter
	quota karakuriquota.Deps
	now   func() time.Time
}

func New(store storage.StorageAdapter, quotaDeps karakuriquota.Deps) *Reader {
	return &Reader{store: store, quota: quotaDeps, now: func() time.Time { return time.Now().UTC() }}
}

// Snapshot reads the deployment over a window.
//
// Individual sources are allowed to fail without failing the snapshot, on the
// same reasoning as the digest: a pack asking "what should I improve" is
// better served by four of five answers than by an error. What it must not do
// is report a missing number as a zero, which is why the shapes carry their
// own "was this measured" flags.
func (r *Reader) Snapshot(ctx context.Context, q coretelemetry.Query) (coretelemetry.Snapshot, error) {
	now := r.now()
	since := q.Since
	if since.IsZero() {
		since = now.Add(-7 * 24 * time.Hour)
	}
	snap := coretelemetry.Snapshot{Since: since, TakenAt: now}

	objectives, err := r.store.ListObjectives(ctx, storage.ObjectiveFilter{TwinID: q.TwinID})
	if err != nil {
		return snap, err
	}

	failing := map[objective.ObjectiveID]int{}
	titles := map[objective.ObjectiveID]string{}

	for _, obj := range objectives {
		titles[obj.ID] = obj.Title
		snap.Objectives.Total++
		switch obj.Status {
		case objective.StatusConverged:
			snap.Objectives.Converged++
		case objective.StatusBlocked:
			snap.Objectives.Blocked++
		case objective.StatusFailed:
			snap.Objectives.Failed++
		}
		if !obj.IsStanding() {
			continue
		}
		snap.Objectives.Standing++

		if st, serr := r.store.GetReconcileState(ctx, obj.ID); serr == nil && st.Paused {
			snap.Bottlenecks = append(snap.Bottlenecks, coretelemetry.Bottleneck{
				Kind:   coretelemetry.BottleneckBlockedObjective,
				Detail: obj.Title + ": " + st.PausedReason,
				Count:  1,
			})
		}

		outcomes, oerr := r.store.ListReconcileOutcomes(ctx, obj.ID, 2000)
		if oerr != nil {
			continue
		}
		for _, o := range outcomes {
			if o.StartedAt.Before(since) || o.StartedAt.After(now) {
				continue
			}
			if o.LoopID == "" {
				snap.Work.Senses++
			} else {
				snap.Work.Reconciles++
			}
			if o.Failed() {
				snap.Work.Failures++
				failing[obj.ID]++
			}
		}
	}

	for id, count := range failing {
		if count < 2 {
			continue // once is an incident, twice is a pattern
		}
		snap.Bottlenecks = append(snap.Bottlenecks, coretelemetry.Bottleneck{
			Kind:   coretelemetry.BottleneckFailingObjective,
			Detail: titles[id],
			Count:  count,
		})
	}

	r.readAudit(ctx, &snap, since, now)
	r.readPending(ctx, &snap, q.TwinID, now)
	r.readSpend(ctx, &snap, q.TwinID, since, now)

	// Ranked, so a pack asking "what should I improve" gets an answer rather
	// than a list it has to sort — and gets the same answer every time,
	// which map iteration alone would not give it.
	sort.SliceStable(snap.Bottlenecks, func(i, j int) bool {
		if snap.Bottlenecks[i].Count != snap.Bottlenecks[j].Count {
			return snap.Bottlenecks[i].Count > snap.Bottlenecks[j].Count
		}
		return snap.Bottlenecks[i].Detail < snap.Bottlenecks[j].Detail
	})
	return snap, nil
}

// readAudit counts actions and escalation outcomes, and finds capabilities
// failing across objectives — which points at an adapter rather than at any
// one objective.
func (r *Reader) readAudit(ctx context.Context, snap *coretelemetry.Snapshot, since, until time.Time) {
	count := func(kind string) int {
		events, err := r.store.ListToolEvents(ctx, storage.ToolEventFilter{
			Kind: kind, CreatedAtSince: &since, Limit: 5000,
		})
		if err != nil {
			return 0
		}
		n := 0
		for _, e := range events {
			if !e.CreatedAt.After(until) {
				n++
			}
		}
		return n
	}
	snap.Escalation.Escalations = count(storage.ToolEventEscalation)
	snap.Escalation.Approvals = count(storage.ToolEventApproval)
	snap.Escalation.Rejections = count(storage.ToolEventRejection)

	executes, err := r.store.ListToolEvents(ctx, storage.ToolEventFilter{
		Kind: storage.ToolEventExecute, CreatedAtSince: &since, Limit: 5000,
	})
	if err != nil {
		return
	}
	failingCap := map[string]int{}
	for _, e := range executes {
		if e.CreatedAt.After(until) {
			continue
		}
		snap.Work.Actions++
		if !e.Success && e.Capability != "" {
			failingCap[e.Capability]++
		}
	}
	for capID, n := range failingCap {
		if n < 3 {
			continue
		}
		snap.Bottlenecks = append(snap.Bottlenecks, coretelemetry.Bottleneck{
			Kind:   coretelemetry.BottleneckFailingCapability,
			Detail: capID,
			Count:  n,
		})
	}
}

func (r *Reader) readPending(ctx context.Context, snap *coretelemetry.Snapshot, twinID string, now time.Time) {
	pending, err := r.store.ListPendingCheckpoints(ctx, twinID)
	if err != nil {
		return
	}
	snap.Escalation.Pending = len(pending)
	stale := 0
	for _, cp := range pending {
		if now.Sub(cp.CreatedAt) > staleDecisionAfter {
			stale++
		}
	}
	if stale > 0 {
		snap.Bottlenecks = append(snap.Bottlenecks, coretelemetry.Bottleneck{
			Kind:   coretelemetry.BottleneckStaleDecision,
			Detail: "decisions waiting more than a day",
			Count:  stale,
		})
	}
}

func (r *Reader) readSpend(ctx context.Context, snap *coretelemetry.Snapshot, twinID string, since, until time.Time) {
	q := cost.Query{Since: since, Until: until, GroupBy: []cost.GroupBy{cost.ByProvider}}
	if twinID != "" {
		q.Subjects = []quota.Key{karakuriquota.CostSubject(twinID)}
	}
	buckets, err := r.quota.CostReport(ctx, q)
	if err != nil || len(buckets) == 0 {
		return
	}
	snap.Spend.ByProvider = map[string]float64{}
	for _, b := range buckets {
		provider := ""
		if len(b.Key) > 0 {
			provider = b.Key[0]
		}
		snap.Spend.Cost += b.Cost
		snap.Spend.ByProvider[provider] += b.Cost
	}
	snap.Spend.Priced = snap.Spend.Cost > 0
}
