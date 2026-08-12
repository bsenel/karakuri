package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	corecontainer "github.com/bsenel/karakuri/internal/core/container"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	featurecontainer "github.com/bsenel/karakuri/internal/feature/container"
	"github.com/go-chi/chi/v5"
)

// ContainerHandler serves the tenancy tree: organisations, teams and projects.
//
// The route middleware answers "may you write containers at all". What it
// cannot answer is *which* container, because the parent a request names is in
// its body rather than its URL — so every mutation here re-checks against the
// specific container it touches. That is what makes the hierarchy govern
// changes to itself: an acme administrator cannot create a team inside globex,
// because creating under a parent requires a grant covering that parent.
type ContainerHandler struct {
	Containers *featurecontainer.Service

	// Authorizer answers the per-container checks. Required: without it every
	// mutation is refused rather than allowed, because a handler that cannot
	// tell which tenant a request belongs to must not guess.
	Authorizer auth.Authorizer
}

// Create adds a container under a parent the caller must already hold.
func (h *ContainerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind     string `json:"kind"`
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "body must be JSON")
		return
	}

	kind := corecontainer.Kind(strings.TrimSpace(body.Kind))
	if !kind.Valid() {
		authError(w, http.StatusBadRequest, "bad_request", "kind must be org, team or project")
		return
	}
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok || h.Authorizer == nil {
		authError(w, http.StatusForbidden, "forbidden", "this request cannot be attributed to a principal")
		return
	}
	// Creating *inside* something requires holding it. Creating a root — a new
	// organisation, or a project — is inside nothing, so it is checked against
	// the unrestricted scope, which only an unscoped grant covers. Minting a
	// tenant is a different privilege from running one, and without this check
	// anyone who could create a team could create an organisation beside their
	// own and grow from there.
	if body.ParentID == "" {
		if !h.allowed(w, r, principal, karakuriauth.ActionContainerWrite, auth.ResourceRef{}) {
			return
		}
	} else if !h.mayReach(w, r, body.ParentID, karakuriauth.ActionContainerWrite) {
		return
	}

	c, err := h.Containers.Create(r.Context(), featurecontainer.CreateRequest{
		Kind: kind, Name: body.Name, ParentID: body.ParentID,
	})
	if err != nil {
		writeContainerError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, c)
}

// Get returns one container.
func (h *ContainerHandler) Get(w http.ResponseWriter, r *http.Request) {
	c, err := h.Containers.Get(r.Context(), containerID(r))
	if err != nil {
		writeContainerError(w, err)
		return
	}
	writeJSON(w, c)
}

// List returns containers, filtered by kind and parent.
//
// It is not scope-filtered. The tree is the map of the organisation — knowing
// that a team called Engineering exists is not access to anything inside it —
// and a user who cannot see the containers above their own cannot be shown
// where they are. What lives inside a container is filtered, which is the part
// that matters.
func (h *ContainerHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := corecontainer.Filter{
		Kind:     corecontainer.Kind(q.Get("kind")),
		ParentID: q.Get("parent_id"),
		Name:     q.Get("name"),
	}
	// "roots" is a separate parameter because an absent parent_id has to mean
	// "do not filter" rather than "the roots".
	f.RootsOnly = q.Get("roots") == "true"

	list, err := h.Containers.List(r.Context(), f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, list)
}

// Rename changes a container's display name, which changes no label and so
// rewrites no binding.
func (h *ContainerHandler) Rename(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "body must be JSON")
		return
	}
	id := containerID(r)
	if !h.mayReach(w, r, id, karakuriauth.ActionContainerWrite) {
		return
	}

	c, err := h.Containers.Rename(r.Context(), id, body.Name)
	if err != nil {
		writeContainerError(w, err)
		return
	}
	writeJSON(w, c)
}

// Reparent moves a container, and is the one operation that needs a covering
// grant at both ends.
//
// Holding only the destination would let somebody pull a team — and everything
// in it — out of a tenant they have no claim on. Holding only the source would
// let them push it into one. Azure enforces the same pairing on management
// groups for the same reason.
func (h *ContainerHandler) Reparent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ParentID string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "body must be JSON")
		return
	}
	id := containerID(r)

	current, err := h.Containers.Get(r.Context(), id)
	if err != nil {
		writeContainerError(w, err)
		return
	}
	// The container being moved, its old parent and its new one. Checking the
	// container itself as well as its parent matters when it is a root: there
	// is no old parent to hold, and the container is the only thing that
	// identifies where it came from.
	if !h.mayReach(w, r, id, karakuriauth.ActionContainerMove) {
		return
	}
	if current.ParentID != "" && !h.mayReach(w, r, current.ParentID, karakuriauth.ActionContainerMove) {
		return
	}
	if body.ParentID != "" && !h.mayReach(w, r, body.ParentID, karakuriauth.ActionContainerMove) {
		return
	}

	moved, err := h.Containers.Reparent(r.Context(), id, body.ParentID)
	if err != nil {
		writeContainerError(w, err)
		return
	}
	writeJSON(w, moved)
}

// Delete removes a leaf container.
func (h *ContainerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := containerID(r)
	if !h.mayReach(w, r, id, karakuriauth.ActionContainerWrite) {
		return
	}
	if err := h.Containers.Delete(r.Context(), id); err != nil {
		writeContainerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetResourceContainers places a resource in containers, replacing whatever it
// was in.
//
// Two checks, and both are load-bearing. The caller must hold the resource,
// which is what stops somebody sharing a twin they cannot touch; and they must
// hold every container they are putting it into, which is what stops them
// filing another tenant's twin into their own team and reading it that way.
//
// Azure requires the same pairing when a resource joins a service group —
// permission on the group *and* write on the resource — and they can afford to
// be laxer, because their group membership grants no access to the members. It
// does here, which is the whole point of a project, so both halves are checked.
func (h *ContainerHandler) SetResourceContainers(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ResourceType string   `json:"resource_type"`
		ResourceID   string   `json:"resource_id"`
		ContainerIDs []string `json:"container_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		authError(w, http.StatusBadRequest, "bad_request", "body must be JSON")
		return
	}
	if body.ResourceType == "" || body.ResourceID == "" {
		authError(w, http.StatusBadRequest, "bad_request", "resource_type and resource_id are required")
		return
	}

	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok || h.Authorizer == nil {
		authError(w, http.StatusForbidden, "forbidden", "this request cannot be attributed to a principal")
		return
	}

	// Holding the resource means being able to change it, not merely read it:
	// putting a twin in a project grants everyone in that project access to it,
	// which is not a reader's decision to make.
	scopes, err := h.Containers.ScopesOf(r.Context(), body.ResourceType, body.ResourceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resource := auth.Resource(body.ResourceType, body.ResourceID).WithScopes(scopes...)
	if !h.allowed(w, r, principal, writeActionFor(body.ResourceType), resource) {
		return
	}

	for _, id := range body.ContainerIDs {
		if !h.mayReach(w, r, id, karakuriauth.ActionContainerWrite) {
			return
		}
	}

	if err := h.Containers.SetResourceContainers(r.Context(), body.ResourceType, body.ResourceID, body.ContainerIDs); err != nil {
		writeContainerError(w, err)
		return
	}
	placed, err := h.Containers.ScopesOf(r.Context(), body.ResourceType, body.ResourceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"resource_type": body.ResourceType,
		"resource_id":   body.ResourceID,
		"scopes":        placed,
	})
}

// GetResourceContainers returns the labels a resource carries.
func (h *ContainerHandler) GetResourceContainers(w http.ResponseWriter, r *http.Request) {
	resourceType := r.URL.Query().Get("resource_type")
	resourceID := r.URL.Query().Get("resource_id")
	if resourceType == "" || resourceID == "" {
		authError(w, http.StatusBadRequest, "bad_request", "resource_type and resource_id are required")
		return
	}
	scopes, err := h.Containers.ScopesOf(r.Context(), resourceType, resourceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"resource_type": resourceType,
		"resource_id":   resourceID,
		"scopes":        scopes,
	})
}

// mayReach checks the caller holds a container, writing the refusal itself and
// reporting whether the request should continue.
//
// The container is checked as a resource carrying its own ancestry, so a grant
// on an organisation reaches the teams inside it without this function knowing
// anything about the tree.
func (h *ContainerHandler) mayReach(w http.ResponseWriter, r *http.Request, containerID string, action auth.Action) bool {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok || h.Authorizer == nil {
		authError(w, http.StatusForbidden, "forbidden", "this request cannot be attributed to a principal")
		return false
	}
	c, err := h.Containers.Get(r.Context(), containerID)
	if err != nil {
		writeContainerError(w, err)
		return false
	}
	closure, err := h.Containers.Closure(r.Context(), containerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}
	ref := auth.Resource("container", c.ID).WithScopes(closure...)
	return h.allowed(w, r, principal, action, ref)
}

func (h *ContainerHandler) allowed(w http.ResponseWriter, r *http.Request, principal auth.Principal, action auth.Action, ref auth.ResourceRef) bool {
	decision, err := h.Authorizer.Authorize(r.Context(), principal, action, ref)
	if err != nil {
		// Fail closed, the same way the middleware does: an authorizer that
		// cannot answer must not be read as a yes.
		http.Error(w, "authorization could not be evaluated", http.StatusInternalServerError)
		return false
	}
	if !decision.Allowed {
		authError(w, http.StatusForbidden, "forbidden", decision.Reason)
		return false
	}
	return true
}

// writeActionFor names the permission that counts as "holding" a resource.
//
// Unknown types fall back to the container write action, so a resource type
// this handler has not been taught about needs an unscoped grant rather than
// being placeable by anyone who can write containers.
func writeActionFor(resourceType string) auth.Action {
	switch resourceType {
	case "twin":
		return karakuriauth.ActionTwinUpdate
	case "objective":
		return karakuriauth.ActionObjectiveUpdate
	default:
		return karakuriauth.ActionContainerWrite
	}
}

func containerID(r *http.Request) string { return chi.URLParam(r, "id") }

func writeContainerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, coreerrors.ErrContainerNotFound):
		authError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, coreerrors.ErrConflict):
		authError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, coreerrors.ErrInvalidInput):
		authError(w, http.StatusBadRequest, "bad_request", err.Error())
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
