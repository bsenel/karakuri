package handler

import (
	"net/http"

	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/go-chi/chi/v5"
)

type ReviewHandler struct {
	Store storage.StorageAdapter
}

func (h *ReviewHandler) ListBySession(w http.ResponseWriter, r *http.Request) {
	sha := chi.URLParam(r, "sha")
	reviews, err := h.Store.GetReviews(r.Context(), sha)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, reviews)
}

func (h *ReviewHandler) Get(w http.ResponseWriter, r *http.Request) {
	sha := chi.URLParam(r, "sha")
	arts, err := h.Store.QueryArtifacts(r.Context(), storage.ArtifactFilter{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	for _, a := range arts {
		if a.SHA == sha {
			writeJSON(w, a)
			return
		}
	}
	http.NotFound(w, r)
}
