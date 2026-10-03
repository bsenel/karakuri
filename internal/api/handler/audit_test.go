package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/bsenel/karakuri/internal/api/handler"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// auditStore holds a fixed audit log, records the filter it was asked for, and
// applies the three provenance fields of it the way storage does. It embeds
// the interface for everything the audit handler never calls.
type auditStore struct {
	storage.StorageAdapter
	events []storage.ToolEvent
	filter storage.ToolEventFilter
}

func (s *auditStore) ListToolEvents(_ context.Context, f storage.ToolEventFilter) ([]storage.ToolEvent, error) {
	s.filter = f
	out := []storage.ToolEvent{}
	for _, e := range s.events {
		if f.Provider != "" && e.Provider != f.Provider {
			continue
		}
		if f.Model != "" && e.Model != f.Model {
			continue
		}
		if f.TemplateID != "" && e.TemplateID != f.TemplateID {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// auditLog is three rows that between them differ in each provenance field.
func auditLog() []storage.ToolEvent {
	return []storage.ToolEvent{
		{ID: "ev-a", ObjectiveID: "obj-1", Kind: storage.ToolEventEscalation,
			Provider: "anthropic", Model: "model-a", TemplateID: "tmpl-green-build", AutonomyRung: "propose"},
		{ID: "ev-b", ObjectiveID: "obj-1", Kind: storage.ToolEventExecute,
			Provider: "fallback", Model: "model-b", TemplateID: "tmpl-green-build", AutonomyRung: "act"},
		{ID: "ev-c", ObjectiveID: "obj-2", Kind: storage.ToolEventExecute,
			Provider: "anthropic", Model: "model-b", TemplateID: "tmpl-triage"},
	}
}

func getAudit(t *testing.T, store *auditStore, query string) []map[string]any {
	t.Helper()
	h := &handler.AuditHandler{Store: store}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit"+query, nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("response is not a JSON array: %v: %s", err, rec.Body)
	}
	return rows
}

// The audit response names what produced each decision under the field names
// the OpenAPI spec and the /audit page share.
func TestAuditListCarriesDecisionProvenance(t *testing.T) {
	rows := getAudit(t, &auditStore{events: auditLog()}, "")
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	for field, want := range map[string]string{
		"provider":      "anthropic",
		"model":         "model-a",
		"template_id":   "tmpl-green-build",
		"autonomy_rung": "propose",
	} {
		if got := rows[0][field]; got != want {
			t.Errorf("%s = %v, want %q", field, got, want)
		}
	}
	// A one-shot run has no rung, and says so by omitting it.
	if _, ok := rows[2]["autonomy_rung"]; ok {
		t.Errorf("autonomy_rung = %v on a row that recorded none, want it omitted", rows[2]["autonomy_rung"])
	}
}

// Each query parameter reaches the filter, and the listing comes back narrowed
// by it. A parameter the handler drops lists all three rows.
func TestAuditListFiltersByDecisionProvenance(t *testing.T) {
	cases := map[string]struct {
		query  string
		filter storage.ToolEventFilter
		ids    []string
	}{
		"provider": {
			query:  "?provider=anthropic",
			filter: storage.ToolEventFilter{Provider: "anthropic"},
			ids:    []string{"ev-a", "ev-c"},
		},
		"model": {
			query:  "?model=model-b",
			filter: storage.ToolEventFilter{Model: "model-b"},
			ids:    []string{"ev-b", "ev-c"},
		},
		"template": {
			query:  "?template=tmpl-triage",
			filter: storage.ToolEventFilter{TemplateID: "tmpl-triage"},
			ids:    []string{"ev-c"},
		},
		"all three": {
			query:  "?provider=anthropic&model=model-b&template=tmpl-triage",
			filter: storage.ToolEventFilter{Provider: "anthropic", Model: "model-b", TemplateID: "tmpl-triage"},
			ids:    []string{"ev-c"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := &auditStore{events: auditLog()}
			rows := getAudit(t, store, tc.query)

			if store.filter.Provider != tc.filter.Provider {
				t.Errorf("filter provider = %q, want %q", store.filter.Provider, tc.filter.Provider)
			}
			if store.filter.Model != tc.filter.Model {
				t.Errorf("filter model = %q, want %q", store.filter.Model, tc.filter.Model)
			}
			if store.filter.TemplateID != tc.filter.TemplateID {
				t.Errorf("filter template id = %q, want %q", store.filter.TemplateID, tc.filter.TemplateID)
			}

			ids := make([]string, 0, len(rows))
			for _, r := range rows {
				id, _ := r["id"].(string)
				ids = append(ids, id)
			}
			slices.Sort(ids)
			if !slices.Equal(ids, tc.ids) {
				t.Errorf("listed %v, want %v", ids, tc.ids)
			}
		})
	}
}
