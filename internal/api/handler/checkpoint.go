package handler

import (
	"encoding/json"
	"net/http"

	"github.com/bsenel/karakuri/internal/core/entity"
	"github.com/bsenel/karakuri/internal/feature/checkpoint"
	"github.com/bsenel/karakuri/internal/feature/orchestrator"
	"github.com/go-chi/chi/v5"
)

type CheckpointHandler struct {
	Checkpoints  *checkpoint.Service
	Orchestrator *orchestrator.Service
}

func (h *CheckpointHandler) List(w http.ResponseWriter, r *http.Request) {
	sha := chi.URLParam(r, "sha")
	cps, err := h.Checkpoints.List(r.Context(), sha)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, cps)
}

type resolveReq struct {
	Decision string `json:"decision"`
}

func (h *CheckpointHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	sha := chi.URLParam(r, "sha")
	id := chi.URLParam(r, "id")
	var req resolveReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := h.Orchestrator.ResolveCheckpoint(r.Context(), sha, id, req.Decision); err != nil {
		if err2 := h.Checkpoints.Resolve(r.Context(), id, entity.CheckpointDecision(req.Decision)); err2 != nil {
			http.Error(w, err2.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, map[string]string{"status": "resolved"})
}
