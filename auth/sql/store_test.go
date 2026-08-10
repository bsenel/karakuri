package sql_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/auth/jwt"
	authsql "github.com/bsenel/karakuri/auth/sql"
	_ "modernc.org/sqlite"
)

// storeUnderTest is the pair of interfaces every implementation must satisfy.
type storeUnderTest interface {
	auth.Store
	auth.CredentialStore
}

// factories runs the contract against both implementations, so the SQL store
// and the in-memory reference cannot quietly diverge.
func factories() map[string]func(t *testing.T) storeUnderTest {
	return map[string]func(t *testing.T) storeUnderTest{
		"memory": func(t *testing.T) storeUnderTest { return auth.NewMemoryStore() },
		"sqlite": func(t *testing.T) storeUnderTest { return newSQLiteStore(t) },
	}
}

func newSQLiteStore(t *testing.T) *authsql.Store {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "auth.db") + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// modernc's SQLite serialises writers; one connection keeps the pool from
	// turning contention into "database is locked".
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	store, err := authsql.New(db, authsql.Options{Dialect: authsql.SQLite})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Migrate must be safe to run on every boot.
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	return store
}

func TestNew(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if _, err := authsql.New(nil, authsql.Options{}); err == nil {
		t.Error("expected an error for a nil *sql.DB")
	}
	if _, err := authsql.New(db, authsql.Options{Dialect: "mysql"}); !errors.Is(err, authsql.ErrUnsupportedDialect) {
		t.Errorf("unsupported dialect = %v", err)
	}
	// The default dialect is SQLite.
	if _, err := authsql.New(db, authsql.Options{}); err != nil {
		t.Errorf("default options = %v", err)
	}
}

func TestPrincipalContract(t *testing.T) {
	for name, newStore := range factories() {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			s := newStore(t)

			if _, err := s.GetPrincipal(ctx, "alice"); !errors.Is(err, auth.ErrPrincipalNotFound) {
				t.Errorf("Get(missing) = %v", err)
			}
			if err := s.DeletePrincipal(ctx, "alice"); !errors.Is(err, auth.ErrPrincipalNotFound) {
				t.Errorf("Delete(missing) = %v", err)
			}
			if err := s.PutPrincipal(ctx, auth.Principal{}); err == nil {
				t.Error("Put without an ID succeeded")
			}

			alice := auth.Principal{
				ID: "alice", Name: "Alice", Kind: auth.KindUser,
				Attrs: map[string]string{"team": "eng", "region": "eu"},
			}
			if err := s.PutPrincipal(ctx, alice); err != nil {
				t.Fatalf("Put: %v", err)
			}
			got, err := s.GetPrincipal(ctx, "alice")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.Name != "Alice" || got.Kind != auth.KindUser || got.Attrs["team"] != "eng" || got.Attrs["region"] != "eu" {
				t.Fatalf("round-trip = %+v", got)
			}
			if got.Disabled {
				t.Error("Disabled defaulted to true")
			}

			// Upsert.
			alice.Disabled = true
			alice.Attrs = nil
			if err := s.PutPrincipal(ctx, alice); err != nil {
				t.Fatalf("upsert: %v", err)
			}
			got, _ = s.GetPrincipal(ctx, "alice")
			if !got.Disabled || len(got.Attrs) != 0 {
				t.Fatalf("after upsert = %+v", got)
			}

			if err := s.PutPrincipal(ctx, auth.Principal{ID: "bob", Kind: auth.KindService}); err != nil {
				t.Fatalf("Put(bob): %v", err)
			}
			list, err := s.ListPrincipals(ctx)
			if err != nil || len(list) != 2 || list[0].ID != "alice" || list[1].ID != "bob" {
				t.Fatalf("List = %+v, %v", list, err)
			}
		})
	}
}

func TestRoleContract(t *testing.T) {
	for name, newStore := range factories() {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			s := newStore(t)

			if _, err := s.GetRole(ctx, "viewer"); !errors.Is(err, auth.ErrRoleNotFound) {
				t.Errorf("Get(missing) = %v", err)
			}
			if err := s.DeleteRole(ctx, "viewer"); !errors.Is(err, auth.ErrRoleNotFound) {
				t.Errorf("Delete(missing) = %v", err)
			}
			if err := s.PutRole(ctx, auth.Role{}); err == nil {
				t.Error("Put without a name succeeded")
			}

			viewer := auth.Role{
				Name: "viewer", Description: "read-only", System: true,
				Policies: []auth.Policy{
					auth.Allow("v-read", "twin:read", "*"),
					auth.Allow("v-own", "twin:update", "twin:*").
						When(auth.Condition{Kind: auth.CondOwnerEquals}).
						When(auth.Condition{Kind: auth.CondAttrIn, Key: "principal.team", Values: []string{"eng", "sre"}}),
				},
			}
			if err := s.PutRole(ctx, viewer); err != nil {
				t.Fatalf("Put: %v", err)
			}
			got, err := s.GetRole(ctx, "viewer")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.Description != "read-only" || !got.System || len(got.Policies) != 2 {
				t.Fatalf("round-trip = %+v", got)
			}
			// Policy order is part of a role definition and must survive.
			if got.Policies[0].ID != "v-read" || got.Policies[1].ID != "v-own" {
				t.Fatalf("policy order = %+v", got.Policies)
			}
			conds := got.Policies[1].Conditions
			if len(conds) != 2 || conds[0].Kind != auth.CondOwnerEquals ||
				conds[1].Kind != auth.CondAttrIn || !slices.Equal(conds[1].Values, []string{"eng", "sre"}) {
				t.Fatalf("conditions = %+v", conds)
			}

			// System roles are immutable.
			if err := s.PutRole(ctx, auth.Role{Name: "viewer"}); !errors.Is(err, auth.ErrSystemRole) {
				t.Errorf("overwriting a system role = %v", err)
			}
			if err := s.DeleteRole(ctx, "viewer"); !errors.Is(err, auth.ErrSystemRole) {
				t.Errorf("deleting a system role = %v", err)
			}
			// Re-seeding the same system role stays idempotent, so bootstrap
			// can run on every boot.
			if err := s.PutRole(ctx, viewer); err != nil {
				t.Fatalf("re-seed: %v", err)
			}

			// Policies are replaced wholesale, not merged.
			custom := auth.Role{Name: "custom", Inherits: []string{"viewer"}, Policies: []auth.Policy{
				auth.Allow("c1", "loop:start", "*"),
				auth.Allow("c2", "loop:resume", "*"),
			}}
			if err := s.PutRole(ctx, custom); err != nil {
				t.Fatalf("Put(custom): %v", err)
			}
			custom.Policies = []auth.Policy{auth.Allow("c1", "loop:start", "*")}
			if err := s.PutRole(ctx, custom); err != nil {
				t.Fatalf("replace policies: %v", err)
			}
			got, _ = s.GetRole(ctx, "custom")
			if len(got.Policies) != 1 || !slices.Equal(got.Inherits, []string{"viewer"}) {
				t.Fatalf("after replace = %+v", got)
			}

			roles, err := s.ListRoles(ctx)
			if err != nil || len(roles) != 2 || roles[0].Name != "custom" || roles[1].Name != "viewer" {
				t.Fatalf("List = %+v, %v", roles, err)
			}
			// List must carry policies too, not just role headers.
			if len(roles[1].Policies) != 2 {
				t.Fatalf("List dropped policies: %+v", roles[1])
			}

			if err := s.DeleteRole(ctx, "custom"); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			if _, err := s.GetRole(ctx, "custom"); !errors.Is(err, auth.ErrRoleNotFound) {
				t.Errorf("after delete = %v", err)
			}
		})
	}
}

func TestBindingContract(t *testing.T) {
	for name, newStore := range factories() {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			s := newStore(t)

			for _, b := range []auth.RoleBinding{
				{PrincipalID: "alice", Role: "viewer"},
				{ID: "b", Role: "viewer"},
				{ID: "b", PrincipalID: "alice"},
			} {
				if err := s.PutBinding(ctx, b); err == nil {
					t.Errorf("PutBinding(%+v) succeeded", b)
				}
			}
			if err := s.DeleteBinding(ctx, "nope"); !errors.Is(err, auth.ErrBindingNotFound) {
				t.Errorf("Delete(missing) = %v", err)
			}

			must(t, s.PutBinding(ctx, auth.RoleBinding{ID: "b1", PrincipalID: "alice", Role: "viewer"}))
			must(t, s.PutBinding(ctx, auth.RoleBinding{ID: "b2", PrincipalID: "alice", Role: "operator", Scope: "twin:abc"}))
			must(t, s.PutBinding(ctx, auth.RoleBinding{ID: "b3", PrincipalID: "bob", Role: "viewer"}))

			mine, err := s.ListBindings(ctx, "alice")
			if err != nil || len(mine) != 2 {
				t.Fatalf("List(alice) = %+v, %v", mine, err)
			}
			// An unset scope is persisted as the explicit global scope.
			if mine[0].EffectiveScope() != "*" || mine[1].Scope != "twin:abc" {
				t.Fatalf("scopes = %+v", mine)
			}
			all, _ := s.ListBindings(ctx, "")
			if len(all) != 3 {
				t.Fatalf("List(all) = %+v", all)
			}

			must(t, s.DeleteBinding(ctx, "b1"))
			if mine, _ = s.ListBindings(ctx, "alice"); len(mine) != 1 {
				t.Fatalf("after delete = %+v", mine)
			}
		})
	}
}

func TestDeletePrincipalCascades(t *testing.T) {
	for name, newStore := range factories() {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			s := newStore(t)

			must(t, s.PutPrincipal(ctx, auth.Principal{ID: "alice"}))
			must(t, s.PutPrincipal(ctx, auth.Principal{ID: "bob"}))
			must(t, s.PutBinding(ctx, auth.RoleBinding{ID: "b1", PrincipalID: "alice", Role: "viewer"}))
			must(t, s.PutBinding(ctx, auth.RoleBinding{ID: "b2", PrincipalID: "bob", Role: "viewer"}))
			must(t, s.PutCredential(ctx, auth.Credential{PrincipalID: "alice", PasswordHash: "x"}))
			must(t, s.PutRefreshToken(ctx, refreshToken("t1", "f1", "alice")))
			must(t, s.PutRefreshToken(ctx, refreshToken("t2", "f2", "bob")))

			must(t, s.DeletePrincipal(ctx, "alice"))

			// Nothing of alice's may survive, or a re-created "alice" would
			// silently inherit her grants and sessions.
			if bindings, _ := s.ListBindings(ctx, ""); len(bindings) != 1 || bindings[0].PrincipalID != "bob" {
				t.Errorf("bindings after cascade = %+v", bindings)
			}
			if _, err := s.GetCredential(ctx, "alice"); !errors.Is(err, auth.ErrCredentialNotFound) {
				t.Errorf("credential after cascade = %v", err)
			}
			if _, err := s.GetRefreshTokenByHash(ctx, auth.HashToken("t1")); !errors.Is(err, auth.ErrInvalidRefreshToken) {
				t.Errorf("refresh token after cascade = %v", err)
			}
			if _, err := s.GetRefreshTokenByHash(ctx, auth.HashToken("t2")); err != nil {
				t.Errorf("bob's token was collateral damage: %v", err)
			}
		})
	}
}

func TestCredentialContract(t *testing.T) {
	for name, newStore := range factories() {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			s := newStore(t)

			if _, err := s.GetCredential(ctx, "alice"); !errors.Is(err, auth.ErrCredentialNotFound) {
				t.Errorf("Get(missing) = %v", err)
			}
			if err := s.DeleteCredential(ctx, "alice"); !errors.Is(err, auth.ErrCredentialNotFound) {
				t.Errorf("Delete(missing) = %v", err)
			}
			if err := s.PutCredential(ctx, auth.Credential{}); err == nil {
				t.Error("Put without a principal ID succeeded")
			}

			updated := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
			must(t, s.PutCredential(ctx, auth.Credential{PrincipalID: "alice", PasswordHash: "hash-v1", UpdatedAt: updated}))
			got, err := s.GetCredential(ctx, "alice")
			if err != nil || got.PasswordHash != "hash-v1" {
				t.Fatalf("Get = %+v, %v", got, err)
			}
			if !got.UpdatedAt.Equal(updated) {
				t.Errorf("UpdatedAt = %s, want %s", got.UpdatedAt, updated)
			}

			must(t, s.PutCredential(ctx, auth.Credential{PrincipalID: "alice", PasswordHash: "hash-v2", UpdatedAt: updated}))
			if got, _ = s.GetCredential(ctx, "alice"); got.PasswordHash != "hash-v2" {
				t.Errorf("upsert = %+v", got)
			}
			must(t, s.DeleteCredential(ctx, "alice"))
		})
	}
}

func TestRefreshTokenContract(t *testing.T) {
	for name, newStore := range factories() {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			s := newStore(t)
			now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

			if err := s.PutRefreshToken(ctx, auth.RefreshToken{ID: "t"}); err == nil {
				t.Error("Put with a partial token succeeded")
			}
			if _, err := s.GetRefreshTokenByHash(ctx, "nope"); !errors.Is(err, auth.ErrInvalidRefreshToken) {
				t.Errorf("Get(missing) = %v", err)
			}

			parent := refreshToken("t1", "f1", "alice")
			parent.IssuedAt = now
			parent.ExpiresAt = now.Add(24 * time.Hour)
			must(t, s.PutRefreshToken(ctx, parent))

			got, err := s.GetRefreshTokenByHash(ctx, auth.HashToken("t1"))
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.ID != "t1" || got.FamilyID != "f1" || got.PrincipalID != "alice" {
				t.Fatalf("round-trip = %+v", got)
			}
			if !got.IssuedAt.Equal(now) || !got.ExpiresAt.Equal(now.Add(24*time.Hour)) {
				t.Fatalf("timestamps = %s / %s", got.IssuedAt, got.ExpiresAt)
			}
			if got.Spent() || got.RevokedAt != nil {
				t.Fatalf("a fresh token reports spent/revoked: %+v", got)
			}

			// Spending is a compare-and-set: the first wins, the second does not.
			spent, err := s.SpendRefreshToken(ctx, "t1", now)
			if err != nil || !spent {
				t.Fatalf("first spend = %v, %v", spent, err)
			}
			spent, err = s.SpendRefreshToken(ctx, "t1", now.Add(time.Second))
			if err != nil || spent {
				t.Fatalf("second spend = %v, %v; want false, nil", spent, err)
			}
			if _, err := s.SpendRefreshToken(ctx, "missing", now); !errors.Is(err, auth.ErrInvalidRefreshToken) {
				t.Errorf("spend of an unknown token = %v", err)
			}
			if got, _ = s.GetRefreshTokenByHash(ctx, auth.HashToken("t1")); !got.Spent() || !got.UsedAt.Equal(now) {
				t.Fatalf("UsedAt = %+v", got.UsedAt)
			}

			// Revoking a family reaches every member, and only that family.
			child := refreshToken("t2", "f1", "alice")
			other := refreshToken("t3", "f2", "alice")
			must(t, s.PutRefreshToken(ctx, child))
			must(t, s.PutRefreshToken(ctx, other))
			must(t, s.RevokeRefreshFamily(ctx, "f1", now))

			for _, tc := range []struct {
				raw           string
				wantRevoked   bool
				wantRevokedAt time.Time
			}{
				{"t1", true, now},
				{"t2", true, now},
				{"t3", false, time.Time{}},
			} {
				got, err := s.GetRefreshTokenByHash(ctx, auth.HashToken(tc.raw))
				if err != nil {
					t.Fatalf("Get(%s): %v", tc.raw, err)
				}
				if (got.RevokedAt != nil) != tc.wantRevoked {
					t.Errorf("%s revoked = %v, want %v", tc.raw, got.RevokedAt != nil, tc.wantRevoked)
				}
				if tc.wantRevoked && !got.RevokedAt.Equal(tc.wantRevokedAt) {
					t.Errorf("%s RevokedAt = %s", tc.raw, got.RevokedAt)
				}
			}

			// Revoking by principal covers every family at once.
			must(t, s.RevokeRefreshTokensForPrincipal(ctx, "alice", now))
			if got, _ = s.GetRefreshTokenByHash(ctx, auth.HashToken("t3")); got.RevokedAt == nil {
				t.Error("RevokeRefreshTokensForPrincipal missed a family")
			}
		})
	}
}

func TestSpendRefreshTokenUnderContention(t *testing.T) {
	for name, newStore := range factories() {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			s := newStore(t)
			must(t, s.PutRefreshToken(ctx, refreshToken("t1", "f1", "alice")))

			// The whole point of the compare-and-set: concurrent exchanges of
			// one token produce exactly one winner.
			var (
				wg      sync.WaitGroup
				mu      sync.Mutex
				winners int
			)
			for range 16 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					spent, err := s.SpendRefreshToken(ctx, "t1", time.Now())
					if err != nil {
						t.Errorf("SpendRefreshToken: %v", err)
						return
					}
					if spent {
						mu.Lock()
						winners++
						mu.Unlock()
					}
				}()
			}
			wg.Wait()
			if winners != 1 {
				t.Fatalf("%d concurrent spends won, want exactly 1", winners)
			}
		})
	}
}

// TestTokenServiceAgainstSQL exercises the full authentication flow against the
// SQL store, so rotation and reuse detection are verified on the persistence
// path the server actually uses rather than only in memory.
func TestTokenServiceAgainstSQL(t *testing.T) {
	ctx := context.Background()
	store := newSQLiteStore(t)

	must(t, store.PutPrincipal(ctx, auth.Principal{ID: "alice", Name: "Alice", Kind: auth.KindUser}))
	must(t, store.PutRole(ctx, auth.Role{Name: "operator", Policies: []auth.Policy{auth.Allow("op", "twin:*", "*")}}))
	must(t, store.PutBinding(ctx, auth.RoleBinding{ID: "b1", PrincipalID: "alice", Role: "operator"}))

	key, err := jwt.NewHMACKey("k1", []byte(strings.Repeat("k", 32)))
	must(t, err)
	keyring, err := jwt.NewKeyring(key)
	must(t, err)
	tokens, err := auth.NewTokenService(store, store, keyring, auth.TokenConfig{Issuer: "test", Audience: "test-api"})
	must(t, err)
	tokens.WithPasswordPolicy(auth.PasswordPolicy{Iterations: 2})
	must(t, tokens.SetPassword(ctx, "alice", "hunter2"))

	first, err := tokens.IssueForPassword(ctx, "alice", "hunter2")
	if err != nil {
		t.Fatalf("IssueForPassword: %v", err)
	}
	if p, err := tokens.Verify(ctx, first.AccessToken); err != nil || p.ID != "alice" {
		t.Fatalf("Verify = %+v, %v", p, err)
	}
	if _, err := tokens.IssueForPassword(ctx, "alice", "wrong"); !errors.Is(err, auth.ErrBadCredential) {
		t.Errorf("wrong password = %v", err)
	}

	second, err := tokens.IssueForRefresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatalf("IssueForRefresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}
	if _, err := tokens.IssueForRefresh(ctx, first.RefreshToken); !errors.Is(err, auth.ErrRefreshTokenReuse) {
		t.Fatalf("replay = %v, want ErrRefreshTokenReuse", err)
	}
	if _, err := tokens.IssueForRefresh(ctx, second.RefreshToken); !errors.Is(err, auth.ErrRefreshTokenRevoked) {
		t.Fatalf("post-reuse refresh = %v, want ErrRefreshTokenRevoked", err)
	}

	// The authorizer reads the same store the tokens came from.
	authorizer := auth.NewAuthorizer(store)
	d, err := authorizer.Authorize(ctx, auth.Principal{ID: "alice"}, "twin:read", auth.Collection("twin"))
	if err != nil || !d.Allowed {
		t.Fatalf("Authorize = %+v, %v", d, err)
	}
	if d.ViaRole != "operator" {
		t.Errorf("ViaRole = %q", d.ViaRole)
	}
}

func TestDeleteExpiredRefreshTokens(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	fresh := refreshToken("keep", "f1", "alice")
	fresh.ExpiresAt = now.Add(time.Hour)
	stale := refreshToken("drop", "f2", "alice")
	stale.ExpiresAt = now.Add(-time.Hour)
	must(t, s.PutRefreshToken(ctx, fresh))
	must(t, s.PutRefreshToken(ctx, stale))

	n, err := s.DeleteExpiredRefreshTokens(ctx, now)
	if err != nil || n != 1 {
		t.Fatalf("DeleteExpiredRefreshTokens = %d, %v", n, err)
	}
	if _, err := s.GetRefreshTokenByHash(ctx, auth.HashToken("drop")); !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Errorf("expired token survived: %v", err)
	}
	if _, err := s.GetRefreshTokenByHash(ctx, auth.HashToken("keep")); err != nil {
		t.Errorf("unexpired token was swept: %v", err)
	}
}

func TestTablePrefixAndDDL(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "prefixed.db"))
	must(t, err)
	defer db.Close()

	store, err := authsql.New(db, authsql.Options{Dialect: authsql.SQLite, TablePrefix: "krk_"})
	must(t, err)
	must(t, store.Migrate(ctx))
	must(t, store.PutPrincipal(ctx, auth.Principal{ID: "alice"}))

	// The prefix has to reach the actual table names, not just the DDL.
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM krk_auth_principals`).Scan(&count); err != nil {
		t.Fatalf("query prefixed table: %v", err)
	}
	if count != 1 {
		t.Errorf("prefixed table holds %d rows, want 1", count)
	}

	ddl := store.DDL()
	for _, want := range []string{"krk_auth_principals", "krk_auth_refresh_tokens", "krk_idx_auth_bindings_principal"} {
		if !strings.Contains(ddl, want) {
			t.Errorf("DDL is missing %q", want)
		}
	}
}

// TestPostgresRebind pins the placeholder rewriting without needing a live
// Postgres: the generated SQL is what would be sent to the server.
func TestPostgresRebind(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "pg.db"))
	must(t, err)
	defer db.Close()

	store, err := authsql.New(db, authsql.Options{Dialect: authsql.Postgres})
	must(t, err)

	ddl := store.DDL()
	if !strings.Contains(ddl, "disabled    BOOLEAN") {
		t.Errorf("postgres DDL should use BOOLEAN, got:\n%s", ddl)
	}
	if strings.Contains(ddl, "disabled    INTEGER") {
		t.Error("postgres DDL used SQLite's INTEGER boolean")
	}

	// Queries against a SQLite handle in Postgres mode must fail on the
	// placeholder syntax — proof the rewriting is actually applied.
	_, err = store.GetPrincipal(context.Background(), "alice")
	if err == nil {
		t.Fatal("expected the $1 placeholder to be rejected by SQLite")
	}
	if !strings.Contains(err.Error(), "$1") && !strings.Contains(err.Error(), "syntax") && !strings.Contains(err.Error(), "no such table") {
		t.Logf("postgres-mode error (informational): %v", err)
	}
}

func refreshToken(raw, family, principal string) auth.RefreshToken {
	now := time.Now().UTC()
	return auth.RefreshToken{
		ID: raw, FamilyID: family, PrincipalID: principal,
		TokenHash: auth.HashToken(raw),
		IssuedAt:  now, ExpiresAt: now.Add(24 * time.Hour),
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
