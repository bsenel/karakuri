package handler

import (
	"encoding/json"
	"net/http"

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

func (h *CheckpointHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.Checkpoints.Resolve(r.Context(), id, corecheckpoint.Decision{Choice: req.Decision}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "resolved"})
}
