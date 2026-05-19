package domain

import (
	"context"
	"fmt"
	"sync"
)

type Registry struct {
	mu    sync.RWMutex
	packs map[string]Pack
	hints []PlannerHint
}

func NewRegistry() *Registry {
	return &Registry{packs: make(map[string]Pack)}
}

func (r *Registry) Register(ctx context.Context, p Pack, cfg Config) error {
	if p.ID() == "" {
		return fmt.Errorf("domain pack ID must not be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.packs[p.ID()]; exists {
		return fmt.Errorf("domain %q already registered", p.ID())
	}
	if err := p.Init(ctx, cfg); err != nil {
		return fmt.Errorf("init domain %q: %w", p.ID(), err)
	}
	r.packs[p.ID()] = p
	r.hints = append(r.hints, p.PlannerHints()...)
	return nil
}

func (r *Registry) Get(id string) (Pack, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.packs[id]
	return p, ok
}

func (r *Registry) List() []Pack {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Pack, 0, len(r.packs))
	for _, p := range r.packs {
		out = append(out, p)
	}
	return out
}

func (r *Registry) PlannerHints() []PlannerHint {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]PlannerHint, len(r.hints))
	copy(out, r.hints)
	return out
}
