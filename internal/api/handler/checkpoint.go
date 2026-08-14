package handler

import (
	"encoding/json"
	"net/http"

	"github.com/bsenel/karakuri/auth"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	"github.com/go-chi/chi/v5"
)

type CheckpointHandler struct {
	Checkpoints *featurecp.Service
}

func (h *CheckpointHandler) ListPending(w http.ResponseWriter, r *http.Request) {
	twinID := r.URL.Query().Get("twin_id")
	cps, err := h.Checkpoints.ListPending(r.Context(), twinID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, cps)
}

func (h *CheckpointHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cp, err := h.Checkpoints.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, cp)
}

// resolveRequest is the body of POST /checkpoints/{id}/resolve.
// Phase 13.5 adds the modifications block so operators can drop actions,
// add constraints, and assert a confidence floor in a single call.
type resolveRequest struct {
	Decision string `json:"decision"`
	// Choice is accepted as an alias for Decision so direct callers that
	// mirror the Decision struct shape (choice/note/approver) work too.
	Choice        string                        `json:"choice"`
	Note          string                        `json:"note"`
	Approver      string                        `json:"approver"`
	Modifications *corecheckpoint.Modifications `json:"modifications,omitempty"`
}

func (h *CheckpointHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req resolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	choice := req.Decision
	if choice == "" {
		choice = req.Choice
	}
	// The approver is who the audit trail says authorized this — so it must be
	// the authenticated caller, not a self-asserted body field. See F-10.
	approver := req.Approver
	if p, ok := auth.PrincipalFromContext(r.Context()); ok {
		approver = p.ID
	}
	if err := h.Checkpoints.Resolve(r.Context(), id, corecheckpoint.Decision{
		Choice:        choice,
		Note:          req.Note,
		Approver:      approver,
		Modifications: req.Modifications,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "resolved"})
}
