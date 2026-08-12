package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
)

// Keycloak end-to-end.
//
// The stub provider in federation_test.go proves this server drives the
// authorization-code flow correctly. What it cannot prove is that Karakuri
// agrees with a *real* identity provider about discovery documents, JWKS
// shapes, audiences and where groups live — every one of which is somewhere
// implementations differ. That is what this file is for.
//
// It exercises the bearer path rather than the browser one, deliberately: a
// browser flow needs a login form filled in, and driving Keycloak's HTML is a
// test of Keycloak's HTML. Presenting a real Keycloak-issued ID token to
// Provider.Resolve puts the same verification, claim mapping and provisioning
// code under a real provider's output, which is the part worth proving.
//
// Skipped unless KEYCLOAK_URL is set, so `go test ./...` still works on a
// laptop with nothing running. CI sets it; see the identity job in ci.yml.

const (
	keycloakRealm    = "karakuri"
	keycloakClient   = "karakuri-api"
	keycloakSecret   = "karakuri-client-secret"
	keycloakUser     = "alice"
	keycloakPassword = "alice-password"
	keycloakGroup    = "karakuri-operators"
)

func keycloakURL(t *testing.T) string {
	t.Helper()
	base := strings.TrimSuffix(os.Getenv("KEYCLOAK_URL"), "/")
	if base == "" {
		t.Skip("KEYCLOAK_URL is not set; skipping the live identity-provider test")
	}
	return base
}

// TestKeycloakBearerLogin provisions a realm, obtains a genuine ID token, and
// presents it to Karakuri as a bearer credential.
func TestKeycloakBearerLogin(t *testing.T) {
	base := keycloakURL(t)
	adminToken := keycloakAdminToken(t, base)
	provisionKeycloakRealm(t, base, adminToken)

	issuer := base + "/realms/" + keycloakRealm
	baseURL, _, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Auth.Provider = config.AuthProviderOIDC
		cfg.Auth.Frontend.PublicURL = "http://127.0.0.1"
		cfg.Auth.OIDC.IssuerURL = issuer
		cfg.Auth.OIDC.ClientID = keycloakClient
		cfg.Auth.OIDC.ClientSecret = keycloakSecret
		cfg.Auth.RoleMap.Groups = roleGrants(map[string][]string{keycloakGroup: {"operator"}})
	})
	defer cleanup()

	idToken := keycloakIDToken(t, issuer)

	// The identity provider's own token, straight into Karakuri.
	me := decodeJSON(t, doJSON(t, idToken, http.MethodGet, baseURL+"/api/v1/auth/me", nil))
	principal, _ := me["principal"].(map[string]any)
	id, _ := principal["id"].(string)
	if !strings.HasPrefix(id, "oidc:") {
		t.Fatalf("principal = %q, want a namespaced federated principal", id)
	}
	roles, _ := me["roles"].([]any)
	if len(roles) != 1 || roles[0] != "operator" {
		t.Fatalf("roles = %v, want [operator] from Keycloak's group membership", roles)
	}

	// And the permission that role carries actually works.
	twins := doJSON(t, idToken, http.MethodGet, baseURL+"/api/v1/twins", nil)
	defer twins.Body.Close()
	assertStatus(t, twins, http.StatusOK)

	// A token whose signature no longer covers its contents must not
	// authenticate, however real the rest of it looks.
	tampered := doJSON(t, idToken+"tampered", http.MethodGet, baseURL+"/api/v1/auth/me", nil)
	defer tampered.Body.Close()
	assertStatus(t, tampered, http.StatusUnauthorized)
}

func keycloakAdminToken(t *testing.T, base string) string {
	t.Helper()
	user := envOr("KEYCLOAK_ADMIN", "admin")
	password := envOr("KEYCLOAK_ADMIN_PASSWORD", "admin")

	return keycloakToken(t, base+"/realms/master/protocol/openid-connect/token", url.Values{
		"grant_type": {"password"},
		"client_id":  {"admin-cli"},
		"username":   {user},
		"password":   {password},
	}, "access_token")
}

func keycloakIDToken(t *testing.T, issuer string) string {
	t.Helper()
	return keycloakToken(t, issuer+"/protocol/openid-connect/token", url.Values{
		"grant_type":    {"password"},
		"client_id":     {keycloakClient},
		"client_secret": {keycloakSecret},
		"username":      {keycloakUser},
		"password":      {keycloakPassword},
		"scope":         {"openid"},
	}, "id_token")
}

func keycloakToken(t *testing.T, endpoint string, form url.Values, field string) string {
	t.Helper()

	resp, err := http.PostForm(endpoint, form)
	if err != nil {
		t.Fatalf("token request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token request = %d: %s", resp.StatusCode, body)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	token, _ := out[field].(string)
	if token == "" {
		t.Fatalf("token response carried no %s: %s", field, body)
	}
	return token
}

// provisionKeycloakRealm creates the realm, client, group and user this test
// needs, through the admin API rather than a realm-import file — a file would
// have to be mounted into the service container, which GitHub Actions services
// do not make easy.
//
// Every step tolerates "already exists" so a re-run against a warm Keycloak
// works.
func provisionKeycloakRealm(t *testing.T, base, adminToken string) {
	t.Helper()

	adminPost(t, adminToken, base+"/admin/realms", map[string]any{
		"realm":   keycloakRealm,
		"enabled": true,
	})

	adminPost(t, adminToken, base+"/admin/realms/"+keycloakRealm+"/clients", map[string]any{
		"clientId":                  keycloakClient,
		"secret":                    keycloakSecret,
		"enabled":                   true,
		"publicClient":              false,
		"directAccessGrantsEnabled": true,
		"standardFlowEnabled":       true,
		"redirectUris":              []string{"http://127.0.0.1/*"},
		// Group membership is not in an ID token unless a mapper puts it there,
		// and the claim name is this mapper's to choose. Karakuri reads
		// "groups" by default, so that is what this asks for.
		"protocolMappers": []map[string]any{{
			"name":           "groups",
			"protocol":       "openid-connect",
			"protocolMapper": "oidc-group-membership-mapper",
			"config": map[string]string{
				"claim.name":           "groups",
				"full.path":            "false",
				"id.token.claim":       "true",
				"access.token.claim":   "true",
				"userinfo.token.claim": "true",
			},
		}},
	})

	adminPost(t, adminToken, base+"/admin/realms/"+keycloakRealm+"/groups", map[string]any{
		"name": keycloakGroup,
	})

	adminPost(t, adminToken, base+"/admin/realms/"+keycloakRealm+"/users", map[string]any{
		"username":      keycloakUser,
		"email":         keycloakUser + "@example.com",
		"firstName":     "Alice",
		"lastName":      "Federated",
		"enabled":       true,
		"emailVerified": true,
		"groups":        []string{"/" + keycloakGroup},
		"credentials": []map[string]any{{
			"type":      "password",
			"value":     keycloakPassword,
			"temporary": false,
		}},
	})
}

func adminPost(t *testing.T, token, endpoint string, body any) {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal %s: %v", endpoint, err)
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("build request %s: %v", endpoint, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode < 300, resp.StatusCode == http.StatusConflict:
		// Created, or already there from an earlier run.
		return
	default:
		out, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s = %d: %s", endpoint, resp.StatusCode, out)
	}
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
