package auth

import (
	"fmt"
	"slices"
	"strings"
)

// Role is a named, composable set of policies. Inherits names other roles whose
// policies are folded in, so "admin inherits operator inherits viewer" is
// expressed once rather than restated three times.
//
// System roles are built-ins the deployment ships with. They are immutable:
// letting an operator edit "admin" is the fastest way to lock everyone out.
type Role struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Inherits    []string `json:"inherits,omitempty"`
	Policies    []Policy `json:"policies,omitempty"`
	System      bool     `json:"system,omitempty"`
}

// Clone deep-copies a role.
func (r Role) Clone() Role {
	r.Inherits = slices.Clone(r.Inherits)
	policies := make([]Policy, len(r.Policies))
	for i, p := range r.Policies {
		policies[i] = p.Clone()
	}
	r.Policies = policies
	return r
}

// GrantedPolicy is a policy together with the role it reached the principal
// through. The authorizer carries it into the decision trace so a denial can
// name the role responsible, not just the policy.
type GrantedPolicy struct {
	Policy
	ViaRole string `json:"via_role"`
}

// EffectivePolicies flattens a role's own policies with those of every role it
// inherits, breadth-first, nearest role first. A policy ID reached through more
// than one inheritance path is kept once, attributed to the role that reached it
// first.
//
// Returns ErrRoleNotFound when a named role is missing and ErrRoleCycle when
// inheritance loops.
func EffectivePolicies(name string, index map[string]Role) ([]GrantedPolicy, error) {
	if _, ok := index[name]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrRoleNotFound, name)
	}
	if err := detectCycle(name, index, map[string]bool{}, nil); err != nil {
		return nil, err
	}

	var (
		out     []GrantedPolicy
		seen    = map[string]bool{}
		visited = map[string]bool{name: true}
		queue   = []string{name}
	)
	for len(queue) > 0 {
		role := index[queue[0]]
		queue = queue[1:]

		for _, p := range role.Policies {
			if p.ID != "" {
				if seen[p.ID] {
					continue
				}
				seen[p.ID] = true
			}
			out = append(out, GrantedPolicy{Policy: p.Clone(), ViaRole: role.Name})
		}

		// Missing parents cannot reach here: detectCycle above walks the same
		// graph and reports ErrRoleNotFound first.
		for _, parent := range role.Inherits {
			if visited[parent] {
				continue
			}
			visited[parent] = true
			queue = append(queue, parent)
		}
	}
	return out, nil
}

// detectCycle walks the inheritance graph depth-first, keeping the current
// recursion stack so a repeat visit on that stack — and only there — is a cycle.
func detectCycle(name string, index map[string]Role, onStack map[string]bool, stack []string) error {
	if onStack[name] {
		return fmt.Errorf("%w: %s", ErrRoleCycle, strings.Join(append(stack, name), " -> "))
	}
	role, ok := index[name]
	if !ok {
		return fmt.Errorf("%w: %q", ErrRoleNotFound, name)
	}
	onStack[name] = true
	stack = append(stack, name)
	for _, parent := range role.Inherits {
		if err := detectCycle(parent, index, onStack, stack); err != nil {
			return err
		}
	}
	delete(onStack, name)
	return nil
}

// RoleIndex builds the name→role map EffectivePolicies expects.
func RoleIndex(roles []Role) map[string]Role {
	index := make(map[string]Role, len(roles))
	for _, r := range roles {
		index[r.Name] = r
	}
	return index
}

// ValidateRoles checks every role in a set resolves without a missing parent or
// a cycle. Call it once at seed time.
func ValidateRoles(roles []Role) error {
	index := RoleIndex(roles)
	for _, r := range roles {
		if _, err := EffectivePolicies(r.Name, index); err != nil {
			return err
		}
	}
	return nil
}
