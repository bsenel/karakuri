package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"testing"

	extauth "github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/config"
	corecontainer "github.com/bsenel/karakuri/internal/core/container"
	featurecontainer "github.com/bsenel/karakuri/internal/feature/container"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// seedTenancy builds a database with two organisations whose teams are both
// called "eng", and hands back the path so the server can be started against
// it.
//
// The containers have to exist before the server boots, because the role map
// resolves their names to IDs at startup — which is the point: a name that
// matches nothing is a configuration error an operator reads, not a login that
// silently grants less than intended.
func seedTenancy(t *testing.T) (dbPath string, svc *featurecontainer.Service, acmeEng, globexEng corecontainer.Container) {
	t.Helper()
	dbPath = filepath.Join(t.TempDir(), "tenancy.db")
	db, err := platformdb.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, dbPath); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc = featurecontainer.NewService(storage.NewGORMStorage(db))

	ctx := context.Background()
	mk := func(kind corecontainer.Kind, name, parent string) corecontainer.Container {
		c, err := svc.Create(ctx, featurecontainer.CreateRequest{Kind: kind, Name: name, ParentID: parent})
		if err != nil {
			t.Fatalf("create %s %q: %v", kind, name, err)
		}
		return c
	}
	acme := mk(corecontainer.KindOrg, "acme", "")
	globex := mk(corecontainer.KindOrg, "globex", "")
	return dbPath, svc, mk(corecontainer.KindTeam, "eng", acme.ID), mk(corecontainer.KindTeam, "eng", globex.ID)
}

// managedBindingScopes reads the scopes of the bindings the provisioner owns,
// straight out of the table, because what a login wrote is the thing under
// test — not what an API is willing to report about it.
func managedBindingScopes(t *testing.T, dbPath string) []string {
	t.Helper()
	db, err := platformdb.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	var scopes []string
	err = db.Raw(`SELECT scope FROM auth_role_bindings WHERE id LIKE ? ORDER BY scope`,
		extauth.ManagedBindingPrefix+"%").Scan(&scopes).Error
	if err != nil {
		t.Fatalf("read bindings: %v", err)
	}
	return scopes
}

func createTwin(t *testing.T, baseURL, token, name string) string {
	t.Helper()
	resp := doJSON(t, token, http.MethodPost, baseURL+"/api/v1/twins",
		map[string]any{"name": name, "kind": "person", "domain": "software"})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var twin struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&twin); err != nil {
		t.Fatalf("decode twin: %v", err)
	}
	if twin.ID == "" {
		t.Fatal("twin came back with no id")
	}
	return twin.ID
}

func createObjective(t *testing.T, baseURL, token, twinID, title string) string {
	t.Helper()
	resp := doJSON(t, token, http.MethodPost, baseURL+"/api/v1/objectives",
		map[string]any{"title": title, "domain": "software", "twin_id": twinID})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var obj struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		t.Fatalf("decode objective: %v", err)
	}
	if obj.ID == "" {
		t.Fatal("objective came back with no id")
	}
	return obj.ID
}

// listIDs reads a collection endpoint with the federated session in the jar.
func listIDs(t *testing.T, client *http.Client, url string) []string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("list %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list %s = %d, want 200", url, resp.StatusCode)
	}
	return decodeIDs(t, resp)
}

func listIDsWithToken(t *testing.T, token, url string) []string {
	t.Helper()
	resp := doJSON(t, token, http.MethodGet, url, nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	return decodeIDs(t, resp)
}

func decodeIDs(t *testing.T, resp *http.Response) []string {
	t.Helper()
	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ID)
	}
	return out
}

// The hole Phase 16 opened, closed and pinned: a federated user mapped into one
// team may read that team's twin and nothing in the other tenant — even though
// both teams are called "eng".
func TestFederatedGrantIsConfinedToItsTeam(t *testing.T) {
	dbPath, svc, acmeEng, globexEng := seedTenancy(t)

	idp := newStubIdP(t, "karakuri", []string{"acme-engineers"})
	baseURL, adminToken, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Database.DSN = dbPath
		cfg.Auth.Provider = config.AuthProviderOIDC
		cfg.Auth.Frontend.LoginRedirect = "/dashboard"
		cfg.Auth.OIDC.IssuerURL = idp.server.URL
		cfg.Auth.OIDC.ClientID = idp.clientID
		cfg.Auth.OIDC.ClientSecret = "shh"
		cfg.Auth.Frontend.PublicURL = "http://127.0.0.1"
		cfg.Auth.OIDC.RedirectURL = "http://127.0.0.1/api/v1/auth/sso/callback"
		cfg.Auth.RoleMap.Groups = map[string][]config.AuthRoleGrantConfig{
			"acme-engineers": {{Role: "operator", Org: "acme", Team: "eng"}},
		}
	})
	defer cleanup()

	acmeTwin := createTwin(t, baseURL, adminToken, "acme twin")
	globexTwin := createTwin(t, baseURL, adminToken, "globex twin")

	ctx := context.Background()
	if err := svc.SetResourceContainers(ctx, "twin", acmeTwin, []string{acmeEng.ID}); err != nil {
		t.Fatalf("place acme twin: %v", err)
	}
	if err := svc.SetResourceContainers(ctx, "twin", globexTwin, []string{globexEng.ID}); err != nil {
		t.Fatalf("place globex twin: %v", err)
	}

	client := federatedClient(t)
	resp := completeLogin(t, client, baseURL)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login = %d, want a redirect into the app", resp.StatusCode)
	}

	// The binding the login produced names the team, not everything. Before
	// this, every federated user landed with Scope "*" — a directory group of
	// two hundred people was two hundred globally-scoped principals.
	if got := managedBindingScopes(t, dbPath); len(got) != 1 || got[0] != acmeEng.Label() {
		t.Fatalf("managed binding scopes = %v, want just %s", got, acmeEng.Label())
	}

	get := func(id string) int {
		req, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/twins/"+id, nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("get twin: %v", err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	if code := get(acmeTwin); code != http.StatusOK {
		t.Errorf("GET acme twin = %d, want 200 — the grant covers this team", code)
	}
	if code := get(globexTwin); code != http.StatusForbidden {
		t.Errorf("GET globex twin = %d, want 403 — the other tenant has a team called eng too", code)
	}

	// And the isolation is symmetric: the labels really are different.
	if acmeEng.Label() == globexEng.Label() {
		t.Fatal("two teams called eng share a label")
	}
}

// The isolation fix. GET /twins returned every twin regardless of who asked —
// per-resource denial is not isolation while the listing is all-or-nothing.
func TestListingIsConfinedToTheCallersTenant(t *testing.T) {
	dbPath, svc, acmeEng, globexEng := seedTenancy(t)

	idp := newStubIdP(t, "karakuri", []string{"acme-engineers"})
	baseURL, adminToken, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Database.DSN = dbPath
		cfg.Auth.Provider = config.AuthProviderOIDC
		cfg.Auth.Frontend.LoginRedirect = "/dashboard"
		cfg.Auth.OIDC.IssuerURL = idp.server.URL
		cfg.Auth.OIDC.ClientID = idp.clientID
		cfg.Auth.OIDC.ClientSecret = "shh"
		cfg.Auth.Frontend.PublicURL = "http://127.0.0.1"
		cfg.Auth.OIDC.RedirectURL = "http://127.0.0.1/api/v1/auth/sso/callback"
		cfg.Auth.RoleMap.Groups = map[string][]config.AuthRoleGrantConfig{
			"acme-engineers": {{Role: "operator", Org: "acme", Team: "eng"}},
		}
	})
	defer cleanup()

	ctx := context.Background()
	acmeTwin := createTwin(t, baseURL, adminToken, "acme twin")
	globexTwin := createTwin(t, baseURL, adminToken, "globex twin")
	looseTwin := createTwin(t, baseURL, adminToken, "twin in no container")
	if err := svc.SetResourceContainers(ctx, "twin", acmeTwin, []string{acmeEng.ID}); err != nil {
		t.Fatalf("place acme twin: %v", err)
	}
	if err := svc.SetResourceContainers(ctx, "twin", globexTwin, []string{globexEng.ID}); err != nil {
		t.Fatalf("place globex twin: %v", err)
	}

	// An objective on each twin, which inherits its twin's containers.
	acmeObj := createObjective(t, baseURL, adminToken, acmeTwin, "acme work")
	globexObj := createObjective(t, baseURL, adminToken, globexTwin, "globex work")

	client := federatedClient(t)
	resp := completeLogin(t, client, baseURL)
	_ = resp.Body.Close()

	twins := listIDs(t, client, baseURL+"/api/v1/twins")
	if !slices.Contains(twins, acmeTwin) {
		t.Errorf("twins = %v, want acme's twin", twins)
	}
	if slices.Contains(twins, globexTwin) {
		t.Errorf("twins = %v, want globex's twin hidden", twins)
	}
	// A twin in no container belongs to no tenant, so a team-scoped grant does
	// not reach it — the same answer GET /twins/{id} gives.
	if slices.Contains(twins, looseTwin) {
		t.Errorf("twins = %v, want the uncontained twin hidden from a team-scoped grant", twins)
	}

	objectives := listIDs(t, client, baseURL+"/api/v1/objectives")
	if !slices.Contains(objectives, acmeObj) {
		t.Errorf("objectives = %v, want acme's", objectives)
	}
	if slices.Contains(objectives, globexObj) {
		t.Errorf("objectives = %v, want globex's hidden — inherited from its twin", objectives)
	}

	// The listing agrees with the per-resource check on every row, which is the
	// property that makes it a narrowing rather than a second authority.
	for _, id := range twins {
		req, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/twins/"+id, nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		got, err := client.Do(req)
		if err != nil {
			t.Fatalf("get twin: %v", err)
		}
		_ = got.Body.Close()
		if got.StatusCode != http.StatusOK {
			t.Errorf("listed twin %s answers %d on a direct read", id, got.StatusCode)
		}
	}

	// The administrator holds "*" and still sees everything, so filtering did
	// not become a second denial path.
	all := listIDsWithToken(t, adminToken, baseURL+"/api/v1/twins")
	for _, id := range []string{acmeTwin, globexTwin, looseTwin} {
		if !slices.Contains(all, id) {
			t.Errorf("admin listing = %v, missing %s", all, id)
		}
	}
}

// A deployment that never creates a container behaves exactly as it did before
// Phase 17: an unscoped grant reaches every twin, and no lookup changes a
// decision.
func TestUnscopedGrantsAreUnchanged(t *testing.T) {
	idp := newStubIdP(t, "karakuri", []string{"karakuri-operators"})
	baseURL, adminToken, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Auth.Provider = config.AuthProviderOIDC
		cfg.Auth.Frontend.LoginRedirect = "/dashboard"
		cfg.Auth.OIDC.IssuerURL = idp.server.URL
		cfg.Auth.OIDC.ClientID = idp.clientID
		cfg.Auth.OIDC.ClientSecret = "shh"
		cfg.Auth.Frontend.PublicURL = "http://127.0.0.1"
		cfg.Auth.OIDC.RedirectURL = "http://127.0.0.1/api/v1/auth/sso/callback"
		cfg.Auth.RoleMap.Groups = roleGrants(map[string][]string{
			"karakuri-operators": {"operator"},
		})
	})
	defer cleanup()

	twinID := createTwin(t, baseURL, adminToken, "unowned twin")

	client := federatedClient(t)
	resp := completeLogin(t, client, baseURL)
	_ = resp.Body.Close()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/twins/"+twinID, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	got, err := client.Do(req)
	if err != nil {
		t.Fatalf("get twin: %v", err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("GET twin = %d, want 200 — an unscoped grant still reaches a twin in no container", got.StatusCode)
	}
}
