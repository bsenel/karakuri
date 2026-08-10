package auth

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// Grant is one effective permission a principal holds: a policy, the role it
// came through, and the binding scope it applies within. Listing grants is what
// answers "what can this user actually do?" without probing every endpoint.
type Grant struct {
	Role       string      `json:"role"`
	Scope      string      `json:"scope"`
	PolicyID   string      `json:"policy_id"`
	Action     Action      `json:"action"`
	Resource   string      `json:"resource"`
	Effect     Effect      `json:"effect"`
	Conditions []Condition `json:"conditions,omitempty"`
}

// Grants returns every policy reaching a principal through its role bindings,
// flattened across role inheritance and sorted for stable output.
func (a *StoreAuthorizer) Grants(ctx context.Context, principalID string) ([]Grant, error) {
	bindings, err := a.store.ListBindings(ctx, principalID)
	if err != nil {
		return nil, fmt.Errorf("list bindings for %q: %w", principalID, err)
	}
	index := map[string]Role{}
	var out []Grant
	for _, b := range bindings {
		if err := loadRoleClosure(ctx, a.store, b.Role, index); err != nil {
			return nil, fmt.Errorf("resolve role %q: %w", b.Role, err)
		}
		granted, err := EffectivePolicies(b.Role, index)
		if err != nil {
			return nil, fmt.Errorf("resolve role %q: %w", b.Role, err)
		}
		for _, gp := range granted {
			out = append(out, Grant{
				Role:       gp.ViaRole,
				Scope:      b.EffectiveScope(),
				PolicyID:   gp.ID,
				Action:     gp.Action,
				Resource:   gp.Resource,
				Effect:     gp.Effect,
				Conditions: gp.Conditions,
			})
		}
	}
	slices.SortFunc(out, func(x, y Grant) int {
		if c := strings.Compare(string(x.Action), string(y.Action)); c != 0 {
			return c
		}
		if c := strings.Compare(x.Resource, y.Resource); c != 0 {
			return c
		}
		return strings.Compare(x.PolicyID, y.PolicyID)
	})
	return out, nil
}

// Roles returns the distinct role names bound to a principal, sorted.
func (a *StoreAuthorizer) Roles(ctx context.Context, principalID string) ([]string, error) {
	bindings, err := a.store.ListBindings(ctx, principalID)
	if err != nil {
		return nil, fmt.Errorf("list bindings for %q: %w", principalID, err)
	}
	var out []string
	for _, b := range bindings {
		if !slices.Contains(out, b.Role) {
			out = append(out, b.Role)
		}
	}
	slices.Sort(out)
	return out, nil
}

// ExpandGrants resolves a principal's allow grants into the concrete catalog
// actions they cover, minus anything an unconditional deny takes back. It is
// the list a UI shows on a "your permissions" screen; conditional denies are
// left in place because whether they bite depends on the resource.
func (a *StoreAuthorizer) ExpandGrants(ctx context.Context, principalID string, catalog *Catalog) ([]Action, error) {
	grants, err := a.Grants(ctx, principalID)
	if err != nil {
		return nil, err
	}
	allowed := map[Action]bool{}
	for _, g := range grants {
		if g.Effect != EffectAllow {
			continue
		}
		for _, action := range catalog.Expand(g.Action) {
			allowed[action] = true
		}
	}
	for _, g := range grants {
		if g.Effect != EffectDeny || len(g.Conditions) > 0 {
			continue
		}
		for _, action := range catalog.Expand(g.Action) {
			delete(allowed, action)
		}
	}
	out := make([]Action, 0, len(allowed))
	for action := range allowed {
		out = append(out, action)
	}
	slices.Sort(out)
	return out, nil
}
