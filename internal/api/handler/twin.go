package handler

import (
	"encoding/json"
	"net/http"

	"github.com/bsenel/karakuri/internal/core/twin"
	featuretwin "github.com/bsenel/karakuri/internal/feature/twin"
	"github.com/go-chi/chi/v5"
)

type TwinHandler struct {
	Twins *featuretwin.Service
}

func (h *TwinHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Kind   string `json:"kind"`
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	t, err := h.Twins.Create(r.Context(), featuretwin.CreateRequest{
		Name:   req.Name,
		Kind:   twin.Kind(req.Kind),
		Domain: req.Domain,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, t)
}

func (h *TwinHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := h.Twins.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, t)
}

func (h *TwinHandler) List(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	domain := r.URL.Query().Get("domain")
	twins, err := h.Twins.List(r.Context(), kind, domain)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, twins)
}

func (h *TwinHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := h.Twins.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var req struct {
		Name            string            `json:"name"`
		Domain          string            `json:"domain"`
		AdapterBindings map[string]string `json:"adapter_bindings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name != "" {
		t.Name = req.Name
	}
	if req.Domain != "" {
		t.Domain = req.Domain
	}
	if req.AdapterBindings != nil {
		t.AdapterBindings = req.AdapterBindings
	}
	if err := h.Twins.Update(r.Context(), t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, t)
}

// SetBindings replaces a twin's adapter bindings outright. PATCH-style merge is
// not supported — callers send the full map. Empty map clears all bindings.
func (h *TwinHandler) SetBindings(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := h.Twins.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var req struct {
		AdapterBindings map[string]string `json:"adapter_bindings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	t.AdapterBindings = req.AdapterBindings
	if err := h.Twins.Update(r.Context(), t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, t)
}
