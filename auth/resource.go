package auth

import (
	"maps"
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
