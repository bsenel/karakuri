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

// RoleMap turns the groups a provider asserted into Karakuri roles.
//
// The mapping is one-way and explicit. Nothing is derived from the group name,
// because a provider's directory is not under Karakuri's control: a group
// called "admin" appearing in somebody's LDAP tree must not silently become the
// admin role.
type RoleMap struct {
	// Groups maps an asserted group name to the roles it grants. A group with
	// no entry grants nothing.
	Groups map[string][]string

	// Default is granted to any identity that authenticated but matched no
	// group.
	//
	// It is empty unless an operator sets it, and that is the important default:
	// anybody in the company directory can authenticate against a corporate
	// identity provider, so "matched no group" has to mean no access rather than
	// read-only access to everything.
	Default []string
}

// Roles returns the sorted, de-duplicated roles these groups map to.
func (m RoleMap) Roles(groups []string) []string {
	var out []string
	for _, group := range groups {
		for _, role := range m.Groups[group] {
			if role != "" && !slices.Contains(out, role) {
				out = append(out, role)
			}
		}
	}
	if len(out) == 0 {
		for _, role := range m.Default {
			if role != "" && !slices.Contains(out, role) {
				out = append(out, role)
			}
		}
	}
	slices.Sort(out)
	return out
}

// Mentions returns every role name the map can grant, sorted. Bootstrap code
// uses it to check the configuration names roles that exist, at boot rather
// than at the first login that would have needed one.
func (m RoleMap) Mentions() []string {
	var out []string
	add := func(roles []string) {
		for _, role := range roles {
			if role != "" && !slices.Contains(out, role) {
				out = append(out, role)
			}
		}
	}
	for _, roles := range m.Groups {
		add(roles)
	}
	add(m.Default)
	slices.Sort(out)
	return out
}
