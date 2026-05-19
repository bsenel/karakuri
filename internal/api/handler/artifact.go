package handler

import (
	"encoding/json"
	"net/http"

	"github.com/bsenel/karakuri/internal/feature/artifact"
	"github.com/go-chi/chi/v5"
)

type ArtifactHandler struct {
	Artifacts *artifact.Service
}

func (h *ArtifactHandler) Write(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ObjectiveID string `json:"objective_id"`
		AgentID     string `json:"agent_id"`
		Capability  string `json:"capability"`
		Content     string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	art, err := h.Artifacts.Write(r.Context(), req.ObjectiveID, req.AgentID, req.Capability, []byte(req.Content))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, art)
}

func (h *ArtifactHandler) Get(w http.ResponseWriter, r *http.Request) {
	sha := chi.URLParam(r, "sha")
	content, _, err := h.Artifacts.Read(r.Context(), sha)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write(content)
}

func (h *ArtifactHandler) List(w http.ResponseWriter, r *http.Request) {
	objectiveID := r.URL.Query().Get("objective_id")
	agentID := r.URL.Query().Get("agent_id")
	arts, err := h.Artifacts.List(r.Context(), objectiveID, agentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, arts)
}

func (h *ArtifactHandler) Diff(w http.ResponseWriter, r *http.Request) {
	shaA := chi.URLParam(r, "sha")
	shaB := chi.URLParam(r, "other")
	diff, err := h.Artifacts.Diff(r.Context(), shaA, shaB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"diff": diff})
}
