package sql

import "testing"

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
		{"numbering runs past nine", Postgres,
			`VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			`VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`},
		// Multi-byte characters must not be chopped up on the way through: the
		// rewriter walks runes, and a table prefix or a comment could contain
		// anything.
		{"non-ascii survives", Postgres,
			`SELECT '→' FROM t WHERE k = ?`,
			`SELECT '→' FROM t WHERE k = $1`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := &Backend{dialect: tc.dialect}
			if got := b.rebind(tc.query); got != tc.want {
				t.Errorf("rebind() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLockClause(t *testing.T) {
	// Postgres serialises concurrent takes with FOR UPDATE; SQLite gets the
	// same guarantee from BEGIN IMMEDIATE and would reject the clause outright.
	if got := (&Backend{dialect: Postgres}).lockClause(); got != " FOR UPDATE" {
		t.Errorf("postgres lock clause = %q", got)
	}
	if got := (&Backend{dialect: SQLite}).lockClause(); got != "" {
		t.Errorf("sqlite lock clause = %q, want empty", got)
	}
}

func TestTablePrefixApplies(t *testing.T) {
	b := &Backend{prefix: "krk_"}
	if got := b.table("quota_counters"); got != "krk_quota_counters" {
		t.Errorf("table() = %q", got)
	}
	if got := (&Backend{}).table("quota_counters"); got != "quota_counters" {
		t.Errorf("table() with no prefix = %q", got)
	}
}
