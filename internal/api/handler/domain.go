package handler

import (
	"net/http"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
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
