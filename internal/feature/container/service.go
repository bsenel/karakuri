// Package container manages the tenancy tree and the resource closure derived
// from it.
//
// Every guard that keeps the tree legible lives here rather than in
// authorization: cycles, depth, per-parent name uniqueness, and recomputing a
// resource's labels when the tree moves under it. That split is the whole
// design — because the closure is materialised at write time, an authorization
// check never walks a tree, and nesting costs nothing at request time. It is
// the same trade Zanzibar's Leopard index and Kubernetes HNC both make, and for
// the same reason: the read path is hot and the tree is not.
package container

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/container"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Errors this service returns. They wrap the domain sentinels so an HTTP layer
// can map them to a status without knowing what a container is.
var (
	// ErrCycle is a reparent that would make a container its own ancestor.
	ErrCycle = fmt.Errorf("%w: would create a cycle", coreerrors.ErrInvalidInput)

	// ErrTooDeep is a tree deeper than container.MaxDepth.
	ErrTooDeep = fmt.Errorf("%w: hierarchy deeper than %d levels", coreerrors.ErrInvalidInput, container.MaxDepth)

	// ErrDuplicateName is a name already taken by a sibling of the same kind.
	ErrDuplicateName = fmt.Errorf("%w: name already used by a sibling", coreerrors.ErrConflict)

	// ErrNotEmpty is a delete of a container that still has children.
	ErrNotEmpty = fmt.Errorf("%w: container still has children", coreerrors.ErrConflict)
)

// Store is the slice of persistence this service needs. It is an interface
// rather than storage.StorageAdapter so the tree can be tested without the
// whole storage surface, the way TwinOwnerLookup narrows the same adapter.
type Store interface {
	SaveContainer(ctx context.Context, c container.Container) error
	GetContainer(ctx context.Context, id string) (container.Container, error)
	ListContainers(ctx context.Context, f container.Filter) ([]container.Container, error)
	DeleteContainer(ctx context.Context, id string) error

	PutResourceScopes(ctx context.Context, s container.ResourceScopes) error
	GetResourceScopes(ctx context.Context, resourceType, resourceID string) (container.ResourceScopes, error)
	ListScopedResources(ctx context.Context, f container.ScopeFilter) ([]container.ResourceScopes, error)
	DeleteResourceScopes(ctx context.Context, resourceType, resourceID string) error
}

var _ Store = (storage.StorageAdapter)(nil)

// Service is the tree and everything derived from it.
type Service struct {
	store Store
}

func NewService(store Store) *Service { return &Service{store: store} }

// CreateRequest names a new container.
type CreateRequest struct {
	Kind     container.Kind
	Name     string
	ParentID string
}

// Create adds a container, rejecting anything that would make the tree
// ambiguous or unbounded.
func (s *Service) Create(ctx context.Context, req CreateRequest) (container.Container, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return container.Container{}, fmt.Errorf("%w: container needs a name", coreerrors.ErrInvalidInput)
	}
	if !req.Kind.Valid() {
		return container.Container{}, fmt.Errorf("%w: unknown container kind %q", coreerrors.ErrInvalidInput, req.Kind)
	}
	if err := s.checkParent(ctx, req.Kind, req.ParentID); err != nil {
		return container.Container{}, err
	}
	if err := s.checkNameFree(ctx, req.Kind, req.ParentID, name, ""); err != nil {
		return container.Container{}, err
	}

	now := time.Now().UTC()
	c := container.Container{
		ID: newID(req.Kind), Kind: req.Kind, Name: name, ParentID: req.ParentID,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.SaveContainer(ctx, c); err != nil {
		return container.Container{}, fmt.Errorf("save container: %w", err)
	}
	return c, nil
}

// Get returns one container.
func (s *Service) Get(ctx context.Context, id string) (container.Container, error) {
	return s.store.GetContainer(ctx, id)
}

// List returns containers matching a filter.
func (s *Service) List(ctx context.Context, f container.Filter) ([]container.Container, error) {
	return s.store.ListContainers(ctx, f)
}

// Rename changes a container's display name. Authorization is untouched by it,
// which is the payoff of scoping on IDs: under a path model every binding
// naming the old name would have to be rewritten, and any that was missed would
// be a silent grant or a silent denial.
func (s *Service) Rename(ctx context.Context, id, name string) (container.Container, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return container.Container{}, fmt.Errorf("%w: container needs a name", coreerrors.ErrInvalidInput)
	}
	c, err := s.store.GetContainer(ctx, id)
	if err != nil {
		return container.Container{}, err
	}
	if c.Name == name {
		return c, nil
	}
	if err := s.checkNameFree(ctx, c.Kind, c.ParentID, name, c.ID); err != nil {
		return container.Container{}, err
	}
	c.Name = name
	c.UpdatedAt = time.Now().UTC()
	if err := s.store.SaveContainer(ctx, c); err != nil {
		return container.Container{}, fmt.Errorf("save container: %w", err)
	}
	return c, nil
}

// Reparent moves a container, then rebuilds the closure of every resource
// beneath it.
//
// The rebuild is the expensive half and it is deliberately here: a resource
// that moved out of an organisation has to stop being visible to that
// organisation the moment the move commits, not the next time somebody reads
// it. HNC makes the same call — reparenting deletes the propagated objects that
// no longer belong to the new ancestry.
func (s *Service) Reparent(ctx context.Context, id, parentID string) (container.Container, error) {
	c, err := s.store.GetContainer(ctx, id)
	if err != nil {
		return container.Container{}, err
	}
	if c.ParentID == parentID {
		return c, nil
	}
	if err := s.checkParent(ctx, c.Kind, parentID); err != nil {
		return container.Container{}, err
	}
	if err := s.checkNoCycle(ctx, c.ID, parentID); err != nil {
		return container.Container{}, err
	}
	if err := s.checkNameFree(ctx, c.Kind, parentID, c.Name, c.ID); err != nil {
		return container.Container{}, err
	}
	// Depth is checked against the deepest descendant, not against the moved
	// container: moving a leaf under a deep parent is fine, moving a whole
	// subtree there may not be.
	depth, err := s.subtreeDepth(ctx, c.ID)
	if err != nil {
		return container.Container{}, err
	}
	above, err := s.ancestors(ctx, parentID)
	if err != nil {
		return container.Container{}, err
	}
	if len(above)+depth > container.MaxDepth {
		return container.Container{}, ErrTooDeep
	}

	c.ParentID = parentID
	c.UpdatedAt = time.Now().UTC()
	if err := s.store.SaveContainer(ctx, c); err != nil {
		return container.Container{}, fmt.Errorf("save container: %w", err)
	}
	if err := s.ReindexSubtree(ctx, c.ID); err != nil {
		return container.Container{}, err
	}
	return c, nil
}

// Delete removes a leaf container.
//
// A container with children is refused rather than cascaded. A cascade here
// would revoke access to everything underneath in one call, and an operator who
// meant to delete an empty team should not be one typo away from that.
func (s *Service) Delete(ctx context.Context, id string) error {
	c, err := s.store.GetContainer(ctx, id)
	if err != nil {
		return err
	}
	children, err := s.store.ListContainers(ctx, container.Filter{ParentID: c.ID})
	if err != nil {
		return fmt.Errorf("list children of %q: %w", c.ID, err)
	}
	if len(children) > 0 {
		return ErrNotEmpty
	}
	// Resources declared in this container lose that membership; anything they
	// inherited through it goes with it. Doing this before the delete means a
	// failure leaves the container in place rather than leaving orphan labels
	// pointing at a container that no longer exists.
	members, err := s.store.ListScopedResources(ctx, container.ScopeFilter{
		Labels: []string{c.Label()}, DirectOnly: true,
	})
	if err != nil {
		return fmt.Errorf("list members of %q: %w", c.ID, err)
	}
	for _, m := range members {
		remaining := slices.DeleteFunc(slices.Clone(m.Direct), func(label string) bool {
			return label == c.Label()
		})
		if err := s.SetResourceContainers(ctx, m.ResourceType, m.ResourceID, labelsToIDs(remaining)); err != nil {
			return err
		}
	}
	return s.store.DeleteContainer(ctx, c.ID)
}

// Closure returns a container's label and every ancestor's, nearest first.
//
// This is the list that ends up in auth.ResourceRef.Scopes, and the reason a
// binding on an org covers a twin in a team three levels down without
// authorization knowing either exists.
func (s *Service) Closure(ctx context.Context, id string) ([]string, error) {
	chain, err := s.chain(ctx, id)
	if err != nil {
		return nil, err
	}
	labels := make([]string, 0, len(chain))
	for _, c := range chain {
		labels = append(labels, c.Label())
	}
	return labels, nil
}

// SetResourceContainers declares which containers a resource belongs to and
// writes the resulting closure.
//
// Passing no containers is how a resource leaves the tree entirely, after which
// it matches exactly what it matched before Phase 17 — its own ID and the
// wildcards. That is not a special case in the code and must not become one.
func (s *Service) SetResourceContainers(ctx context.Context, resourceType, resourceID string, containerIDs []string) error {
	if resourceType == "" || resourceID == "" {
		return fmt.Errorf("%w: resource scopes need a type and an id", coreerrors.ErrInvalidInput)
	}

	var direct, all []string
	for _, id := range containerIDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		closure, err := s.Closure(ctx, id)
		if err != nil {
			return err
		}
		direct = append(direct, closure[0])
		all = append(all, closure...)
	}
	return s.store.PutResourceScopes(ctx, container.ResourceScopes{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Direct:       container.NormalizeLabels(direct),
		All:          container.NormalizeLabels(all),
	})
}

// ScopesOf returns the labels a resource carries — what a caller hands to
// auth.ResourceRef.WithScopes.
func (s *Service) ScopesOf(ctx context.Context, resourceType, resourceID string) ([]string, error) {
	scopes, err := s.store.GetResourceScopes(ctx, resourceType, resourceID)
	if err != nil {
		return nil, err
	}
	return scopes.All, nil
}

// ReindexSubtree recomputes the closure of every resource declared anywhere in
// a container's subtree. Reparent calls it; so should anything else that moves
// the tree.
func (s *Service) ReindexSubtree(ctx context.Context, id string) error {
	subtree, err := s.descendants(ctx, id)
	if err != nil {
		return err
	}
	labels := make([]string, 0, len(subtree))
	for _, c := range subtree {
		labels = append(labels, c.Label())
	}

	// DirectOnly: a resource whose only connection to this subtree was an
	// inherited label is a resource whose declaration lives elsewhere, and
	// recomputing from that declaration is what fixes it — so it is reached
	// through its own container, not this one.
	members, err := s.store.ListScopedResources(ctx, container.ScopeFilter{
		Labels: labels, DirectOnly: true,
	})
	if err != nil {
		return fmt.Errorf("list members of %q: %w", id, err)
	}
	for _, m := range members {
		if err := s.SetResourceContainers(ctx, m.ResourceType, m.ResourceID, labelsToIDs(m.Direct)); err != nil {
			return err
		}
	}
	return nil
}

// chain returns a container and its ancestors, nearest first.
func (s *Service) chain(ctx context.Context, id string) ([]container.Container, error) {
	var out []container.Container
	seen := map[string]bool{}
	for next := id; next != ""; {
		if seen[next] {
			// Unreachable through this service — Reparent rejects cycles — but
			// a hand-edited database is not this loop's problem to run forever
			// over.
			return nil, ErrCycle
		}
		seen[next] = true
		c, err := s.store.GetContainer(ctx, next)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
		if len(out) > container.MaxDepth {
			return nil, ErrTooDeep
		}
		next = c.ParentID
	}
	return out, nil
}

// ancestors returns the chain above a container, excluding it. An empty id has
// none, which is what makes a root creatable.
func (s *Service) ancestors(ctx context.Context, id string) ([]container.Container, error) {
	if id == "" {
		return nil, nil
	}
	return s.chain(ctx, id)
}

// descendants returns a container and everything beneath it.
func (s *Service) descendants(ctx context.Context, id string) ([]container.Container, error) {
	root, err := s.store.GetContainer(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []container.Container{root}
	for i := 0; i < len(out); i++ {
		children, err := s.store.ListContainers(ctx, container.Filter{ParentID: out[i].ID})
		if err != nil {
			return nil, fmt.Errorf("list children of %q: %w", out[i].ID, err)
		}
		out = append(out, children...)
	}
	return out, nil
}

// subtreeDepth returns how many levels a container spans, counting itself as 1.
func (s *Service) subtreeDepth(ctx context.Context, id string) (int, error) {
	subtree, err := s.descendants(ctx, id)
	if err != nil {
		return 0, err
	}
	depth := map[string]int{id: 1}
	deepest := 1
	for _, c := range subtree {
		if c.ID == id {
			continue
		}
		d := depth[c.ParentID] + 1
		depth[c.ID] = d
		if d > deepest {
			deepest = d
		}
	}
	return deepest, nil
}

// checkParent enforces what may sit inside what, and the depth bound.
func (s *Service) checkParent(ctx context.Context, kind container.Kind, parentID string) error {
	if parentID == "" {
		if kind.NeedsParent() {
			return fmt.Errorf("%w: a %s must belong to an organisation", coreerrors.ErrInvalidInput, kind)
		}
		return nil
	}
	parent, err := s.store.GetContainer(ctx, parentID)
	if err != nil {
		return err
	}
	if !kind.CanNestUnder(parent.Kind) {
		return fmt.Errorf("%w: a %s cannot sit inside a %s", coreerrors.ErrInvalidInput, kind, parent.Kind)
	}
	above, err := s.ancestors(ctx, parentID)
	if err != nil {
		return err
	}
	if len(above)+1 > container.MaxDepth {
		return ErrTooDeep
	}
	return nil
}

// checkNoCycle refuses a move that would put a container inside itself.
func (s *Service) checkNoCycle(ctx context.Context, id, parentID string) error {
	if parentID == "" {
		return nil
	}
	if parentID == id {
		return ErrCycle
	}
	above, err := s.ancestors(ctx, parentID)
	if err != nil {
		return err
	}
	for _, c := range above {
		if c.ID == id {
			return ErrCycle
		}
	}
	return nil
}

// checkNameFree enforces per-parent uniqueness, ignoring the container being
// renamed or moved.
//
// The scope of this rule is the whole point: names are unique among siblings
// and nowhere else, so two organisations may each have a team called
// "Engineering" and neither grant reaches the other.
func (s *Service) checkNameFree(ctx context.Context, kind container.Kind, parentID, name, exceptID string) error {
	siblings, err := s.store.ListContainers(ctx, container.Filter{
		Kind: kind, ParentID: parentID, RootsOnly: parentID == "", Name: name,
	})
	if err != nil {
		return fmt.Errorf("check name %q: %w", name, err)
	}
	for _, c := range siblings {
		if c.ID != exceptID {
			return ErrDuplicateName
		}
	}
	return nil
}

// labelsToIDs strips the kind prefix off labels, which is what SetResourceContainers
// takes. It is the inverse of Container.Label and only ever runs on labels this
// service wrote.
func labelsToIDs(labels []string) []string {
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		if _, id, ok := strings.Cut(label, ":"); ok && id != "" {
			out = append(out, id)
		}
	}
	return out
}

// newID mints a container ID with a one-letter kind prefix, so an operator
// reading a log or a binding can tell an org from a team without a lookup. The
// prefix is a convenience; nothing parses it.
func newID(kind container.Kind) string {
	b := make([]byte, 6)
	// crypto/rand.Read cannot fail on any platform this runs on; Go 1.24 made
	// it panic rather than return an error for exactly that reason.
	_, _ = rand.Read(b)
	prefix := "c"
	if len(kind) > 0 {
		prefix = string(kind[0])
	}
	return prefix + "_" + hex.EncodeToString(b)
}
