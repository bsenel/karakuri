package integration_test

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/bsenel/karakuri/config"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

// quotaConfig reads what the server says it is enforcing, and what it was
// configured with.
func quotaConfig(t *testing.T, baseURL, token string) (inForce, configured int, editable bool) {
	t.Helper()
	resp := doJSON(t, token, http.MethodGet, baseURL+"/api/v1/quota", nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		LLMTokens  struct{ Cap int } `json:"llm_tokens"`
		Editable   bool              `json:"editable"`
		Configured struct {
			LLMTokens struct{ Cap int } `json:"llm_tokens"`
		} `json:"configured"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode quota config: %v", err)
	}
	return body.LLMTokens.Cap, body.Configured.LLMTokens.Cap, body.Editable
}

func setTier(t *testing.T, baseURL, token, name string, body map[string]any) *http.Response {
	t.Helper()
	return doJSON(t, token, http.MethodPut, baseURL+"/api/v1/quota/tiers/"+name, body)
}

// The phase's central claim: an edited limit is what the server enforces, it
// survives a restart, and the configured value is still visible beside it.
//
// That last part is not decoration. Once the database wins, an operator reading
// llm_tokens_per_day in a YAML file is reading something that may not be true,
// and the only defence is that the server reports both.
func TestStoredTierSurvivesARestartAndConfigurationIsStillVisible(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tiers.db")
	withDB := func(cfg *config.Config) {
		cfg.Database.DSN = dbPath
		cfg.Quota.LLMTokensPerDay = 1_000_000
	}

	baseURL, adminToken, cleanup := startServerWith(t, withDB)

	inForce, configured, editable := quotaConfig(t, baseURL, adminToken)
	if inForce != 1_000_000 || configured != 1_000_000 {
		t.Fatalf("before any edit: in force %d, configured %d — want both at the file's value", inForce, configured)
	}
	if !editable {
		t.Fatal("a deployment with a database reported its limits as uneditable")
	}

	resp := setTier(t, baseURL, adminToken, "llm-tokens", map[string]any{
		"cap": 5_000_000, "reason": "the team grew to twelve",
	})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Immediately, not a cache TTL later: the writing process invalidates.
	inForce, configured, _ = quotaConfig(t, baseURL, adminToken)
	if inForce != 5_000_000 {
		t.Errorf("in force = %d right after the edit, want 5000000", inForce)
	}
	if configured != 1_000_000 {
		t.Errorf("configured = %d, want the file's 1000000 — the file did not change", configured)
	}
	cleanup()

	// A new process against the same database. This is what "the database is
	// the source of truth" has to mean; a limit that lived only in one server's
	// memory would be a worse version of the config file.
	baseURL, adminToken, cleanup = startServerWith(t, withDB)
	defer cleanup()

	inForce, configured, _ = quotaConfig(t, baseURL, adminToken)
	if inForce != 5_000_000 {
		t.Errorf("in force = %d after a restart, want the stored 5000000", inForce)
	}
	if configured != 1_000_000 {
		t.Errorf("configured = %d after a restart, want 1000000", configured)
	}

	// Stored tiers are readable on their own, with who set them and why — which
	// is what makes a limit reviewable after the fact.
	resp = doJSON(t, adminToken, http.MethodGet, baseURL+"/api/v1/quota/tiers", nil)
	assertStatus(t, resp, http.StatusOK)
	var listed struct {
		Stored []struct {
			Name      string `json:"name"`
			Cap       int    `json:"cap"`
			Reason    string `json:"reason"`
			UpdatedBy string `json:"updated_by"`
		} `json:"stored"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode tiers: %v", err)
	}
	resp.Body.Close()
	if len(listed.Stored) != 1 {
		t.Fatalf("stored tiers = %+v, want the one that was set", listed.Stored)
	}
	if got := listed.Stored[0]; got.Cap != 5_000_000 || got.Reason == "" || got.UpdatedBy == "" {
		t.Errorf("stored tier = %+v, want the cap, the reason and who set it", got)
	}

	// And reset returns the tier to the file, which is what keeps the file
	// meaningful rather than vestigial.
	resp = doJSON(t, adminToken, http.MethodDelete, baseURL+"/api/v1/quota/tiers/llm-tokens", nil)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	if inForce, _, _ = quotaConfig(t, baseURL, adminToken); inForce != 1_000_000 {
		t.Errorf("in force = %d after reset, want the configured 1000000", inForce)
	}
}

// Editing a limit for the whole deployment is quota:admin, not the tenant-scoped
// quota:approve — an operator who may approve a raise for their own team is not
// thereby able to move everybody's ceiling.
func TestEditingATierNeedsQuotaAdmin(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tiers.db")
	baseURL, adminToken, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Database.DSN = dbPath
	})
	defer cleanup()

	operatorToken := createUser(t, baseURL, adminToken, "olive", karakuriauth.RoleOperator)
	resp := setTier(t, baseURL, operatorToken, "llm-tokens", map[string]any{
		"cap": 9_000_000, "reason": "I would like more",
	})
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	resp = doJSON(t, operatorToken, http.MethodDelete, baseURL+"/api/v1/quota/tiers/llm-tokens", nil)
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	// Reading what is in force stays available to everybody: knowing the limit
	// is not the same as being able to move it.
	resp = doJSON(t, operatorToken, http.MethodGet, baseURL+"/api/v1/quota/tiers", nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

// A change nobody wrote a reason for is refused, and so is a tier nobody
// enforces. Both would otherwise store a row that is silently ignored.
func TestTierEditsAreValidated(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tiers.db")
	baseURL, adminToken, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Database.DSN = dbPath
	})
	defer cleanup()

	for _, tc := range []struct {
		name string
		tier string
		body map[string]any
	}{
		{"no reason", "llm-tokens", map[string]any{"cap": 9_000_000}},
		{"no cap", "llm-tokens", map[string]any{"reason": "why not"}},
		{"unknown tier", "storage", map[string]any{"cap": 10, "reason": "why not"}},
		{"window on a daily quota", "adapter", map[string]any{
			"cap": 10, "window": "1m", "reason": "why not"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := setTier(t, baseURL, adminToken, tc.tier, tc.body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}
