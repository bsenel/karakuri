package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bsenel/karakuri/internal/feature/research"
)

type ResearchHandler struct {
	Research *research.Service
}

type researchReq struct {
	Topic   string   `json:"topic"`
	Sources []string `json:"sources,omitempty"`
	Depth   string   `json:"depth,omitempty"`
}

func (h *ResearchHandler) Run(w http.ResponseWriter, r *http.Request) {
	var req researchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sess, err := h.Research.Run(r.Context(), research.Request{
		Topic: req.Topic, Sources: req.Sources, Depth: req.Depth,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, sess)
}

func ParseSources(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
