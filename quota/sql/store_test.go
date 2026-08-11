package sql_test

import (
	"context"
	stdsql "database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/quota"
	"github.com/bsenel/karakuri/quota/quotatest"
	quotasql "github.com/bsenel/karakuri/quota/sql"

	_ "modernc.org/sqlite"
)

// open returns a migrated backend over a fresh SQLite file.
//
// A file rather than :memory:, because the concurrency case in the contract
// suite runs real parallel connections and an in-memory database is per
// connection unless shared — which would quietly turn the one case that proves
// atomicity into a case that proves nothing.
// dsn carries the busy timeout the package doc requires of SQLite callers.
// Without it BEGIN IMMEDIATE fails outright the moment two goroutines contend,
// and the contract's concurrency case would fail for that reason rather than
// for the one it is testing.
func dsn(path string) string {
	return "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

func open(t *testing.T) (*quotasql.Backend, *stdsql.DB) {
	t.Helper()
	db, err := stdsql.Open("sqlite", dsn(filepath.Join(t.TempDir(), "quota.db")))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	b, err := quotasql.New(db, quotasql.Options{Dialect: quotasql.SQLite})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := b.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return b, db
}

// The whole point of the shared suite: this backend has to behave identically
// to the in-memory one, case for case, or the limit an operator configures
// means something different depending on which backend is wired up.
func TestSatisfiesContract(t *testing.T) {
	quotatest.Run(t, func(t *testing.T) quota.Backend {
		b, _ := open(t)
		return b
	})
}

func TestMigrateIsIdempotent(t *testing.T) {
	b, _ := open(t)
	// Called on every boot, so a second call must be a no-op rather than an
	// error about an existing table.
	if err := b.Migrate(context.Background()); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestStatePersistsAcrossBackends(t *testing.T) {
	// The reason to use this backend at all: a restart must not hand everyone a
	// fresh budget.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "quota.db")
	p := quota.Policy{Algorithm: quota.FixedWindow, Limit: 2, Window: time.Hour}
	now := quotatest.Base

	first := func() {
		db, err := stdsql.Open("sqlite", dsn(path))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer db.Close()
		b, _ := quotasql.New(db, quotasql.Options{})
		if err := b.Migrate(ctx); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		for range 2 {
			if d, err := b.Take(ctx, "k", p, 1, now); err != nil || !d.Allowed {
				t.Fatalf("Take: %v allowed=%t", err, d.Allowed)
			}
		}
	}
	first()

	db, err := stdsql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()
	b, _ := quotasql.New(db, quotasql.Options{})
	d, err := b.Take(ctx, "k", p, 1, now)
	if err != nil {
		t.Fatalf("Take after reopen: %v", err)
	}
	if d.Allowed {
		t.Error("a restart handed back a spent budget")
	}
}

func TestSlidingLogTrimsItsTable(t *testing.T) {
	// One row per consumption means the table has to be pruned as entries age
	// out, or a long-lived key grows without bound.
	ctx := context.Background()
	b, db := open(t)
	p := quota.Policy{Algorithm: quota.SlidingLog, Limit: 5, Window: time.Minute}
	now := quotatest.Base

	for i := range 5 {
		if _, err := b.Take(ctx, "k", p, 1, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("Take: %v", err)
		}
	}
	if got := countEvents(t, db); got != 5 {
		t.Fatalf("stored %d events, want 5", got)
	}

	// Well past the window: every earlier entry should be gone, leaving only
	// the one this call adds.
	if _, err := b.Take(ctx, "k", p, 1, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("Take: %v", err)
	}
	if got := countEvents(t, db); got != 1 {
		t.Errorf("stored %d events after the window passed, want 1", got)
	}
}

func TestSweepDropsExpiredState(t *testing.T) {
	ctx := context.Background()
	b, db := open(t)
	p := quota.Policy{Algorithm: quota.SlidingLog, Limit: 5, Window: time.Minute}
	now := quotatest.Base

	if _, err := b.Take(ctx, "k", p, 1, now); err != nil {
		t.Fatalf("Take: %v", err)
	}

	// Inside the retention window, nothing goes.
	deleted, err := b.Sweep(ctx, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if deleted != 0 {
		t.Errorf("swept %d live rows", deleted)
	}

	deleted, err = b.Sweep(ctx, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if deleted != 1 {
		t.Errorf("swept %d rows, want 1", deleted)
	}
	// The events table is keyed off the counters, so orphans go with it.
	if got := countEvents(t, db); got != 0 {
		t.Errorf("%d orphaned events survived the sweep", got)
	}
}

func TestTablePrefix(t *testing.T) {
	// These tables often live beside an application's own schema, so the
	// prefix has to reach every statement — not just the DDL.
	ctx := context.Background()
	db, err := stdsql.Open("sqlite", dsn(filepath.Join(t.TempDir(), "quota.db")))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	b, err := quotasql.New(db, quotasql.Options{TablePrefix: "krk_"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := b.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	p := quota.Policy{Algorithm: quota.SlidingLog, Limit: 2, Window: time.Minute}
	if _, err := b.Take(ctx, "k", p, 1, quotatest.Base); err != nil {
		t.Fatalf("Take: %v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM krk_quota_events`).Scan(&n); err != nil {
		t.Fatalf("prefixed table not used: %v", err)
	}
	if n != 1 {
		t.Errorf("prefixed events table holds %d rows, want 1", n)
	}
}

func TestNewValidatesItsArguments(t *testing.T) {
	if _, err := quotasql.New(nil, quotasql.Options{}); err == nil {
		t.Error("New accepted a nil *sql.DB")
	}
	db, _ := stdsql.Open("sqlite", dsn(filepath.Join(t.TempDir(), "quota.db")))
	defer db.Close()

	if _, err := quotasql.New(db, quotasql.Options{Dialect: "oracle"}); !errors.Is(err, quotasql.ErrUnsupportedDialect) {
		t.Errorf("error = %v, want ErrUnsupportedDialect", err)
	}
	// An unset dialect is SQLite rather than an error, so the common case needs
	// no options at all.
	if _, err := quotasql.New(db, quotasql.Options{}); err != nil {
		t.Errorf("New with default options: %v", err)
	}
}

func TestDDLMatchesMigrate(t *testing.T) {
	// Operators who apply schema by hand render it from here, so it has to be
	// the same statements Migrate runs rather than a copy that drifts.
	b, _ := open(t)
	ddl := b.DDL()
	for _, want := range []string{"quota_counters", "quota_events", "CREATE INDEX", "used_units"} {
		if !strings.Contains(ddl, want) {
			t.Errorf("DDL is missing %q:\n%s", want, ddl)
		}
	}
}

func countEvents(t *testing.T, db *stdsql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM quota_events`).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}
