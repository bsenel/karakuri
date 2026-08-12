package auth

import (
	"maps"
	"slices"
	"strings"
)

// ResourceRef identifies the thing an action is being attempted on. It is
// richer than a bare "twin:abc" string because policy conditions need something
// to read: Owner backs owner_equals, Attrs back attr_equals / attr_in.
//
// An empty ID means the whole collection (list, create). Its String form is
// "twin:*", so a policy scoped to a single twin does not accidentally grant
// "list every twin".
type ResourceRef struct {
	Type  string            `json:"type"`
	ID    string            `json:"id,omitempty"`
	Owner string            `json:"owner,omitempty"`
	Attrs map[string]string `json:"attrs,omitempty"`

	// Scopes are the containers this resource belongs to — its ancestor
	// closure, already flattened: ["team:t_7f2a", "org:o_9c31"]. A binding
	// scoped to any of them covers this resource.
	//
	// It is a set rather than a path on purpose. A path gives a resource one
	// location, which makes sharing across containers an exception mechanism
	// bolted on afterwards; a set lets a resource be multi-homed — its own
	// team, its own org, and a project spanning two organisations — with no
	// second construct and no change to how matching works.
	//
	// Flattening is also what keeps evaluation cheap: the closure is computed
	// where the container tree lives and stored, so authorization never walks a
	// tree, recurses, or needs a depth limit. Empty means the resource belongs
	// to nothing, which is exactly how every resource behaved before containers
	// existed.
	Scopes []string `json:"scopes,omitempty"`
}

// Resource builds a ref for a single object.
func Resource(typ, id string) ResourceRef { return ResourceRef{Type: typ, ID: id} }

// Collection builds a ref for a whole resource type (list / create routes).
func Collection(typ string) ResourceRef { return ResourceRef{Type: typ} }

// WithOwner returns a copy of r carrying an owner, enabling owner_equals.
func (r ResourceRef) WithOwner(owner string) ResourceRef {
	r.Owner = owner
	return r
}

// WithScopes returns a copy of r belonging to the given containers.
//
// The labels are the closure, not just the immediate parent: a twin in a team
// inside an org carries both, so a binding on either covers it. Callers own
// computing that, because they own the tree.
func (r ResourceRef) WithScopes(scopes ...string) ResourceRef {
	r.Scopes = slices.Clone(scopes)
	return r
}

// InScope reports whether pattern covers this resource, by its own identity or
// by any container it belongs to. It is what RoleBinding.covers asks, exposed
// for callers that need the same question outside an authorization decision.
func (r ResourceRef) InScope(pattern string) bool {
	if matchPattern(pattern, r.String()) {
		return true
	}
	for _, label := range r.Scopes {
		if matchPattern(pattern, label) {
			return true
		}
	}
	return false
}

// WithAttr returns a copy of r carrying one additional attribute.
func (r ResourceRef) WithAttr(key, value string) ResourceRef {
	attrs := make(map[string]string, len(r.Attrs)+1)
	maps.Copy(attrs, r.Attrs)
	attrs[key] = value
	r.Attrs = attrs
	return r
}

// String renders the ref in policy-pattern space: "twin:abc" for a single
// object, "twin:*" for a collection, "*" when the type is unset.
func (r ResourceRef) String() string {
	typ := strings.TrimSpace(r.Type)
	if typ == "" {
		return "*"
	}
	id := strings.TrimSpace(r.ID)
	if id == "" {
		id = "*"
	}
	return typ + ":" + id
}
