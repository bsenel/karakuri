package integration_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/app"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// login exchanges credentials for an access token.
func login(t *testing.T, baseURL, id, password string) string {
	t.Helper()
	resp := doJSON(t, "", http.MethodPost, baseURL+"/api/v1/auth/token",
		map[string]any{"id": id, "password": password})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("login as %q: %d %s", id, resp.StatusCode, body)
	}
	var pair struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pair); err != nil {
		t.Fatalf("decode token: %v", err)
	}
	return pair.AccessToken
}

// createUser adds a principal with the given roles and returns its access token.
func createUser(t *testing.T, baseURL, adminToken, id, role string) string {
	t.Helper()
	const password = "correct-horse-battery-staple"
	resp := doJSON(t, adminToken, http.MethodPost, baseURL+"/api/v1/auth/users", map[string]any{
		"id": id, "name": id, "roles": []string{role}, "password": password,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create %q: %d %s", id, resp.StatusCode, body)
	}
	return login(t, baseURL, id, password)
}

// concreteURL turns a route pattern into a requestable path. The IDs need not
// exist: authorization runs before the handler, so a permitted request that
// 404s still proves the permission was granted.
func concreteURL(baseURL, pattern string) string {
	p := strings.NewReplacer(
		"{id}", "does-not-exist",
		"{sha}", "0000000000000000000000000000000000000000",
		"{other}", "1111111111111111111111111111111111111111",
	).Replace(pattern)
	return baseURL + "/api/v1" + p
}

// TestRBACRouteMatrix walks every route in the permission table as each
// built-in role and asserts exactly one thing: whether the request is refused.
//
// Testing for 403-or-not rather than 200 is deliberate. Authorization runs
// before the handler, so an allowed request may still 400 or 404 on a fabricated
// ID — that is the handler's business. Conflating the two would make the test
// about fixture setup instead of about permissions.
func TestRBACRouteMatrix(t *testing.T) {
	baseURL, adminToken, cleanup := startServer(t)
	defer cleanup()

	tokens := map[string]string{
		karakuriauth.RoleAdmin:    adminToken,
		karakuriauth.RoleViewer:   createUser(t, baseURL, adminToken, "vera", karakuriauth.RoleViewer),
		karakuriauth.RoleOperator: createUser(t, baseURL, adminToken, "olive", karakuriauth.RoleOperator),
		karakuriauth.RoleAuditor:  createUser(t, baseURL, adminToken, "aud", karakuriauth.RoleAuditor),
	}

	// Which actions each role is expected to hold. Written out rather than
	// derived from the role definitions, so a mistake in those definitions
	// fails here instead of being mirrored by the test.
	expected := map[string]map[string]bool{
		karakuriauth.RoleViewer: {
			"twin:read": true, "objective:read": true, "loop:read": true,
			"checkpoint:read": true, "artifact:read": true, "memory:read": true,
			"domain:read": true,
		},
		karakuriauth.RoleAuditor: {
			"twin:read": true, "objective:read": true, "loop:read": true,
			"checkpoint:read": true, "artifact:read": true, "memory:read": true,
			"domain:read": true, "audit:read": true,
		},
		karakuriauth.RoleOperator: {
			"twin:read": true, "twin:create": true, "twin:update": true,
			"twin:delete": true, "twin:bind": true,
			"objective:read": true, "objective:create": true, "objective:update": true,
			"loop:read": true, "loop:start": true, "loop:resume": true,
			"checkpoint:read": true, "checkpoint:resolve": true,
			"artifact:read": true, "artifact:write": true,
			"memory:read": true, "memory:write": true, "memory:forget": true,
			"domain:read": true, "research:run": true,
		},
		// admin holds the wildcard.
		karakuriauth.RoleAdmin: nil,
	}

	for _, route := range karakuriauth.Routes() {
		if route.Public || route.Action == "" {
			continue // covered separately by the public/authenticated tests
		}
		for role, token := range tokens {
			name := role + " " + route.Method + " " + route.Pattern
			t.Run(name, func(t *testing.T) {
				allowed := role == karakuriauth.RoleAdmin
				if perms := expected[role]; perms != nil {
					allowed = perms[string(route.Action)]
				}

				resp := doJSON(t, token, route.Method, concreteURL(baseURL, route.Pattern), nil)
				defer resp.Body.Close()

				forbidden := resp.StatusCode == http.StatusForbidden
				if allowed && forbidden {
					body, _ := io.ReadAll(resp.Body)
					t.Errorf("%s should hold %s but was refused: %s", role, route.Action, body)
				}
				if !allowed && !forbidden {
					t.Errorf("%s should not hold %s but got %d", role, route.Action, resp.StatusCode)
				}
			})
		}
	}
}

// TestRBACUnauthenticated pins which routes are reachable with no credential.
func TestRBACUnauthenticated(t *testing.T) {
	baseURL, _, cleanup := startServer(t)
	defer cleanup()

	for _, route := range karakuriauth.Routes() {
		if route.Method != http.MethodGet {
			continue // POST/PUT without a body would 400 before authenticating
		}
		t.Run(route.Method+" "+route.Pattern, func(t *testing.T) {
			resp := doJSON(t, "", route.Method, concreteURL(baseURL, route.Pattern), nil)
			defer resp.Body.Close()

			if route.Public {
				if resp.StatusCode == http.StatusUnauthorized {
					t.Errorf("%s is public but returned 401", route.Pattern)
				}
				return
			}
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s without a token = %d, want 401", route.Pattern, resp.StatusCode)
			}
		})
	}
}

// TestRBACDenyIsAudited proves a refusal is recorded, not just returned. An
// operator reviewing who approved what should also see who was turned away.
func TestRBACDenyIsAudited(t *testing.T) {
	baseURL, adminToken, cleanup := startServer(t)
	defer cleanup()
	veraToken := createUser(t, baseURL, adminToken, "vera", karakuriauth.RoleViewer)

	resp := doJSON(t, veraToken, http.MethodGet, baseURL+"/api/v1/audit", nil)
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	// The write happens on the request path, but the audit row lands via the
	// enforcer hook — give it a moment on slower CI.
	var events []any
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r := doJSON(t, adminToken, http.MethodGet, baseURL+"/api/v1/audit?kind=authz_denied", nil)
		events = decodeJSONSlice(t, r)
		if len(events) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(events) == 0 {
		t.Fatal("denial was not recorded in the audit log")
	}
	first, ok := events[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected audit row shape: %T", events[0])
	}
	if first["agent_id"] != "vera" {
		t.Errorf("audit row principal = %v, want vera", first["agent_id"])
	}
	if first["capability"] != "audit:read" {
		t.Errorf("audit row action = %v, want audit:read", first["capability"])
	}
	if reason, _ := first["escalation_reason"].(string); reason == "" {
		t.Error("audit row carries no reason — a denial that cannot explain itself is not much of a record")
	}
}

// TestRBACTokenLifecycle covers login, /auth/me, rotation, reuse detection and
// revocation over real HTTP.
func TestRBACTokenLifecycle(t *testing.T) {
	baseURL, adminToken, cleanup := startServer(t)
	defer cleanup()

	const password = "correct-horse-battery-staple"
	resp := doJSON(t, adminToken, http.MethodPost, baseURL+"/api/v1/auth/users", map[string]any{
		"id": "olive", "roles": []string{karakuriauth.RoleOperator}, "password": password,
	})
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Login.
	resp = doJSON(t, "", http.MethodPost, baseURL+"/api/v1/auth/token",
		map[string]any{"id": "olive", "password": password})
	assertStatus(t, resp, http.StatusOK)
	var first struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&first); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if first.TokenType != "Bearer" || first.ExpiresIn <= 0 {
		t.Fatalf("token response = %+v", first)
	}

	// A wrong password is rejected, and indistinguishably from an unknown user.
	for _, body := range []map[string]any{
		{"id": "olive", "password": "wrong"},
		{"id": "nobody", "password": password},
	} {
		r := doJSON(t, "", http.MethodPost, baseURL+"/api/v1/auth/token", body)
		assertStatus(t, r, http.StatusUnauthorized)
		r.Body.Close()
	}

	// /auth/me reports identity and effective permissions.
	resp = doJSON(t, first.AccessToken, http.MethodGet, baseURL+"/api/v1/auth/me", nil)
	assertStatus(t, resp, http.StatusOK)
	me := decodeJSON(t, resp)
	roles, _ := me["roles"].([]any)
	if len(roles) != 1 || roles[0] != karakuriauth.RoleOperator {
		t.Errorf("/auth/me roles = %v", me["roles"])
	}
	perms, _ := me["permissions"].([]any)
	if len(perms) == 0 {
		t.Error("/auth/me returned no permissions")
	}
	var sawAudit bool
	for _, p := range perms {
		if p == "audit:read" {
			sawAudit = true
		}
	}
	if sawAudit {
		t.Error("operator's effective permissions include audit:read")
	}

	// Rotation: the refresh token changes, and the old access token still works
	// until it expires on its own.
	resp = doJSON(t, "", http.MethodPost, baseURL+"/api/v1/auth/refresh",
		map[string]any{"refresh_token": first.RefreshToken})
	assertStatus(t, resp, http.StatusOK)
	var second struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&second); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}

	// Replaying the spent token is treated as a leak: the family dies.
	resp = doJSON(t, "", http.MethodPost, baseURL+"/api/v1/auth/refresh",
		map[string]any{"refresh_token": first.RefreshToken})
	assertStatus(t, resp, http.StatusUnauthorized)
	reuse := decodeJSON(t, resp)
	if reuse["error"] != "token_reuse" {
		t.Errorf("replay error = %v, want token_reuse", reuse["error"])
	}

	resp = doJSON(t, "", http.MethodPost, baseURL+"/api/v1/auth/refresh",
		map[string]any{"refresh_token": second.RefreshToken})
	assertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()

	// A garbage token is rejected rather than treated as anonymous.
	resp = doJSON(t, "not-a-jwt", http.MethodGet, baseURL+"/api/v1/twins", nil)
	assertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}

// TestRBACScopedBinding proves a role granted over one twin does not reach
// another — the difference between "olive is an operator" and "olive is an
// operator on twin X".
func TestRBACScopedBinding(t *testing.T) {
	baseURL, adminToken, cleanup := startServer(t)
	defer cleanup()

	resp := doJSON(t, adminToken, http.MethodPost, baseURL+"/api/v1/twins", map[string]any{
		"name": "scoped", "kind": "team", "domain": "software",
	})
	assertStatus(t, resp, http.StatusOK)
	twinID, _ := decodeJSON(t, resp)["id"].(string)
	if twinID == "" {
		t.Fatal("no twin id returned")
	}

	const password = "correct-horse-battery-staple"
	resp = doJSON(t, adminToken, http.MethodPost, baseURL+"/api/v1/auth/users", map[string]any{
		"id": "scout", "roles": []string{karakuriauth.RoleViewer},
		"scope": "twin:" + twinID, "password": password,
	})
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	scoutToken := login(t, baseURL, "scout", password)

	// In scope.
	resp = doJSON(t, scoutToken, http.MethodGet, baseURL+"/api/v1/twins/"+twinID, nil)
	if resp.StatusCode == http.StatusForbidden {
		t.Error("scoped viewer was refused its own twin")
	}
	resp.Body.Close()

	// Out of scope.
	resp = doJSON(t, scoutToken, http.MethodGet, baseURL+"/api/v1/twins/some-other-twin", nil)
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	// The collection is not covered by a single-object scope either.
	resp = doJSON(t, scoutToken, http.MethodGet, baseURL+"/api/v1/twins", nil)
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()
}

// TestRBACCheckEndpoint exercises the policy debugger, including the trace it
// returns — a denial that cannot explain itself is not much use.
func TestRBACCheckEndpoint(t *testing.T) {
	baseURL, adminToken, cleanup := startServer(t)
	defer cleanup()
	createUser(t, baseURL, adminToken, "vera", karakuriauth.RoleViewer)

	resp := doJSON(t, adminToken, http.MethodPost, baseURL+"/api/v1/auth/check", map[string]any{
		"principal": "vera", "action": "loop:start", "resource": "loop:*",
	})
	assertStatus(t, resp, http.StatusOK)
	decision := decodeJSON(t, resp)
	if allowed, _ := decision["allowed"].(bool); allowed {
		t.Error("viewer was reported as able to start loops")
	}
	if reason, _ := decision["reason"].(string); reason == "" {
		t.Error("decision carries no reason")
	}

	resp = doJSON(t, adminToken, http.MethodPost, baseURL+"/api/v1/auth/check", map[string]any{
		"principal": "vera", "action": "twin:read", "resource": "twin:abc",
	})
	assertStatus(t, resp, http.StatusOK)
	decision = decodeJSON(t, resp)
	if allowed, _ := decision["allowed"].(bool); !allowed {
		t.Errorf("viewer should read twins: %v", decision["reason"])
	}
	if via, _ := decision["via_role"].(string); via != karakuriauth.RoleViewer {
		t.Errorf("via_role = %v, want viewer", decision["via_role"])
	}

	// An action outside the catalog is a client error, not a silent denial.
	resp = doJSON(t, adminToken, http.MethodPost, baseURL+"/api/v1/auth/check", map[string]any{
		"principal": "vera", "action": "twin:teleport", "resource": "*",
	})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

// TestRBACSSEQueryToken covers the one place a token legitimately travels in a
// query string: EventSource cannot set an Authorization header.
func TestRBACSSEQueryToken(t *testing.T) {
	baseURL, adminToken, cleanup := startServer(t)
	defer cleanup()

	resp := doJSON(t, adminToken, http.MethodPost, baseURL+"/api/v1/twins", map[string]any{
		"name": "streamer", "kind": "team", "domain": "software",
	})
	assertStatus(t, resp, http.StatusOK)
	twinID, _ := decodeJSON(t, resp)["id"].(string)

	streamURL := baseURL + "/api/v1/twins/" + twinID + "/events"

	// Without a credential the stream is refused.
	anon := doJSON(t, "", http.MethodGet, streamURL, nil)
	if anon.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous SSE = %d, want 401", anon.StatusCode)
	}
	anon.Body.Close()

	// The query parameter is accepted on this path.
	req, err := http.NewRequest(http.MethodGet, streamURL+"?access_token="+adminToken, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	streamResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(streamResp.Body)
		t.Fatalf("SSE with query token = %d: %s", streamResp.StatusCode, body)
	}
	if ct := streamResp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q", ct)
	}
	// Read one line so the handler is genuinely serving, not just accepted.
	_, _ = bufio.NewReader(streamResp.Body).ReadString('\n')

	// The same trick does not work on an ordinary REST route: query strings end
	// up in access logs, so the fallback is scoped to streams.
	rest := doJSON(t, "", http.MethodGet, baseURL+"/api/v1/twins?access_token="+adminToken, nil)
	if rest.StatusCode != http.StatusUnauthorized {
		t.Errorf("REST route accepted a query token: %d", rest.StatusCode)
	}
	rest.Body.Close()
}

// TestBootstrapRequiresPassword pins the fail-closed path added after CodeQL
// flagged the previous behaviour: a database with no principals used to mint an
// administrator and log its generated password at WARN.
//
// That is worse than it sounds in this codebase — Karakuri fans logs out to
// Datadog, Loki, Elasticsearch and CloudWatch, so "logged once" meant the
// credential was copied to every configured sink. The server now refuses to
// start instead, exactly as it does with no JWT signing key.
func TestBootstrapRequiresPassword(t *testing.T) {
	ctx := context.Background()
	dbFile, err := os.CreateTemp("", "karakuri-bootstrap-*.db")
	if err != nil {
		t.Fatalf("temp db: %v", err)
	}
	dbPath := dbFile.Name()
	dbFile.Close()
	t.Cleanup(func() { os.Remove(dbPath) })

	cfg := config.Default()
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = dbPath
	cfg.Auth.JWT.Keys = []config.JWTKeyConfig{{
		ID: "test", Algorithm: "HS256", Active: true,
		Secret: strings.Repeat("integration-test-signing-key", 2),
	}}
	t.Setenv(cfg.Auth.Bootstrap.PasswordEnv, "")

	gormDB, err := platformdb.Open(cfg.Database.Driver, cfg.Database.DSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(gormDB, cfg.Database.DSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := storage.NewGORMStorage(gormDB)

	_, err = app.BuildAuth(ctx, gormDB, store, cfg)
	if err == nil {
		t.Fatal("BuildAuth succeeded with no bootstrap password on an empty database")
	}
	if !errors.Is(err, karakuriauth.ErrNoBootstrapPassword) {
		t.Fatalf("error = %v, want ErrNoBootstrapPassword", err)
	}
	// The message has to tell an operator what to set, not just that something
	// is missing.
	if !strings.Contains(err.Error(), cfg.Auth.Bootstrap.PasswordEnv) {
		t.Errorf("error does not name the env var to set: %v", err)
	}

	// With the password supplied, the same empty database bootstraps fine.
	t.Setenv(cfg.Auth.Bootstrap.PasswordEnv, "a-chosen-password")
	if _, err := app.BuildAuth(ctx, gormDB, store, cfg); err != nil {
		t.Fatalf("BuildAuth with a password: %v", err)
	}
}
