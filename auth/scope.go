package auth

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// Containers and listing.
//
// Authorization answers a boolean about one resource. Listing asks the opposite
// question — *which* resources may I see — and answering it by checking every
// row does not scale. The systems with the most general authorization models
// have the worst listing story for exactly this reason.
//
// The way out is that a binding scope already names a set. GrantedScopes hands
// those names back so a caller can turn them into a query, rather than making
// this package guess at storage.

// ScopeGrants is what a principal may and may not do, expressed as the scopes
// their bindings cover.
//
// Allow and Deny are kept apart rather than resolved against each other,
// because resolving them needs the container tree — whether team:t_7f2a sits
// inside org:o_9c31 — and this module deliberately does not know that. The
// caller owns the tree, so the caller does the subtraction: candidates whose
// closure meets Allow, minus those whose closure meets Deny.
type ScopeGrants struct {
	// Allow lists the scopes under which the action is granted.
	Allow []string

	// Deny lists the scopes under which it is unconditionally taken back.
	//
	// Conditional denies are absent, and cannot be otherwise: whether one bites
	// depends on the resource, which is not in hand when building a query. A
	// listing filtered on these is therefore a *narrowing*, and the per-resource
	// check stays authoritative for anything conditional — the same caveat
	// ExpandGrants carries for the same reason.
	Deny []string
}

// Unrestricted reports whether the grants cover everything, in which case a
// caller can skip filtering entirely.
func (g ScopeGrants) Unrestricted() bool {
	return len(g.Deny) == 0 && slices.Contains(g.Allow, "*")
}

// Empty reports whether nothing at all is granted, in which case a caller can
// skip the query entirely and return nothing.
func (g ScopeGrants) Empty() bool { return len(g.Allow) == 0 }

// GrantedScopes returns the scopes under which a principal may perform an
// action.
//
// It is one indexed read of the principal's bindings — which is why this is
// affordable here and expensive in systems that model permissions as a
// relationship graph, where the same question means walking it.
func (a *StoreAuthorizer) GrantedScopes(ctx context.Context, principalID string, action Action) (ScopeGrants, error) {
	grants, err := a.Grants(ctx, principalID)
	if err != nil {
		return ScopeGrants{}, err
	}

	var out ScopeGrants
	for _, g := range grants {
		if !matchPattern(string(g.Action), string(action)) {
			continue
		}
		switch {
		case g.Effect == EffectAllow:
			if !slices.Contains(out.Allow, g.Scope) {
				out.Allow = append(out.Allow, g.Scope)
			}
		case g.Effect == EffectDeny && len(g.Conditions) == 0:
			if !slices.Contains(out.Deny, g.Scope) {
				out.Deny = append(out.Deny, g.Scope)
			}
		}
	}
	slices.Sort(out.Allow)
	slices.Sort(out.Deny)
	return out, nil
}

// ScopeLabel builds the label a container is named by.
//
// Labels carry IDs, never display names. Two organisations may both have a team
// called "Engineering", and if the label were the name, a grant on one would
// silently cover the other — a cross-tenant grant produced by nothing more than
// two people picking the same word. Names belong in the interface; identity
// belongs in the database.
func ScopeLabel(typ, id string) string { return typ + ":" + id }

// ValidateScopes checks that every label on a resource is usable as a scope.
//
// Wildcards are rejected: a label says what a resource *is inside*, and a
// resource inside "everything" is a resource whose containment says nothing.
// Patterns belong on the binding, which is the side doing the asking.
func ValidateScopes(scopes []string) error {
	for _, label := range scopes {
		if err := validatePattern(label); err != nil {
			return fmt.Errorf("scope label %q: %w", label, err)
		}
		if label == "*" || strings.HasSuffix(label, ":*") {
			return fmt.Errorf("%w: scope label %q must name one container, not a pattern", ErrInvalidPattern, label)
		}
	}
	return nil
}
