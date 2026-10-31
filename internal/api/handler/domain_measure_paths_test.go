package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
)

// docs/roadmap.md, Phase 41 step 9: GET /domains/capabilities writes the
// registry's capability.Capability values as they are, which is what
// `krk verifier list` reads, so a verifier's declared measure paths are on it
// as measure_paths.
func TestListCapabilitiesCarriesMeasurePaths(t *testing.T) {
	reg := capability.NewRegistry()
	if err := reg.Register(capability.Capability{
		ID: "software.verify.run_tests", Domain: "software", Verifiable: true,
		MeasurePaths: []string{"**/*_test.go"},
	}); err != nil {
		t.Fatal(err)
	}
	h := &DomainHandler{Capabilities: reg}

	rec := httptest.NewRecorder()
	h.ListCapabilities(rec, httptest.NewRequest(http.MethodGet, "/api/v1/domains/capabilities", nil))

	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d capabilities, want 1", len(got))
	}
	paths, ok := got[0]["measure_paths"].([]any)
	if !ok || len(paths) != 1 || paths[0] != "**/*_test.go" {
		t.Errorf("measure_paths = %#v, want [\"**/*_test.go\"]", got[0]["measure_paths"])
	}
}
