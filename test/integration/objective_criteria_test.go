package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"
)

// TestObjectiveCreateWithSuccessCriteria drives POST /objectives with
// success_criteria through the real app (Phase 41 Step 5). The server wraps the
// API in validateResponses, so both answers are also checked against the status
// and schema docs/openapi.yaml gives them; the request bodies are checked here,
// because the wire check reads responses only.
func TestObjectiveCreateWithSuccessCriteria(t *testing.T) {
	baseURL, adminToken, cleanup := startServer(t)
	defer cleanup()

	t.Run("valid", func(t *testing.T) {
		body := map[string]any{
			"title":  "Criteria Objective",
			"domain": "software",
			"success_criteria": []map[string]any{
				{"id": "reviewed", "description": "an open PR with tests", "kind": "judged", "evidence": "the PR's diff and its test files"},
				{"id": "tested", "description": "the tests pass", "kind": "verified", "verifier": "software.verify.run_tests"},
			},
		}
		assertRequestDocumented(t, http.MethodPost, "/objectives", body)

		resp := doJSON(t, adminToken, http.MethodPost, baseURL+"/api/v1/objectives", body)
		assertStatus(t, resp, http.StatusOK)
		created := decodeJSON(t, resp)
		id, _ := created["id"].(string)
		if id == "" {
			t.Fatalf("created objective has no id: %v", created)
		}

		resp2 := doJSON(t, adminToken, http.MethodGet, baseURL+"/api/v1/objectives/"+id, nil)
		assertStatus(t, resp2, http.StatusOK)
		got := decodeJSON(t, resp2)
		criteria, _ := got["success_criteria"].([]any)
		if len(criteria) != 2 {
			t.Fatalf("success_criteria = %v, want the request's two", got["success_criteria"])
		}
		reviewed, _ := criteria[0].(map[string]any)
		if reviewed["id"] != "reviewed" || reviewed["description"] != "an open PR with tests" ||
			reviewed["evidence"] != "the PR's diff and its test files" {
			t.Errorf("first criterion = %v, want the request's judged one with its description and evidence", reviewed)
		}
		tested, _ := criteria[1].(map[string]any)
		if tested["id"] != "tested" || tested["verifier"] != "software.verify.run_tests" {
			t.Errorf("second criterion = %v, want the request's verified one with its verifier", tested)
		}
	})

	t.Run("judged_without_evidence", func(t *testing.T) {
		body := map[string]any{
			"title":       "Unjudgeable Objective",
			"domain":      "software",
			"template_id": "software.objective.code_review",
			"success_criteria": []map[string]any{
				{"id": "reviewed", "description": "an open PR with tests", "kind": "judged"},
			},
		}
		assertRequestDocumented(t, http.MethodPost, "/objectives", body)

		resp := doJSON(t, adminToken, http.MethodPost, baseURL+"/api/v1/objectives", body)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		// validateResponses checks only a JSON body, so a plain-text 400 would
		// slip past the spec unchecked.
		if mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mediaType != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json so the 400 is checked against docs/openapi.yaml", resp.Header.Get("Content-Type"))
		}
		var refused struct {
			Error string `json:"error"`
			Field string `json:"field"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&refused); err != nil {
			t.Fatalf("decode the 400 body: %v", err)
		}
		if refused.Field != "success_criteria[0].evidence" {
			t.Errorf("field = %q, want success_criteria[0].evidence (error: %s)", refused.Field, refused.Error)
		}
	})
}

// assertRequestDocumented checks a request body against the operation
// docs/openapi.yaml gives method and path, so a field the spec does not list
// fails here rather than going unnoticed by the response-only wire check.
func assertRequestDocumented(t *testing.T, method, path string, body any) {
	t.Helper()
	router, err := loadSpecRouter()
	if err != nil {
		t.Fatalf("load docs/openapi.yaml: %v", err)
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest(method, specHost+apiPrefix+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	route, pathParams, err := router.FindRoute(req)
	if err != nil {
		t.Fatalf("%s %s matches no operation in docs/openapi.yaml: %v", method, path, err)
	}
	input := &openapi3filter.RequestValidationInput{
		Request: req, PathParams: pathParams, Route: route,
		// Who may call it is the auth tests' concern; this is about the body.
		Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, MultiError: true},
	}
	if err := openapi3filter.ValidateRequest(context.Background(), input); err != nil {
		t.Errorf("%s %s sends a body docs/openapi.yaml does not describe: %v", method, path, err)
	}
}
