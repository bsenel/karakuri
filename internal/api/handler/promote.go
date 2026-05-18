package handler

import (
	"encoding/json"
	"net/http"

	"github.com/bsenel/karakuri/internal/core/entity"
	"github.com/bsenel/karakuri/internal/feature/autonomous"
	"github.com/go-chi/chi/v5"
)

type PromoteHandler struct {
	Autonomous *autonomous.Service
}

type promoteReq struct {
	Via    string `json:"via"`
	DryRun bool   `json:"dry_run"`
}

func (h *PromoteHandler) Promote(w http.ResponseWriter, r *http.Request) {
	fromSHA := chi.URLParam(r, "sha")
	var req promoteReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	sess, err := h.Autonomous.Promote(r.Context(), fromSHA, entity.SessionMode(req.Via), req.DryRun)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, sess)
}
