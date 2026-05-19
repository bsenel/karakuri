package environment

import (
	"fmt"
	"sync"
)

type Factory struct {
	EnvID       EnvironmentID
	Domain      string
	Description string
	Build       func(cfg map[string]any) (Environment, error)
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

func (r *Registry) Build(id EnvironmentID, cfg map[string]any) (Environment, error) {
	r.mu.RLock()
	f, ok := r.factories[id]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("environment %q not registered", id)
	}
	return f.Build(cfg)
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
