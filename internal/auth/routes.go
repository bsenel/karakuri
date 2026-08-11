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

func ObjectiveResource(r *http.Request) auth.ResourceRef {
	return auth.Resource("objective", chi.URLParam(r, "id"))
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

		{http.MethodGet, "/domains", ActionDomainRead, false},
		{http.MethodGet, "/domains/capabilities", ActionDomainRead, false},
		{http.MethodGet, "/domains/{id}/conformance", ActionDomainRead, false},

		{http.MethodGet, "/quota", ActionQuotaRead, false},
		{http.MethodGet, "/quota/usage", ActionQuotaRead, false},
		{http.MethodPost, "/quota/reset", ActionQuotaAdmin, false},

		{http.MethodPost, "/research", ActionResearchRun, false},
		{http.MethodGet, "/audit", ActionAuditRead, false},
	}
}
