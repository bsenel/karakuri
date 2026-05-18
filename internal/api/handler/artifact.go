package handler

import (
	"net/http"

	"github.com/bsenel/karakuri/internal/feature/artifact"
	"github.com/go-chi/chi/v5"
)

type ArtifactHandler struct {
	Artifacts *artifact.Service
}

func (h *ArtifactHandler) Get(w http.ResponseWriter, r *http.Request) {
	sha := chi.URLParam(r, "sha")
	content, err := h.Artifacts.Read(r.Context(), sha)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write(content)
}

func (h *ArtifactHandler) ListBySession(w http.ResponseWriter, r *http.Request) {
	sha := chi.URLParam(r, "sha")
	arts, err := h.Artifacts.List(r.Context(), sha)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, arts)
}

func (h *ArtifactHandler) Diff(w http.ResponseWriter, r *http.Request) {
	shaA := chi.URLParam(r, "sha")
	shaB := chi.URLParam(r, "other-sha")
	diff, err := h.Artifacts.Diff(r.Context(), shaA, shaB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"diff": diff})
}
