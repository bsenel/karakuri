package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/bsenel/karakuri/internal/feature/audit"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/go-chi/chi/v5"
)

// AuditHandler serves the authority-bounds audit log (Phase 13). Reads
// tool_events filtered by the supplied query string. Listed event Kinds:
// "execute", "escalation", "approval".
type AuditHandler struct {
	Store  storage.StorageAdapter
	Export auditExporter
}

// auditExporter assembles the audit export for a closed window.
type auditExporter interface {
	Export(ctx context.Context, from, to, now time.Time) ([]byte, error)
}

// auditBoundError is a window bound that is missing or is not a timestamp:
// refused before the exporter is asked.
type auditBoundError string

func (e auditBoundError) Error() string { return string(e) }

// exportAuditWindow parses the bounds of a window and returns the exporter's
// document for it, untouched. GET /audit/export and the audit_export MCP tool
// both answer with this.
func exportAuditWindow(ctx context.Context, exp auditExporter, rawFrom, rawTo string) ([]byte, error) {
	from, err := time.Parse(time.RFC3339, rawFrom)
	if err != nil {
		return nil, auditBoundError("from must be an RFC3339 timestamp")
	}
	to, err := time.Parse(time.RFC3339, rawTo)
	if err != nil {
		return nil, auditBoundError("to must be an RFC3339 timestamp")
	}
	return exp.Export(ctx, from, to, time.Now())
}

// ExportWindow returns the audit export for one window.
//
// GET /api/v1/audit/export?from=RFC3339&to=RFC3339
func (h *AuditHandler) ExportWindow(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	data, err := exportAuditWindow(r.Context(), h.Export, q.Get("from"), q.Get("to"))
	if err != nil {
		var bound auditBoundError
		if errors.As(err, &bound) || errors.Is(err, audit.ErrWindow) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// The exporter's bytes are the document: written as they are, so two
	// requests for one window are byte-identical.
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data) // #nosec G705 -- a JSON document the exporter built from stored rows, served as application/json with nosniff (middleware/security.go); never rendered as HTML
}

// listAuditEvents reads the audit log through the filters a caller supplied as
// text, keyed as the query string of GET /audit keys them. The REST route and
// the audit_list MCP tool both list through this, so neither can apply a
// filter the other drops.
func listAuditEvents(ctx context.Context, store storage.StorageAdapter, get func(string) string) ([]storage.ToolEvent, error) {
	f := storage.ToolEventFilter{
		ObjectiveID: get("objective_id"),
		AgentID:     get("agent_id"),
		Kind:        get("kind"),
		Provider:    get("provider"),
		Model:       get("model"),
		TemplateID:  get("template"),
	}

	if v := get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = n
		}
	}
	if v := get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.CreatedAtSince = &t
		}
	}
	if v := get("bounds_violation"); v != "" {
		b := v == "true" || v == "1"
		f.BoundsViolation = &b
	}
	if f.Limit == 0 {
		f.Limit = 100
	}

	return store.ListToolEvents(ctx, f)
}

func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	events, err := listAuditEvents(r.Context(), h.Store, r.URL.Query().Get)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, events)
}

// Get returns one audit row.
//
// GET /api/v1/audit/{id}
func (h *AuditHandler) Get(w http.ResponseWriter, r *http.Request) {
	event, err := h.Store.GetToolEvent(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		// Not-found and unreadable are the same answer here on purpose: an
		// audit log that distinguishes them tells an unauthorized prober which
		// IDs exist.
		http.Error(w, "no such audit event", http.StatusNotFound)
		return
	}
	writeJSON(w, event)
}
