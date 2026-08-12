package integration_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
)

// stubIdP is the smallest OpenID Connect provider that can complete an
// authorization-code flow: discovery, JWKS, an authorization endpoint that
// authenticates nobody in particular, and a token endpoint.
//
// It exists so this suite can prove the whole path through a real Karakuri
// server — login, just-in-time provisioning, session cookies, an authorized API
// call — without Docker. The Keycloak job in CI covers what an actual provider
// does differently.
type stubIdP struct {
	server   *httptest.Server
	key      *rsa.PrivateKey
	inner    *oidctest.Server
	clientID string
	groups   []string
	codes    map[string]string // authorization code → nonce
}

func newStubIdP(t *testing.T, clientID string, groups []string) *stubIdP {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	s := &stubIdP{key: key, clientID: clientID, groups: groups, codes: map[string]string{}}
	s.inner = &oidctest.Server{
		PublicKeys: []oidctest.PublicKey{{PublicKey: key.Public(), KeyID: "k1", Algorithm: "RS256"}},
		Algorithms: []string{"RS256"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", s.inner.ServeHTTP)
	mux.HandleFunc("/keys", s.inner.ServeHTTP)
	mux.HandleFunc("/auth", s.serveAuth)
	mux.HandleFunc("/token", s.serveToken)

	s.server = httptest.NewServer(mux)
	t.Cleanup(s.server.Close)
	s.inner.SetIssuer(s.server.URL)
	return s
}

func (s *stubIdP) serveAuth(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirect, err := url.Parse(q.Get("redirect_uri"))
	if err != nil {
		http.Error(w, "bad redirect_uri", http.StatusBadRequest)
		return
	}
	code := fmt.Sprintf("code-%d", time.Now().UnixNano())
	s.codes[code] = q.Get("nonce")

	rq := redirect.Query()
	rq.Set("code", code)
	rq.Set("state", q.Get("state"))
	redirect.RawQuery = rq.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (s *stubIdP) serveToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	nonce, ok := s.codes[r.Form.Get("code")]
	if !ok {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	delete(s.codes, r.Form.Get("code"))

	groups := make([]any, len(s.groups))
	for i, g := range s.groups {
		groups[i] = g
	}
	now := time.Now()
	claims, _ := json.Marshal(map[string]any{
		"iss": s.server.URL, "aud": s.clientID, "sub": "alice-subject",
		"exp": now.Add(time.Hour).Unix(), "iat": now.Unix(), "nonce": nonce,
		"email": "alice@example.com", "name": "Alice Federated",
		"groups": groups,
	})

	body, _ := json.Marshal(map[string]any{
		"access_token": "stub-access-token",
		"token_type":   "Bearer",
		"expires_in":   3600,
		"id_token":     oidctest.SignIDToken(s.key, "k1", "RS256", string(claims)),
	})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// federatedClient stops at the provider's redirect back into Karakuri.
//
// The callback URL registered with the provider has to be fixed before the
// server starts, but the test harness binds a free port — so the flow is
// interrupted at the redirect and replayed against the address the server
// actually got. A browser would simply follow it.
func federatedClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if strings.HasPrefix(req.URL.Path, "/api/v1/auth/sso/callback") {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

func startOIDCServer(t *testing.T, idp *stubIdP, roles map[string][]string) (string, func()) {
	t.Helper()
	baseURL, _, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Auth.Provider = config.AuthProviderOIDC
		cfg.Auth.Frontend.LoginRedirect = "/dashboard"
		cfg.Auth.OIDC.IssuerURL = idp.server.URL
		cfg.Auth.OIDC.ClientID = idp.clientID
		cfg.Auth.OIDC.ClientSecret = "shh"
		cfg.Auth.RoleMap.Groups = roles
		// PublicURL is what redirect URLs are derived from and must be fixed
		// before the listener exists. The redirect is overridden explicitly for
		// the same reason, and the callback is driven by hand below.
		cfg.Auth.Frontend.PublicURL = "http://127.0.0.1"
		cfg.Auth.OIDC.RedirectURL = "http://127.0.0.1/api/v1/auth/sso/callback"
	})
	return baseURL, cleanup
}

// completeLogin drives login → provider → callback and returns the callback's
// response, with the client's jar holding whatever session came out of it.
func completeLogin(t *testing.T, client *http.Client, baseURL string) *http.Response {
	t.Helper()

	resp, err := client.Get(baseURL + "/api/v1/auth/sso/login")
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected the provider to redirect back, got %d", resp.StatusCode)
	}

	callback, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse callback: %v", err)
	}

	// The callback's own response is the interesting one — its status, its
	// Location, and the cookies it sets. Following the post-login redirect here
	// would replace it with whatever the landing page returned, so this request
	// stops where it lands while still sharing the cookie jar.
	stop := &http.Client{
		Jar:           client.Jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	final, err := stop.Get(baseURL + "/api/v1/auth/sso/callback?" + callback.RawQuery)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	return final
}

// TestFederatedLoginProvisionsAndAuthorizes walks the whole path: an
// unauthenticated browser, a redirect to the identity provider, the callback,
// and an authorized API call made with nothing but the resulting cookies.
func TestFederatedLoginProvisionsAndAuthorizes(t *testing.T) {
	idp := newStubIdP(t, "karakuri", []string{"karakuri-operators"})
	baseURL, cleanup := startOIDCServer(t, idp, map[string][]string{
		"karakuri-operators": {"operator"},
	})
	defer cleanup()

	client := federatedClient(t)
	final := completeLogin(t, client, baseURL)
	defer func() { _ = final.Body.Close() }()

	if final.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(final.Body)
		t.Fatalf("callback = %d, want a redirect: %s", final.StatusCode, body)
	}
	location := final.Header.Get("Location")
	if location != "/dashboard" {
		t.Errorf("post-login redirect = %q, want the configured target", location)
	}
	// The session travels in cookies and nowhere else. A token in a redirect
	// URL lands in history, proxy logs and Referer headers.
	if strings.Contains(strings.ToLower(location), "token") {
		t.Error("the redirect target carries a token")
	}

	me := decodeJSON(t, mustGet(t, client, baseURL+"/api/v1/auth/me"))
	principal, _ := me["principal"].(map[string]any)
	if principal["id"] != "oidc:alice-subject" {
		t.Fatalf("principal = %v, want the namespaced subject", principal["id"])
	}
	if principal["name"] != "Alice Federated" {
		t.Errorf("name = %v, want the provider's display name", principal["name"])
	}
	roles, _ := me["roles"].([]any)
	if len(roles) != 1 || roles[0] != "operator" {
		t.Fatalf("roles = %v, want [operator] from the group mapping", roles)
	}

	// An operator may list twins, and may not create users.
	twins := mustGet(t, client, baseURL+"/api/v1/twins")
	if twins.StatusCode != http.StatusOK {
		t.Errorf("GET /twins = %d, want 200 for an operator", twins.StatusCode)
	}
	_ = twins.Body.Close()

	forbidden, err := client.Post(baseURL+"/api/v1/auth/users", "application/json",
		strings.NewReader(`{"id":"mallory","roles":["admin"]}`))
	if err != nil {
		t.Fatalf("post users: %v", err)
	}
	defer func() { _ = forbidden.Body.Close() }()
	if forbidden.StatusCode != http.StatusForbidden {
		t.Errorf("POST /auth/users = %d, want 403 — operator is not admin", forbidden.StatusCode)
	}
}

// A user who authenticates but matches no mapped group gets in and can do
// nothing. That is the intended shape: everybody in a corporate directory can
// authenticate, so authentication cannot imply authorization.
func TestFederatedLoginUnmappedUserHoldsNothing(t *testing.T) {
	idp := newStubIdP(t, "karakuri", []string{"marketing"})
	baseURL, cleanup := startOIDCServer(t, idp, map[string][]string{
		"karakuri-operators": {"operator"},
	})
	defer cleanup()

	client := federatedClient(t)
	final := completeLogin(t, client, baseURL)
	_ = final.Body.Close()
	if final.StatusCode != http.StatusFound {
		t.Fatalf("callback = %d, want the login to succeed", final.StatusCode)
	}

	me := decodeJSON(t, mustGet(t, client, baseURL+"/api/v1/auth/me"))
	if roles, _ := me["roles"].([]any); len(roles) != 0 {
		t.Fatalf("roles = %v, want none", roles)
	}
	twins := mustGet(t, client, baseURL+"/api/v1/twins")
	defer func() { _ = twins.Body.Close() }()
	if twins.StatusCode != http.StatusForbidden {
		t.Errorf("GET /twins = %d, want 403 for a user with no roles", twins.StatusCode)
	}
}

// Logging in twice must not accumulate bindings, and a group removed at the
// provider must be revoked here.
func TestFederatedLoginReconcilesOnEveryLogin(t *testing.T) {
	idp := newStubIdP(t, "karakuri", []string{"karakuri-operators"})
	baseURL, cleanup := startOIDCServer(t, idp, map[string][]string{
		"karakuri-operators": {"operator"},
		"karakuri-viewers":   {"viewer"},
	})
	defer cleanup()

	first := federatedClient(t)
	_ = completeLogin(t, first, baseURL).Body.Close()
	me := decodeJSON(t, mustGet(t, first, baseURL+"/api/v1/auth/me"))
	if roles, _ := me["roles"].([]any); len(roles) != 1 || roles[0] != "operator" {
		t.Fatalf("first login roles = %v", roles)
	}

	// Moved to a different group at the provider.
	idp.groups = []string{"karakuri-viewers"}

	second := federatedClient(t)
	_ = completeLogin(t, second, baseURL).Body.Close()
	me = decodeJSON(t, mustGet(t, second, baseURL+"/api/v1/auth/me"))
	roles, _ := me["roles"].([]any)
	if len(roles) != 1 || roles[0] != "viewer" {
		t.Fatalf("second login roles = %v, want [viewer] — the old grant was not revoked", roles)
	}
}

// Password login has to keep working while a federated provider is configured.
// It is the break-glass path when the identity provider is unreachable, and the
// reason Phase 16 did not re-introduce a static shared token.
func TestPasswordLoginSurvivesFederation(t *testing.T) {
	idp := newStubIdP(t, "karakuri", []string{"karakuri-operators"})
	baseURL, adminToken, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Auth.Provider = config.AuthProviderOIDC
		cfg.Auth.Frontend.PublicURL = "http://127.0.0.1"
		cfg.Auth.OIDC.IssuerURL = idp.server.URL
		cfg.Auth.OIDC.ClientID = "karakuri"
		cfg.Auth.RoleMap.Groups = map[string][]string{"karakuri-operators": {"operator"}}
	})
	defer cleanup()

	if adminToken == "" {
		t.Fatal("the bootstrap administrator could not log in with a password")
	}
	resp := doJSON(t, adminToken, http.MethodGet, baseURL+"/api/v1/auth/users", nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
}

// A deployment with no federated provider still answers the SSO routes, with an
// explanation rather than a bare router 404.
func TestSSOConfigWithoutProvider(t *testing.T) {
	baseURL, _, cleanup := startServer(t)
	defer cleanup()

	body := decodeJSON(t, mustGet(t, http.DefaultClient, baseURL+"/api/v1/auth/sso/config"))
	if body["provider"] != "bearer" {
		t.Errorf("provider = %v, want bearer", body["provider"])
	}
	if body["enabled"] != false {
		t.Errorf("enabled = %v, want false", body["enabled"])
	}
	if body["password_login"] != true {
		t.Errorf("password_login = %v, want true", body["password_login"])
	}

	resp := mustGet(t, http.DefaultClient, baseURL+"/api/v1/auth/sso/login")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /auth/sso/login without a provider = %d, want 404", resp.StatusCode)
	}
}

func mustGet(t *testing.T, client *http.Client, target string) *http.Response {
	t.Helper()
	resp, err := client.Get(target)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	return resp
}
