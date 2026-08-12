// Package costtest holds the behavioural contract every cost.Ledger must
// satisfy.
//
// It exists for the reason quotatest does: the in-memory and SQL ledgers must
// not silently diverge, because a report that says different things depending
// on which backend is wired up is worse than no report. A new ledger is
// finished when [Run] passes against it.
//
// Time is supplied to every case rather than slept through, so the suite is
// deterministic.
package costtest

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/bsenel/karakuri/quota"
	"github.com/bsenel/karakuri/quota/cost"
)

// Base is the instant the cases hang off. Deliberately mid-afternoon and not a
// round number: a ledger that only buckets correctly when the clock sits on
// midnight is broken, and this is what catches it.
var Base = time.Date(2026, 8, 10, 14, 37, 11, 0, time.UTC)

// NewLedger builds a fresh, empty ledger. Each case gets its own.
type NewLedger func(t *testing.T) cost.Ledger

// Run executes the full contract.
func Run(t *testing.T, newLedger NewLedger) {
	t.Helper()
	cases := []struct {
		name string
		fn   func(*testing.T, cost.Ledger)
	}{
		{"an empty ledger reports nothing", emptyLedger},
		{"a total folds every event", grandTotal},
		{"grouping by provider", byProvider},
		{"grouping by day buckets on UTC days", byDay},
		{"grouping by several dimensions", bySeveral},
		{"a multi-label event counts under each label", byLabel},
		{"a label filter narrows which labels are reported", byLabelFiltered},
		{"grouping by resource", byResource},
		{"a resource with no type keys on its id", untypedResource},
		{"the range excludes its upper bound", rangeIsHalfOpen},
		{"filters narrow", filters},
		{"an empty label filter does not widen", emptyFilterDoesNotWiden},
		{"buckets come back largest first", ordering},
		{"ties break stably", stableTies},
		{"a limit keeps the largest", limit},
		{"an invalid event is refused", invalidEvent},
		{"an invalid query is refused", invalidQuery},
		{"units are recorded even when nothing is priced", unpricedUnits},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.fn(t, newLedger(t))
		})
	}
}

func event(subject quota.Key, provider, model string, units, price float64, at time.Time, labels ...string) cost.Event {
	return cost.Event{
		Subject: subject, ResourceType: "twin", ResourceID: string(subject),
		Provider: provider, Model: model,
		Units: units, UnitKind: cost.UnitTokens, Cost: price,
		OccurredAt: at, Labels: labels,
	}
}

func record(t *testing.T, l cost.Ledger, events ...cost.Event) {
	t.Helper()
	for _, e := range events {
		if err := l.Record(context.Background(), e); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
}

func aggregate(t *testing.T, l cost.Ledger, q cost.Query) []cost.Bucket {
	t.Helper()
	got, err := l.Aggregate(context.Background(), q)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	return got
}

// close reports whether two sums agree to within floating-point noise. Money
// summed in a different order lands a few ulps apart, and a ledger is not wrong
// for that.
func close(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func emptyLedger(t *testing.T, l cost.Ledger) {
	if got := aggregate(t, l, cost.Query{}); len(got) != 0 {
		t.Fatalf("an empty ledger reported %+v", got)
	}
	if got := aggregate(t, l, cost.Query{GroupBy: []cost.GroupBy{cost.ByProvider}}); len(got) != 0 {
		t.Fatalf("an empty ledger reported %+v when grouped", got)
	}
}

func grandTotal(t *testing.T, l cost.Ledger) {
	record(t, l,
		event("twin|t1", "anthropic", "opus", 1000, 1.5, Base),
		event("twin|t2", "google", "gemini", 500, 0.25, Base.Add(time.Hour)),
	)

	got := aggregate(t, l, cost.Query{})
	if len(got) != 1 {
		t.Fatalf("ungrouped = %+v, want one bucket", got)
	}
	if !close(got[0].Units, 1500) || !close(got[0].Cost, 1.75) || got[0].Events != 2 {
		t.Fatalf("total = %+v, want 1500 units, 1.75, two events", got[0])
	}
	if len(got[0].Key) != 0 {
		t.Fatalf("an ungrouped bucket has key %v, want none", got[0].Key)
	}
}

func byProvider(t *testing.T, l cost.Ledger) {
	record(t, l,
		event("twin|t1", "anthropic", "opus", 1000, 1.5, Base),
		event("twin|t1", "anthropic", "haiku", 200, 0.1, Base),
		event("twin|t2", "google", "gemini", 500, 0.25, Base),
	)

	got := aggregate(t, l, cost.Query{GroupBy: []cost.GroupBy{cost.ByProvider}})
	if len(got) != 2 {
		t.Fatalf("by provider = %+v, want two buckets", got)
	}
	if got[0].Key[0] != "anthropic" || !close(got[0].Cost, 1.6) || got[0].Events != 2 {
		t.Fatalf("first bucket = %+v, want anthropic at 1.6 over two events", got[0])
	}
	if got[1].Key[0] != "google" || !close(got[1].Cost, 0.25) {
		t.Fatalf("second bucket = %+v", got[1])
	}
}

func byDay(t *testing.T, l cost.Ledger) {
	// Late one day and early the next, in UTC — the pair that catches a ledger
	// bucketing on a local zone.
	record(t, l,
		event("twin|t1", "anthropic", "opus", 100, 1, Base),
		event("twin|t1", "anthropic", "opus", 200, 2, Base.Add(12*time.Hour)),
	)

	got := aggregate(t, l, cost.Query{GroupBy: []cost.GroupBy{cost.ByDay}})
	if len(got) != 2 {
		t.Fatalf("by day = %+v, want two days", got)
	}
	want := map[string]float64{"2026-08-10": 1, "2026-08-11": 2}
	for _, b := range got {
		if len(b.Key) != 1 {
			t.Fatalf("bucket key = %v, want one dimension", b.Key)
		}
		if !close(b.Cost, want[b.Key[0]]) {
			t.Fatalf("day %q cost %v, want %v", b.Key[0], b.Cost, want[b.Key[0]])
		}
	}
}

func bySeveral(t *testing.T, l cost.Ledger) {
	record(t, l,
		event("twin|t1", "anthropic", "opus", 100, 1, Base),
		event("twin|t1", "anthropic", "haiku", 100, 2, Base),
		event("twin|t2", "anthropic", "opus", 100, 4, Base),
	)

	got := aggregate(t, l, cost.Query{GroupBy: []cost.GroupBy{cost.BySubject, cost.ByModel}})
	if len(got) != 3 {
		t.Fatalf("by subject and model = %+v, want three buckets", got)
	}
	for _, b := range got {
		if len(b.Key) != 2 {
			t.Fatalf("bucket key = %v, want two dimensions in the order asked for", b.Key)
		}
	}
	if got[0].Key[0] != "twin|t2" || got[0].Key[1] != "opus" || !close(got[0].Cost, 4) {
		t.Fatalf("largest bucket = %+v", got[0])
	}
}

// An event in a team and an org belongs to both, so it is counted under each.
// The buckets of a label report therefore do not sum to the grand total, and
// that is correct rather than a bug — nothing else in the package double-counts.
func byLabel(t *testing.T, l cost.Ledger) {
	record(t, l,
		event("twin|t1", "anthropic", "opus", 100, 3, Base, "team:t_eng", "org:o_acme"),
		event("twin|t2", "anthropic", "opus", 100, 1, Base, "org:o_acme"),
	)

	got := aggregate(t, l, cost.Query{GroupBy: []cost.GroupBy{cost.ByLabel}})
	byName := map[string]cost.Bucket{}
	for _, b := range got {
		byName[b.Key[0]] = b
	}
	if b, ok := byName["org:o_acme"]; !ok || !close(b.Cost, 4) || b.Events != 2 {
		t.Fatalf("org bucket = %+v, want both events", b)
	}
	if b, ok := byName["team:t_eng"]; !ok || !close(b.Cost, 3) || b.Events != 1 {
		t.Fatalf("team bucket = %+v, want the one event in it", b)
	}

	// An event in no container still appears, or a label report and an
	// ungrouped one would disagree for a reason nobody could see.
	record(t, l, event("twin|t3", "anthropic", "opus", 100, 5, Base))
	got = aggregate(t, l, cost.Query{GroupBy: []cost.GroupBy{cost.ByLabel}})
	var unlabelled bool
	for _, b := range got {
		if b.Key[0] == "" && close(b.Cost, 5) {
			unlabelled = true
		}
	}
	if !unlabelled {
		t.Fatalf("an event in no container vanished from a label report: %+v", got)
	}
}

// The tenancy case. A reader who may see one org groups by label and gets that
// org's bucket and no other — otherwise a scoped report would name the
// containers it is filtering out, which leaks the shape of another tenant.
func byLabelFiltered(t *testing.T, l cost.Ledger) {
	record(t, l,
		event("twin|t1", "anthropic", "opus", 100, 3, Base, "team:t_eng", "org:o_acme"),
		event("twin|t2", "anthropic", "opus", 100, 7, Base, "org:o_globex"),
	)

	got := aggregate(t, l, cost.Query{
		Labels:  []string{"org:o_acme"},
		GroupBy: []cost.GroupBy{cost.ByLabel},
	})
	if len(got) != 1 {
		t.Fatalf("filtered label report = %+v, want one bucket", got)
	}
	if got[0].Key[0] != "org:o_acme" || !close(got[0].Cost, 3) {
		t.Fatalf("bucket = %+v, want only the readable org", got[0])
	}
}

func byResource(t *testing.T, l cost.Ledger) {
	record(t, l,
		event("twin|t1", "anthropic", "opus", 100, 1, Base),
		event("twin|t2", "anthropic", "opus", 100, 2, Base),
	)

	got := aggregate(t, l, cost.Query{GroupBy: []cost.GroupBy{cost.ByResource}})
	if len(got) != 2 {
		t.Fatalf("by resource = %+v, want two", got)
	}
	// Type-qualified, so a twin and an objective sharing an id are two rows.
	if got[0].Key[0] != "twin:twin|t2" {
		t.Fatalf("largest = %+v, want the resource type in the key", got[0])
	}
}

// Half-open, so two adjacent ranges cannot both claim the same event and a
// month-by-month report neither double-counts nor drops a day.
func rangeIsHalfOpen(t *testing.T, l cost.Ledger) {
	at := cost.Day(Base)
	record(t, l,
		event("twin|t1", "anthropic", "opus", 100, 1, at),
		event("twin|t1", "anthropic", "opus", 100, 2, at.AddDate(0, 0, 1)),
	)

	first := aggregate(t, l, cost.Query{Since: at, Until: at.AddDate(0, 0, 1)})
	if len(first) != 1 || !close(first[0].Cost, 1) {
		t.Fatalf("first day = %+v, want only the event at its start", first)
	}
	second := aggregate(t, l, cost.Query{Since: at.AddDate(0, 0, 1), Until: at.AddDate(0, 0, 2)})
	if len(second) != 1 || !close(second[0].Cost, 2) {
		t.Fatalf("second day = %+v", second)
	}
}

func filters(t *testing.T, l cost.Ledger) {
	record(t, l,
		event("twin|t1", "anthropic", "opus", 100, 1, Base, "org:o_acme"),
		event("twin|t2", "google", "gemini", 100, 2, Base, "org:o_globex"),
	)

	for name, tc := range map[string]struct {
		query cost.Query
		want  float64
	}{
		"by subject":  {cost.Query{Subjects: []quota.Key{"twin|t1"}}, 1},
		"by provider": {cost.Query{Providers: []string{"google"}}, 2},
		"by label":    {cost.Query{Labels: []string{"org:o_acme"}}, 1},
		"combined":    {cost.Query{Subjects: []quota.Key{"twin|t1"}, Providers: []string{"anthropic"}}, 1},
	} {
		t.Run(name, func(t *testing.T) {
			got := aggregate(t, l, tc.query)
			if len(got) != 1 || !close(got[0].Cost, tc.want) {
				t.Fatalf("%s = %+v, want %v", name, got, tc.want)
			}
		})
	}

	// Filters that match nothing return nothing rather than everything.
	if got := aggregate(t, l, cost.Query{Subjects: []quota.Key{"twin|nobody"}}); len(got) != 0 {
		t.Fatalf("an unmatched subject filter returned %+v", got)
	}
	if got := aggregate(t, l, cost.Query{Labels: []string{"org:o_nowhere"}}); len(got) != 0 {
		t.Fatalf("an unmatched label filter returned %+v", got)
	}
}

// An empty filter means "do not narrow", so a caller that means "nothing" must
// not reach the ledger at all — the same rule the scoped twin listing follows.
func emptyFilterDoesNotWiden(t *testing.T, l cost.Ledger) {
	record(t, l, event("twin|t1", "anthropic", "opus", 100, 1, Base, "org:o_acme"))

	got := aggregate(t, l, cost.Query{Labels: []string{}, Subjects: []quota.Key{}})
	if len(got) != 1 || !close(got[0].Cost, 1) {
		t.Fatalf("empty filters = %+v, want everything", got)
	}
}

func ordering(t *testing.T, l cost.Ledger) {
	record(t, l,
		event("twin|t1", "a", "m", 1, 1, Base),
		event("twin|t2", "b", "m", 1, 3, Base),
		event("twin|t3", "c", "m", 1, 2, Base),
	)
	got := aggregate(t, l, cost.Query{GroupBy: []cost.GroupBy{cost.BySubject}})
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Cost < got[i].Cost {
			t.Fatalf("buckets are not largest first: %+v", got)
		}
	}
}

// Two buckets costing the same must come back in the same order every time, or
// a paged report shows one row twice and drops another. Everything costing zero
// is the common case here: a deployment that has configured no rates.
func stableTies(t *testing.T, l cost.Ledger) {
	record(t, l,
		event("twin|t3", "a", "m", 100, 0, Base),
		event("twin|t1", "a", "m", 100, 0, Base),
		event("twin|t2", "a", "m", 50, 0, Base),
	)

	var first []cost.Bucket
	for range 5 {
		got := aggregate(t, l, cost.Query{GroupBy: []cost.GroupBy{cost.BySubject}})
		if len(got) != 3 {
			t.Fatalf("got %+v", got)
		}
		if first == nil {
			first = got
			continue
		}
		for i := range got {
			if got[i].Key[0] != first[i].Key[0] {
				t.Fatalf("run %d ordered %v, first run ordered %v", i, keys(got), keys(first))
			}
		}
	}
	// Units break a cost tie before the key does, so the bigger consumer leads
	// even when neither costs anything.
	if first[2].Key[0] != "twin|t2" {
		t.Fatalf("order = %v, want the smallest consumer last", keys(first))
	}
}

func keys(buckets []cost.Bucket) []string {
	out := make([]string, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, b.Key...)
	}
	return out
}

// A resource with no type is keyed on its id alone rather than on ":id".
func untypedResource(t *testing.T, l cost.Ledger) {
	e := event("twin|t1", "a", "m", 100, 1, Base)
	e.ResourceType = ""
	e.ResourceID = "bare"
	record(t, l, e)

	got := aggregate(t, l, cost.Query{GroupBy: []cost.GroupBy{cost.ByResource}})
	if len(got) != 1 || got[0].Key[0] != "bare" {
		t.Fatalf("untyped resource = %+v, want the bare id", got)
	}
}

func limit(t *testing.T, l cost.Ledger) {
	for i, c := range []float64{1, 5, 3, 2} {
		record(t, l, event(quota.Key("twin|t"+string(rune('1'+i))), "a", "m", 1, c, Base))
	}
	got := aggregate(t, l, cost.Query{GroupBy: []cost.GroupBy{cost.BySubject}, Limit: 2})
	if len(got) != 2 {
		t.Fatalf("limit 2 returned %d buckets", len(got))
	}
	if !close(got[0].Cost, 5) || !close(got[1].Cost, 3) {
		t.Fatalf("limit kept %+v, want the two largest", got)
	}
}

func invalidEvent(t *testing.T, l cost.Ledger) {
	ctx := context.Background()
	valid := event("twin|t1", "anthropic", "opus", 100, 1, Base)

	cases := map[string]func(cost.Event) cost.Event{
		"no subject":     func(e cost.Event) cost.Event { e.Subject = ""; return e },
		"no unit kind":   func(e cost.Event) cost.Event { e.UnitKind = ""; return e },
		"negative units": func(e cost.Event) cost.Event { e.Units = -1; return e },
		"negative cost":  func(e cost.Event) cost.Event { e.Cost = -1; return e },
		"no timestamp":   func(e cost.Event) cost.Event { e.OccurredAt = time.Time{}; return e },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if err := l.Record(ctx, mutate(valid)); !errors.Is(err, cost.ErrInvalidEvent) {
				t.Fatalf("Record = %v, want ErrInvalidEvent", err)
			}
		})
	}
	if got := aggregate(t, l, cost.Query{}); len(got) != 0 {
		t.Fatalf("a refused event was recorded anyway: %+v", got)
	}
}

func invalidQuery(t *testing.T, l cost.Ledger) {
	ctx := context.Background()
	if _, err := l.Aggregate(ctx, cost.Query{GroupBy: []cost.GroupBy{"colour"}}); !errors.Is(err, cost.ErrInvalidEvent) {
		t.Fatalf("unknown grouping = %v, want ErrInvalidEvent", err)
	}
	if _, err := l.Aggregate(ctx, cost.Query{Since: Base, Until: Base.Add(-time.Hour)}); !errors.Is(err, cost.ErrInvalidEvent) {
		t.Fatalf("backwards range = %v, want ErrInvalidEvent", err)
	}
}

// A model nobody priced still consumed tokens, and losing them because the rate
// table is out of date would make the ledger useless exactly when a new model
// is the thing to watch.
func unpricedUnits(t *testing.T, l cost.Ledger) {
	record(t, l, event("twin|t1", "brand-new", "model-x", 5000, 0, Base))

	got := aggregate(t, l, cost.Query{})
	if len(got) != 1 || !close(got[0].Units, 5000) || !close(got[0].Cost, 0) {
		t.Fatalf("unpriced = %+v, want the units kept and no money invented", got)
	}
}
