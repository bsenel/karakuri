package llm

import (
	"context"
	"sync"
)

type Registry struct {
	mu        sync.RWMutex
	providers map[string]ProviderAdapter
	fallback  map[string]string
}

func NewRegistry(fallback map[string]string) *Registry {
	return &Registry{
		providers: make(map[string]ProviderAdapter),
		fallback:  fallback,
	}
}

func (r *Registry) Register(p ProviderAdapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

func (r *Registry) Get(name string) (ProviderAdapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	if ok && p.Available(context.Background()) {
		return p, true
	}
	if fb, ok := r.fallback[name]; ok {
		if p2, ok2 := r.providers[fb]; ok2 {
			return p2, true
		}
	}
	if p, ok := r.providers["claude"]; ok {
		return p, true
	}
	return nil, false
}

func (r *Registry) Active() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var names []string
	for name, p := range r.providers {
		if p.Available(context.Background()) {
			names = append(names, name)
		}
	}
	return names
}

func (r *Registry) All() map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]bool)
	for name, p := range r.providers {
		out[name] = p.Available(context.Background())
	}
	return out
}
