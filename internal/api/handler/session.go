package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/bsenel/karakuri/internal/core/entity"
	"github.com/bsenel/karakuri/internal/feature/orchestrator"
	"github.com/bsenel/karakuri/internal/feature/session"
	"github.com/go-chi/chi/v5"
)

type SessionHandler struct {
	Sessions     *session.Service
	Orchestrator *orchestrator.Service
}

type createSessionReq struct {
	Mode      string `json:"mode"`
	Input     string `json:"input"`
	ParentSHA string `json:"parent_sha,omitempty"`
}

func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createSessionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sess, err := h.Sessions.Create(r.Context(), session.CreateRequest{
		Mode: entity.SessionMode(req.Mode), Input: req.Input, ParentSHA: req.ParentSHA,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, sess)
}

func (h *SessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	sha := chi.URLParam(r, "sha")
	sess, err := h.Sessions.Get(r.Context(), sha)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, sess)
}

func (h *SessionHandler) List(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	sessions, err := h.Sessions.List(r.Context(), mode, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, sessions)
}

func (h *SessionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	sha := chi.URLParam(r, "sha")
	if err := h.Sessions.Delete(r.Context(), sha); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SessionHandler) Run(w http.ResponseWriter, r *http.Request) {
	sha := chi.URLParam(r, "sha")
	go func() {
		ctx := context.Background()
		_ = h.Orchestrator.Run(ctx, sha)
	}()
	writeJSON(w, map[string]string{"status": "started", "session_sha": sha})
}

func (h *SessionHandler) Status(w http.ResponseWriter, r *http.Request) {
	sha := chi.URLParam(r, "sha")
	state, err := h.Orchestrator.GetStatus(r.Context(), sha)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]string{"session_sha": sha, "state": string(state)})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
