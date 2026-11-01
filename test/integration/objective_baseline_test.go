package integration_test

import (
	"encoding/json"
	"mime"
	"net/http"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
)

// TestObjectiveSetBaseline drives PUT /objectives/{id}/criteria/{criterion_id}/baseline
// through the real app (Phase 41 step 7 part D2). validateResponses checks the
// 200 and the JSON 400 against docs/openapi.yaml; the request body is checked
// here, because the wire check reads responses only.
func TestObjectiveSetBaseline(t *testing.T) {
	// No pack ships a verifier that declares a metric, so the test brings one.
	latency := capability.Capability{
		ID: "software.verify.test_latency", Domain: "software", Verifiable: true,
		Metrics: []capability.Metric{{Name: "p95_latency_ms", Unit: "ms", Direction: capability.DirectionLowerIsBetter}},
	}
	baseURL, adminToken, cleanup := startServerWith(t, nil, latency)
	defer cleanup()

	create := map[string]any{
		"title":  "Keep latency where it started",
		"domain": "software",
		"success_criteria": []map[string]any{
			{"id": "tested", "description": "the tests pass", "kind": "verified", "verifier": "software.verify.run_tests"},
			{
				"id": "latency", "description": "p95 latency is not more than 10% above where it started",
				"kind": "measured", "verifier": string(latency.ID), "metric": "p95_latency_ms",
				"op": "<=", "target": 10, "unit": "ms",
				"baseline": "at_start", "change": "percent",
			},
		},
	}
	assertRequestDocumented(t, http.MethodPost, "/objectives", create)
	resp := doJSON(t, adminToken, http.MethodPost, baseURL+"/api/v1/objectives", create)
	assertStatus(t, resp, http.StatusOK)
	id, _ := decodeJSON(t, resp)["id"].(string)
	if id == "" {
		t.Fatal("created objective has no id")
	}
	path := "/objectives/" + id + "/criteria/latency/baseline"

	t.Run("sets", func(t *testing.T) {
		body := map[string]any{"value": 200}
		assertRequestDocumented(t, http.MethodPut, path, body)

		resp := doJSON(t, adminToken, http.MethodPut, baseURL+"/api/v1"+path, body)
		assertStatus(t, resp, http.StatusOK)
		got := decodeJSON(t, resp)
		criteria, _ := got["success_criteria"].([]any)
		var baseline any
		for _, c := range criteria {
			if m, _ := c.(map[string]any); m["id"] == "latency" {
				baseline = m["baseline_value"]
			}
		}
		if baseline != 200.0 {
			t.Errorf("latency baseline_value = %v, want 200 (objective %v)", baseline, got)
		}
	})

	t.Run("refuses_a_verified_criterion", func(t *testing.T) {
		resp := doJSON(t, adminToken, http.MethodPut,
			baseURL+"/api/v1/objectives/"+id+"/criteria/tested/baseline", map[string]any{"value": 200})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		// validateResponses checks only a JSON body.
		if mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mediaType != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", resp.Header.Get("Content-Type"))
		}
		var refused struct {
			Field string `json:"field"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&refused); err != nil {
			t.Fatalf("decode the 400 body: %v", err)
		}
		if refused.Field != "kind" {
			t.Errorf("field = %q, want kind", refused.Field)
		}
	})
}
