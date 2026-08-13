package auth

import (
	"context"
	"net/http"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/internal/core/twin"
	"github.com/go-chi/chi/v5"
)

// Resource functions turn a routed request into the thing being acted on.
// They run after chi has matched the route, so URL parameters are available.
//
// A route with no {id} targets the collection, which ResourceRef renders as
// "twin:*" — so a policy scoped to one twin does not accidentally grant
// "list every twin".

func TwinResource(r *http.Request) auth.ResourceRef {
	return auth.Resource("twin", chi.URLParam(r, "id"))
}

// TwinOwnerLookup reads a twin's owner. It is an interface rather than the
// storage adapter itself so this package does not depend on the whole storage
// surface for one field.
type TwinOwnerLookup interface {
	GetTwin(ctx context.Context, id string) (twin.DigitalTwin, error)
}

// OwnedTwinResource is TwinResource plus the twin's owner, which is what lets
// an owner_equals condition fire.
//
// The lookup costs one read per request on twin routes. That is the price of
// expressing ownership in policy instead of as an `if` inside every handler —
// and the reason it is confined to routes that actually name a twin.
func OwnedTwinResource(lookup TwinOwnerLookup) auth.ResourceFunc {
	return func(r *http.Request) auth.ResourceRef {
		id := chi.URLParam(r, "id")
		ref := auth.Resource("twin", id)
		if id == "" || lookup == nil {
			return ref
		}
		t, err := lookup.GetTwin(r.Context(), id)
		if err != nil {
			// A twin that cannot be read has no owner to match. The request
			// falls through to whatever unconditional policy applies, and the
			// handler reports the 404 — an authorization layer should not be
			// the thing that decides a resource does not exist.
			return ref
		}
		return ref.WithOwner(t.OwnerID)
	}
}

// ClosureLookup reads a container's own ancestry. Containers are not in the
// resource_scopes table — they are the tree that table is derived from — so
// their labels come from here rather than from ScopeLookup.
type ClosureLookup interface {
	Closure(ctx context.Context, id string) ([]string, error)
}

// ContainerResource is the container a route names, carrying its ancestry, so a
// grant on an organisation reaches the teams inside it.
//
// Without the closure a container-scoped administrator could not rename their
// own team: the ref would be a bare "container:t_7f2a" and their binding names
// the org above it.
func ContainerResource(lookup ClosureLookup) auth.ResourceFunc {
	return func(r *http.Request) auth.ResourceRef {
		id := chi.URLParam(r, "id")
		ref := auth.Resource("container", id)
		if id == "" || lookup == nil {
			return ref
		}
		closure, err := lookup.Closure(r.Context(), id)
		if err != nil {
			// No labels narrows the decision to an unscoped grant, and the
			// handler reports whether the container exists — an authorization
			// layer should not be the thing that decides that.
			return ref
		}
		return ref.WithScopes(closure...)
	}
}

// CollectionResource is a fixed collection ref, for routes whose real subject
// arrives in the body rather than the URL.
func CollectionResource(typ string) auth.ResourceFunc {
	return func(*http.Request) auth.ResourceRef { return auth.Collection(typ) }
}

func ObjectiveResource(r *http.Request) auth.ResourceRef {
	return auth.Resource("objective", chi.URLParam(r, "id"))
}

// ScopeLookup reads the containers a resource belongs to — its ancestor
// closure, already flattened. It is the container service narrowed to the one
// method the request path needs.
type ScopeLookup interface {
	ScopesOf(ctx context.Context, resourceType, resourceID string) ([]string, error)
}

// Scoped wraps a resource function so the decision can see which organisation,
// team or project the resource sits in.
//
// It composes rather than replaces: OwnedTwinResource still supplies the owner,
// and this adds the containers on top, because ownership and containment are
// independent questions about the same resource.
//
// A resource in no container carries no labels and matches exactly what it
// matched before containers existed. That is why this can be applied to every
// route without a flag: on a deployment that never creates an org, it is a
// lookup that returns nothing and changes no decision.
//
// A failed lookup attaches no scopes rather than failing the request. The
// consequence is a narrower decision — a binding scoped to a container will not
// match — so the failure mode is denial, not a resource that quietly escapes
// its tenant. The same reasoning as OwnedTwinResource, whose lookup failure
// leaves the owner unset.
func Scoped(lookup ScopeLookup, inner auth.ResourceFunc) auth.ResourceFunc {
	if lookup == nil {
		return inner
	}
	return func(r *http.Request) auth.ResourceRef {
		ref := inner(r)
		if ref.ID == "" {
			// A collection route names no resource, so there is nothing to
			// look up. Whether the principal may list is answered by the
			// binding's own scope, and which rows come back is PR4's question.
			return ref
		}
		scopes, err := lookup.ScopesOf(r.Context(), ref.Type, ref.ID)
		if err != nil || len(scopes) == 0 {
			return ref
		}
		return ref.WithScopes(scopes...)
	}
}

func LoopResource(r *http.Request) auth.ResourceRef {
	return auth.Resource("loop", chi.URLParam(r, "id"))
}

func CheckpointResource(r *http.Request) auth.ResourceRef {
	return auth.Resource("checkpoint", chi.URLParam(r, "id"))
}

func ArtifactResource(r *http.Request) auth.ResourceRef {
	return auth.Resource("artifact", chi.URLParam(r, "sha"))
}

func DomainResource(r *http.Request) auth.ResourceRef {
	return auth.Resource("domain", chi.URLParam(r, "id"))
}

// Route records one API route and the permission it demands. The table below is
// the readable form of what internal/api/server.go wires up, and the integration
// suite walks it to assert the expected 200/403 for every role — so a route that
// loses its permission in server.go fails a test rather than quietly opening up.
type Route struct {
	Method  string
	Pattern string
	Action  auth.Action

	// Public marks routes deliberately outside authentication.
	Public bool
}

// Routes returns the full /api/v1 surface with its permission requirements.
func Routes() []Route {
	return []Route{
		// /health is how a load balancer and the SPA's login screen probe a
		// server they cannot yet authenticate against.
		{http.MethodGet, "/health", "", true},

		{http.MethodPost, "/auth/token", "", true},   // login: the credential IS the request
		{http.MethodPost, "/auth/refresh", "", true}, // rotation: same

		// Federated login. Public for the same reason /auth/token is: these are
		// how somebody with no credential acquires one. The SAML metadata
		// endpoint is public because metadata is what an administrator hands
		// the identity provider, and it carries only public information.
		{http.MethodGet, "/auth/sso/config", "", true},
		{http.MethodGet, "/auth/sso/login", "", true},
		{http.MethodGet, "/auth/sso/callback", "", true},
		// Redeeming a CLI handoff code: the code IS the credential, so
		// requiring one to reach this would be circular.
		{http.MethodPost, "/auth/sso/exchange", "", true},
		{http.MethodGet, "/auth/saml/metadata", "", true},
		{http.MethodPost, "/auth/saml/acs", "", true},
		{http.MethodGet, "/auth/me", "", false},      // any authenticated principal
		{http.MethodPost, "/auth/revoke", "", false}, // revoking your own session

		{http.MethodGet, "/auth/users", ActionAuthRead, false},
		{http.MethodPost, "/auth/users", ActionAuthWrite, false},
		{http.MethodGet, "/auth/roles", ActionAuthRead, false},
		{http.MethodGet, "/auth/policies", ActionAuthRead, false},
		{http.MethodPost, "/auth/bindings", ActionAuthWrite, false},
		{http.MethodPost, "/auth/check", ActionAuthRead, false},

		{http.MethodPost, "/twins", ActionTwinCreate, false},
		{http.MethodGet, "/twins", ActionTwinRead, false},
		{http.MethodGet, "/twins/{id}", ActionTwinRead, false},
		{http.MethodPut, "/twins/{id}", ActionTwinUpdate, false},
		{http.MethodPut, "/twins/{id}/bindings", ActionTwinBind, false},
		{http.MethodGet, "/twins/{id}/events", ActionTwinRead, false},

		{http.MethodPost, "/objectives", ActionObjectiveCreate, false},
		{http.MethodGet, "/objectives", ActionObjectiveRead, false},
		{http.MethodGet, "/objectives/templates", ActionObjectiveRead, false},
		{http.MethodGet, "/objectives/{id}", ActionObjectiveRead, false},
		{http.MethodPost, "/objectives/{id}/status", ActionObjectiveUpdate, false},
		{http.MethodGet, "/objectives/{id}/events", ActionObjectiveRead, false},

		{http.MethodPost, "/loops", ActionLoopStart, false},
		{http.MethodGet, "/loops/{id}/status", ActionLoopRead, false},
		{http.MethodPost, "/loops/{id}/resume", ActionLoopResume, false},

		{http.MethodGet, "/checkpoints", ActionCheckpointRead, false},
		{http.MethodGet, "/checkpoints/{id}", ActionCheckpointRead, false},
		{http.MethodPost, "/checkpoints/{id}/resolve", ActionCheckpointResolve, false},

		{http.MethodGet, "/artifacts", ActionArtifactRead, false},
		{http.MethodPost, "/artifacts", ActionArtifactWrite, false},
		{http.MethodGet, "/artifacts/{sha}", ActionArtifactRead, false},
		{http.MethodGet, "/artifacts/{sha}/diff/{other}", ActionArtifactRead, false},

		{http.MethodPost, "/memory/store", ActionMemoryWrite, false},
		{http.MethodPost, "/memory/recall", ActionMemoryRead, false},
		{http.MethodPost, "/memory/forget", ActionMemoryForget, false},

		// The tenancy tree (Phase 17). Reading it is how a user finds out which
		// organisation a twin is in; writing it is confined by the tree itself,
		// which the handler re-checks against the specific container named in
		// the body — a URL-shaped check cannot see a parent that arrives as
		// JSON.
		{http.MethodPost, "/containers", ActionContainerWrite, false},
		{http.MethodGet, "/containers", ActionContainerRead, false},
		{http.MethodGet, "/containers/{id}", ActionContainerRead, false},
		{http.MethodPost, "/containers/{id}/name", ActionContainerWrite, false},
		{http.MethodPost, "/containers/{id}/parent", ActionContainerMove, false},
		{http.MethodDelete, "/containers/{id}", ActionContainerWrite, false},
		{http.MethodGet, "/containers/resources", ActionContainerRead, false},
		{http.MethodPost, "/containers/resources", ActionContainerWrite, false},

		{http.MethodGet, "/domains", ActionDomainRead, false},
		{http.MethodGet, "/domains/capabilities", ActionDomainRead, false},
		{http.MethodGet, "/domains/{id}/conformance", ActionDomainRead, false},

		{http.MethodGet, "/quota", ActionQuotaRead, false},
		{http.MethodGet, "/quota/usage", ActionQuotaRead, false},
		{http.MethodPost, "/quota/reset", ActionQuotaAdmin, false},

		// Self-service limits and spend (Phase 18). Approving re-checks in the
		// handler against the subject the request names, the same way container
		// writes do — a URL-shaped check cannot see a subject that arrives as
		// part of a stored request.
		{http.MethodPost, "/quota/requests", ActionQuotaRequest, false},
		{http.MethodGet, "/quota/requests", ActionQuotaRead, false},
		{http.MethodPost, "/quota/requests/{id}/decide", ActionQuotaApprove, false},
		{http.MethodGet, "/cost", ActionCostRead, false},

		// The limits an operator has stored (Phase 19). Editing one changes it
		// for the whole deployment, which is why it is quota:admin rather than
		// the tenant-scoped quota:approve.
		// The deployment-wide event stream (Phase 19). Gated on twin:read
		// because that is what it shows; the per-event filter then confines it
		// to the same twins the listing would return.
		{http.MethodGet, "/events", ActionTwinRead, false},

		{http.MethodGet, "/quota/tiers", ActionQuotaRead, false},
		{http.MethodPut, "/quota/tiers/{name}", ActionQuotaAdmin, false},
		{http.MethodDelete, "/quota/tiers/{name}", ActionQuotaAdmin, false},

		{http.MethodPost, "/research", ActionResearchRun, false},
		{http.MethodGet, "/audit", ActionAuditRead, false},
	}
}
