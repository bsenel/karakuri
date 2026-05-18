package handler

import (
	"net/http"

	"github.com/bsenel/karakuri/internal/feature/delivery"
	"github.com/go-chi/chi/v5"
)

type WorktreeHandler struct {
	Delivery *delivery.Service
}

func (h *WorktreeHandler) List(w http.ResponseWriter, r *http.Request) {
	sha := chi.URLParam(r, "sha")
	wts, err := h.Delivery.ListWorktrees(r.Context(), sha)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, wts)
}
