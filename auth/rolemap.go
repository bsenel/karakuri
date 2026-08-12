package auth

import (
	"slices"
	"strings"
)

// ClaimPath addresses a value inside a decoded claim set, as dotted segments:
// "groups", "realm_access.roles", "resource_access.karakuri.roles".
//
// Providers disagree about where group membership lives and there is no
// standard claim for it, so the path is configuration rather than a constant.
type ClaimPath string

// Strings resolves the path to a list of strings.
//
// A string yields one element, a list of strings yields all of them, and
// anything else — a number, an object, a missing key — yields none. Resolution
// is deliberately total: a claim set that does not carry the configured path is
// a user with no groups, not an error, because the alternative is that one
// provider's missing optional claim breaks every login.
func (p ClaimPath) Strings(claims map[string]any) []string {
	value, ok := p.resolve(claims)
	if !ok {
		return nil
	}
	switch v := value.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []string:
		return slices.Clone(v)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// First resolves the path to a single string, or "" when it does not resolve to
// one. It is what Email and Name are read with.
func (p ClaimPath) First(claims map[string]any) string {
	if all := p.Strings(claims); len(all) > 0 {
		return all[0]
	}
	return ""
}

func (p ClaimPath) resolve(claims map[string]any) (any, bool) {
	if p == "" || claims == nil {
		return nil, false
	}
	segments := strings.Split(string(p), ".")
	var current any = claims
	for _, segment := range segments {
		node, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		if current, ok = node[segment]; !ok {
			return nil, false
		}
	}
	return current, true
}

// RoleGrant is a role and the scope it is granted over.
//
// The scope is why this is a struct rather than a role name. Without it every
// federated user lands with their role over everything, so a directory group of
// two hundred people becomes two hundred globally-scoped principals — which is
// exactly what a tenancy tree exists to prevent. An empty Scope still means
// "*", because that is what a binding with no scope has always meant.
type RoleGrant struct {
	Role  string
	Scope string
}

// EffectiveScope returns the grant's scope, defaulting to "*".
func (g RoleGrant) EffectiveScope() string {
	if g.Scope == "" {
		return "*"
	}
	return g.Scope
}

// RoleMap turns the groups a provider asserted into scoped role grants.
//
// The mapping is one-way and explicit. Nothing is derived from the group name,
// because a provider's directory is not under Karakuri's control: a group
// called "admin" appearing in somebody's LDAP tree must not silently become the
// admin role.
type RoleMap struct {
	// Groups maps an asserted group name to what it grants. A group with no
	// entry grants nothing.
	Groups map[string][]RoleGrant

	// Default is granted to any identity that authenticated but matched no
	// group.
	//
	// It is empty unless an operator sets it, and that is the important default:
	// anybody in the company directory can authenticate against a corporate
	// identity provider, so "matched no group" has to mean no access rather than
	// read-only access to everything.
	Default []RoleGrant
}

// Grants returns the sorted, de-duplicated grants these groups map to.
//
// Two groups mapping the same role to different scopes yield two grants, not
// one: somebody who is an operator in two teams holds it in both, and
// collapsing them would either widen or narrow what the directory said.
func (m RoleMap) Grants(groups []string) []RoleGrant {
	var out []RoleGrant
	for _, group := range groups {
		out = appendGrants(out, m.Groups[group])
	}
	if len(out) == 0 {
		out = appendGrants(out, m.Default)
	}
	slices.SortFunc(out, compareGrants)
	return out
}

// Mentions returns every role name the map can grant, sorted. Bootstrap code
// uses it to check the configuration names roles that exist, at boot rather
// than at the first login that would have needed one.
func (m RoleMap) Mentions() []string {
	var out []string
	add := func(grants []RoleGrant) {
		for _, g := range grants {
			if g.Role != "" && !slices.Contains(out, g.Role) {
				out = append(out, g.Role)
			}
		}
	}
	for _, grants := range m.Groups {
		add(grants)
	}
	add(m.Default)
	slices.Sort(out)
	return out
}

// Scopes returns every scope the map can grant over, sorted, excluding the
// unrestricted one. It is what a host application validates against its own
// container tree at boot: a scope naming a team that does not exist is a typo,
// and finding it here beats finding it when somebody cannot see their twins.
func (m RoleMap) Scopes() []string {
	var out []string
	add := func(grants []RoleGrant) {
		for _, g := range grants {
			scope := g.EffectiveScope()
			if scope != "*" && !slices.Contains(out, scope) {
				out = append(out, scope)
			}
		}
	}
	for _, grants := range m.Groups {
		add(grants)
	}
	add(m.Default)
	slices.Sort(out)
	return out
}

func appendGrants(out, grants []RoleGrant) []RoleGrant {
	for _, g := range grants {
		if g.Role == "" {
			continue
		}
		g.Scope = g.EffectiveScope()
		if !slices.Contains(out, g) {
			out = append(out, g)
		}
	}
	return out
}

func compareGrants(a, b RoleGrant) int {
	if a.Role != b.Role {
		return strings.Compare(a.Role, b.Role)
	}
	return strings.Compare(a.Scope, b.Scope)
}
