package handler

import (
	"net/http"

	"github.com/bsenel/karakuri/internal/conformance"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/go-chi/chi/v5"
)

type DomainHandler struct {
	Domains      *domain.Registry
	Capabilities *capability.Registry
}

type domainInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

func (h *DomainHandler) List(w http.ResponseWriter, _ *http.Request) {
	packs := h.Domains.List()
	out := make([]domainInfo, len(packs))
	for i, p := range packs {
		out[i] = domainInfo{ID: p.ID(), Name: p.Name(), Version: p.Version(), Description: p.Description()}
	}
	writeJSON(w, out)
}

func (h *DomainHandler) ListCapabilities(w http.ResponseWriter, r *http.Request) {
	dom := r.URL.Query().Get("domain")
	if dom != "" {
		writeJSON(w, h.Capabilities.ListByDomain(dom))
		return
	}
	writeJSON(w, h.Capabilities.List())
}

// Conformance runs the conformance suite against the named domain pack and returns the results.
func (h *DomainHandler) Conformance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pack, ok := h.Domains.Get(id)
	if !ok {
		http.Error(w, `{"error":"domain not found"}`, http.StatusNotFound)
		return
	}
	results := conformance.New().Run(r.Context(), pack)
	writeJSON(w, results)
}
