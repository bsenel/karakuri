package agent

import (
	"fmt"
	"sync"
)

type Registry struct {
	mu      sync.RWMutex
	agents  map[AgentID]Definition
}

func NewRegistry() *Registry {
	return &Registry{agents: make(map[AgentID]Definition)}
}

func (r *Registry) Register(def Definition) error {
	if def.ID == "" {
		return fmt.Errorf("agent ID must not be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.agents[def.ID]; exists {
		return fmt.Errorf("agent %q already registered", def.ID)
	}
	r.agents[def.ID] = def
	return nil
}

func (r *Registry) Get(id AgentID) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.agents[id]
	return d, ok
}

func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.agents))
	for _, d := range r.agents {
		out = append(out, d)
	}
	return out
}

func (r *Registry) ListByDomain(domain string) []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Definition
	for _, d := range r.agents {
		if d.Domain == domain {
			out = append(out, d)
		}
	}
	return out
}
