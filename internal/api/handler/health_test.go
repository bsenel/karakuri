package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bsenel/karakuri/internal/api/handler"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/tools"
)

func TestHealthHandler(t *testing.T) {
	reg := llm.NewRegistry(nil)
	claude, _ := llm.NewClaudeProvider()
	reg.Register(claude)
	exporters := observability.NewExporterRegistry()
	exporters.Register(observability.NewLocalFileExporter(t.TempDir(), "ndjson", "ndjson"))

	h := &handler.HealthHandler{
		Providers: reg, Tools: tools.NewRegistry(), Exporters: exporters,
		Worktrees: nil, RepoPath: ".",
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected ok, got %v", body["status"])
	}
}
