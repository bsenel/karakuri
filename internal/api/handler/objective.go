package handler

import (
	"encoding/json"
	"net/http"

	"github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/core/objective"
	featureobj "github.com/bsenel/karakuri/internal/feature/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/go-chi/chi/v5"
)

type ObjectiveHandler struct {
	Objectives *featureobj.Service

	// Scopes narrows a listing to the objectives the caller may read, the same
	// way TwinHandler.Scopes does. An objective inherits its twin's containers,
	// so a grant on an organisation reaches the work being done in it.
	Scopes karakuriauth.ScopeAuthorizer
}

func (h *ObjectiveHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title             string   `json:"title"`
		Description       string   `json:"description"`
		Domain            string   `json:"domain"`
		AdditionalDomains []string `json:"additional_domains"`
		Priority          int      `json:"priority"`
		MaxIterations     int      `json:"max_iterations"`
		TwinID            string   `json:"twin_id"`
		TemplateID        string   `json:"template_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	obj, err := h.Objectives.Create(r.Context(), featureobj.CreateRequest{
		Title:             req.Title,
		Description:       req.Description,
		Domain:            req.Domain,
		AdditionalDomains: req.AdditionalDomains,
		Priority:          req.Priority,
		MaxIterations:     req.MaxIterations,
		TwinID:            req.TwinID,
		TemplateID:        req.TemplateID,
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

// List returns the objectives the caller may read. See TwinHandler.List for why
// the filter lives in the handler and why it is a narrowing rather than the
// authority.
func (h *ObjectiveHandler) List(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	visible, hidden, err := karakuriauth.ListFor(
		r.Context(), h.Scopes, principal.ID, karakuriauth.ActionObjectiveRead, "objective")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	objs, err := h.Objectives.List(r.Context(), storage.ObjectiveFilter{
		TwinID:  r.URL.Query().Get("twin_id"),
		Status:  r.URL.Query().Get("status"),
		Visible: visible,
		Hidden:  hidden,
	})
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
