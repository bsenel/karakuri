package quota

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/quota"
	"github.com/bsenel/karakuri/quota/cost"
)

// Cost attribution, in Karakuri's terms.
//
// The ledger knows nothing about twins or teams. What lives here is where the
// spend comes from, who pays for it, and — the part that matters after Phase 17
// — which containers it belonged to when it happened.

// ScopeLookup reads the containers a resource sits in. It is the container
// service narrowed to the one method recording needs.
type ScopeLookup interface {
	ScopesOf(ctx context.Context, resourceType, resourceID string) ([]string, error)
}

// Recorder turns work into ledger entries.
//
// It is a struct rather than a function because recording needs three things a
// call site does not have: a price table, the container tree, and somewhere to
// publish. A zero Recorder discards, so the loop never has to check.
type Recorder struct {
	Ledger cost.Ledger
	Pricer cost.Pricer

	// Containers supplies the labels an event carries. Optional: without it
	// events are recorded unlabelled, which costs a deployment with no tenancy
	// tree nothing and would cost one with a tree its per-team reports.
	Containers ScopeLookup

	// Hub receives a cost_recorded event per write, which is what a live
	// dashboard follows.
	Hub *event.Hub

	// Now is the clock. Nil means time.Now.
	Now func() time.Time
}

// Enabled reports whether anything is actually recorded.
func (r *Recorder) Enabled() bool { return r != nil && r.Ledger != nil }

// Spend is one thing that cost money, before pricing and labelling.
type Spend struct {
	TwinID string

	// ResourceType and ResourceID name what the spend was on. Empty defaults to
	// the twin, which is what a model call charges.
	ResourceType string
	ResourceID   string

	Provider string
	Model    string
	Units    float64
	UnitKind string
}

// Price returns what a spend costs, without recording it.
//
// Exposed because a per-run ceiling has to be enforced while the run is still
// going, and the ledger only answers after the fact. Zero when no pricer is
// configured, which is the same answer Record would write.
func (r *Recorder) Price(s Spend) float64 {
	if r == nil || r.Pricer == nil || s.Units <= 0 {
		return 0
	}
	return r.Pricer.Price(s.Provider, s.Model, s.UnitKind, s.Units)
}

// Record prices a spend, attaches the resource's containers, and writes it.
//
// Failures are logged rather than returned. The work is already done and paid
// for upstream, so a ledger that cannot be written must not fail the request
// that generated the charge — losing a record is bad, and losing the work
// somebody has already been billed for is worse. This is the same trade the
// loop already makes when charging tokens.
func (r *Recorder) Record(ctx context.Context, s Spend) {
	if !r.Enabled() || s.Units <= 0 {
		// Providers that cannot report usage send zero. Recording a
		// zero-unit event would fill the ledger with rows that say nothing.
		return
	}

	resourceType, resourceID := s.ResourceType, s.ResourceID
	if resourceType == "" {
		resourceType, resourceID = "twin", s.TwinID
	}

	e := cost.Event{
		Subject:      TwinKey(s.TwinID),
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Provider:     s.Provider,
		Model:        s.Model,
		Units:        s.Units,
		UnitKind:     s.UnitKind,
		OccurredAt:   r.now(),
		Labels:       r.labels(ctx, resourceType, resourceID, s.TwinID),
	}
	if r.Pricer != nil {
		e.Cost = r.Pricer.Price(s.Provider, s.Model, s.UnitKind, s.Units)
	}

	if err := r.Ledger.Record(ctx, e); err != nil {
		slog.Error("cost event could not be recorded; the work still happened",
			"twin", s.TwinID, "provider", s.Provider, "units", s.Units, "err", err)
		return
	}
	r.publish(ctx, e)
}

// labels reads the containers the resource belongs to, falling back to the
// twin's when the resource itself is in none.
//
// The fallback is what makes an objective's spend land in its twin's team even
// on a deployment where objectives were never placed in containers — and it
// costs nothing where they were, because an objective inherits its twin's
// containers at creation anyway.
func (r *Recorder) labels(ctx context.Context, resourceType, resourceID, twinID string) []string {
	if r.Containers == nil {
		return nil
	}
	if labels, err := r.Containers.ScopesOf(ctx, resourceType, resourceID); err == nil && len(labels) > 0 {
		return labels
	}
	if resourceType == "twin" || twinID == "" {
		return nil
	}
	labels, err := r.Containers.ScopesOf(ctx, "twin", twinID)
	if err != nil {
		// An unlabelled event is still worth recording: the spend happened, and
		// dropping it because the tree could not be read would lose money that
		// a per-team report is only one dimension of.
		return nil
	}
	return labels
}

func (r *Recorder) publish(ctx context.Context, e cost.Event) {
	if r.Hub == nil {
		return
	}
	// TwinID is the twin that pays, not the resource the spend was on. Those
	// differ for a tool call, whose resource is the objective — and publishing
	// an objective ID under twin_id would route the event to the wrong
	// per-twin stream and describe the wrong thing to a dashboard.
	twinID := e.ResourceID
	if e.ResourceType != "twin" {
		twinID = strings.TrimPrefix(string(e.Subject), "twin|")
	}
	r.Hub.Publish(ctx, event.Event{
		Type:   event.TypeCostRecorded,
		TwinID: twinID,
		Payload: map[string]any{
			"resource_type": e.ResourceType,
			"resource_id":   e.ResourceID,
			"subject":       string(e.Subject),
			"provider":      e.Provider,
			"model":         e.Model,
			"units":         e.Units,
			"unit_kind":     e.UnitKind,
			"cost":          e.Cost,
			"labels":        e.Labels,
		},
		Timestamp: e.OccurredAt,
	})
}

func (r *Recorder) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now().UTC()
}

// CostReport answers what was spent, filtered to what the caller may see.
//
// The filter is not optional and not the ledger's job: a caller that has not
// worked out which containers a reader may see must not reach this, for the
// same reason the twin listing refuses to widen on empty input. See
// internal/api/handler.QuotaHandler.CostReport, which resolves it from the same
// bindings the per-resource check reads.
func (d Deps) CostReport(ctx context.Context, q cost.Query) ([]cost.Bucket, error) {
	if !d.Costs.Enabled() {
		return nil, nil
	}
	return d.Costs.Ledger.Aggregate(ctx, q)
}

// CostSubject is the subject key a twin's spend is recorded under, exported so
// a caller building a query names the same thing the recorder wrote.
func CostSubject(twinID string) quota.Key { return TwinKey(twinID) }

// sweeper is the optional prune a ledger may support. Optional because it is a
// storage concern: the in-memory ledger has nothing to prune, and a ledger that
// keeps everything is a legitimate choice rather than a broken implementation.
type sweeper interface {
	Sweep(ctx context.Context, before time.Time) (int64, error)
}

// SweepCosts drops raw events older than the retention horizon and reports how
// many went.
//
// Only the events. The daily rollup survives pruning, which is what lets an
// operator keep a year of totals and a month of drill-down — the totals are the
// expensive thing to lose and the cheap thing to keep.
//
// A ledger that cannot prune, or a retention of zero, is a no-op: keeping
// everything is a choice, and one an operator makes by not setting the horizon.
func (d Deps) SweepCosts(ctx context.Context, retention time.Duration, now time.Time) (int64, error) {
	if !d.Costs.Enabled() || retention <= 0 {
		return 0, nil
	}
	s, ok := d.Costs.Ledger.(sweeper)
	if !ok {
		return 0, nil
	}
	return s.Sweep(ctx, now.Add(-retention))
}
