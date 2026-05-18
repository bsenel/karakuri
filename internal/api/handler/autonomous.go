package handler

import (
	"context"
	"net/http"
	"sync"

	"github.com/bsenel/karakuri/internal/core/entity"
	"github.com/bsenel/karakuri/internal/feature/autonomous"
	"github.com/bsenel/karakuri/internal/feature/session"
)

type AutonomousHandler struct {
	Autonomous *autonomous.Service
	Sessions   *session.Service
	mu         sync.Mutex
	paused     bool
}

func (h *AutonomousHandler) Run(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	if h.paused {
		h.mu.Unlock()
		http.Error(w, "autonomous paused", http.StatusConflict)
		return
	}
	h.mu.Unlock()
	sess, err := h.Sessions.Create(r.Context(), session.CreateRequest{Mode: entity.ModeAutonomous, Input: "auto-cycle"})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	go func() { _ = h.Autonomous.RunCycle(context.Background(), sess.SHA) }()
	writeJSON(w, map[string]string{"session_sha": sess.SHA, "status": "running"})
}

func (h *AutonomousHandler) Status(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	paused := h.paused
	h.mu.Unlock()
	writeJSON(w, map[string]bool{"paused": paused})
}

func (h *AutonomousHandler) Pause(w http.ResponseWriter, _ *http.Request) {
	h.mu.Lock()
	h.paused = true
	h.mu.Unlock()
	writeJSON(w, map[string]string{"status": "paused"})
}

func (h *AutonomousHandler) Resume(w http.ResponseWriter, _ *http.Request) {
	h.mu.Lock()
	h.paused = false
	h.mu.Unlock()
	writeJSON(w, map[string]string{"status": "resumed"})
}

