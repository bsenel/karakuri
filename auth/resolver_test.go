package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bsenel/karakuri/auth/jwt"
)

func TestBearerToken(t *testing.T) {
	cases := []struct {
		header string
		want   string
		errIs  error
	}{
		{"Bearer abc123", "abc123", nil},
		{"bearer abc123", "abc123", nil}, // scheme is case-insensitive per RFC 7235
		{"Bearer   abc123  ", "abc123", nil},
		{"", "", ErrNoCredential},
		{"abc123", "", ErrMalformedCredential},
		{"Basic dXNlcjpwdw==", "", ErrMalformedCredential},
		{"Bearer ", "", ErrMalformedCredential},
		{"Bearer    ", "", ErrMalformedCredential},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		if c.header != "" {
			r.Header.Set("Authorization", c.header)
		}
		got, err := BearerToken(r)
		if c.errIs != nil {
			if !errors.Is(err, c.errIs) {
				t.Errorf("BearerToken(%q) err = %v, want %v", c.header, err, c.errIs)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("BearerToken(%q) = %q, %v", c.header, got, err)
		}
	}
}

func TestJWTResolver(t *testing.T) {
	ctx := context.Background()
	_, svc, _ := tokenFixture(t)
	pair, err := svc.IssueForPassword(ctx, "alice", "hunter2")
	if err != nil {
		t.Fatalf("IssueForPassword: %v", err)
	}
	resolver := NewJWTResolver(svc, "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/twins", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	p, err := resolver.Resolve(req)
	if err != nil || p.ID != "alice" {
		t.Fatalf("Resolve = %+v, %v", p, err)
	}

	// No credential at all.
	if _, err := resolver.Resolve(httptest.NewRequest(http.MethodGet, "/api/v1/twins", nil)); !errors.Is(err, ErrNoCredential) {
		t.Errorf("missing header = %v", err)
	}
	// A credential that is present but not ours.
	bad := httptest.NewRequest(http.MethodGet, "/api/v1/twins", nil)
	bad.Header.Set("Authorization", "Bearer garbage")
	if _, err := resolver.Resolve(bad); !errors.Is(err, jwt.ErrMalformedToken) {
		t.Errorf("bad token = %v", err)
	}
}

func TestJWTResolverQueryParamFallback(t *testing.T) {
	ctx := context.Background()
	_, svc, _ := tokenFixture(t)
	pair, _ := svc.IssueForPassword(ctx, "alice", "hunter2")
	// The query fallback is opt-in: the default constructor does not enable it.
	resolver := &JWTResolver{Tokens: svc, AllowQueryParam: SSEQueryParamPolicy}

	// Where a cookie genuinely cannot work, SSE endpoints may accept the token
	// in the query string.
	sse := httptest.NewRequest(http.MethodGet, "/api/v1/objectives/o1/events?access_token="+pair.AccessToken, nil)
	if p, err := resolver.Resolve(sse); err != nil || p.ID != "alice" {
		t.Fatalf("SSE query token = %+v, %v", p, err)
	}

	// An Accept header asking for a stream qualifies too.
	accept := httptest.NewRequest(http.MethodGet, "/api/v1/stream?access_token="+pair.AccessToken, nil)
	accept.Header.Set("Accept", "text/event-stream")
	if p, err := resolver.Resolve(accept); err != nil || p.ID != "alice" {
		t.Fatalf("Accept-based fallback = %+v, %v", p, err)
	}

	// Ordinary endpoints do not — query strings land in access logs, so the
	// fallback stays scoped to the one case that cannot avoid it.
	rest := httptest.NewRequest(http.MethodGet, "/api/v1/twins?access_token="+pair.AccessToken, nil)
	if _, err := resolver.Resolve(rest); !errors.Is(err, ErrNoCredential) {
		t.Errorf("REST endpoint accepted a query token: %v", err)
	}

	// Eligible path, but no token supplied.
	empty := httptest.NewRequest(http.MethodGet, "/api/v1/objectives/o1/events", nil)
	if _, err := resolver.Resolve(empty); !errors.Is(err, ErrNoCredential) {
		t.Errorf("empty query token = %v", err)
	}

	// A malformed header is not a missing one: it must not silently fall back
	// to the query string.
	header := httptest.NewRequest(http.MethodGet, "/api/v1/objectives/o1/events?access_token="+pair.AccessToken, nil)
	header.Header.Set("Authorization", "Basic dXNlcjpwdw==")
	if _, err := resolver.Resolve(header); !errors.Is(err, ErrMalformedCredential) {
		t.Errorf("malformed header with a valid query token = %v, want ErrMalformedCredential", err)
	}

	// The default constructor disables the fallback entirely.
	strict := NewJWTResolver(svc, "")
	if _, err := strict.Resolve(sse); !errors.Is(err, ErrNoCredential) {
		t.Errorf("header-only resolver accepted a query token: %v", err)
	}
}

func TestJWTResolverInAuthenticateMiddleware(t *testing.T) {
	ctx := context.Background()
	store, svc, _ := tokenFixture(t)
	pair, _ := svc.IssueForPassword(ctx, "alice", "hunter2")

	authz := NewAuthorizer(store)
	h := Authenticate(NewJWTResolver(svc, ""))(RequirePermission(authz, "twin:read", nil)(okHandler()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/twins", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated + authorized = %d, want 200", rec.Code)
	}

	// Same token, an action the operator role does not grant.
	denied := Authenticate(NewJWTResolver(svc, ""))(RequirePermission(authz, "audit:read", nil)(okHandler()))
	req = httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec = httptest.NewRecorder()
	denied.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unauthorized action = %d, want 403", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/twins", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d, want 401", rec.Code)
	}
}

func TestJWTResolverCookie(t *testing.T) {
	ctx := context.Background()
	_, svc, _ := tokenFixture(t)
	pair, _ := svc.IssueForPassword(ctx, "alice", "hunter2")
	resolver := NewJWTResolver(svc, "karakuri_access")

	// A browser sends the access token as an httpOnly cookie — unreadable by
	// injected script, and attached to EventSource automatically.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/twins", nil)
	req.AddCookie(&http.Cookie{Name: "karakuri_access", Value: pair.AccessToken})
	if p, err := resolver.Resolve(req); err != nil || p.ID != "alice" {
		t.Fatalf("cookie auth = %+v, %v", p, err)
	}

	// The header still wins when both are present, so an API client is never
	// silently authenticated as whoever the browser session belongs to.
	other, _ := svc.IssueForPrincipal(ctx, "alice")
	both := httptest.NewRequest(http.MethodGet, "/api/v1/twins", nil)
	both.AddCookie(&http.Cookie{Name: "karakuri_access", Value: "not-a-jwt"})
	both.Header.Set("Authorization", "Bearer "+other.AccessToken)
	if p, err := resolver.Resolve(both); err != nil || p.ID != "alice" {
		t.Fatalf("header should win over cookie: %+v, %v", p, err)
	}

	// A malformed header is still a client bug, not a reason to fall through.
	malformed := httptest.NewRequest(http.MethodGet, "/api/v1/twins", nil)
	malformed.AddCookie(&http.Cookie{Name: "karakuri_access", Value: pair.AccessToken})
	malformed.Header.Set("Authorization", "Basic dXNlcjpwdw==")
	if _, err := resolver.Resolve(malformed); !errors.Is(err, ErrMalformedCredential) {
		t.Errorf("malformed header with a valid cookie = %v", err)
	}

	// A wrong cookie name is no credential at all.
	wrong := httptest.NewRequest(http.MethodGet, "/api/v1/twins", nil)
	wrong.AddCookie(&http.Cookie{Name: "something_else", Value: pair.AccessToken})
	if _, err := resolver.Resolve(wrong); !errors.Is(err, ErrNoCredential) {
		t.Errorf("unrelated cookie = %v", err)
	}

	// An empty CookieName disables the source entirely.
	off := NewJWTResolver(svc, "")
	if _, err := off.Resolve(req); !errors.Is(err, ErrNoCredential) {
		t.Errorf("resolver with no cookie name accepted one: %v", err)
	}
}

func TestCookieConfigSession(t *testing.T) {
	ctx := context.Background()
	_, svc, _ := tokenFixture(t)
	pair, _ := svc.IssueForPassword(ctx, "alice", "hunter2")

	cfg := CookieConfig{
		AccessName: "karakuri_access", RefreshName: "karakuri_refresh",
		AccessPath: "/api/v1", RefreshPath: "/api/v1/auth",
		AccessTTL: 15 * time.Minute, RefreshTTL: 720 * time.Hour,
	}

	rec := httptest.NewRecorder()
	cfg.SetSession(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", nil), pair)

	byName := map[string]*http.Cookie{}
	for _, c := range rec.Result().Cookies() {
		byName[c.Name] = c
	}
	if len(byName) != 2 {
		t.Fatalf("set %d cookies, want 2", len(byName))
	}

	access, refresh := byName["karakuri_access"], byName["karakuri_refresh"]
	if access.Value != pair.AccessToken || refresh.Value != pair.RefreshToken {
		t.Fatal("cookie values do not carry the issued tokens")
	}
	for _, c := range []*http.Cookie{access, refresh} {
		// The properties that make this safer than localStorage: unreadable by
		// script, and never sent cross-site.
		if !c.HttpOnly {
			t.Errorf("%s is not HttpOnly — script could read it", c.Name)
		}
		if c.SameSite != http.SameSiteStrictMode {
			t.Errorf("%s SameSite = %v, want Strict (this is the CSRF defence)", c.Name, c.SameSite)
		}
	}
	// The refresh token is scoped to the endpoints that spend it, so ordinary
	// API calls never carry the long-lived credential.
	if refresh.Path != "/api/v1/auth" {
		t.Errorf("refresh cookie path = %q", refresh.Path)
	}
	if access.MaxAge != 900 || refresh.MaxAge != 720*3600 {
		t.Errorf("max-age = %d / %d", access.MaxAge, refresh.MaxAge)
	}

	// Secure by default, including on a plain-HTTP request: an unencrypted
	// session cookie is one an attacker can read off the wire, so the default
	// must not depend on how this particular request happened to arrive.
	for _, c := range []*http.Cookie{access, refresh} {
		if !c.Secure {
			t.Errorf("%s is not Secure", c.Name)
		}
	}
	tlsReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", nil)
	tlsReq.Header.Set("X-Forwarded-Proto", "https")
	rec = httptest.NewRecorder()
	cfg.SetSession(rec, tlsReq, pair)
	for _, c := range rec.Result().Cookies() {
		if !c.Secure {
			t.Errorf("%s not Secure behind an HTTPS-terminating proxy", c.Name)
		}
	}

	// The one escape hatch, and only where it is asked for: plain-HTTP local
	// development. Behind TLS the flag changes nothing.
	dev := cfg
	dev.InsecureAllowHTTP = true
	for _, tc := range []struct {
		name       string
		req        *http.Request
		wantSecure bool
	}{
		{"plain HTTP", httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", nil), false},
		{"forwarded HTTPS", tlsReq, true},
	} {
		rec = httptest.NewRecorder()
		dev.SetSession(rec, tc.req, pair)
		for _, c := range rec.Result().Cookies() {
			if c.Secure != tc.wantSecure {
				t.Errorf("InsecureAllowHTTP/%s: %s Secure = %t, want %t", tc.name, c.Name, c.Secure, tc.wantSecure)
			}
		}
		rec = httptest.NewRecorder()
		dev.ClearSession(rec, tc.req)
		for _, c := range rec.Result().Cookies() {
			if c.Secure != tc.wantSecure {
				t.Errorf("InsecureAllowHTTP/%s: cleared %s Secure = %t, want %t", tc.name, c.Name, c.Secure, tc.wantSecure)
			}
		}
	}

	// Reading the refresh token back.
	read := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	read.AddCookie(&http.Cookie{Name: "karakuri_refresh", Value: pair.RefreshToken})
	if got := cfg.Refresh(read); got != pair.RefreshToken {
		t.Errorf("Refresh() = %q", got)
	}
	if got := cfg.Refresh(httptest.NewRequest(http.MethodPost, "/x", nil)); got != "" {
		t.Errorf("Refresh() with no cookie = %q", got)
	}
	if got := (CookieConfig{}).Refresh(read); got != "" {
		t.Errorf("Refresh() with no configured name = %q", got)
	}

	// Clearing expires both.
	rec = httptest.NewRecorder()
	cfg.ClearSession(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/revoke", nil))
	cleared := rec.Result().Cookies()
	if len(cleared) != 2 {
		t.Fatalf("cleared %d cookies, want 2", len(cleared))
	}
	for _, c := range cleared {
		if c.MaxAge >= 0 || c.Value != "" {
			t.Errorf("%s not expired: value=%q max-age=%d", c.Name, c.Value, c.MaxAge)
		}
		if !c.Secure || !c.HttpOnly {
			t.Errorf("%s cleared with weaker attributes than it was set with", c.Name)
		}
	}
}
