package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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
	resolver := NewJWTResolver(svc)

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
	resolver := NewJWTResolver(svc)

	// EventSource cannot set headers, so SSE endpoints accept the token in the
	// query string.
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

	// Header-only policy disables the fallback entirely.
	strict := &JWTResolver{Tokens: svc}
	if _, err := strict.Resolve(sse); !errors.Is(err, ErrNoCredential) {
		t.Errorf("header-only resolver accepted a query token: %v", err)
	}
}

func TestJWTResolverInAuthenticateMiddleware(t *testing.T) {
	ctx := context.Background()
	store, svc, _ := tokenFixture(t)
	pair, _ := svc.IssueForPassword(ctx, "alice", "hunter2")

	authz := NewAuthorizer(store)
	h := Authenticate(NewJWTResolver(svc))(RequirePermission(authz, "twin:read", nil)(okHandler()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/twins", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated + authorized = %d, want 200", rec.Code)
	}

	// Same token, an action the operator role does not grant.
	denied := Authenticate(NewJWTResolver(svc))(RequirePermission(authz, "audit:read", nil)(okHandler()))
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
