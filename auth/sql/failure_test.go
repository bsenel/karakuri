package sql_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/bsenel/karakuri/auth"
	authsql "github.com/bsenel/karakuri/auth/sql"
	_ "modernc.org/sqlite"
)

// TestSurfacesDatabaseErrors runs every method against a closed handle. A store
// that swallows a database outage is worse than one that fails: an authorizer
// reading zero bindings from a broken database would deny every request without
// anyone knowing why, and a silent write failure would lose a revocation.
func TestSurfacesDatabaseErrors(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "closed.db"))
	must(t, err)
	store, err := authsql.New(db, authsql.Options{Dialect: authsql.SQLite})
	must(t, err)
	must(t, store.Migrate(ctx))
	must(t, db.Close())

	now := time.Now()
	calls := map[string]func() error{
		"Migrate":                         func() error { return store.Migrate(ctx) },
		"GetPrincipal":                    func() error { _, err := store.GetPrincipal(ctx, "alice"); return err },
		"ListPrincipals":                  func() error { _, err := store.ListPrincipals(ctx); return err },
		"PutPrincipal":                    func() error { return store.PutPrincipal(ctx, auth.Principal{ID: "alice"}) },
		"DeletePrincipal":                 func() error { return store.DeletePrincipal(ctx, "alice") },
		"GetRole":                         func() error { _, err := store.GetRole(ctx, "viewer"); return err },
		"ListRoles":                       func() error { _, err := store.ListRoles(ctx); return err },
		"PutRole":                         func() error { return store.PutRole(ctx, auth.Role{Name: "viewer"}) },
		"DeleteRole":                      func() error { return store.DeleteRole(ctx, "viewer") },
		"ListBindings":                    func() error { _, err := store.ListBindings(ctx, "alice"); return err },
		"PutBinding":                      func() error { return store.PutBinding(ctx, auth.RoleBinding{ID: "b", PrincipalID: "a", Role: "r"}) },
		"DeleteBinding":                   func() error { return store.DeleteBinding(ctx, "b") },
		"GetCredential":                   func() error { _, err := store.GetCredential(ctx, "alice"); return err },
		"PutCredential":                   func() error { return store.PutCredential(ctx, auth.Credential{PrincipalID: "alice"}) },
		"DeleteCredential":                func() error { return store.DeleteCredential(ctx, "alice") },
		"PutRefreshToken":                 func() error { return store.PutRefreshToken(ctx, refreshToken("t", "f", "alice")) },
		"GetRefreshTokenByHash":           func() error { _, err := store.GetRefreshTokenByHash(ctx, "hash"); return err },
		"SpendRefreshToken":               func() error { _, err := store.SpendRefreshToken(ctx, "t", now); return err },
		"RevokeRefreshFamily":             func() error { return store.RevokeRefreshFamily(ctx, "f", now) },
		"RevokeRefreshTokensForPrincipal": func() error { return store.RevokeRefreshTokensForPrincipal(ctx, "alice", now) },
		"DeleteExpiredRefreshTokens":      func() error { _, err := store.DeleteExpiredRefreshTokens(ctx, now); return err },
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s returned nil against a closed database", name)
		}
	}
}

// TestSpendRefreshTokenOnMissingRowAfterFailedUpdate covers the branch where the
// UPDATE matches nothing because the row is gone rather than already spent.
func TestSpendRefreshTokenOnMissingRow(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	if _, err := s.SpendRefreshToken(ctx, "never-existed", time.Now()); err == nil {
		t.Error("spending a nonexistent token returned nil")
	}
}

// TestCorruptJSONColumns proves a malformed JSON column is reported rather than
// silently decoded into an empty value — an unreadable policy set must not look
// like a role that grants nothing.
func TestCorruptJSONColumns(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "corrupt.db"))
	must(t, err)
	defer db.Close()

	store, err := authsql.New(db, authsql.Options{Dialect: authsql.SQLite})
	must(t, err)
	must(t, store.Migrate(ctx))

	_, err = db.ExecContext(ctx,
		`INSERT INTO auth_principals (id, name, kind, attrs_json, disabled) VALUES ('alice', '', 'user', '{oops', 0)`)
	must(t, err)
	if _, err := store.GetPrincipal(ctx, "alice"); err == nil {
		t.Error("GetPrincipal accepted a corrupt attrs_json")
	}
	if _, err := store.ListPrincipals(ctx); err == nil {
		t.Error("ListPrincipals accepted a corrupt attrs_json")
	}

	_, err = db.ExecContext(ctx,
		`INSERT INTO auth_roles (name, description, inherits_json, system) VALUES ('viewer', '', '[oops', 0)`)
	must(t, err)
	if _, err := store.GetRole(ctx, "viewer"); err == nil {
		t.Error("GetRole accepted a corrupt inherits_json")
	}
	if _, err := store.ListRoles(ctx); err == nil {
		t.Error("ListRoles accepted a corrupt inherits_json")
	}

	_, err = db.ExecContext(ctx, `UPDATE auth_roles SET inherits_json = '[]' WHERE name = 'viewer'`)
	must(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO auth_policies (id, role_name, ordinal, action, resource, effect, conditions_json)
		 VALUES ('p1', 'viewer', 0, 'twin:read', '*', 'allow', '[oops')`)
	must(t, err)
	if _, err := store.GetRole(ctx, "viewer"); err == nil {
		t.Error("GetRole accepted a corrupt conditions_json")
	}
}

// TestEmptyJSONColumnsDecodeToZeroValues pins the other side of that coin: an
// empty or literal-null column is legitimately absent data, not corruption.
func TestEmptyJSONColumnsDecodeToZeroValues(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "empty.db"))
	must(t, err)
	defer db.Close()

	store, err := authsql.New(db, authsql.Options{Dialect: authsql.SQLite})
	must(t, err)
	must(t, store.Migrate(ctx))

	_, err = db.ExecContext(ctx,
		`INSERT INTO auth_principals (id, name, kind, attrs_json, disabled) VALUES ('alice', '', 'user', '', 0)`)
	must(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO auth_roles (name, description, inherits_json, system) VALUES ('viewer', '', 'null', 0)`)
	must(t, err)

	p, err := store.GetPrincipal(ctx, "alice")
	if err != nil || len(p.Attrs) != 0 {
		t.Errorf("empty attrs_json = %+v, %v", p.Attrs, err)
	}
	r, err := store.GetRole(ctx, "viewer")
	if err != nil || len(r.Inherits) != 0 {
		t.Errorf("null inherits_json = %+v, %v", r.Inherits, err)
	}
}

// TestRefreshTokenPreservesUsedAndRevokedTimestamps covers persisting a token
// that already carries used/revoked stamps, which is what a store-to-store copy
// or a migration would write.
func TestRefreshTokenPreservesUsedAndRevokedTimestamps(t *testing.T) {
	ctx := context.Background()
	used := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	revoked := used.Add(time.Minute)

	for name, newStore := range factories() {
		t.Run(name, func(t *testing.T) {
			s := newStore(t)
			tok := refreshToken("t1", "f1", "alice")
			tok.UsedAt = &used
			tok.RevokedAt = &revoked
			tok.ParentID = "t0"
			must(t, s.PutRefreshToken(ctx, tok))

			got, err := s.GetRefreshTokenByHash(ctx, auth.HashToken("t1"))
			must(t, err)
			if got.ParentID != "t0" {
				t.Errorf("ParentID = %q", got.ParentID)
			}
			if got.UsedAt == nil || !got.UsedAt.Equal(used) {
				t.Errorf("UsedAt = %v, want %s", got.UsedAt, used)
			}
			if got.RevokedAt == nil || !got.RevokedAt.Equal(revoked) {
				t.Errorf("RevokedAt = %v, want %s", got.RevokedAt, revoked)
			}
		})
	}
}

// TestPartialSchemaFailures drops individual tables to exercise the failure
// paths *inside* the multi-statement transactions. These are the cases that
// matter most: a half-applied schema migration must abort the whole operation
// rather than leave a principal deleted but its grants intact.
func TestPartialSchemaFailures(t *testing.T) {
	ctx := context.Background()

	newStore := func(t *testing.T, drop string) (*authsql.Store, *sql.DB) {
		t.Helper()
		db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "partial.db"))
		must(t, err)
		t.Cleanup(func() { _ = db.Close() })
		store, err := authsql.New(db, authsql.Options{Dialect: authsql.SQLite})
		must(t, err)
		must(t, store.Migrate(ctx))

		must(t, store.PutPrincipal(ctx, auth.Principal{ID: "alice"}))
		must(t, store.PutRole(ctx, auth.Role{Name: "viewer", Policies: []auth.Policy{auth.Allow("v1", "twin:read", "*")}}))
		must(t, store.PutBinding(ctx, auth.RoleBinding{ID: "b1", PrincipalID: "alice", Role: "viewer"}))
		must(t, store.PutCredential(ctx, auth.Credential{PrincipalID: "alice", PasswordHash: "x"}))

		_, err = db.ExecContext(ctx, "DROP TABLE "+drop)
		must(t, err)
		return store, db
	}

	t.Run("policies table missing", func(t *testing.T) {
		store, _ := newStore(t, "auth_policies")
		if _, err := store.GetRole(ctx, "viewer"); err == nil {
			t.Error("GetRole returned nil with the policies table gone")
		}
		if _, err := store.ListRoles(ctx); err == nil {
			t.Error("ListRoles returned nil with the policies table gone")
		}
		if err := store.PutRole(ctx, auth.Role{Name: "other"}); err == nil {
			t.Error("PutRole returned nil with the policies table gone")
		}
	})

	t.Run("bindings table missing", func(t *testing.T) {
		store, db := newStore(t, "auth_role_bindings")
		if err := store.DeletePrincipal(ctx, "alice"); err == nil {
			t.Fatal("DeletePrincipal returned nil with the bindings table gone")
		}
		// The transaction must have rolled back — a principal deleted without
		// its grants going too is exactly the state to avoid.
		var count int
		must(t, db.QueryRowContext(ctx, `SELECT COUNT(1) FROM auth_principals WHERE id = 'alice'`).Scan(&count))
		if count != 1 {
			t.Error("the principal delete was committed despite the transaction failing")
		}
	})

	t.Run("credentials table missing", func(t *testing.T) {
		store, _ := newStore(t, "auth_credentials")
		if err := store.DeletePrincipal(ctx, "alice"); err == nil {
			t.Error("DeletePrincipal returned nil with the credentials table gone")
		}
		if _, err := store.GetCredential(ctx, "alice"); err == nil {
			t.Error("GetCredential returned nil with the credentials table gone")
		}
	})

	t.Run("roles table missing", func(t *testing.T) {
		store, _ := newStore(t, "auth_roles")
		if err := store.DeleteRole(ctx, "viewer"); err == nil {
			t.Error("DeleteRole returned nil with the roles table gone")
		}
	})
}
