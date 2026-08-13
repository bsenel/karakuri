// Package cost records what was spent and answers what it added up to.
//
// It is a sibling of the quota module rather than part of it, because the two
// answer opposite questions. A quota is asked *before* the work: may this
// proceed. A cost is recorded *after*: this is what it took. Conflating them
// produces a limiter that has to know prices and a ledger that can refuse
// things, and neither is better at its job for it.
//
// The module has no external dependencies and no application vocabulary. A
// Subject is a quota.Key, a resource is an opaque string, and prices arrive
// through a Pricer the caller supplies — a price table is configuration, and
// configuration is the host's to read.
package cost

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bsenel/karakuri/quota"
)

// ErrInvalidEvent is returned for an event a ledger cannot record.
var ErrInvalidEvent = errors.New("cost: invalid event")

// Unit kinds this package expects to see. They are strings rather than an
// enum because a caller may meter something nobody here thought of, and a
// ledger that refuses an unknown unit is a ledger that loses the spend.
const (
	// UnitTokens is what a language model charges for.
	UnitTokens = "tokens"

	// UnitCalls is what everything else charges for: one invocation of a tool,
	// an adapter, an API.
	UnitCalls = "calls"
)

// Event is one thing that cost money.
type Event struct {
	// Subject is who pays, in the same key space quotas are counted under, so
	// "what did this twin spend" and "what is this twin's remaining budget"
	// name the same thing.
	Subject quota.Key

	// ResourceType and ResourceID say what the spend was on — a twin, an
	// objective. Opaque here; they exist so a report can drill from a total
	// down to the thing that produced it.
	ResourceType string
	ResourceID   string

	// Provider and Model attribute the spend. Model is empty for anything that
	// is not a model call.
	Provider string
	Model    string

	// Units is how much was consumed, in UnitKind. Float rather than int
	// because a call is one and a token is many and a fractional unit is
	// perfectly ordinary once a gateway is reporting them.
	Units    float64
	UnitKind string

	// Cost is what that came to, in whole currency units, as the Pricer said.
	// Zero is a legitimate answer — an unpriced model, a free tier — and is
	// recorded rather than dropped, because the units are still worth having.
	Cost float64

	OccurredAt time.Time

	// Labels are the containers the resource belonged to when the spend
	// happened, copied rather than derived.
	//
	// Copied is the important word. Deriving them at query time would mean a
	// report changes retroactively when a resource is moved between teams —
	// last month's spend would migrate to the new team, which is wrong for
	// money that has already been spent, and would make two runs of the same
	// report disagree.
	Labels []string
}

// Validate reports whether the event can be recorded.
func (e Event) Validate() error {
	if e.Subject == "" {
		return fmt.Errorf("%w: event has no subject", ErrInvalidEvent)
	}
	if strings.TrimSpace(e.UnitKind) == "" {
		return fmt.Errorf("%w: event for %q has no unit kind", ErrInvalidEvent, e.Subject)
	}
	if e.Units < 0 {
		return fmt.Errorf("%w: event for %q consumed %v units", ErrInvalidEvent, e.Subject, e.Units)
	}
	if e.Cost < 0 {
		return fmt.Errorf("%w: event for %q cost %v", ErrInvalidEvent, e.Subject, e.Cost)
	}
	if e.OccurredAt.IsZero() {
		return fmt.Errorf("%w: event for %q has no timestamp", ErrInvalidEvent, e.Subject)
	}
	return nil
}

// GroupBy names a dimension a report buckets on.
type GroupBy string

const (
	ByDay      GroupBy = "day"
	BySubject  GroupBy = "subject"
	ByResource GroupBy = "resource"
	ByProvider GroupBy = "provider"
	ByModel    GroupBy = "model"
	ByLabel    GroupBy = "label"
)

// Valid reports whether g is a dimension this package knows.
func (g GroupBy) Valid() bool {
	switch g {
	case ByDay, BySubject, ByResource, ByProvider, ByModel, ByLabel:
		return true
	}
	return false
}

// Query asks what was spent.
//
// Since and Until bound it; the rest narrow it. An empty Query over an empty
// range is a whole-history total, which is expensive and occasionally what
// somebody wants.
type Query struct {
	Since time.Time
	Until time.Time

	// Subjects and Labels narrow to particular payers or containers. Both are
	// unions: any subject, any label.
	//
	// A caller enforcing tenancy passes the labels the reader may see. An empty
	// slice means "do not filter", so a caller that means "nothing" must not
	// reach the ledger at all — the same rule the twin listing follows, and for
	// the same reason.
	Subjects []quota.Key
	Labels   []string

	// Providers narrows to particular providers.
	Providers []string

	// GroupBy is the dimensions to bucket on, applied in order. Empty means one
	// bucket holding the grand total.
	GroupBy []GroupBy

	// Limit caps how many buckets come back, largest cost first. Zero means all
	// of them.
	Limit int
}

// Validate reports whether the query is answerable.
func (q Query) Validate() error {
	for _, g := range q.GroupBy {
		if !g.Valid() {
			return fmt.Errorf("%w: unknown grouping %q", ErrInvalidEvent, g)
		}
	}
	if !q.Since.IsZero() && !q.Until.IsZero() && q.Until.Before(q.Since) {
		return fmt.Errorf("%w: range ends (%s) before it starts (%s)", ErrInvalidEvent, q.Until, q.Since)
	}
	return nil
}

// matches reports whether an event falls inside the query's filters. Exported
// behaviour lives on Ledger implementations; this is what the in-memory one and
// the shared contract agree an implementation must do.
func (q Query) matches(e Event) bool {
	if !q.Since.IsZero() && e.OccurredAt.Before(q.Since) {
		return false
	}
	// Until is exclusive, so a day-long range is [midnight, midnight) and two
	// adjacent ranges cannot both claim the same event.
	if !q.Until.IsZero() && !e.OccurredAt.Before(q.Until) {
		return false
	}
	if len(q.Subjects) > 0 && !contains(q.Subjects, e.Subject) {
		return false
	}
	if len(q.Providers) > 0 && !contains(q.Providers, e.Provider) {
		return false
	}
	if len(q.Labels) > 0 && !anyLabel(q.Labels, e.Labels) {
		return false
	}
	return true
}

// Bucket is one row of a report.
type Bucket struct {
	// Key names the bucket, one entry per GroupBy dimension in the order they
	// were asked for. Empty for an ungrouped total.
	Key []string

	Units float64
	Cost  float64

	// Events is how many events the bucket folds together, which is what tells
	// a reader whether a number is one expensive call or a thousand cheap ones.
	Events int
}

// Ledger records spend and reports on it.
//
// Record is on a hot path — once per model call, once per tool call — and
// Aggregate is not, which is the asymmetry any implementation should optimise
// for.
type Ledger interface {
	// Record stores one event.
	Record(ctx context.Context, e Event) error

	// Aggregate answers a query, largest cost first within each grouping.
	Aggregate(ctx context.Context, q Query) ([]Bucket, error)
}

func contains[T comparable](haystack []T, needle T) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

func anyLabel(want, have []string) bool {
	for _, w := range want {
		if contains(have, w) {
			return true
		}
	}
	return false
}

// Day truncates a timestamp to the UTC day a rollup buckets it under.
//
// UTC rather than a local zone, so that a report run in two offices agrees
// about which day a call fell on — and so that a deployment moving between
// zones does not reshuffle its history.
func Day(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
