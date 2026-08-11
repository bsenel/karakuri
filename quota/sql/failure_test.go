package sql_test

import (
	"context"
	stdsql "database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/bsenel/karakuri/quota"
	"github.com/bsenel/karakuri/quota/quotatest"
	quotasql "github.com/bsenel/karakuri/quota/sql"
)

var testPolicy = quota.Policy{Algorithm: quota.SlidingLog, Limit: 5, Window: time.Minute}

// A closed handle is the cheapest fault injector there is, and it reaches every
// method's error path. What matters is that a database problem surfaces as an
// error rather than as a Decision — the contract reserves errors for "I could
// not find out", and a limiter that reports "allowed" when its store is down
// has silently stopped limiting.
func TestClosedDatabaseSurfacesErrors(t *testing.T) {
	ctx := context.Background()
	db, err := stdsql.Open("sqlite", dsn(filepath.Join(t.TempDir(), "quota.db")))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	b, err := quotasql.New(db, quotasql.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := b.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	t.Run("Take", func(t *testing.T) {
		d, err := b.Take(ctx, "k", testPolicy, 1, quotatest.Base)
		if err == nil {
			t.Fatal("Take succeeded against a closed database")
		}
		if d.Allowed {
			t.Error("a failed Take reported the request as allowed")
		}
	})
	t.Run("Peek", func(t *testing.T) {
		if _, err := b.Peek(ctx, "k", testPolicy, quotatest.Base); err == nil {
			t.Error("Peek succeeded against a closed database")
		}
	})
	t.Run("Reset", func(t *testing.T) {
		if err := b.Reset(ctx, "k"); err == nil {
			t.Error("Reset succeeded against a closed database")
		}
	})
	t.Run("Sweep", func(t *testing.T) {
		if _, err := b.Sweep(ctx, quotatest.Base); err == nil {
			t.Error("Sweep succeeded against a closed database")
		}
	})
	t.Run("Migrate", func(t *testing.T) {
		if err := b.Migrate(ctx); err == nil {
			t.Error("Migrate succeeded against a closed database")
		}
	})
}

func TestInvalidPolicyIsRejectedBeforeTheDatabase(t *testing.T) {
	// Cheap to check and expensive to get wrong: a policy with no window would
	// otherwise reach the query layer and produce a confusing SQL error rather
	// than the configuration error it is.
	ctx := context.Background()
	b, _ := open(t)
	bad := quota.Policy{Algorithm: quota.FixedWindow, Limit: 1}

	if _, err := b.Take(ctx, "k", bad, 1, quotatest.Base); !errors.Is(err, quota.ErrInvalidPolicy) {
		t.Errorf("Take error = %v, want ErrInvalidPolicy", err)
	}
	if _, err := b.Peek(ctx, "k", bad, quotatest.Base); !errors.Is(err, quota.ErrInvalidPolicy) {
		t.Errorf("Peek error = %v, want ErrInvalidPolicy", err)
	}
}

func TestDriftedSchemaSurfacesAsAnError(t *testing.T) {
	// These tables live in somebody else's database, so they can be altered out
	// from under us. A write that no longer fits has to fail loudly and roll
	// back — the alternative is a counter that silently stops recording while
	// the limiter keeps saying yes.
	ctx := context.Background()
	b, db := open(t)

	if _, err := db.Exec(`DROP TABLE quota_events`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE quota_events (
		key TEXT NOT NULL, at_ms BIGINT NOT NULL, n BIGINT NOT NULL, tenant TEXT NOT NULL)`); err != nil {
		t.Fatalf("recreate: %v", err)
	}

	// Reads and the delete still work; the insert cannot satisfy the new column.
	if _, err := b.Take(ctx, "k", testPolicy, 1, quotatest.Base); err == nil {
		t.Fatal("Take succeeded against a table it cannot write to")
	}

	// And the counter row must not have been left updated by the half-applied
	// transaction — that is what the rollback is for.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM quota_counters WHERE used_units > 0`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("%d counter rows survived a rolled-back take", n)
	}
}

// The Postgres code path differs from SQLite in three places: numbered
// placeholders, the FOR UPDATE clause, and an ordinary BeginTx instead of
// BEGIN IMMEDIATE. SQLite accepts $1 placeholders, so a Postgres-dialect
// backend pointed at SQLite exercises that path without a Postgres server —
// succeeding where the statement is portable and failing on FOR UPDATE, which
// is exactly the clause that only Postgres understands.
func TestPostgresDialectPath(t *testing.T) {
	ctx := context.Background()
	db, err := stdsql.Open("sqlite", dsn(filepath.Join(t.TempDir(), "quota.db")))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	b, err := quotasql.New(db, quotasql.Options{Dialect: quotasql.Postgres})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := b.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Reset is portable across both, so this drives the transaction to a commit.
	if err := b.Reset(ctx, "k"); err != nil {
		t.Errorf("Reset on the Postgres path: %v", err)
	}
	// Take is not: SQLite rejects FOR UPDATE, so this drives the rollback.
	if _, err := b.Take(ctx, "k", testPolicy, 1, quotatest.Base); err == nil {
		t.Error("SQLite accepted FOR UPDATE — this case is no longer testing the Postgres path")
	}
}

// Postgres itself is not exercised here. Running the contract against a real
// server would mean adding a Postgres driver to this module's dependencies for
// a test that skips by default, and the case above already covers the three
// places the Postgres path differs. Verifying against a live server is a
// release-time step, noted in the module README rather than faked here.
