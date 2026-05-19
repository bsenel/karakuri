package handler

import (
	"encoding/json"
	"net/http"

	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/twin"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
	"github.com/go-chi/chi/v5"
)

type LoopHandler struct {
	Loop featureloop.Service
}

func (h *LoopHandler) Start(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ObjectiveID string `json:"objective_id"`
		TwinID      string `json:"twin_id"`
		MaxIter     int    `json:"max_iter"`
		WatchMode   bool   `json:"watch_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := h.Loop.Run(r.Context(), loop.Request{
		Objective: objective.Objective{ID: objective.ObjectiveID(req.ObjectiveID)},
		Twin:      twin.DigitalTwin{ID: req.TwinID},
		MaxIter:   req.MaxIter,
		WatchMode: req.WatchMode,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}

func (h *LoopHandler) Status(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	status, err := h.Loop.Status(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, status)
}

func (h *LoopHandler) Resume(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := h.Loop.Resume(r.Context(), id, corecheckpoint.Decision{Choice: req.Decision})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}
