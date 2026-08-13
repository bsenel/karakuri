package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bsenel/karakuri/config"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

// Revoking is bounded by the same rule as granting: you can only take away a
// scope you already hold.
//
// This is not the harmless direction. A raise somebody is relying on, removed by
// an administrator of another tenant, is that tenant's work stopping — a tenancy
// model that stops at reads is not one.
func TestRevokingABindingNeedsTheScope(t *testing.T) {
	dbPath, svc, acmeEng, globexEng := seedTenancy(t)
	baseURL, adminToken, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Database.DSN = dbPath
	})
	defer cleanup()

	ctx := context.Background()
	globexTwin := createTwin(t, baseURL, adminToken, "globex twin")
	if err := svc.SetResourceContainers(ctx, "twin", globexTwin, []string{globexEng.ID}); err != nil {
		t.Fatalf("place globex twin: %v", err)
	}

	// A binding in the other tenant, which is what ann must not be able to
	// touch.
	resp := doJSON(t, adminToken, http.MethodPost, baseURL+"/api/v1/auth/users", map[string]any{
		"id": "gwen", "roles": []string{karakuriauth.RoleViewer},
		"scope": globexEng.Label(), "password": "correct-horse-battery-staple",
	})
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = doJSON(t, adminToken, http.MethodGet, baseURL+"/api/v1/auth/bindings?principal=gwen", nil)
	assertStatus(t, resp, http.StatusOK)
	var bindings []struct {
		ID    string `json:"id"`
		Scope string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&bindings); err != nil {
		t.Fatalf("decode bindings: %v", err)
	}
	resp.Body.Close()
	if len(bindings) != 1 || bindings[0].Scope != globexEng.Label() {
		t.Fatalf("bindings = %+v, want gwen's one binding on globex's team", bindings)
	}

	annToken := createScopedUser(t, baseURL, adminToken, "ann",
		karakuriauth.RoleAdmin, acmeEng.Label())

	resp = doJSON(t, annToken, http.MethodDelete,
		baseURL+"/api/v1/auth/bindings/"+bindings[0].ID, nil)
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	// The administrator holding the wildcard can, which is what makes the
	// refusal above about scope rather than about the route.
	resp = doJSON(t, adminToken, http.MethodDelete,
		baseURL+"/api/v1/auth/bindings/"+bindings[0].ID, nil)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	resp = doJSON(t, adminToken, http.MethodGet, baseURL+"/api/v1/auth/bindings?principal=gwen", nil)
	assertStatus(t, resp, http.StatusOK)
	bindings = nil
	if err := json.NewDecoder(resp.Body).Decode(&bindings); err != nil {
		t.Fatalf("decode bindings: %v", err)
	}
	resp.Body.Close()
	if len(bindings) != 0 {
		t.Errorf("bindings after revoke = %+v, want none", bindings)
	}
}

// Deleting a principal takes its bindings with it. Leaving them behind would
// leave rows granting roles to an identity that no longer exists — harmless
// until somebody recreates the same ID and silently inherits them.
func TestDeletingAUserTakesItsBindings(t *testing.T) {
	baseURL, adminToken, cleanup := startServer(t)
	defer cleanup()

	createUser(t, baseURL, adminToken, "vera", karakuriauth.RoleViewer)

	resp := doJSON(t, adminToken, http.MethodDelete, baseURL+"/api/v1/auth/users/vera", nil)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	resp = doJSON(t, adminToken, http.MethodGet, baseURL+"/api/v1/auth/bindings?principal=vera", nil)
	assertStatus(t, resp, http.StatusOK)
	var bindings []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&bindings); err != nil {
		t.Fatalf("decode bindings: %v", err)
	}
	resp.Body.Close()
	if len(bindings) != 0 {
		t.Errorf("bindings after deleting the principal = %+v, want none", bindings)
	}

	// Deleting again says so rather than pretending.
	resp = doJSON(t, adminToken, http.MethodDelete, baseURL+"/api/v1/auth/users/vera", nil)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// Deleting the account you are signed in as leaves nobody able to undo it, and
// on a single-administrator deployment that is the whole deployment.
func TestAPrincipalCannotDeleteItself(t *testing.T) {
	baseURL, adminToken, cleanup := startServer(t)
	defer cleanup()

	resp := doJSON(t, adminToken, http.MethodDelete, baseURL+"/api/v1/auth/users/admin", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}

	// And the account still works, so the refusal did not half-apply.
	resp2 := doJSON(t, adminToken, http.MethodGet, baseURL+"/api/v1/auth/me", nil)
	defer resp2.Body.Close()
	assertStatus(t, resp2, http.StatusOK)
}

// An override list is a list of who asked for more and why, so it is filtered
// by the same rule that governs approving one.
func TestOverrideListingIsConfinedToTheCallersTenant(t *testing.T) {
	dbPath, svc, acmeEng, globexEng := seedTenancy(t)
	baseURL, adminToken, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Database.DSN = dbPath
	})
	defer cleanup()

	ctx := context.Background()
	acmeTwin := createTwin(t, baseURL, adminToken, "acme twin")
	globexTwin := createTwin(t, baseURL, adminToken, "globex twin")
	if err := svc.SetResourceContainers(ctx, "twin", acmeTwin, []string{acmeEng.ID}); err != nil {
		t.Fatalf("place acme twin: %v", err)
	}
	if err := svc.SetResourceContainers(ctx, "twin", globexTwin, []string{globexEng.ID}); err != nil {
		t.Fatalf("place globex twin: %v", err)
	}

	// One approved raise in each tenant.
	robToken := createUser(t, baseURL, adminToken, "rob", karakuriauth.RoleViewer)
	for _, twin := range []string{acmeTwin, globexTwin} {
		id := submitRaise(t, baseURL, robToken, twin, 5000)
		resp := decide(t, baseURL, adminToken, id, true)
		assertStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	}

	annToken := createScopedUser(t, baseURL, adminToken, "ann",
		karakuriauth.RoleAdmin, acmeEng.Label())

	list := func(token string) []struct {
		Subject string `json:"subject"`
		Name    string `json:"name"`
		Cap     int    `json:"cap"`
	} {
		t.Helper()
		resp := doJSON(t, token, http.MethodGet, baseURL+"/api/v1/quota/overrides", nil)
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusOK)
		var out []struct {
			Subject string `json:"subject"`
			Name    string `json:"name"`
			Cap     int    `json:"cap"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode overrides: %v", err)
		}
		return out
	}

	if got := list(adminToken); len(got) != 2 {
		t.Errorf("admin sees %+v, want both raises", got)
	}
	annSees := list(annToken)
	if len(annSees) != 1 {
		t.Fatalf("ann sees %+v, want only acme's raise", annSees)
	}
	if annSees[0].Subject != "twin|"+acmeTwin {
		t.Errorf("ann sees %q, want acme's twin", annSees[0].Subject)
	}

	// Revoking is bounded the same way: ann cannot take back globex's raise.
	resp := doJSON(t, annToken, http.MethodDelete,
		baseURL+"/api/v1/quota/overrides/twin|"+globexTwin+"/llm-tokens", nil)
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	// Her own tenant's, she can — and the limit goes back at once.
	resp = doJSON(t, annToken, http.MethodDelete,
		baseURL+"/api/v1/quota/overrides/twin|"+acmeTwin+"/llm-tokens", nil)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	if got := llmTokenLimit(t, baseURL, adminToken, acmeTwin); got == 5000 {
		t.Error("the revoked raise is still in force")
	}
	if got := list(adminToken); len(got) != 1 {
		t.Errorf("overrides after revoke = %+v, want only globex's", got)
	}
}
