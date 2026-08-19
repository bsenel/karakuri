package loop

import (
	"context"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// SelectAgent picks the agent definition an objective runs under.
//
// For a cross-domain objective it walks the declared domains in order and
// takes the first pack that exposes an agent; when none do, it synthesises a
// minimal ReAct agent tagged with the primary domain so an objective in a
// domain with no agents still runs rather than failing at wiring.
//
// Exported because the reconcile supervisor needs the same answer before the
// loop starts: it renders an objective's earned autonomy into
// agent.AuthorityBounds, and it cannot do that without knowing whose bounds
// they are. Two implementations of "which agent runs this" would be two
// answers, and the one the supervisor used to compute authority would be the
// wrong one exactly when the packs disagreed.
func SelectAgent(domReg *domain.Registry, obj objective.Objective, explicit coreagent.Definition) coreagent.Definition {
	if explicit.ID != "" {
		return explicit
	}
	domains := obj.AllDomains()
	if len(domains) == 0 {
		domains = []string{obj.Domain}
	}
	if domReg != nil {
		for _, d := range domains {
			pack, ok := domReg.Get(d)
			if !ok || len(pack.AgentDefinitions()) == 0 {
				continue
			}
			return pack.AgentDefinitions()[0]
		}
	}
	return coreagent.Definition{
		ID:                coreagent.AgentID(obj.Domain + "-default"),
		Name:              obj.Domain + " Agent",
		Domain:            obj.Domain,
		ReasoningStrategy: coreagent.ReasoningReAct,
		// No pack claimed this domain, so nobody has declared what this
		// agent may do. Zero autonomous actions is the honest bound: it
		// plans, and a human approves before anything happens. Stated
		// rather than left to the zero value, because a bound nobody
		// wrote down is the thing that goes wrong quietly.
		Authority: coreagent.AuthorityBounds{MaxAutonomousActions: 0},
	}
}

// BuildEnvironments constructs every environment an objective observes, across
// the union of its domains, resolving each twin's adapter bindings so a
// multi-tenant deployment reaches the right GitHub or Linear instance
// (ADR 006).
//
// A factory that fails to build publishes adapter_skipped and is left out
// rather than failing the objective: some environments are optional, and an
// unconfigured Slack should not stop a git objective.
//
// Exported for the same reason as SelectAgent. The reconcile supervisor senses
// drift by hashing exactly these environments, and if it built a different set
// than the loop observes, it would be watching one world and converging
// another.
func BuildEnvironments(
	ctx context.Context,
	store storage.StorageAdapter,
	envReg *environment.Registry,
	hub *event.Hub,
	obj objective.Objective,
) []environment.Environment {
	if envReg == nil {
		return nil
	}
	var adapterBindings map[string]string
	if obj.TwinID != "" && store != nil {
		if t, err := store.GetTwin(ctx, obj.TwinID); err == nil {
			adapterBindings = t.AdapterBindings
		}
	}
	buildCtx := environment.BuildContext{TwinID: obj.TwinID, AdapterBindings: adapterBindings}

	var envs []environment.Environment
	seen := make(map[string]bool)
	for _, d := range obj.AllDomains() {
		for _, fac := range envReg.ListByDomain(d) {
			key := string(fac.EnvID)
			if seen[key] {
				continue
			}
			seen[key] = true
			env, err := fac.Build(buildCtx)
			if err != nil {
				if hub != nil {
					hub.Publish(ctx, event.Event{
						Type:        event.TypeAdapterSkipped,
						ObjectiveID: string(obj.ID),
						TwinID:      obj.TwinID,
						Payload:     map[string]any{"env_id": string(fac.EnvID), "error": err.Error()},
						Timestamp:   time.Now().UTC(),
					})
				}
				continue
			}
			envs = append(envs, env)
		}
	}
	return envs
}
