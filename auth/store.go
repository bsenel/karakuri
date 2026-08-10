package auth

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// Store persists the authorization model. Implementations must return deep
// copies: the authorizer reads concurrently and must never be able to mutate
// stored state through a returned value.
//
// Policies are reached through roles rather than stored standalone, because a
// policy with no role has nobody to apply to.
type Store interface {
	GetPrincipal(ctx context.Context, id string) (Principal, error)
	ListPrincipals(ctx context.Context) ([]Principal, error)
	PutPrincipal(ctx context.Context, p Principal) error
	DeletePrincipal(ctx context.Context, id string) error

	GetRole(ctx context.Context, name string) (Role, error)
	ListRoles(ctx context.Context) ([]Role, error)
	PutRole(ctx context.Context, r Role) error
	DeleteRole(ctx context.Context, name string) error

	ListBindings(ctx context.Context, principalID string) ([]RoleBinding, error)
	PutBinding(ctx context.Context, b RoleBinding) error
	DeleteBinding(ctx context.Context, id string) error
}

// MemoryStore is the reference Store implementation. It is the one used by the
// examples and the test suite, and is production-usable for deployments whose
// principal set is seeded from config at boot.
type MemoryStore struct {
	mu         sync.RWMutex
	principals map[string]Principal
	roles      map[string]Role
	bindings   map[string]RoleBinding
}

// NewMemoryStore returns an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		principals: map[string]Principal{},
		roles:      map[string]Role{},
		bindings:   map[string]RoleBinding{},
	}
}

var _ Store = (*MemoryStore)(nil)

func (s *MemoryStore) GetPrincipal(_ context.Context, id string) (Principal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.principals[id]
	if !ok {
		return Principal{}, fmt.Errorf("%w: %q", ErrPrincipalNotFound, id)
	}
	return p.Clone(), nil
}

func (s *MemoryStore) ListPrincipals(_ context.Context) ([]Principal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Principal, 0, len(s.principals))
	for _, p := range s.principals {
		out = append(out, p.Clone())
	}
	slices.SortFunc(out, func(a, b Principal) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

func (s *MemoryStore) PutPrincipal(_ context.Context, p Principal) error {
	if p.ID == "" {
		return fmt.Errorf("%w: principal ID is required", ErrInvalidPattern)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.principals[p.ID] = p.Clone()
	return nil
}

func (s *MemoryStore) DeletePrincipal(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.principals[id]; !ok {
		return fmt.Errorf("%w: %q", ErrPrincipalNotFound, id)
	}
	delete(s.principals, id)
	// Bindings outlive nothing: a deleted principal must not leave grants behind.
	for bid, b := range s.bindings {
		if b.PrincipalID == id {
			delete(s.bindings, bid)
		}
	}
	return nil
}

func (s *MemoryStore) GetRole(_ context.Context, name string) (Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.roles[name]
	if !ok {
		return Role{}, fmt.Errorf("%w: %q", ErrRoleNotFound, name)
	}
	return r.Clone(), nil
}

func (s *MemoryStore) ListRoles(_ context.Context) ([]Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Role, 0, len(s.roles))
	for _, r := range s.roles {
		out = append(out, r.Clone())
	}
	slices.SortFunc(out, func(a, b Role) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func (s *MemoryStore) PutRole(_ context.Context, r Role) error {
	if r.Name == "" {
		return fmt.Errorf("%w: role name is required", ErrInvalidPattern)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.roles[r.Name]; ok && existing.System && !r.System {
		return fmt.Errorf("%w: %q", ErrSystemRole, r.Name)
	}
	s.roles[r.Name] = r.Clone()
	return nil
}

func (s *MemoryStore) DeleteRole(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.roles[name]
	if !ok {
		return fmt.Errorf("%w: %q", ErrRoleNotFound, name)
	}
	if r.System {
		return fmt.Errorf("%w: %q", ErrSystemRole, name)
	}
	delete(s.roles, name)
	return nil
}

func (s *MemoryStore) ListBindings(_ context.Context, principalID string) ([]RoleBinding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RoleBinding, 0, len(s.bindings))
	for _, b := range s.bindings {
		if principalID == "" || b.PrincipalID == principalID {
			out = append(out, b)
		}
	}
	slices.SortFunc(out, func(a, b RoleBinding) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

func (s *MemoryStore) PutBinding(_ context.Context, b RoleBinding) error {
	if b.ID == "" || b.PrincipalID == "" || b.Role == "" {
		return fmt.Errorf("%w: binding needs an ID, principal and role", ErrInvalidPattern)
	}
	if err := validatePattern(b.EffectiveScope()); err != nil {
		return fmt.Errorf("binding %q scope: %w", b.ID, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bindings[b.ID] = b
	return nil
}

func (s *MemoryStore) DeleteBinding(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.bindings[id]; !ok {
		return fmt.Errorf("%w: %q", ErrBindingNotFound, id)
	}
	delete(s.bindings, id)
	return nil
}
