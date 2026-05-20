package environment

import (
	"fmt"
	"sync"
)

// BuildContext carries the per-loop context an environment factory needs to
// build a tenant-specific environment instance. Currently used to resolve
// twin-bound adapter instances at construction time; extensible without
// breaking callers (add fields, keep existing zero-value behavior).
type BuildContext struct {
	TwinID          string
	AdapterBindings map[string]string // slot name → instance name
}

type Factory struct {
	EnvID       EnvironmentID
	Domain      string
	Description string
	Build       func(ctx BuildContext) (Environment, error)
}

type Registry struct {
	mu        sync.RWMutex
	factories map[EnvironmentID]Factory
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[EnvironmentID]Factory)}
}

func (r *Registry) Register(f Factory) error {
	if f.EnvID == "" {
		return fmt.Errorf("environment ID must not be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[f.EnvID]; exists {
		return fmt.Errorf("environment %q already registered", f.EnvID)
	}
	r.factories[f.EnvID] = f
	return nil
}

func (r *Registry) Build(id EnvironmentID, ctx BuildContext) (Environment, error) {
	r.mu.RLock()
	f, ok := r.factories[id]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("environment %q not registered", id)
	}
	return f.Build(ctx)
}

func (r *Registry) List() []Factory {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Factory, 0, len(r.factories))
	for _, f := range r.factories {
		out = append(out, f)
	}
	return out
}

func (r *Registry) ListByDomain(domain string) []Factory {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Factory
	for _, f := range r.factories {
		if f.Domain == domain {
			out = append(out, f)
		}
	}
	return out
}
