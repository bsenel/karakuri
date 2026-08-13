package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/go-chi/chi/v5"
)

// AuditHandler serves the authority-bounds audit log (Phase 13). Reads
// tool_events filtered by the supplied query string. Listed event Kinds:
// "execute", "escalation", "approval".
type AuditHandler struct {
	Store storage.StorageAdapter
}

func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := storage.ToolEventFilter{
		ObjectiveID: q.Get("objective_id"),
		AgentID:     q.Get("agent_id"),
		Kind:        q.Get("kind"),
	}

	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = n
		}
	}
	if v := q.Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.CreatedAtSince = &t
		}
	}
	if v := q.Get("bounds_violation"); v != "" {
		b := v == "true" || v == "1"
		f.BoundsViolation = &b
	}
	if f.Limit == 0 {
		f.Limit = 100
	}

	events, err := h.Store.ListToolEvents(r.Context(), f)
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
