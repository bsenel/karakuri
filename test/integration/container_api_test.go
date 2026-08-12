package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"

	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

// createContainer posts a container and returns its id, failing on anything but
// 201.
func createContainer(t *testing.T, baseURL, token, kind, name, parentID string) string {
	t.Helper()
	resp := doJSON(t, token, http.MethodPost, baseURL+"/api/v1/containers", map[string]any{
		"kind": kind, "name": name, "parent_id": parentID,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusCreated)

	var c struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		t.Fatalf("decode container: %v", err)
	}
	if c.ID == "" {
		t.Fatal("container came back with no id")
	}
	return c.ID
}

func post(t *testing.T, baseURL, token, path string, body any) *http.Response {
	t.Helper()
	return doJSON(t, token, http.MethodPost, baseURL+path, body)
}

// The hierarchy governs changes to itself. An administrator scoped to one
// organisation manages that organisation and cannot reach into another — which
// is what stops the tree from being decoration.
func TestContainerWritesAreConfinedToTheTenant(t *testing.T) {
	baseURL, adminToken, cleanup := startServer(t)
	defer cleanup()

	acme := createContainer(t, baseURL, adminToken, "org", "acme", "")
	globex := createContainer(t, baseURL, adminToken, "org", "globex", "")

	// An operator bound to acme, and nothing else.
	const password = "correct-horse-battery-staple"
	resp := post(t, baseURL, adminToken, "/api/v1/auth/users", map[string]any{
		"id": "amy", "roles": []string{karakuriauth.RoleOperator},
		"scope": "org:" + acme, "password": password,
	})
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	amy := login(t, baseURL, "amy", password)

	// Inside her own organisation: allowed.
	resp = post(t, baseURL, amy, "/api/v1/containers", map[string]any{
		"kind": "team", "name": "eng", "parent_id": acme,
	})
	assertStatus(t, resp, http.StatusCreated)
	var team struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&team)
	resp.Body.Close()

	// Inside somebody else's: refused.
	resp = post(t, baseURL, amy, "/api/v1/containers", map[string]any{
		"kind": "team", "name": "eng", "parent_id": globex,
	})
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	// Minting a new tenant is a different privilege from running one, so a
	// root org is refused too.
	resp = post(t, baseURL, amy, "/api/v1/containers", map[string]any{
		"kind": "org", "name": "amy-holdings",
	})
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	// Renaming her own team is fine; renaming the other organisation is not.
	resp = post(t, baseURL, amy, "/api/v1/containers/"+team.ID+"/name", map[string]any{"name": "Engineering"})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = post(t, baseURL, amy, "/api/v1/containers/"+globex+"/name", map[string]any{"name": "pwned"})
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()
}

// Reparenting needs a covering grant at both ends. Holding only the destination
// would let somebody pull a team — and everything in it — out of a tenant they
// have no claim on.
func TestReparentRequiresBothEnds(t *testing.T) {
	baseURL, adminToken, cleanup := startServer(t)
	defer cleanup()

	acme := createContainer(t, baseURL, adminToken, "org", "acme", "")
	globex := createContainer(t, baseURL, adminToken, "org", "globex", "")
	acmeEng := createContainer(t, baseURL, adminToken, "team", "eng", acme)

	const password = "correct-horse-battery-staple"
	resp := post(t, baseURL, adminToken, "/api/v1/auth/users", map[string]any{
		"id": "gil", "roles": []string{karakuriauth.RoleOperator},
		"scope": "org:" + globex, "password": password,
	})
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	gil := login(t, baseURL, "gil", password)

	// Gil holds globex, the destination. He does not hold acme, the source, so
	// he cannot help himself to acme's team.
	resp = post(t, baseURL, gil, "/api/v1/containers/"+acmeEng+"/parent", map[string]any{"parent_id": globex})
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	// The administrator holds both and can.
	resp = post(t, baseURL, adminToken, "/api/v1/containers/"+acmeEng+"/parent", map[string]any{"parent_id": globex})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

// You can only grant a scope you already hold. Without this rule, the
// permission to manage bindings is the permission to manage every tenant.
func TestYouMayOnlyGrantAScopeYouHold(t *testing.T) {
	baseURL, adminToken, cleanup := startServer(t)
	defer cleanup()

	acme := createContainer(t, baseURL, adminToken, "org", "acme", "")
	globex := createContainer(t, baseURL, adminToken, "org", "globex", "")
	acmeEng := createContainer(t, baseURL, adminToken, "team", "eng", acme)

	const password = "correct-horse-battery-staple"
	resp := post(t, baseURL, adminToken, "/api/v1/auth/users", map[string]any{
		"id": "ada", "roles": []string{karakuriauth.RoleAdmin},
		"scope": "org:" + acme, "password": password,
	})
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	ada := login(t, baseURL, "ada", password)

	// A principal for her to grant things to.
	resp = post(t, baseURL, adminToken, "/api/v1/auth/users", map[string]any{
		"id": "bob", "password": password,
	})
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Inside acme: allowed, including a team she reaches through the org.
	resp = post(t, baseURL, ada, "/api/v1/auth/bindings", map[string]any{
		"principal_id": "bob", "role": karakuriauth.RoleViewer, "scope": "team:" + acmeEng,
	})
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Into globex: refused. This is the escalation the rule exists to stop —
	// she is an administrator, just not of that tenant.
	resp = post(t, baseURL, ada, "/api/v1/auth/bindings", map[string]any{
		"principal_id": "bob", "role": karakuriauth.RoleViewer, "scope": "org:" + globex,
	})
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	// And she cannot widen her own reach to everything.
	resp = post(t, baseURL, ada, "/api/v1/auth/bindings", map[string]any{
		"principal_id": "ada", "role": karakuriauth.RoleAdmin, "scope": "*",
	})
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	// Creating principals stays an unscoped privilege, deliberately: the
	// endpoint upserts a principal and sets its password, so a tenant-scoped
	// administrator who could reach it could reset the bootstrap
	// administrator's password by naming that id. Bindings are the part that
	// containment makes safe to delegate; identities are not.
	resp = post(t, baseURL, ada, "/api/v1/auth/users", map[string]any{
		"id": "mallory", "roles": []string{karakuriauth.RoleAdmin},
		"scope": "*", "password": password,
	})
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	// And the refusal left nothing behind — the principal was never written.
	resp = doJSON(t, adminToken, http.MethodGet, baseURL+"/api/v1/auth/users", nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	var users []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	for _, u := range users {
		if u.ID == "mallory" {
			t.Fatal("a refused user creation still wrote the principal")
		}
	}
}

// Sharing a twin into a project requires holding the twin *and* the project.
// Either half alone is an escalation: holding only the project would let
// somebody file another tenant's twin into a space they control and read it
// there.
func TestSharingIntoAProjectRequiresBothHalves(t *testing.T) {
	baseURL, adminToken, cleanup := startServer(t)
	defer cleanup()

	acme := createContainer(t, baseURL, adminToken, "org", "acme", "")
	delta := createContainer(t, baseURL, adminToken, "project", "delta", "")
	acmeTwin := createTwin(t, baseURL, adminToken, "acme twin")

	// Place the twin in acme so an acme grant reaches it.
	resp := post(t, baseURL, adminToken, "/api/v1/containers/resources", map[string]any{
		"resource_type": "twin", "resource_id": acmeTwin, "container_ids": []string{acme},
	})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	const password = "correct-horse-battery-staple"
	// Pat holds the project but nothing in acme.
	resp = post(t, baseURL, adminToken, "/api/v1/auth/users", map[string]any{
		"id": "pat", "roles": []string{karakuriauth.RoleOperator},
		"scope": "project:" + delta, "password": password,
	})
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	pat := login(t, baseURL, "pat", password)

	resp = post(t, baseURL, pat, "/api/v1/containers/resources", map[string]any{
		"resource_type": "twin", "resource_id": acmeTwin,
		"container_ids": []string{acme, delta},
	})
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	// The administrator holds both halves, and the twin ends up in acme and in
	// the project at once — the multi-homing that makes cross-tenant
	// collaboration work without a second construct.
	resp = post(t, baseURL, adminToken, "/api/v1/containers/resources", map[string]any{
		"resource_type": "twin", "resource_id": acmeTwin,
		"container_ids": []string{acme, delta},
	})
	assertStatus(t, resp, http.StatusOK)
	var placed struct {
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&placed); err != nil {
		t.Fatalf("decode placement: %v", err)
	}
	resp.Body.Close()
	if len(placed.Scopes) != 2 {
		t.Fatalf("scopes = %v, want the org and the project", placed.Scopes)
	}

	// And now Pat reaches it — through the project, without acme granting
	// anything else away.
	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/twins/"+acmeTwin, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+pat)
	got, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get twin: %v", err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("GET shared twin = %d, want 200 via the project", got.StatusCode)
	}
}
