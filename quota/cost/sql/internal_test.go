package sql

import (
	"strings"
	"testing"
)

// The dialect-specific code is unit-tested rather than exercised against a live
// Postgres, matching quota/sql: these are the only three places the two
// dialects differ, and each is a pure string transformation.

func TestRebind(t *testing.T) {
	tests := []struct {
		name    string
		dialect Dialect
		query   string
		want    string
	}{
		{"sqlite passes through", SQLite,
			`SELECT a FROM t WHERE k = ? AND n > ?`,
			`SELECT a FROM t WHERE k = ? AND n > ?`},
		{"postgres numbers each placeholder", Postgres,
			`SELECT a FROM t WHERE k = ? AND n > ?`,
			`SELECT a FROM t WHERE k = $1 AND n > $2`},
		{"no placeholders", Postgres,
			`DELETE FROM t`,
			`DELETE FROM t`},
		// Record binds eleven values, so the numbering has to run past nine.
		{"numbering runs past nine", Postgres,
			`VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			`VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`},
		{"non-ascii survives", Postgres,
			`SELECT '→' FROM t WHERE k = ?`,
			`SELECT '→' FROM t WHERE k = $1`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := &Ledger{dialect: tc.dialect}
			if got := l.rebind(tc.query); got != tc.want {
				t.Errorf("rebind() = %q, want %q", got, tc.want)
			}
		})
	}
}

// The two dialects capitalise the proposed row in an upsert differently, and
// the rollup's whole correctness rests on that clause.
func TestExcluded(t *testing.T) {
	if got := (&Ledger{dialect: SQLite}).excluded("units"); got != "excluded.units" {
		t.Errorf("sqlite = %q", got)
	}
	if got := (&Ledger{dialect: Postgres}).excluded("units"); got != "EXCLUDED.units" {
		t.Errorf("postgres = %q", got)
	}
}

// The Postgres schema differs from SQLite's in one column type; everything else
// about the two has to be identical or a report would depend on the database.
func TestSchemaDiffersOnlyInColumnTypes(t *testing.T) {
	sqlite := (&Ledger{dialect: SQLite}).DDL()
	postgres := (&Ledger{dialect: Postgres}).DDL()

	if !strings.Contains(sqlite, "REAL") || strings.Contains(sqlite, "DOUBLE PRECISION") {
		t.Errorf("sqlite DDL uses the wrong float type:\n%s", sqlite)
	}
	if !strings.Contains(postgres, "DOUBLE PRECISION") || strings.Contains(postgres, "REAL") {
		t.Errorf("postgres DDL uses the wrong float type:\n%s", postgres)
	}
	// Same tables, same indices, same primary key.
	for _, want := range []string{
		"cost_events", "cost_daily",
		"cost_events_occurred_idx", "cost_events_subject_idx", "cost_daily_day_idx",
		"PRIMARY KEY (day_ms, subject, resource_type, resource_id, provider, model, unit_kind)",
	} {
		if !strings.Contains(sqlite, want) {
			t.Errorf("sqlite DDL is missing %q", want)
		}
		if !strings.Contains(postgres, want) {
			t.Errorf("postgres DDL is missing %q", want)
		}
	}
}

func TestSplitLabels(t *testing.T) {
	if got := splitLabels(""); got != nil {
		t.Errorf("empty = %v, want nil — an event in no container has no labels", got)
	}
	got := splitLabels("team:t_eng\norg:o_acme")
	if len(got) != 2 || got[0] != "team:t_eng" || got[1] != "org:o_acme" {
		t.Errorf("splitLabels = %v", got)
	}
}

func TestPlaceholders(t *testing.T) {
	if got := placeholders(1); got != "?" {
		t.Errorf("one = %q", got)
	}
	if got := placeholders(3); got != "?,?,?" {
		t.Errorf("three = %q", got)
	}
}
