package oidc_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc/oidctest"
)

// stubIdP is an OpenID Connect provider small enough to reason about.
//
// go-oidc ships oidctest, which serves discovery and JWKS and signs tokens, but
// no token endpoint — it was built for verification tests, not flow tests. This
// wraps it with the /auth and /token halves so the authorization-code flow can
// be driven end to end without Docker, and so key rotation can be forced rather
// than waited for. The Keycloak job in CI covers what a real provider does
// differently; this covers what this package does.
type stubIdP struct {
	Server *httptest.Server

	mu       sync.Mutex
	key      *rsa.PrivateKey
	keyID    string
	inner    *oidctest.Server
	codes    map[string]stubCode
	clientID string

	// TokenResponse, when set, replaces the token endpoint's body. It is how
	// a response with no id_token is exercised.
	TokenResponse func() (int, string)

	// TamperNonce issues the ID token with a nonce the login flow never asked
	// for — what a replayed token from a different session looks like.
	TamperNonce bool
}

type stubCode struct {
	nonce     string
	challenge string
	claims    map[string]any
}

func newStubIdP(t *testing.T, clientID string) *stubIdP {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	s := &stubIdP{
		key:      key,
		keyID:    "key-1",
		codes:    map[string]stubCode{},
		clientID: clientID,
	}
	s.inner = &oidctest.Server{
		PublicKeys: []oidctest.PublicKey{{PublicKey: key.Public(), KeyID: s.keyID, Algorithm: "RS256"}},
		Algorithms: []string{"RS256"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", s.serveInner)
	mux.HandleFunc("/keys", s.serveInner)
	mux.HandleFunc("/auth", s.serveAuth)
	mux.HandleFunc("/token", s.serveToken)

	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Server.Close)
	s.inner.SetIssuer(s.Server.URL)
	return s
}

func (s *stubIdP) serveInner(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	inner := s.inner
	s.mu.Unlock()
	inner.ServeHTTP(w, r)
}

// Rotate replaces the signing key, as a provider does periodically. Tokens
// signed after it carry an unknown key ID, which is what forces the cached JWKS
// to refetch.
func (s *stubIdP) Rotate(t *testing.T) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.key, s.keyID = key, s.keyID+"-rotated"
	s.inner = &oidctest.Server{
		PublicKeys: []oidctest.PublicKey{{PublicKey: key.Public(), KeyID: s.keyID, Algorithm: "RS256"}},
		Algorithms: []string{"RS256"},
	}
	s.inner.SetIssuer(s.Server.URL)
}

// Claims returns a well-formed claim set for a user, which a test then bends.
func (s *stubIdP) Claims(subject string, groups ...string) map[string]any {
	now := time.Now()
	raw := make([]any, len(groups))
	for i, g := range groups {
		raw[i] = g
	}
	return map[string]any{
		"iss":    s.Server.URL,
		"aud":    s.clientID,
		"sub":    subject,
		"exp":    now.Add(time.Hour).Unix(),
		"iat":    now.Unix(),
		"email":  subject + "@example.com",
		"name":   "User " + subject,
		"groups": raw,
	}
}

// SignToken signs a claim set with the current key.
func (s *stubIdP) SignToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	body, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	s.mu.Lock()
	key, keyID := s.key, s.keyID
	s.mu.Unlock()
	return oidctest.SignIDToken(key, keyID, "RS256", string(body))
}

// serveAuth is the authorization endpoint. It does not render a login page —
// the user is always already authenticated — and immediately redirects back
// with a code.
func (s *stubIdP) serveAuth(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirect, err := url.Parse(q.Get("redirect_uri"))
	if err != nil {
		http.Error(w, "bad redirect_uri", http.StatusBadRequest)
		return
	}

	subject := q.Get("login_as")
	if subject == "" {
		subject = "alice"
	}
	code, err := randomHex()
	if err != nil {
		http.Error(w, "no randomness", http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	s.codes[code] = stubCode{
		nonce:     q.Get("nonce"),
		challenge: q.Get("code_challenge"),
		claims:    s.Claims(subject, "karakuri-operators"),
	}
	s.mu.Unlock()

	rq := redirect.Query()
	rq.Set("code", code)
	rq.Set("state", q.Get("state"))
	redirect.RawQuery = rq.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

// serveToken exchanges a code for tokens. It checks the PKCE challenge is
// present, which is enough to prove this package sent one.
func (s *stubIdP) serveToken(w http.ResponseWriter, r *http.Request) {
	if s.TokenResponse != nil {
		status, body := s.TokenResponse()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	issued, ok := s.codes[r.Form.Get("code")]
	delete(s.codes, r.Form.Get("code"))
	s.mu.Unlock()
	if !ok {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	if issued.challenge == "" || r.Form.Get("code_verifier") == "" {
		http.Error(w, `{"error":"invalid_request","error_description":"PKCE missing"}`, http.StatusBadRequest)
		return
	}

	claims := issued.claims
	switch {
	case s.TamperNonce:
		claims["nonce"] = "a-nonce-from-somebody-elses-login"
	case issued.nonce != "":
		claims["nonce"] = issued.nonce
	}
	body, _ := json.Marshal(map[string]any{
		"access_token": "stub-access-token",
		"token_type":   "Bearer",
		"expires_in":   3600,
		"id_token":     s.signWith(claims),
	})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (s *stubIdP) signWith(claims map[string]any) string {
	body, _ := json.Marshal(claims)
	s.mu.Lock()
	key, keyID := s.key, s.keyID
	s.mu.Unlock()
	return oidctest.SignIDToken(key, keyID, "RS256", string(body))
}

func randomHex() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", buf), nil
}
