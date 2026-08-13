package auth

import (
	"context"
	"strings"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/internal/core/container"
)

// ScopeResolver turns a binding scope into the thing it names. It is the
// container service narrowed to what granting needs.
type ScopeResolver interface {
	Closure(ctx context.Context, id string) ([]string, error)
	ScopesOf(ctx context.Context, resourceType, resourceID string) ([]string, error)
}

// ResourceForScope renders a binding scope as the resource it refers to, with
// its containers attached.
//
// "team:t_7f2a" is the team, carrying the org above it. "twin:abc" is the twin,
// carrying whatever containers it sits in. "*" is everything, which only an
// unscoped grant covers.
func ResourceForScope(ctx context.Context, containers ScopeResolver, scope string) auth.ResourceRef {
	scope = strings.TrimSpace(scope)
	if scope == "" || scope == "*" {
		return auth.ResourceRef{}
	}
	typ, id, ok := strings.Cut(scope, ":")
	if !ok || id == "" || id == "*" {
		// A wildcard over a type — "twin:*". There is no single thing it names
		// and nothing to look up, so it stands as the collection.
		return auth.Collection(typ)
	}

	ref := auth.Resource(typ, id)
	if containers == nil {
		return ref
	}
	// A container carries its ancestry; anything else carries the containers it
	// was placed in. Either way the labels are what let a grant higher up the
	// tree cover this.
	var (
		labels []string
		err    error
	)
	if container.Kind(typ).Valid() {
		labels, err = containers.Closure(ctx, id)
	} else {
		labels, err = containers.ScopesOf(ctx, typ, id)
	}
	if err != nil {
		// Attaching nothing narrows the decision: only a grant naming this
		// resource outright, or an unscoped one, will cover it. Failing closed
		// here means a lookup error refuses a grant rather than widening one.
		return ref
	}
	return ref.WithScopes(labels...)
}

// MayGrant reports whether a principal may hand out a binding at this scope,
// and why not when they may not.
//
// The rule is that you can only grant what you already hold. Without it, the
// permission to manage bindings is the permission to manage every tenant: an
// administrator scoped to one organisation could write themselves a binding
// over another, and the tree would be decoration. With it, granting is bounded
// by the same containment as everything else — an acme administrator can invite
// somebody from globex *into acme*, because acme's resources are theirs to
// share, and cannot put themselves into globex.
func MayGrant(ctx context.Context, a auth.Authorizer, containers ScopeResolver, principal auth.Principal, scope string) (bool, string, error) {
	allowed, reason, err := MayActOn(ctx, a, containers, principal, ActionAuthWrite, scope)
	if err != nil || allowed {
		return allowed, "", err
	}
	return false, "you can only grant a scope you already hold: " + reason, nil
}

// MayActOn answers whether a principal holds an action over the thing a scope
// names, resolving the scope's containers first.
//
// It is the body of MayGrant, separated because granting is not the only
// decision shaped this way: approving a quota raise is the same question asked
// of a different action, and a route-level check cannot ask it — the subject
// arrives inside a stored record rather than in the URL.
func MayActOn(ctx context.Context, a auth.Authorizer, containers ScopeResolver, principal auth.Principal, action auth.Action, scope string) (bool, string, error) {
	if a == nil {
		return false, "no authorizer is configured", nil
	}
	decision, err := a.Authorize(ctx, principal, action, ResourceForScope(ctx, containers, scope))
	if err != nil {
		return false, "", err
	}
	if decision.Allowed {
		return true, "", nil
	}
	return false, decision.Reason, nil
}
