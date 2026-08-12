package sql_test

import (
	"context"
	stdsql "database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/bsenel/karakuri/quota"
	"github.com/bsenel/karakuri/quota/cost"
	"github.com/bsenel/karakuri/quota/cost/costtest"
	costsql "github.com/bsenel/karakuri/quota/cost/sql"

	_ "modernc.org/sqlite"
)

// A file rather than :memory:, and with the busy timeout quota/sql's package
// doc requires of SQLite callers — this ledger takes the same write lock.
func dsn(path string) string {
	return "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

func open(t *testing.T) (*costsql.Ledger, *stdsql.DB) {
	t.Helper()
	db, err := stdsql.Open("sqlite", dsn(filepath.Join(t.TempDir(), "cost.db")))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	l, err := costsql.New(db, costsql.Options{Dialect: costsql.SQLite})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := l.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return l, db
}

// The whole point of the shared suite: this ledger has to answer identically to
// the in-memory one, case for case, or a report means something different
// depending on which is wired up.
func TestSatisfiesContract(t *testing.T) {
	costtest.Run(t, func(t *testing.T) cost.Ledger {
		l, _ := open(t)
		return l
	})
}

func event(subject quota.Key, provider, model string, units, price float64, at time.Time, labels ...string) cost.Event {
	return cost.Event{
		Subject: subject, ResourceType: "twin", ResourceID: string(subject),
		Provider: provider, Model: model,
		Units: units, UnitKind: cost.UnitTokens, Cost: price,
		OccurredAt: at, Labels: labels,
	}
}

// The rollup is maintained on the write path, so it can never lag the events it
// summarises. This is what a background aggregator would put at risk.
func TestRollupMatchesTheEvents(t *testing.T) {
	l, db := open(t)
	ctx := context.Background()
	at := costtest.Base

	for i := range 10 {
		if err := l.Record(ctx, event("twin|t1", "anthropic", "opus", 100, 0.5, at.Add(time.Duration(i)*time.Minute))); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	var rows, events int
	var units, amount float64
	if err := db.QueryRow(
		`SELECT COUNT(*), SUM(events), SUM(units), SUM(cost_amount) FROM cost_daily`,
	).Scan(&rows, &events, &units, &amount); err != nil {
		t.Fatalf("read rollup: %v", err)
	}
	// Ten events on one day, one provider, one model: one rolled-up row.
	if rows != 1 {
		t.Fatalf("rollup rows = %d, want one", rows)
	}
	if events != 10 || units != 1000 || amount != 5 {
		t.Fatalf("rollup = %d events, %v units, %v cost", events, units, amount)
	}

	var rawCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM cost_events`).Scan(&rawCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if rawCount != 10 {
		t.Fatalf("raw events = %d, want ten", rawCount)
	}

	// And the report agrees with both.
	got, err := l.Aggregate(ctx, cost.Query{})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(got) != 1 || got[0].Events != 10 || got[0].Units != 1000 || got[0].Cost != 5 {
		t.Fatalf("Aggregate = %+v, want it to agree with the tables", got)
	}
}

// Raw events are prunable and the rollup is not, which is the shape most
// deployments want: last year is a number, last week is a list.
func TestSweepKeepsTheRollup(t *testing.T) {
	l, _ := open(t)
	ctx := context.Background()
	old := cost.Day(costtest.Base)
	recent := old.AddDate(0, 0, 30)

	if err := l.Record(ctx, event("twin|t1", "anthropic", "opus", 100, 1, old)); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := l.Record(ctx, event("twin|t1", "anthropic", "opus", 200, 2, recent)); err != nil {
		t.Fatalf("Record: %v", err)
	}

	swept, err := l.Sweep(ctx, recent)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if swept != 1 {
		t.Fatalf("swept %d rows, want the one past the horizon", swept)
	}

	// The old day is still reportable.
	got, err := l.Aggregate(ctx, cost.Query{GroupBy: []cost.GroupBy{cost.ByDay}})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("after sweep = %+v, want both days still in the rollup", got)
	}

	// But its individual calls are gone.
	events, err := l.Events(ctx, cost.Query{})
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) != 1 || !events[0].OccurredAt.Equal(recent) {
		t.Fatalf("events after sweep = %+v, want only the recent one", events)
	}
}

// The drill-down: from a total to the calls behind it.
func TestEventsDrillDown(t *testing.T) {
	l, _ := open(t)
	ctx := context.Background()
	at := costtest.Base

	if err := l.Record(ctx, event("twin|t1", "anthropic", "opus", 100, 1, at, "org:o_acme")); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := l.Record(ctx, event("twin|t2", "google", "gemini", 200, 2, at.Add(time.Hour), "org:o_globex")); err != nil {
		t.Fatalf("Record: %v", err)
	}

	all, err := l.Events(ctx, cost.Query{})
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(all) != 2 || all[0].Subject != "twin|t2" {
		t.Fatalf("Events = %+v, want both, newest first", all)
	}
	if len(all[0].Labels) != 1 || all[0].Labels[0] != "org:o_globex" {
		t.Fatalf("labels did not round trip: %+v", all[0].Labels)
	}

	for name, tc := range map[string]struct {
		query cost.Query
		want  int
	}{
		"by subject":  {cost.Query{Subjects: []quota.Key{"twin|t1"}}, 1},
		"by provider": {cost.Query{Providers: []string{"google"}}, 1},
		"by label":    {cost.Query{Labels: []string{"org:o_acme"}}, 1},
		"by range":    {cost.Query{Since: at.Add(30 * time.Minute)}, 1},
		"limit":       {cost.Query{Limit: 1}, 1},
		"no match":    {cost.Query{Subjects: []quota.Key{"twin|nobody"}}, 0},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := l.Events(ctx, tc.query)
			if err != nil {
				t.Fatalf("Events: %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("Events = %+v, want %d", got, tc.want)
			}
		})
	}

	if _, err := l.Events(ctx, cost.Query{GroupBy: []cost.GroupBy{"colour"}}); err == nil {
		t.Error("Events accepted an invalid query")
	}
}

// A twin moving between teams must not split its day into two rows, or a report
// would show one twin as two.
func TestLabelsAreCarriedNotKeyed(t *testing.T) {
	l, db := open(t)
	ctx := context.Background()
	at := costtest.Base

	if err := l.Record(ctx, event("twin|t1", "anthropic", "opus", 100, 1, at, "team:t_eng")); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := l.Record(ctx, event("twin|t1", "anthropic", "opus", 100, 1, at.Add(time.Hour), "team:t_ops")); err != nil {
		t.Fatalf("Record: %v", err)
	}

	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM cost_daily`).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Fatalf("rollup rows = %d, want one — the move must not split the day", rows)
	}
	// The raw events keep both, so the drill-down still shows where each call
	// actually sat.
	events, err := l.Events(ctx, cost.Query{})
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v, want both", events)
	}
}

func TestNewRejectsBadInput(t *testing.T) {
	if _, err := costsql.New(nil, costsql.Options{}); err == nil {
		t.Error("New accepted a nil database")
	}
	db, _ := stdsql.Open("sqlite", dsn(filepath.Join(t.TempDir(), "x.db")))
	defer db.Close()
	if _, err := costsql.New(db, costsql.Options{Dialect: "oracle"}); err == nil {
		t.Error("New accepted an unknown dialect")
	}
	// The default is SQLite, so a caller who does not care need not say.
	if _, err := costsql.New(db, costsql.Options{}); err != nil {
		t.Errorf("New with no dialect: %v", err)
	}
}

func TestTablePrefixAndDDL(t *testing.T) {
	db, err := stdsql.Open("sqlite", dsn(filepath.Join(t.TempDir(), "prefixed.db")))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	l, err := costsql.New(db, costsql.Options{Dialect: costsql.SQLite, TablePrefix: "karakuri_"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := l.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := l.Record(context.Background(),
		event("twin|t1", "anthropic", "opus", 100, 1, costtest.Base)); err != nil {
		t.Fatalf("Record: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM karakuri_cost_events`).Scan(&n); err != nil {
		t.Fatalf("the prefix did not reach the tables: %v", err)
	}
	if n != 1 {
		t.Fatalf("prefixed table holds %d rows", n)
	}

	// DDL is what an operator applying schema by hand runs, so it has to name
	// the same tables Migrate created.
	ddl := l.DDL()
	for _, want := range []string{"karakuri_cost_events", "karakuri_cost_daily"} {
		if !contains(ddl, want) {
			t.Errorf("DDL does not mention %q", want)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

// Migrate runs on every boot, so it has to be safe to run twice.
func TestMigrateIsIdempotent(t *testing.T) {
	l, _ := open(t)
	if err := l.Migrate(context.Background()); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

// A closed database fails rather than silently reporting nothing.
func TestClosedDatabase(t *testing.T) {
	l, db := open(t)
	ctx := context.Background()
	db.Close()

	if err := l.Record(ctx, event("twin|t1", "anthropic", "opus", 1, 1, costtest.Base)); err == nil {
		t.Error("Record on a closed database succeeded")
	}
	if _, err := l.Aggregate(ctx, cost.Query{}); err == nil {
		t.Error("Aggregate on a closed database succeeded")
	}
	if _, err := l.Events(ctx, cost.Query{}); err == nil {
		t.Error("Events on a closed database succeeded")
	}
	if _, err := l.Sweep(ctx, time.Now()); err == nil {
		t.Error("Sweep on a closed database succeeded")
	}
	if err := l.Migrate(ctx); err == nil {
		t.Error("Migrate on a closed database succeeded")
	}
}
