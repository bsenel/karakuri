package handler

import (
	"encoding/json"
	"net/http"

	"github.com/bsenel/karakuri/internal/feature/research"
)

type ResearchHandler struct {
	Research *research.Service
}

func (h *ResearchHandler) Run(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TwinID      string   `json:"twin_id"`
		ObjectiveID string   `json:"objective_id"`
		AgentID     string   `json:"agent_id"`
		Topic       string   `json:"topic"`
		Sources     []string `json:"sources,omitempty"`
		Depth       string   `json:"depth,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := h.Research.Run(r.Context(), research.Request{
		TwinID:      req.TwinID,
		ObjectiveID: req.ObjectiveID,
		AgentID:     req.AgentID,
		Topic:       req.Topic,
		Sources:     req.Sources,
		Depth:       req.Depth,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}
