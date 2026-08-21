package environment

import (
	"fmt"
	"sync"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/telemetry"
)

// BuildContext carries the per-loop context an environment factory needs to
// build a tenant-specific environment instance. Currently used to resolve
// twin-bound adapter instances at construction time; extensible without
// breaking callers (add fields, keep existing zero-value behavior).
type BuildContext struct {
	TwinID          string
	AdapterBindings map[string]string // slot name → instance name

	// Telemetry lets a pack observe this deployment's own behaviour — how
	// often it escalates, what it spends, which objectives keep failing.
	//
	// Nil for every pack that does not ask for it, which is all of them except
	// the karakuri pack (Phase 22). It is a read-only port declared in
	// internal/core/telemetry: a pack that could write there could rewrite the
	// evidence of what it did, and the point of letting Karakuri watch itself
	// is that the watching is trustworthy.
	Telemetry telemetry.Reader
}

type Factory struct {
	EnvID       EnvironmentID
	Domain      string
	Description string
	Build       func(ctx BuildContext) (Environment, error)

	// Serves names the capabilities this environment executes. It is what the
	// loop routes on: an action for a capability exactly one environment serves
	// goes there, whatever env_id the plan named.
	//
	// It is a declaration because the alternative was asking the model. Routing
	// used to be by Action.EnvID alone, which the planner chooses — so a plan
	// that wrote code without naming software.env.cli_agent reached noopEnv and
	// reported "unimplemented", and a planner hint stating the pairing was the
	// only thing holding the write path together. Which environment runs a
	// capability is a fact about the pack, known when it is written; a model
	// re-deriving it per plan can only get it wrong.
	//
	// Empty is honest for an environment that acts on nothing — the observe-only
	// ones — and leaves routing exactly where it was.
	Serves []capability.CapabilityID
}

type Registry struct {
	mu        sync.RWMutex
	factories map[EnvironmentID]Factory

	// routes is the reverse index of Factory.Serves, maintained on Register.
	//
	// A slice rather than a single ID because two packs may legitimately serve
	// the same capability, and because a capability served twice within one
	// pack must be visible as ambiguous rather than resolved by map iteration
	// order. Routing declines to guess between candidates; conformance is what
	// fails the pack that created the ambiguity.
	routes map[capability.CapabilityID][]EnvironmentID

	// telemetry is handed to every factory on its BuildContext.
	//
	// It lives on the registry rather than being threaded through the two
	// services that build environments, because the registry is already the
	// single thing both of them hold and because there is exactly one reader
	// per process. Set once at bootstrap; nil everywhere it is not.
	telemetry telemetry.Reader
}

// SetTelemetry wires the read-only self-observation port. Called once at
// bootstrap, before any environment is built.
func (r *Registry) SetTelemetry(t telemetry.Reader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.telemetry = t
}

// Telemetry returns the configured reader, or nil. Callers building a
// BuildContext copy it in; a pack that does not want it never notices.
func (r *Registry) Telemetry() telemetry.Reader {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.telemetry
}

func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[EnvironmentID]Factory),
		routes:    make(map[capability.CapabilityID][]EnvironmentID),
	}
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
	for _, capID := range f.Serves {
		if capID == "" {
			continue
		}
		r.routes[capID] = append(r.routes[capID], f.EnvID)
	}
	return nil
}

// ServedBy returns the environments that declared they execute this
// capability, in registration order. Empty when nothing claims it.
//
// Callers route on a single candidate and fall back on none or several: a
// capability two environments both claim has no answer the registry can give,
// and picking one would reintroduce the silent misrouting this replaced.
func (r *Registry) ServedBy(capID capability.CapabilityID) []EnvironmentID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := r.routes[capID]
	if len(ids) == 0 {
		return nil
	}
	return append([]EnvironmentID(nil), ids...)
}

func (r *Registry) Build(id EnvironmentID, ctx BuildContext) (Environment, error) {
	r.mu.RLock()
	f, ok := r.factories[id]
	tel := r.telemetry
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("environment %q not registered", id)
	}
	if ctx.Telemetry == nil {
		ctx.Telemetry = tel
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
