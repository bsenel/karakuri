package sql_test

import (
	"context"
	"testing"

	"github.com/bsenel/karakuri/quota/cost"
	"github.com/bsenel/karakuri/quota/cost/costtest"
)

// A drifted schema — someone dropped a table, or a migration half-applied — has
// to fail loudly. A ledger that swallowed it would report a total that quietly
// omits everything written since.
func TestDriftedSchema(t *testing.T) {
	l, db := open(t)
	ctx := context.Background()

	// The rollup is gone but the event table is not, so the insert succeeds and
	// the fold fails. The transaction must take the insert back with it.
	if _, err := db.Exec(`DROP TABLE cost_daily`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := l.Record(ctx, event("twin|t1", "anthropic", "opus", 100, 1, costtest.Base)); err == nil {
		t.Fatal("Record succeeded with the rollup table missing")
	}

	var raw int
	if err := db.QueryRow(`SELECT COUNT(*) FROM cost_events`).Scan(&raw); err != nil {
		t.Fatalf("count: %v", err)
	}
	if raw != 0 {
		t.Fatalf("the event survived a failed rollup: %d rows — the two must land together", raw)
	}

	// And with the event table gone too, the insert itself fails.
	if _, err := db.Exec(`DROP TABLE cost_events`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := l.Record(ctx, event("twin|t1", "anthropic", "opus", 100, 1, costtest.Base)); err == nil {
		t.Fatal("Record succeeded with no tables at all")
	}
	if _, err := l.Aggregate(ctx, cost.Query{}); err == nil {
		t.Fatal("Aggregate succeeded with no tables")
	}
	if _, err := l.Sweep(ctx, costtest.Base); err == nil {
		t.Fatal("Sweep succeeded with no tables")
	}
}

// A cancelled context must not start a write, which is what the check after the
// write mutex is for.
func TestCancelledContext(t *testing.T) {
	l, _ := open(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := l.Record(ctx, event("twin|t1", "anthropic", "opus", 100, 1, costtest.Base)); err == nil {
		t.Fatal("Record succeeded on a cancelled context")
	}
}
