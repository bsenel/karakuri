package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/auth/jwt"
)

func tokenFixture(t *testing.T) (*MemoryStore, *TokenService, func(time.Time)) {
	t.Helper()
	ctx := context.Background()
	store := NewMemoryStore()
	mustPut(t, store.PutRole(ctx, Role{Name: "operator", Policies: []Policy{Allow("op", "twin:*", "*")}}))
	mustPut(t, store.PutPrincipal(ctx, Principal{ID: "alice", Name: "Alice", Kind: KindUser}))
	mustPut(t, store.PutBinding(ctx, RoleBinding{ID: "b1", PrincipalID: "alice", Role: "operator", Scope: "twin:*"}))

	key, err := jwt.NewHMACKey("k1", []byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatalf("NewHMACKey: %v", err)
	}
	kr, err := jwt.NewKeyring(key)
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	svc, err := NewTokenService(store, store, kr, TokenConfig{
		Issuer:     "karakuri",
		Audience:   "karakuri-api",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewTokenService: %v", err)
	}
	svc.WithPasswordPolicy(testPolicy).WithClock(func() time.Time { return now })

	if err := svc.SetPassword(ctx, "alice", "hunter2"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	return store, svc, func(at time.Time) { now = at }
}

func TestNewTokenServiceValidation(t *testing.T) {
	store := NewMemoryStore()
	key, _ := jwt.NewHMACKey("k1", []byte(strings.Repeat("k", 32)))
	kr, _ := jwt.NewKeyring(key)

	if _, err := NewTokenService(nil, store, kr, TokenConfig{}); err == nil {
		t.Error("expected an error with a nil store")
	}
	if _, err := NewTokenService(store, nil, kr, TokenConfig{}); err == nil {
		t.Error("expected an error with a nil credential store")
	}
	// There is deliberately no insecure default signing key.
	if _, err := NewTokenService(store, store, nil, TokenConfig{}); !errors.Is(err, jwt.ErrNoActiveKey) {
		t.Errorf("nil keyring = %v, want ErrNoActiveKey", err)
	}
	if _, err := NewTokenService(store, store, &jwt.Keyring{}, TokenConfig{}); !errors.Is(err, jwt.ErrNoActiveKey) {
		t.Errorf("empty keyring = %v, want ErrNoActiveKey", err)
	}

	svc, err := NewTokenService(store, store, kr, TokenConfig{})
	if err != nil {
		t.Fatalf("NewTokenService: %v", err)
	}
	if svc.cfg.AccessTTL != 15*time.Minute || svc.cfg.RefreshTTL != 30*24*time.Hour {
		t.Errorf("TTL defaults = %v / %v", svc.cfg.AccessTTL, svc.cfg.RefreshTTL)
	}
}

func TestIssueForPassword(t *testing.T) {
	ctx := context.Background()
	_, svc, _ := tokenFixture(t)

	pair, err := svc.IssueForPassword(ctx, "alice", "hunter2")
	if err != nil {
		t.Fatalf("IssueForPassword: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("empty token in pair")
	}
	if pair.TokenType != "Bearer" || pair.ExpiresIn != 900 {
		t.Errorf("pair = %+v", pair)
	}

	principal, err := svc.Verify(ctx, pair.AccessToken)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if principal.ID != "alice" || principal.Name != "Alice" {
		t.Errorf("verified principal = %+v", principal)
	}

	// Advisory claims carry the bindings so a UI can render without a round trip.
	claims, err := jwt.Parse(pair.AccessToken, svc.keys, jwt.Validation{Now: svc.nowFunc})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "operator" || len(claims.Scopes) != 1 || claims.Scopes[0] != "twin:*" {
		t.Errorf("claims roles/scopes = %v / %v", claims.Roles, claims.Scopes)
	}
	if claims.Type != TokenTypeAccess || claims.Issuer != "karakuri" || claims.Audience != "karakuri-api" {
		t.Errorf("claims = %+v", claims)
	}
	if claims.ID == "" {
		t.Error("access token has no jti")
	}
}

func TestIssueForPasswordRejections(t *testing.T) {
	ctx := context.Background()
	store, svc, _ := tokenFixture(t)
	mustPut(t, store.PutPrincipal(ctx, Principal{ID: "ghost", Disabled: true}))
	mustPut(t, store.PutPrincipal(ctx, Principal{ID: "svc", Kind: KindService}))
	if err := svc.SetPassword(ctx, "ghost", "pw"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	cases := map[string][2]string{
		"wrong password":  {"alice", "nope"},
		"unknown user":    {"nobody", "hunter2"},
		"disabled user":   {"ghost", "pw"},
		"no password set": {"svc", "anything"},
	}
	for name, c := range cases {
		// Every failure returns the same error: a caller must not be able to
		// tell "no such user" from "wrong password".
		if _, err := svc.IssueForPassword(ctx, c[0], c[1]); !errors.Is(err, ErrBadCredential) {
			t.Errorf("%s = %v, want ErrBadCredential", name, err)
		}
	}
}

func TestSetPasswordUnknownPrincipal(t *testing.T) {
	_, svc, _ := tokenFixture(t)
	if err := svc.SetPassword(context.Background(), "nobody", "pw"); !errors.Is(err, ErrPrincipalNotFound) {
		t.Errorf("SetPassword(unknown) = %v", err)
	}
}

func TestIssueForPrincipal(t *testing.T) {
	ctx := context.Background()
	store, svc, _ := tokenFixture(t)
	mustPut(t, store.PutPrincipal(ctx, Principal{ID: "ci", Kind: KindService}))
	mustPut(t, store.PutBinding(ctx, RoleBinding{ID: "b-ci", PrincipalID: "ci", Role: "operator"}))

	// Service accounts hold no password — an admin mints their first token.
	pair, err := svc.IssueForPrincipal(ctx, "ci")
	if err != nil {
		t.Fatalf("IssueForPrincipal: %v", err)
	}
	if p, err := svc.Verify(ctx, pair.AccessToken); err != nil || p.ID != "ci" {
		t.Fatalf("Verify = %+v, %v", p, err)
	}

	if _, err := svc.IssueForPrincipal(ctx, "nobody"); !errors.Is(err, ErrPrincipalNotFound) {
		t.Errorf("unknown principal = %v", err)
	}
	mustPut(t, store.PutPrincipal(ctx, Principal{ID: "off", Disabled: true}))
	if _, err := svc.IssueForPrincipal(ctx, "off"); !errors.Is(err, ErrPrincipalDisabled) {
		t.Errorf("disabled principal = %v", err)
	}
}

func TestRefreshRotation(t *testing.T) {
	ctx := context.Background()
	store, svc, _ := tokenFixture(t)

	first, err := svc.IssueForPassword(ctx, "alice", "hunter2")
	if err != nil {
		t.Fatalf("IssueForPassword: %v", err)
	}
	second, err := svc.IssueForRefresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatalf("IssueForRefresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}
	third, err := svc.IssueForRefresh(ctx, second.RefreshToken)
	if err != nil {
		t.Fatalf("second refresh: %v", err)
	}

	// The chain stays in one family, linked parent to child.
	tokens := store.ListRefreshTokens(ctx, "alice")
	if len(tokens) != 3 {
		t.Fatalf("got %d tokens in the chain, want 3", len(tokens))
	}
	family := tokens[0].FamilyID
	linked := 0
	for _, tok := range tokens {
		if tok.FamilyID != family {
			t.Fatalf("token %s escaped the family", tok.ID)
		}
		if tok.ParentID != "" {
			linked++
		}
	}
	if linked != 2 {
		t.Errorf("%d tokens carry a parent, want 2", linked)
	}
	if _, err := svc.Verify(ctx, third.AccessToken); err != nil {
		t.Errorf("access token from the third pair = %v", err)
	}
}

func TestRefreshReuseRevokesFamily(t *testing.T) {
	ctx := context.Background()
	_, svc, _ := tokenFixture(t)

	first, _ := svc.IssueForPassword(ctx, "alice", "hunter2")
	second, err := svc.IssueForRefresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatalf("IssueForRefresh: %v", err)
	}

	// Replaying a spent token is evidence of a leak: rotation means the
	// legitimate holder never replays.
	if _, err := svc.IssueForRefresh(ctx, first.RefreshToken); !errors.Is(err, ErrRefreshTokenReuse) {
		t.Fatalf("replay = %v, want ErrRefreshTokenReuse", err)
	}
	// …and the whole lineage dies with it, including the token the attacker or
	// the victim is currently holding.
	if _, err := svc.IssueForRefresh(ctx, second.RefreshToken); !errors.Is(err, ErrRefreshTokenRevoked) {
		t.Fatalf("post-reuse refresh = %v, want ErrRefreshTokenRevoked", err)
	}

	// A fresh login starts a new family and is unaffected.
	if _, err := svc.IssueForPassword(ctx, "alice", "hunter2"); err != nil {
		t.Errorf("re-login after reuse = %v", err)
	}
}

func TestRefreshRejections(t *testing.T) {
	ctx := context.Background()
	store, svc, setNow := tokenFixture(t)

	if _, err := svc.IssueForRefresh(ctx, "not-a-token"); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("unknown token = %v", err)
	}

	pair, _ := svc.IssueForPassword(ctx, "alice", "hunter2")
	setNow(time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)) // past the 24h refresh TTL
	if _, err := svc.IssueForRefresh(ctx, pair.RefreshToken); !errors.Is(err, ErrRefreshTokenExpired) {
		t.Errorf("expired token = %v, want ErrRefreshTokenExpired", err)
	}
	setNow(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))

	// A principal disabled after login cannot refresh — the check is at
	// exchange time, not only at login.
	pair, _ = svc.IssueForPassword(ctx, "alice", "hunter2")
	mustPut(t, store.PutPrincipal(ctx, Principal{ID: "alice", Disabled: true}))
	if _, err := svc.IssueForRefresh(ctx, pair.RefreshToken); !errors.Is(err, ErrPrincipalDisabled) {
		t.Errorf("disabled principal = %v, want ErrPrincipalDisabled", err)
	}

	// A refresh token whose principal was deleted outright.
	mustPut(t, store.PutPrincipal(ctx, Principal{ID: "temp"}))
	tmpPair, err := svc.IssueForPrincipal(ctx, "temp")
	if err != nil {
		t.Fatalf("IssueForPrincipal: %v", err)
	}
	mustPut(t, store.PutPrincipal(ctx, Principal{ID: "temp2"}))
	// Delete only the principal record, leaving the token behind.
	store.mu.Lock()
	delete(store.principals, "temp")
	store.mu.Unlock()
	if _, err := svc.IssueForRefresh(ctx, tmpPair.RefreshToken); !errors.Is(err, ErrPrincipalNotFound) {
		t.Errorf("deleted principal = %v", err)
	}
}

func TestRevoke(t *testing.T) {
	ctx := context.Background()
	_, svc, _ := tokenFixture(t)

	pair, _ := svc.IssueForPassword(ctx, "alice", "hunter2")
	if err := svc.Revoke(ctx, pair.RefreshToken); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := svc.IssueForRefresh(ctx, pair.RefreshToken); !errors.Is(err, ErrRefreshTokenRevoked) {
		t.Errorf("revoked token = %v", err)
	}
	if err := svc.Revoke(ctx, "nope"); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("Revoke(unknown) = %v", err)
	}

	// "Log out everywhere" kills every family the principal holds.
	a, _ := svc.IssueForPassword(ctx, "alice", "hunter2")
	b, _ := svc.IssueForPassword(ctx, "alice", "hunter2")
	if err := svc.RevokeAllForPrincipal(ctx, "alice"); err != nil {
		t.Fatalf("RevokeAllForPrincipal: %v", err)
	}
	for i, tok := range []string{a.RefreshToken, b.RefreshToken} {
		if _, err := svc.IssueForRefresh(ctx, tok); !errors.Is(err, ErrRefreshTokenRevoked) {
			t.Errorf("session %d = %v, want ErrRefreshTokenRevoked", i, err)
		}
	}
}

func TestVerifyRejections(t *testing.T) {
	ctx := context.Background()
	store, svc, setNow := tokenFixture(t)
	pair, _ := svc.IssueForPassword(ctx, "alice", "hunter2")

	if _, err := svc.Verify(ctx, "garbage"); !errors.Is(err, jwt.ErrMalformedToken) {
		t.Errorf("garbage = %v", err)
	}
	setNow(time.Date(2026, 6, 1, 13, 0, 0, 0, time.UTC)) // past the 15m access TTL
	if _, err := svc.Verify(ctx, pair.AccessToken); !errors.Is(err, jwt.ErrExpired) {
		t.Errorf("expired access token = %v", err)
	}
	setNow(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))

	// Disabling a principal takes effect on the next request, not whenever its
	// access token happens to expire — Verify reloads from the store.
	mustPut(t, store.PutPrincipal(ctx, Principal{ID: "alice", Disabled: true}))
	if _, err := svc.Verify(ctx, pair.AccessToken); !errors.Is(err, ErrPrincipalDisabled) {
		t.Errorf("disabled principal = %v, want ErrPrincipalDisabled", err)
	}
	store.mu.Lock()
	delete(store.principals, "alice")
	store.mu.Unlock()
	if _, err := svc.Verify(ctx, pair.AccessToken); !errors.Is(err, ErrPrincipalNotFound) {
		t.Errorf("deleted principal = %v", err)
	}
}

func TestTokenServiceUsesRealClockByDefault(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	mustPut(t, store.PutPrincipal(ctx, Principal{ID: "alice"}))
	key, _ := jwt.NewHMACKey("k1", []byte(strings.Repeat("k", 32)))
	kr, _ := jwt.NewKeyring(key)
	svc, err := NewTokenService(store, store, kr, TokenConfig{})
	if err != nil {
		t.Fatalf("NewTokenService: %v", err)
	}
	pair, err := svc.IssueForPrincipal(ctx, "alice")
	if err != nil {
		t.Fatalf("IssueForPrincipal: %v", err)
	}
	if time.Until(pair.ExpiresAt) <= 0 {
		t.Fatalf("ExpiresAt = %s, expected it in the future", pair.ExpiresAt)
	}
	if _, err := svc.Verify(ctx, pair.AccessToken); err != nil {
		t.Errorf("Verify: %v", err)
	}
}

func TestConcurrentRefreshSpendsOnce(t *testing.T) {
	ctx := context.Background()
	_, svc, _ := tokenFixture(t)
	pair, _ := svc.IssueForPassword(ctx, "alice", "hunter2")

	// Eight clients racing on the same refresh token. Spending is a
	// compare-and-set, so exactly one wins and the rest are treated as replays.
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		okCount int
		errs    []error
	)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.IssueForRefresh(ctx, pair.RefreshToken)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				okCount++
			} else {
				errs = append(errs, err)
			}
		}()
	}
	wg.Wait()

	if okCount != 1 {
		t.Fatalf("%d concurrent refreshes succeeded, want exactly 1 (errors: %v)", okCount, errs)
	}
	for _, err := range errs {
		if !errors.Is(err, ErrRefreshTokenReuse) && !errors.Is(err, ErrRefreshTokenRevoked) {
			t.Errorf("unexpected concurrent error: %v", err)
		}
	}
}

func TestSpendRefreshTokenIsCompareAndSet(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Now().UTC()
	mustPut(t, store.PutRefreshToken(ctx, RefreshToken{
		ID: "t1", FamilyID: "f1", PrincipalID: "alice", TokenHash: HashToken("raw"),
		IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}))

	if spent, err := store.SpendRefreshToken(ctx, "t1", now); err != nil || !spent {
		t.Fatalf("first spend = %v, %v; want true, nil", spent, err)
	}
	if spent, err := store.SpendRefreshToken(ctx, "t1", now); err != nil || spent {
		t.Fatalf("second spend = %v, %v; want false, nil", spent, err)
	}
	if _, err := store.SpendRefreshToken(ctx, "missing", now); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("spend of an unknown token = %v", err)
	}
}

func TestIssueSurfacesStoreErrors(t *testing.T) {
	ctx := context.Background()
	base, svc, _ := tokenFixture(t)
	boom := errors.New("boom")
	svc.store = failingStore{Store: base, bindingErr: boom}

	if _, err := svc.IssueForPrincipal(ctx, "alice"); !errors.Is(err, boom) {
		t.Errorf("binding listing error = %v, want boom", err)
	}
}
