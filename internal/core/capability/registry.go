package capability

import (
	"fmt"
	"sync"
)

type Registry struct {
	mu           sync.RWMutex
	capabilities map[CapabilityID]Capability
}

func NewRegistry() *Registry {
	return &Registry{capabilities: make(map[CapabilityID]Capability)}
}

func (r *Registry) Register(c Capability) error {
	if c.ID == "" {
		return fmt.Errorf("capability ID must not be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.capabilities[c.ID]; exists {
		return fmt.Errorf("capability %q already registered", c.ID)
	}
	r.capabilities[c.ID] = c
	return nil
}

func (r *Registry) Get(id CapabilityID) (Capability, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.capabilities[id]
	return c, ok
}

func (r *Registry) List() []Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Capability, 0, len(r.capabilities))
	for _, c := range r.capabilities {
		out = append(out, c)
	}
	return out
}

func (r *Registry) ListByDomain(domain string) []Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Capability
	for _, c := range r.capabilities {
		if c.Domain == domain {
			out = append(out, c)
		}
	}
	return out
}
