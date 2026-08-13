package quota_test

import (
	"context"
	stdsql "database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/event"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota/cost"
	costsql "github.com/bsenel/karakuri/quota/cost/sql"

	_ "modernc.org/sqlite"
)

// openCostDB opens a file-backed SQLite database with the busy timeout the
// ledger's write lock needs.
func openCostDB(t *testing.T) *stdsql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cost.db")
	db, err := stdsql.Open("sqlite",
		"file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

type stubScopes struct {
	labels map[string][]string
	err    error
}

func (s *stubScopes) ScopesOf(_ context.Context, resourceType, resourceID string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.labels[resourceType+":"+resourceID], nil
}

func recorder(t *testing.T, containers karakuriquota.ScopeLookup) (*karakuriquota.Recorder, *cost.MemoryLedger) {
	t.Helper()
	ledger := cost.NewMemoryLedger()
	return &karakuriquota.Recorder{
		Ledger: ledger,
		Pricer: cost.NewStaticPricer([]cost.Rate{
			{Provider: "anthropic", Model: "opus", UnitKind: cost.UnitTokens, PerUnit: 0.000015},
		}),
		Containers: containers,
		Now:        func() time.Time { return time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC) },
	}, ledger
}

func TestRecorderPricesAndLabels(t *testing.T) {
	ctx := context.Background()
	r, ledger := recorder(t, &stubScopes{labels: map[string][]string{
		"twin:t1": {"team:t_eng", "org:o_acme"},
	}})

	r.Record(ctx, karakuriquota.Spend{
		TwinID: "t1", Provider: "anthropic", Model: "opus",
		Units: 1_000_000, UnitKind: cost.UnitTokens,
	})

	got, err := ledger.Aggregate(ctx, cost.Query{GroupBy: []cost.GroupBy{cost.ByLabel}})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("buckets = %+v, want the team and the org", got)
	}
	// Priced from the configured table.
	for _, b := range got {
		if b.Cost != 15 {
			t.Fatalf("bucket %v cost %v, want 15", b.Key, b.Cost)
		}
	}
}

// An objective inherits its twin's containers when it has none of its own,
// which is what puts a tool call's spend in the right team on a deployment
// where objectives were never placed explicitly.
func TestRecorderFallsBackToTheTwinsContainers(t *testing.T) {
	ctx := context.Background()
	r, ledger := recorder(t, &stubScopes{labels: map[string][]string{
		"twin:t1": {"org:o_acme"},
	}})

	r.Record(ctx, karakuriquota.Spend{
		TwinID: "t1", ResourceType: "objective", ResourceID: "obj-1",
		Provider: "github", Units: 1, UnitKind: cost.UnitCalls,
	})

	got, err := ledger.Aggregate(ctx, cost.Query{Labels: []string{"org:o_acme"}})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(got) != 1 || got[0].Events != 1 {
		t.Fatalf("buckets = %+v, want the objective under its twin's org", got)
	}
}

// The spend happened whatever the tree says, so an unreadable container service
// costs the per-team dimension and not the money.
func TestRecorderRecordsWhenTheTreeCannotBeRead(t *testing.T) {
	ctx := context.Background()
	r, ledger := recorder(t, &stubScopes{err: errors.New("database down")})

	r.Record(ctx, karakuriquota.Spend{
		TwinID: "t1", Provider: "anthropic", Model: "opus",
		Units: 1_000_000, UnitKind: cost.UnitTokens,
	})

	got, err := ledger.Aggregate(ctx, cost.Query{})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(got) != 1 || got[0].Cost != 15 {
		t.Fatalf("buckets = %+v, want the spend recorded unlabelled", got)
	}
}

// Providers that cannot report usage send zero. Recording those would fill the
// ledger with rows that say nothing.
func TestRecorderIgnoresZeroUnits(t *testing.T) {
	ctx := context.Background()
	r, ledger := recorder(t, nil)

	r.Record(ctx, karakuriquota.Spend{TwinID: "t1", Provider: "gemini-cli", Units: 0, UnitKind: cost.UnitTokens})
	if ledger.Len() != 0 {
		t.Fatalf("a zero-unit spend was recorded")
	}
}

// A deployment with no ledger must not make the loop branch.
func TestRecorderZeroValueDiscards(t *testing.T) {
	var r *karakuriquota.Recorder
	r.Record(context.Background(), karakuriquota.Spend{TwinID: "t1", Units: 100, UnitKind: cost.UnitTokens})
	if r.Enabled() {
		t.Error("a nil recorder reported itself enabled")
	}
	empty := &karakuriquota.Recorder{}
	empty.Record(context.Background(), karakuriquota.Spend{TwinID: "t1", Units: 100, UnitKind: cost.UnitTokens})
	if empty.Enabled() {
		t.Error("a recorder with no ledger reported itself enabled")
	}
}

// A dashboard follows the hub, so every write has to publish.
func TestRecorderPublishes(t *testing.T) {
	ctx := context.Background()
	hub := event.NewHub()
	r, _ := recorder(t, nil)
	r.Hub = hub

	sub, unsub := hub.Subscribe(ctx, "_global")
	defer unsub()
	r.Record(ctx, karakuriquota.Spend{
		TwinID: "t1", Provider: "anthropic", Model: "opus",
		Units: 1_000_000, UnitKind: cost.UnitTokens,
	})

	select {
	case ev := <-sub:
		if ev.Type != event.TypeCostRecorded {
			t.Fatalf("event type = %q", ev.Type)
		}
		if ev.Payload["cost"] != 15.0 {
			t.Fatalf("payload cost = %v, want 15", ev.Payload["cost"])
		}
	case <-time.After(time.Second):
		t.Fatal("no cost_recorded event was published")
	}
}

// A ledger that cannot prune, or a retention of zero, must not be an error —
// keeping everything is a choice, and the memory ledger has nothing to prune.
func TestSweepCostsIsANoOpWithoutASweeper(t *testing.T) {
	ctx := context.Background()
	r, _ := recorder(t, nil)
	deps := karakuriquota.Deps{Costs: r}

	if n, err := deps.SweepCosts(ctx, 24*time.Hour, time.Now()); err != nil || n != 0 {
		t.Fatalf("SweepCosts on a memory ledger = %d, %v; want 0, nil", n, err)
	}
	// Zero retention keeps everything, which is how an operator says so.
	if n, err := deps.SweepCosts(ctx, 0, time.Now()); err != nil || n != 0 {
		t.Fatalf("SweepCosts with no horizon = %d, %v; want 0, nil", n, err)
	}
	// And a deployment with no ledger at all does not branch.
	var none karakuriquota.Deps
	if n, err := none.SweepCosts(ctx, 24*time.Hour, time.Now()); err != nil || n != 0 {
		t.Fatalf("SweepCosts with no ledger = %d, %v; want 0, nil", n, err)
	}
}

// The horizon is the thing under test: events older than it go, the rest stay,
// and the daily rollup survives either way — which is what makes a short
// retention cost the drill-down and not the totals.
func TestSweepCostsPrunesEventsAndKeepsTheRollup(t *testing.T) {
	ctx := context.Background()
	db := openCostDB(t)
	ledger, err := costsql.New(db, costsql.Options{Dialect: costsql.SQLite})
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	if err := ledger.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	record := func(at time.Time, amount float64) {
		t.Helper()
		err := ledger.Record(ctx, cost.Event{
			Subject: karakuriquota.CostSubject("t1"), ResourceType: "twin", ResourceID: "t1",
			Provider: "anthropic", Units: 1, UnitKind: cost.UnitCalls,
			Cost: amount, OccurredAt: at,
		})
		if err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	record(now.AddDate(0, 0, -40), 10) // past the horizon
	record(now.AddDate(0, 0, -2), 5)   // inside it

	deps := karakuriquota.Deps{Costs: &karakuriquota.Recorder{Ledger: ledger}}
	n, err := deps.SweepCosts(ctx, 30*24*time.Hour, now)
	if err != nil {
		t.Fatalf("SweepCosts: %v", err)
	}
	if n != 1 {
		t.Fatalf("pruned %d rows, want the one past the horizon", n)
	}

	events, err := ledger.Events(ctx, cost.Query{})
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) != 1 || events[0].Cost != 5 {
		t.Fatalf("events = %+v, want only the recent one", events)
	}

	// The totals are read from the rollup, so they still include the pruned
	// spend. Losing them is the expensive thing; losing the drill-down is not.
	buckets, err := ledger.Aggregate(ctx, cost.Query{})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(buckets) != 1 || buckets[0].Cost != 15 {
		t.Fatalf("total = %+v, want 15 — the rollup outlives the events", buckets)
	}
}
