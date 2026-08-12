// Package container holds the tenancy tree: the organisations, teams and
// projects that resources belong to.
//
// It exists because authorization deliberately does not know about it. The
// `auth` module matches a binding's scope against a set of labels carried on
// the resource (`auth.ResourceRef.Scopes`) and never walks a tree, recurses, or
// bounds depth during a check. Something has to own the tree that produces
// those labels, compute the ancestor closure, and keep it current when the tree
// changes. That is this package and the service over it.
package container

import (
	"strings"
	"time"
)

// Kind is what a container is.
type Kind string

const (
	// KindOrg is a tenant boundary. Orgs may nest, the way Azure management
	// groups do, so a holding company can sit above its subsidiaries.
	KindOrg Kind = "org"

	// KindTeam is a group inside an organisation.
	KindTeam Kind = "team"

	// KindProject is a collaboration space that deliberately has no parent.
	//
	// This is the construct that crosses tenant boundaries: a resource can
	// carry its own team, its own org, *and* a project spanning two
	// organisations, because scopes are a set rather than a path. Giving a
	// project a parent would put it back inside one tenant and defeat the
	// point.
	KindProject Kind = "project"
)

// MaxDepth caps how deep the tree may nest, counting the root as depth 1.
//
// Six is Azure's own limit on management-group nesting, and it is a limit for
// the same reason: nothing legible needs more, while an unbounded tree turns
// every closure recomputation into an unbounded walk. Depth is bounded here, at
// write time, so authorization never has to think about it.
const MaxDepth = 6

// parents says which kinds a container of a given kind may sit inside. An empty
// list means the kind is always a root.
var parents = map[Kind][]Kind{
	KindOrg:     {KindOrg},
	KindTeam:    {KindOrg, KindTeam},
	KindProject: {},
}

// Valid reports whether k is a kind this package knows.
func (k Kind) Valid() bool {
	_, ok := parents[k]
	return ok
}

// CanNestUnder reports whether a container of kind k may have a parent of kind
// p. A team belongs to an org or another team; an org may nest under an org; a
// project never has a parent.
func (k Kind) CanNestUnder(p Kind) bool {
	for _, allowed := range parents[k] {
		if allowed == p {
			return true
		}
	}
	return false
}

// NeedsParent reports whether a container of kind k is meaningless at the root.
// A team without an organisation is, because there is no tenant it belongs to.
func (k Kind) NeedsParent() bool { return k == KindTeam }

// Container is one node of the tenancy tree.
type Container struct {
	ID   string `json:"id"`
	Kind Kind   `json:"kind"`

	// Name is for people. It is unique among its siblings of the same kind and
	// nothing else — two organisations may both have a team called
	// "Engineering", and authorization never sees either string.
	Name string `json:"name"`

	// ParentID is empty for a root org or any project.
	ParentID string `json:"parent_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Label is how this container appears in a binding scope and on a resource:
// "team:t_7f2a".
//
// It carries the ID and never the name. If it carried the name, a grant on
// Acme's "Engineering" would silently cover Globex's — a cross-tenant grant
// produced by nothing more than two people picking the same word. Microsoft
// documents this same mistake as "this common error" against their own
// management groups.
//
// The format must agree with auth.ScopeLabel, which builds the same string on
// the authorization side. TestLabelMatchesAuthScopeLabel pins that; the two
// cannot be one function because this package is dependency-free domain and
// that one lives in a separate module.
func (c Container) Label() string { return string(c.Kind) + ":" + c.ID }

// Filter narrows a container listing. Every field is optional.
type Filter struct {
	Kind Kind

	// ParentID selects children of one container. Use RootsOnly for the
	// containers that have no parent at all, which an empty ParentID cannot
	// express because empty is also "do not filter".
	ParentID  string
	RootsOnly bool

	// Name selects an exact name, which together with Kind and ParentID is the
	// per-parent uniqueness key — this is how a name resolves to an ID.
	Name string
}

// ResourceScopes is one resource's membership in the tree.
//
// Direct is what somebody declared: the containers this resource was put in.
// All is that closed over ancestry — the labels a binding is actually matched
// against, which is what lands in auth.ResourceRef.Scopes.
//
// Keeping both is what makes the tree editable. Reparenting a team has to
// recompute every affected resource's closure, and a closure alone cannot be
// recomputed from itself: the declaration is the source of truth and the
// closure is derived from it, the same split Kubernetes HNC keeps between a
// namespace's parent and the objects it propagates.
type ResourceScopes struct {
	ResourceType string   `json:"resource_type"`
	ResourceID   string   `json:"resource_id"`
	Direct       []string `json:"direct,omitempty"`
	All          []string `json:"all,omitempty"`
}

// ScopeFilter narrows a scoped-resource listing.
type ScopeFilter struct {
	// ResourceType restricts to one type ("twin"). Empty means every type.
	ResourceType string

	// Labels selects resources carrying any of these labels. Empty matches
	// nothing rather than everything: the callers are authorization filters,
	// and a filter that silently widens to "all rows" when its input is empty
	// is how a listing leaks.
	Labels []string

	// DirectOnly matches against declared membership rather than the closure.
	// Reindexing after a reparent needs it; an authorization filter never does.
	DirectOnly bool
}

// NormalizeLabels trims, drops empties and de-duplicates, preserving order.
// Both a declaration and a closure go through it so neither can carry a
// duplicate into the database or a decision trace.
func NormalizeLabels(labels []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, label)
	}
	return out
}
