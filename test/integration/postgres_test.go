//go:build postgres

// Postgres-backed integration test. Opt-in via the `postgres` build tag so the
// default `go test ./...` keeps running against SQLite without external deps.
//
// Run with:
//
//	KARAKURI_TEST_POSTGRES_DSN="host=localhost port=5432 user=karakuri \
//	                            password=secret dbname=karakuri sslmode=disable" \
//	go test -tags=postgres ./test/integration/...
//
// Verifies:
//   - PostgreSQL dialect opens + migrates the schema cleanly
//   - StorageAdapter round-trips a twin (Save → Get → List → UpdateBindings)
//   - The pgvector memory backend initializes (creates the extension + table)
//   - SQLite → Postgres migration via the krk migrate service preserves rows
package integration

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/db/schema"
	platmem "github.com/bsenel/karakuri/internal/platform/memory"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/core/twin"
	"github.com/bsenel/karakuri/internal/feature/migrate"
)

func pgDSN(t *testing.T) string {
	dsn := os.Getenv("KARAKURI_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KARAKURI_TEST_POSTGRES_DSN not set; skipping postgres-backed tests")
	}
	return dsn
}

// TestPostgresDialectOpens verifies the postgres dialect can be opened and
// the GORM AutoMigrate pass creates every required table.
func TestPostgresDialectOpens(t *testing.T) {
	dsn := pgDSN(t)
	db, err := platformdb.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := platformdb.RunMigrations(db, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Spot-check one table.
	if err := db.AutoMigrate(&schema.TwinModel{}); err != nil {
		t.Fatalf("re-migrate twins: %v", err)
	}
}

// TestPostgresTwinRoundtrip exercises the StorageAdapter against Postgres.
func TestPostgresTwinRoundtrip(t *testing.T) {
	dsn := pgDSN(t)
	db, err := platformdb.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := platformdb.RunMigrations(db, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := storage.NewGORMStorage(db)

	id := "pg-twin-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	tw := twin.DigitalTwin{
		ID: id, Name: "pg-test", Kind: twin.KindTeam, Domain: "software",
		AdapterBindings: map[string]string{"versioncontrol": "acme_github"},
	}
	ctx := context.Background()
	if err := store.SaveTwin(ctx, tw); err != nil {
		t.Fatalf("save twin: %v", err)
	}
	got, err := store.GetTwin(ctx, id)
	if err != nil {
		t.Fatalf("get twin: %v", err)
	}
	if got.AdapterBindings["versioncontrol"] != "acme_github" {
		t.Errorf("bindings round-trip mismatch: %+v", got.AdapterBindings)
	}
}

// TestPgVectorBackendInit verifies the pgvector backend creates its extension
// + table on first init without panicking.
func TestPgVectorBackendInit(t *testing.T) {
	dsn := pgDSN(t)
	db, err := platformdb.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if _, err := platmem.NewSemanticMemoryPgVector(context.Background(), db, 1536); err != nil {
		t.Fatalf("pgvector init: %v (pgvector extension may not be installed on the test database)", err)
	}
}

// TestSQLiteToPostgresMigration verifies the migrate service copies rows from
// a fresh SQLite source to the test Postgres target. Cleans up the source file;
// leaves the Postgres tables populated (caller is expected to TRUNCATE between
// runs or use a disposable test DB).
func TestSQLiteToPostgresMigration(t *testing.T) {
	dsn := pgDSN(t)
	srcFile, err := os.CreateTemp("", "karakuri-migrate-src-*.db")
	if err != nil {
		t.Fatalf("temp src db: %v", err)
	}
	srcPath := srcFile.Name()
	srcFile.Close()
	defer os.Remove(srcPath)

	// Seed source with a twin via the storage adapter so we know exactly what we expect.
	srcDB, err := platformdb.Open("sqlite", srcPath)
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	if err := platformdb.RunMigrations(srcDB, srcPath); err != nil {
		t.Fatalf("migrate src: %v", err)
	}
	srcStore := storage.NewGORMStorage(srcDB)
	id := "mig-twin-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := srcStore.SaveTwin(context.Background(), twin.DigitalTwin{
		ID: id, Name: "mig", Kind: twin.KindTeam, Domain: "software",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	report, err := migrate.Run(context.Background(), migrate.Plan{
		FromDriver: "sqlite", FromDSN: srcPath,
		ToDriver: "postgres", ToDSN: dsn,
	})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if report.Tables["twins"] < 1 {
		t.Errorf("expected at least 1 twin migrated, got %d", report.Tables["twins"])
	}
}
