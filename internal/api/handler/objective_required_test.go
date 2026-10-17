package handler_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/api/handler"
	"github.com/bsenel/karakuri/internal/core/objective"
	featureobj "github.com/bsenel/karakuri/internal/feature/objective"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

func objectiveHandler(t *testing.T) *handler.ObjectiveHandler {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "objective.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := featureobj.NewService(storage.NewGORMStorage(db))
	svc.RegisterTemplate(objective.Template{ID: "tmpl.tested", SuccessCriteria: []objective.Criterion{
		{ID: "tested", Description: "the tests pass"},
	}})
	return &handler.ObjectiveHandler{Objectives: svc}
}

// Phase 41 Step 1: an objective with no required criterion is the caller's
// mistake, so it is a 400 that says what is missing, not a 500.
func TestCreateObjectiveWithNoRequiredCriterionIsABadRequest(t *testing.T) {
	h := objectiveHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/objectives",
		strings.NewReader(`{"title":"plain","domain":"software"}`))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "required success criterion") {
		t.Errorf("body %q does not say what is missing", rec.Body)
	}
}

// And one with a required criterion is created as before.
func TestCreateObjectiveWithARequiredCriterionSucceeds(t *testing.T) {
	h := objectiveHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/objectives",
		strings.NewReader(`{"title":"work","domain":"software","template_id":"tmpl.tested"}`))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
}
