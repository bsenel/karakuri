package handler

import (
	"encoding/json"
	"net/http"

	"github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/core/twin"
	featuretwin "github.com/bsenel/karakuri/internal/feature/twin"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/go-chi/chi/v5"
)

type TwinHandler struct {
	Twins *featuretwin.Service

	// Scopes narrows a listing to the twins the caller may read. Nil disables
	// filtering, which is what a deployment with no authorizer wired has.
	Scopes karakuriauth.ScopeAuthorizer
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
	// Stamp the creator so ownership conditions have something to match. A
	// request with no principal cannot reach here — every mutating route is
	// authenticated — but an unowned twin is still a valid state (rows created
	// before Phase 14), so this does not assert.
	owner, _ := auth.PrincipalFromContext(r.Context())
	t, err := h.Twins.Create(r.Context(), featuretwin.CreateRequest{
		Name:    req.Name,
		Kind:    twin.Kind(req.Kind),
		Domain:  req.Domain,
		OwnerID: owner.ID,
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

// List returns the twins the caller may read.
//
// The permission check on this route answers "may you list twins at all"; it
// cannot answer "which ones", because that question is about rows the request
// has not named. So the listing is filtered here, from the same bindings the
// per-resource check reads — otherwise per-resource denial would not be
// isolation, and GET /twins would be all-or-nothing.
//
// The filter is a narrowing, not the authority. Conditional denies cannot
// appear in GrantedScopes — whether one bites depends on the resource — so
// GET /twins/{id} stays the authoritative check for any twin listed here.
func (h *TwinHandler) List(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	visible, hidden, err := karakuriauth.ListFor(
		r.Context(), h.Scopes, principal.ID, karakuriauth.ActionTwinRead, "twin")
	if err != nil {
		// Fail closed: an authorizer that cannot answer must not produce an
		// unfiltered list.
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	twins, err := h.Twins.List(r.Context(), storage.TwinFilter{
		Kind:    r.URL.Query().Get("kind"),
		Domain:  r.URL.Query().Get("domain"),
		Visible: visible,
		Hidden:  hidden,
	})
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
