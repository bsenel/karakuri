package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// ScopeAuthorizer answers which scopes a principal holds an action over. It is
// auth.StoreAuthorizer narrowed to the one method a listing needs.
type ScopeAuthorizer interface {
	GrantedScopes(ctx context.Context, principalID string, action auth.Action) (auth.ScopeGrants, error)
}

// ScopedCollection makes a list route reachable by a principal whose bindings
// are container-scoped.
//
// A collection route names no resource, so its ref is "twin:*" and a binding
// scoped "team:t_7f2a" does not match it — which would mean anyone bound to a
// team could read every twin they own individually but could not call
// GET /twins at all. That is not isolation, it is a dead end.
//
// The route check and the row check are different questions. This one answers
// "may you list twins at all", so the ref carries the containers the caller
// actually holds for the action: a principal with a grant somewhere passes and
// the handler's filter decides which rows, and a principal with no grant at all
// carries nothing, matches nothing, and is refused exactly as before.
//
// It is confined to list routes. Every other collection route — POST /twins,
// say — keeps the older, stricter behaviour, because "may you create a twin" is
// not a question a filtered result set can answer afterwards.
func ScopedCollection(a ScopeAuthorizer, action auth.Action, inner auth.ResourceFunc) auth.ResourceFunc {
	if a == nil {
		return inner
	}
	return func(r *http.Request) auth.ResourceRef {
		ref := inner(r)
		if ref.ID != "" {
			return ref
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			return ref
		}
		grants, err := a.GrantedScopes(r.Context(), principal.ID, action)
		if err != nil {
			// Fail closed: no labels means only an unscoped binding reaches the
			// collection, which is what happened before containers existed.
			return ref
		}
		var labels []string
		for _, scope := range grants.Allow {
			// Wildcards already match the collection ref through the ordinary
			// grammar; adding them as labels would say nothing new.
			if scope == "*" || strings.HasSuffix(scope, ":*") {
				continue
			}
			labels = append(labels, scope)
		}
		if len(labels) == 0 {
			return ref
		}
		return ref.WithScopes(labels...)
	}
}

// ListSelectors turns the scopes a principal holds into a storage filter over
// one resource type.
//
// Authorization is a boolean about one resource; listing asks the opposite
// question, and answering it by checking every row does not scale. A binding
// scope already names a set, so this translates those names into something a
// database can index on and hands the query back to the caller.
//
// A nil visible means the principal may see everything and the listing should
// not be filtered at all. An empty-but-non-nil visible means they may see
// nothing — a distinction the storage layer relies on, because a filter that
// widens to every row when its input is empty is how a listing leaks.
func ListSelectors(grants auth.ScopeGrants, resourceType string) (visible *storage.ScopeSelector, hidden storage.ScopeSelector) {
	if grants.Unrestricted() {
		return nil, storage.ScopeSelector{}
	}
	allow := selectorFor(grants.Allow, resourceType)
	if allow.unrestricted {
		// Everything of this type is visible, but a deny may still remove some
		// of it — so this is not the same as holding "*".
		visible = nil
	} else {
		visible = &allow.ScopeSelector
	}
	return visible, selectorFor(grants.Deny, resourceType).ScopeSelector
}

// ListFor is ListSelectors for the common case: read the principal's grants for
// an action, then translate them.
func ListFor(ctx context.Context, a ScopeAuthorizer, principalID string, action auth.Action, resourceType string) (*storage.ScopeSelector, storage.ScopeSelector, error) {
	if a == nil || principalID == "" {
		// No authorizer or no principal is an internal caller, not an
		// anonymous one — every API route is authenticated before it reaches a
		// handler. Filtering here would silently empty those listings.
		return nil, storage.ScopeSelector{}, nil
	}
	grants, err := a.GrantedScopes(ctx, principalID, action)
	if err != nil {
		return nil, storage.ScopeSelector{}, err
	}
	visible, hidden := ListSelectors(grants, resourceType)
	return visible, hidden, nil
}

type selector struct {
	storage.ScopeSelector
	unrestricted bool
}

// selectorFor sorts scope patterns into the three shapes storage can index.
//
// The grammar is auth's — exact, "<prefix>:*", or bare "*" — so the translation
// lives here rather than in the database layer, and it must agree with
// matchPattern: anything this classifies differently would make a filtered list
// disagree with the per-resource check on the same row.
func selectorFor(scopes []string, resourceType string) selector {
	var out selector
	for _, scope := range scopes {
		switch {
		case scope == "*":
			out.unrestricted = true
		case scope == resourceType+":*":
			// Every resource of this type, by identity — no container needed.
			out.unrestricted = true
		case strings.HasSuffix(scope, ":*"):
			// A whole kind of container: "org:*". Rare, but carried so a
			// filtered listing is never narrower than the per-row check.
			out.LabelPrefixes = append(out.LabelPrefixes, strings.TrimSuffix(scope, "*"))
		case strings.HasPrefix(scope, resourceType+":"):
			out.IDs = append(out.IDs, strings.TrimPrefix(scope, resourceType+":"))
		default:
			out.Labels = append(out.Labels, scope)
		}
	}
	return out
}
