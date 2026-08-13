package cost

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync"
	"time"
)

// MemoryLedger keeps every event in this process.
//
// It is the reference implementation the shared contract defines correct
// behaviour against, and it is genuinely useful for a single-replica
// deployment that only wants today's numbers. It is the wrong choice for
// anything that has to survive a restart or answer "what did we spend last
// quarter", because it holds every event forever in memory — see quota/cost/sql.
//
// Zero value is not usable; call NewMemoryLedger.
type MemoryLedger struct {
	mu     sync.Mutex
	events []Event
}

// NewMemoryLedger returns an empty in-process ledger.
func NewMemoryLedger() *MemoryLedger { return &MemoryLedger{} }

var _ Ledger = (*MemoryLedger)(nil)

// Len reports how many events are held. Exported for the same reason
// MemoryBackend.Len is: unbounded growth is this implementation's failure mode,
// and this is how you watch for it.
func (l *MemoryLedger) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.events)
}

func (l *MemoryLedger) Record(_ context.Context, e Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	e.Labels = slices.Clone(e.Labels)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, e)
	return nil
}

func (l *MemoryLedger) Aggregate(_ context.Context, q Query) ([]Bucket, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}

	l.mu.Lock()
	matched := make([]Event, 0, len(l.events))
	for _, e := range l.events {
		if q.matches(e) {
			matched = append(matched, e)
		}
	}
	l.mu.Unlock()

	return Fold(matched, q), nil
}

// Fold buckets events by a query's grouping.
//
// It is exported because every ledger has to agree about it: a SQL
// implementation groups in the database, but the rules for what a bucket key
// looks like, how a multi-label event is counted, and how ties break must be
// identical or the same question gets two answers depending on which backend is
// wired up.
func Fold(events []Event, q Query) []Bucket {
	type acc struct {
		key    []string
		units  float64
		cost   float64
		events int
	}
	order := make([]string, 0, len(events))
	byKey := map[string]*acc{}

	add := func(key []string, e Event) {
		id := strings.Join(key, "\x00")
		a, ok := byKey[id]
		if !ok {
			a = &acc{key: key}
			byKey[id] = a
			order = append(order, id)
		}
		a.units += e.Units
		a.cost += e.Cost
		a.events++
	}

	for _, e := range events {
		for _, key := range bucketKeys(e, q) {
			add(key, e)
		}
	}

	out := make([]Bucket, 0, len(order))
	for _, id := range order {
		a := byKey[id]
		out = append(out, Bucket{Key: a.key, Units: a.units, Cost: a.cost, Events: a.events})
	}

	// Largest cost first, then the key, so a report is stable when several
	// buckets cost the same — including when everything costs zero because
	// nothing is priced.
	slices.SortFunc(out, func(a, b Bucket) int {
		if c := cmp.Compare(b.Cost, a.Cost); c != 0 {
			return c
		}
		if c := cmp.Compare(b.Units, a.Units); c != 0 {
			return c
		}
		return slices.Compare(a.Key, b.Key)
	})
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out
}

// bucketKeys returns the keys one event contributes to.
//
// Usually exactly one. The exception is ByLabel: an event carrying a team and
// an org belongs to both, so it is counted under each — which is what makes
// "spend by team" and "spend by org" both correct, and means the buckets of a
// label report deliberately do not sum to the total. Nothing else in this
// package double-counts.
func bucketKeys(e Event, q Query) [][]string {
	if len(q.GroupBy) == 0 {
		return [][]string{nil}
	}

	keys := [][]string{make([]string, 0, len(q.GroupBy))}
	for _, g := range q.GroupBy {
		if g != ByLabel {
			value := dimension(e, g)
			for i := range keys {
				keys[i] = append(keys[i], value)
			}
			continue
		}
		labels := q.Labels
		if len(labels) == 0 {
			labels = e.Labels
		} else {
			labels = intersect(labels, e.Labels)
		}
		if len(labels) == 0 {
			// An event in no container still happened, and hiding it would make
			// the totals of a label report disagree with an ungrouped one for a
			// reason nobody could see.
			labels = []string{""}
		}
		expanded := make([][]string, 0, len(keys)*len(labels))
		for _, prefix := range keys {
			for _, label := range labels {
				expanded = append(expanded, append(slices.Clone(prefix), label))
			}
		}
		keys = expanded
	}
	return keys
}

func dimension(e Event, g GroupBy) string {
	switch g {
	case ByDay:
		return Day(e.OccurredAt).Format(time.DateOnly)
	case BySubject:
		return string(e.Subject)
	case ByResource:
		if e.ResourceType == "" {
			return e.ResourceID
		}
		return e.ResourceType + ":" + e.ResourceID
	case ByProvider:
		return e.Provider
	case ByModel:
		return e.Model
	default:
		return ""
	}
}

func intersect(want, have []string) []string {
	var out []string
	for _, w := range want {
		if contains(have, w) {
			out = append(out, w)
		}
	}
	return out
}
