package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bsenel/karakuri/internal/api/handler"
	"github.com/bsenel/karakuri/internal/core/objective"
	featureobj "github.com/bsenel/karakuri/internal/feature/objective"
)

// The template listing is where an operator sees how a pack author regards
// each template before creating an objective from it. Every template carries
// "risk" with the declared value, and one whose author said nothing carries
// the field empty rather than omitting it.
func TestListTemplatesCarriesEachTemplatesRisk(t *testing.T) {
	declared := map[string]objective.RiskClass{
		"tmpl-tidy":        objective.RiskRoutine,
		"tmpl-green-build": objective.RiskConsequential,
		"tmpl-triage":      objective.RiskHigh,
		"tmpl-unsaid":      objective.RiskUnclassified,
	}
	svc := featureobj.NewService(nil)
	for id, risk := range declared {
		svc.RegisterTemplate(objective.Template{ID: id, Title: id, Domain: "test", Risk: risk})
	}
	h := &handler.ObjectiveHandler{Objectives: svc}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/objectives/templates", nil)
	rec := httptest.NewRecorder()
	h.ListTemplates(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	var listed []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("response is not a JSON array: %v: %s", err, rec.Body)
	}
	if len(listed) != len(declared) {
		t.Fatalf("listed %d templates, want %d: %s", len(listed), len(declared), rec.Body)
	}
	for _, tmpl := range listed {
		id, _ := tmpl["id"].(string)
		want, known := declared[id]
		if !known {
			t.Errorf("listed a template nobody registered: %v", tmpl["id"])
			continue
		}
		got, ok := tmpl["risk"]
		if !ok {
			t.Errorf("template %q has no risk field", id)
			continue
		}
		if got != string(want) {
			t.Errorf("template %q risk = %v, want %q", id, got, want)
		}
	}
}
