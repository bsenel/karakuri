package handler

import (
	"encoding/json"
	"net/http"

	"github.com/bsenel/karakuri/internal/core/objective"
	featureobj "github.com/bsenel/karakuri/internal/feature/objective"
	"github.com/go-chi/chi/v5"
)

type ObjectiveHandler struct {
	Objectives *featureobj.Service
}

func (h *ObjectiveHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Domain      string `json:"domain"`
		Priority    int    `json:"priority"`
		TwinID      string `json:"twin_id"`
		TemplateID  string `json:"template_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	obj, err := h.Objectives.Create(r.Context(), featureobj.CreateRequest{
		Title:       req.Title,
		Description: req.Description,
		Domain:      req.Domain,
		Priority:    req.Priority,
		TwinID:      req.TwinID,
		TemplateID:  req.TemplateID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, obj)
}

func (h *ObjectiveHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	obj, err := h.Objectives.Get(r.Context(), objective.ObjectiveID(id))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, obj)
}

func (h *ObjectiveHandler) List(w http.ResponseWriter, r *http.Request) {
	twinID := r.URL.Query().Get("twin_id")
	status := r.URL.Query().Get("status")
	objs, err := h.Objectives.List(r.Context(), twinID, status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, objs)
}

func (h *ObjectiveHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.Objectives.UpdateStatus(r.Context(), objective.ObjectiveID(id), objective.ObjectiveStatus(req.Status)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": req.Status})
}

func (h *ObjectiveHandler) ListTemplates(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, h.Objectives.ListTemplates())
}
