package oidc_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/auth/oidc"
)

const testClientID = "karakuri"

func newProvisioner(t *testing.T) (*auth.Provisioner, auth.Store) {
	t.Helper()
	store := auth.NewMemoryStore()
	for _, name := range []string{"admin", "operator", "viewer"} {
		if err := store.PutRole(context.Background(), auth.Role{Name: name}); err != nil {
			t.Fatalf("seed role: %v", err)
		}
	}
	return &auth.Provisioner{
		Store:  store,
		Prefix: oidc.DefaultPrefix,
		Roles: auth.RoleMap{Groups: map[string][]auth.RoleGrant{
			"karakuri-operators": {{Role: "operator"}},
			"karakuri-admins":    {{Role: "admin"}},
		}},
	}, store
}

func newProvider(t *testing.T, idp *stubIdP, mutate func(*oidc.Config)) (*oidc.Provider, auth.Store) {
	t.Helper()
	provisioner, store := newProvisioner(t)
	cfg := oidc.Config{
		IssuerURL:         idp.Server.URL,
		ClientID:          testClientID,
		ClientSecret:      "shh",
		RedirectURL:       "https://karakuri.example.com/api/v1/auth/sso/callback",
		StateKey:          []byte("a-32-byte-state-signing-key-here"),
		InsecureAllowHTTP: true,
		HTTPClient:        idp.Server.Client(),
	}
	if mutate != nil {
		mutate(&cfg)
	}
	p, err := oidc.New(context.Background(), cfg, provisioner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p, store
}

func bearer(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/twins", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

func TestNewValidatesConfig(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t, testClientID)
	provisioner, _ := newProvisioner(t)
	good := oidc.Config{IssuerURL: idp.Server.URL, ClientID: testClientID, StateKey: []byte("k"), HTTPClient: idp.Server.Client()}

	cases := []struct {
		name    string
		mutate  func(*oidc.Config)
		wantErr error
	}{
		{name: "no issuer", mutate: func(c *oidc.Config) { c.IssuerURL = "" }, wantErr: oidc.ErrNoIssuer},
		{name: "blank issuer", mutate: func(c *oidc.Config) { c.IssuerURL = "   " }, wantErr: oidc.ErrNoIssuer},
		{name: "no client ID", mutate: func(c *oidc.Config) { c.ClientID = "" }, wantErr: oidc.ErrNoClientID},
		{name: "no state key", mutate: func(c *oidc.Config) { c.StateKey = nil }, wantErr: oidc.ErrNoStateKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := good
			tc.mutate(&cfg)
			if _, err := oidc.New(context.Background(), cfg, provisioner); !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}

	t.Run("no provisioner", func(t *testing.T) {
		t.Parallel()
		if _, err := oidc.New(context.Background(), good, nil); !errors.Is(err, oidc.ErrNoProvisioner) {
			t.Fatalf("err = %v, want ErrNoProvisioner", err)
		}
	})

	// Discovery runs once, here, so a wrong issuer URL is a startup failure
	// rather than a login failure.
	t.Run("undiscoverable issuer", func(t *testing.T) {
		t.Parallel()
		cfg := good
		cfg.IssuerURL = idp.Server.URL + "/nowhere"
		if _, err := oidc.New(context.Background(), cfg, provisioner); err == nil {
			t.Fatal("New with an undiscoverable issuer returned nil")
		}
	})
}

func TestResolveProvisionsFromBearerToken(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t, testClientID)
	p, store := newProvider(t, idp, nil)

	token := idp.SignToken(t, idp.Claims("8f3c", "karakuri-operators"))
	principal, err := p.Resolve(bearer(token))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if principal.ID != "oidc:8f3c" {
		t.Errorf("principal ID = %q, want oidc:8f3c", principal.ID)
	}
	if principal.Name != "User 8f3c" {
		t.Errorf("Name = %q", principal.Name)
	}
	if principal.Attrs["email"] != "8f3c@example.com" {
		t.Errorf("Attrs = %v", principal.Attrs)
	}

	bindings, err := store.ListBindings(context.Background(), "oidc:8f3c")
	if err != nil {
		t.Fatalf("ListBindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].Role != "operator" {
		t.Fatalf("bindings = %+v, want one operator binding", bindings)
	}
}

func TestResolveRejects(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t, testClientID)
	other := newStubIdP(t, testClientID)
	p, _ := newProvider(t, idp, nil)

	t.Run("no credential", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/twins", nil)
		// Must be ErrNoCredential specifically, or a ChainResolver would stop
		// here instead of trying the next resolver.
		if _, err := p.Resolve(r); !errors.Is(err, auth.ErrNoCredential) {
			t.Fatalf("err = %v, want ErrNoCredential", err)
		}
	})

	t.Run("garbage token", func(t *testing.T) {
		t.Parallel()
		if _, err := p.Resolve(bearer("not-a-jwt")); err == nil {
			t.Fatal("a garbage token was accepted")
		}
	})

	t.Run("signed by another issuer", func(t *testing.T) {
		t.Parallel()
		token := other.SignToken(t, other.Claims("mallory", "karakuri-admins"))
		if _, err := p.Resolve(bearer(token)); err == nil {
			t.Fatal("a token from an unrelated issuer was accepted")
		}
	})

	t.Run("wrong audience", func(t *testing.T) {
		t.Parallel()
		claims := idp.Claims("alice")
		claims["aud"] = "some-other-client"
		if _, err := p.Resolve(bearer(idp.SignToken(t, claims))); err == nil {
			t.Fatal("a token minted for another client was accepted")
		}
	})

	t.Run("expired", func(t *testing.T) {
		t.Parallel()
		claims := idp.Claims("alice")
		claims["exp"] = 1
		if _, err := p.Resolve(bearer(idp.SignToken(t, claims))); err == nil {
			t.Fatal("an expired token was accepted")
		}
	})
}

// A user in no mapped group authenticates but holds nothing. That is the
// intended shape: authentication is not authorization, and everybody in a
// corporate directory can authenticate.
func TestResolveUnmappedUserGetsNoRoles(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t, testClientID)
	p, store := newProvider(t, idp, nil)

	token := idp.SignToken(t, idp.Claims("bob", "marketing"))
	principal, err := p.Resolve(bearer(token))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	bindings, err := store.ListBindings(context.Background(), principal.ID)
	if err != nil {
		t.Fatalf("ListBindings: %v", err)
	}
	if len(bindings) != 0 {
		t.Fatalf("bindings = %+v, want none", bindings)
	}
}

// Providers disagree about where groups live; Keycloak nests them.
func TestResolveHonoursClaimPaths(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t, testClientID)
	p, store := newProvider(t, idp, func(c *oidc.Config) {
		c.GroupsClaim = "realm_access.roles"
		c.NameClaim = "preferred_username"
	})

	claims := idp.Claims("keycloak-user")
	delete(claims, "groups")
	claims["realm_access"] = map[string]any{"roles": []any{"karakuri-admins"}}
	claims["preferred_username"] = "kc"

	principal, err := p.Resolve(bearer(idp.SignToken(t, claims)))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if principal.Name != "kc" {
		t.Errorf("Name = %q, want the configured claim", principal.Name)
	}
	bindings, _ := store.ListBindings(context.Background(), principal.ID)
	if len(bindings) != 1 || bindings[0].Role != "admin" {
		t.Fatalf("bindings = %+v, want admin from the nested claim", bindings)
	}
}

// A provider rotating its signing keys must not require a restart.
func TestResolveSurvivesKeyRotation(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t, testClientID)
	p, _ := newProvider(t, idp, nil)

	if _, err := p.Resolve(bearer(idp.SignToken(t, idp.Claims("alice", "karakuri-operators")))); err != nil {
		t.Fatalf("before rotation: %v", err)
	}
	idp.Rotate(t)
	if _, err := p.Resolve(bearer(idp.SignToken(t, idp.Claims("alice", "karakuri-operators")))); err != nil {
		t.Fatalf("after rotation: %v — the key set did not refetch", err)
	}
}

// TestBrowserFlow drives login and callback the way a browser does: following
// redirects and carrying cookies, with a real server on both ends.
//
// The provider is built inside the test rather than by newProvider because
// RedirectURL has to be this server's own callback, which is not known until
// the server exists.
func TestBrowserFlow(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t, testClientID)

	var seen auth.Principal
	provisioner, store := newProvisioner(t)

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	flow, err := oidc.New(context.Background(), oidc.Config{
		IssuerURL:         idp.Server.URL,
		ClientID:          testClientID,
		ClientSecret:      "shh",
		RedirectURL:       server.URL + "/callback",
		StateKey:          []byte("a-32-byte-state-signing-key-here"),
		InsecureAllowHTTP: true,
		HTTPClient:        idp.Server.Client(),
	}, provisioner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mux.Handle("/login", flow.LoginHandler())
	mux.Handle("/callback", flow.CallbackHandler(
		func(w http.ResponseWriter, _ *http.Request, principal auth.Principal) {
			seen = principal
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("welcome"))
		},
		func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusUnauthorized)
		},
	))

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	resp, err := client.Get(server.URL + "/login")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 — the flow did not complete", resp.StatusCode)
	}
	if seen.ID != "oidc:alice" {
		t.Fatalf("principal = %q, want oidc:alice", seen.ID)
	}
	bindings, _ := store.ListBindings(context.Background(), seen.ID)
	if len(bindings) != 1 || bindings[0].Role != "operator" {
		t.Fatalf("bindings = %+v, want operator", bindings)
	}

	// The flow cookie must not outlive its single use: it carried the PKCE
	// verifier.
	for _, c := range jar.Cookies(mustParse(t, server.URL)) {
		if c.Name == "karakuri_oidc_state" && c.Value != "" {
			t.Errorf("flow cookie survived the callback: %q", c.Value)
		}
	}
}

func TestLoginHandlerRedirectsWithPKCEAndNonce(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t, testClientID)
	p, _ := newProvider(t, idp, nil)

	rec := httptest.NewRecorder()
	p.LoginHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	location := mustParse(t, rec.Header().Get("Location"))
	q := location.Query()
	for _, param := range []string{"state", "nonce", "code_challenge"} {
		if q.Get(param) == "" {
			t.Errorf("redirect is missing %s", param)
		}
	}
	if got := q.Get("code_challenge_method"); got != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", got)
	}
	if got := q.Get("scope"); !strings.Contains(got, "openid") {
		t.Errorf("scope = %q, want it to include openid", got)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	if !cookies[0].HttpOnly {
		t.Error("flow cookie is readable by script")
	}
	if cookies[0].SameSite != http.SameSiteLaxMode {
		t.Error("flow cookie must be Lax — Strict withholds it on the provider's redirect back")
	}
	// The cookie carries the PKCE verifier, so its contents must not be
	// guessable from the redirect the browser just followed.
	if strings.Contains(cookies[0].Value, q.Get("code_challenge")) {
		t.Error("flow cookie leaks the challenge verbatim")
	}
}

func TestCallbackRejects(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t, testClientID)
	p, _ := newProvider(t, idp, nil)

	ok := func(http.ResponseWriter, *http.Request, auth.Principal) {
		t.Error("onSuccess ran on a request that should have failed")
	}
	var lastErr error
	handler := p.CallbackHandler(ok, func(w http.ResponseWriter, _ *http.Request, err error) {
		lastErr = err
		http.Error(w, err.Error(), http.StatusUnauthorized)
	})

	// A valid cookie, to pair with a mismatched state.
	rec := httptest.NewRecorder()
	p.LoginHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	goodCookie := rec.Result().Cookies()[0]

	cases := []struct {
		name    string
		target  string
		cookie  *http.Cookie
		wantErr error
	}{
		{name: "provider refused", target: "/callback?error=access_denied"},
		{name: "no cookie", target: "/callback?code=x&state=y"},
		{name: "malformed cookie", target: "/callback?code=x&state=y", cookie: &http.Cookie{Name: "karakuri_oidc_state", Value: "no-dot"}},
		{name: "bad signature", target: "/callback?code=x&state=y", cookie: &http.Cookie{Name: "karakuri_oidc_state", Value: "abc.ZGVm"}},
		{name: "state mismatch", target: "/callback?code=x&state=not-the-one", cookie: goodCookie, wantErr: oidc.ErrStateMismatch},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lastErr = nil
			r := httptest.NewRequest(http.MethodGet, tc.target, nil)
			if tc.cookie != nil {
				r.AddCookie(tc.cookie)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if tc.wantErr != nil && !errors.Is(lastErr, tc.wantErr) {
				t.Fatalf("err = %v, want %v", lastErr, tc.wantErr)
			}
		})
	}
}

// Without onError, failures still must not reach the success path.
func TestCallbackDefaultErrorHandler(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t, testClientID)
	p, _ := newProvider(t, idp, nil)

	handler := p.CallbackHandler(func(http.ResponseWriter, *http.Request, auth.Principal) {
		t.Error("onSuccess ran without a valid flow")
	}, nil)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/callback?error=access_denied", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestCallbackNoIDToken(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t, testClientID)
	idp.TokenResponse = func() (int, string) {
		return http.StatusOK, `{"access_token":"a","token_type":"Bearer","expires_in":3600}`
	}

	var lastErr error
	provisioner, _ := newProvisioner(t)
	p, err := oidc.New(context.Background(), oidc.Config{
		IssuerURL: idp.Server.URL, ClientID: testClientID,
		StateKey: []byte("a-32-byte-state-signing-key-here"), InsecureAllowHTTP: true,
		HTTPClient: idp.Server.Client(),
	}, provisioner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Start a flow so the callback has a valid cookie and state to present.
	rec := httptest.NewRecorder()
	p.LoginHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	cookie := rec.Result().Cookies()[0]
	state := mustParse(t, rec.Header().Get("Location")).Query().Get("state")

	r := httptest.NewRequest(http.MethodGet, "/callback?code=abc&state="+state, nil)
	r.AddCookie(cookie)
	out := httptest.NewRecorder()
	p.CallbackHandler(
		func(http.ResponseWriter, *http.Request, auth.Principal) { t.Error("onSuccess ran") },
		func(w http.ResponseWriter, _ *http.Request, err error) {
			lastErr = err
			w.WriteHeader(http.StatusUnauthorized)
		},
	).ServeHTTP(out, r)

	if !errors.Is(lastErr, oidc.ErrNoIDToken) {
		t.Fatalf("err = %v, want ErrNoIDToken", lastErr)
	}
}

// The nonce binds an ID token to the login flow that asked for it. A token
// carrying somebody else's nonce is a replay, and must not complete a login
// even though it verifies perfectly well as a signature.
func TestCallbackRejectsReplayedNonce(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t, testClientID)
	idp.TamperNonce = true

	provisioner, _ := newProvisioner(t)
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	p, err := oidc.New(context.Background(), oidc.Config{
		IssuerURL: idp.Server.URL, ClientID: testClientID, RedirectURL: server.URL + "/callback",
		StateKey: []byte("a-32-byte-state-signing-key-here"), InsecureAllowHTTP: true,
		HTTPClient: idp.Server.Client(),
	}, provisioner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var lastErr error
	mux.Handle("/login", p.LoginHandler())
	mux.Handle("/callback", p.CallbackHandler(
		func(http.ResponseWriter, *http.Request, auth.Principal) { t.Error("onSuccess ran on a replayed nonce") },
		func(w http.ResponseWriter, _ *http.Request, err error) {
			lastErr = err
			w.WriteHeader(http.StatusUnauthorized)
		},
	))

	jar, _ := cookiejar.New(nil)
	resp, err := (&http.Client{Jar: jar}).Get(server.URL + "/login")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if !errors.Is(lastErr, oidc.ErrNonceMismatch) {
		t.Fatalf("err = %v, want ErrNonceMismatch", lastErr)
	}
}

func TestCallbackExchangeFailure(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t, testClientID)
	idp.TokenResponse = func() (int, string) {
		return http.StatusBadRequest, `{"error":"invalid_grant"}`
	}
	provisioner, _ := newProvisioner(t)
	p, err := oidc.New(context.Background(), oidc.Config{
		IssuerURL: idp.Server.URL, ClientID: testClientID,
		StateKey: []byte("k"), InsecureAllowHTTP: true, HTTPClient: idp.Server.Client(),
	}, provisioner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rec := httptest.NewRecorder()
	p.LoginHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	cookie := rec.Result().Cookies()[0]
	state := mustParse(t, rec.Header().Get("Location")).Query().Get("state")

	r := httptest.NewRequest(http.MethodGet, "/callback?code=abc&state="+state, nil)
	r.AddCookie(cookie)
	out := httptest.NewRecorder()
	p.CallbackHandler(
		func(http.ResponseWriter, *http.Request, auth.Principal) { t.Error("onSuccess ran") },
		nil,
	).ServeHTTP(out, r)

	if out.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", out.Code)
	}
}

// A disabled principal must not be re-enabled by logging in again.
func TestResolveRefusesDisabledPrincipal(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t, testClientID)
	p, store := newProvider(t, idp, nil)
	token := idp.SignToken(t, idp.Claims("alice", "karakuri-operators"))

	if _, err := p.Resolve(bearer(token)); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if err := store.PutPrincipal(context.Background(), auth.Principal{ID: "oidc:alice", Disabled: true}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, err := p.Resolve(bearer(token)); !errors.Is(err, auth.ErrPrincipalDisabled) {
		t.Fatalf("err = %v, want ErrPrincipalDisabled", err)
	}
}

// The whole point of the ChainResolver ordering: a provider token presented to
// a chain whose first entry is a local resolver still authenticates.
func TestComposesWithChainResolver(t *testing.T) {
	t.Parallel()
	idp := newStubIdP(t, testClientID)
	p, _ := newProvider(t, idp, nil)

	local := auth.ResolverFunc(func(*http.Request) (auth.Principal, error) {
		return auth.Principal{}, errors.New("jwt: signature is not valid")
	})
	chain := auth.ChainResolver{local, p}

	token := idp.SignToken(t, idp.Claims("alice", "karakuri-operators"))
	principal, err := chain.Resolve(bearer(token))
	if err != nil {
		t.Fatalf("chain.Resolve: %v", err)
	}
	if principal.ID != "oidc:alice" {
		t.Fatalf("principal = %q", principal.ID)
	}
}

func TestDefaultScopes(t *testing.T) {
	t.Parallel()
	if !slices.Contains(oidc.DefaultScopes, "openid") {
		t.Fatal("DefaultScopes must include openid")
	}
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}
