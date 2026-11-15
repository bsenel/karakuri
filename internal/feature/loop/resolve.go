package loop

import (
	"context"
	"log/slog"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// FindTemplate looks a template up by ID in every registered pack, since a
// cross-domain objective's template may live outside its primary domain. ok is
// false for an empty ID, a nil registry, or a template no pack declares.
//
// Exported for the same reason as SelectAgent: the supervisor reads the
// template's action limits into the bounds before the loop starts, and the
// loop reads its risk class, and both must find the same template.
func FindTemplate(domReg *domain.Registry, id string) (objective.Template, bool) {
	if id == "" || domReg == nil {
		return objective.Template{}, false
	}
	for _, pack := range domReg.List() {
		for _, tmpl := range pack.ObjectiveTemplates() {
			if tmpl.ID == id {
				return tmpl, true
			}
		}
	}
	return objective.Template{}, false
}

// withTemplateConstraints is the objective as the planner is shown it: its own
// constraints, then each of its template's whose ID it does not already hold.
//
// An objective's constraints are copied from its template only at creation,
// while the template's action limits are read at run time (FindTemplate in the
// supervisor), so an objective created before its template gained a constraint
// is held to limits its planner would otherwise never be told about. The
// result is a copy with a new Constraints slice, so appending never writes
// into the caller's backing array; it is shown to the agent, never saved.
func withTemplateConstraints(domReg *domain.Registry, obj objective.Objective) objective.Objective {
	tmpl, ok := FindTemplate(domReg, obj.TemplateID)
	if !ok {
		return obj
	}
	held := make(map[string]bool, len(obj.Constraints))
	for _, c := range obj.Constraints {
		held[c.ID] = true
	}
	constraints := make([]objective.Constraint, 0, len(obj.Constraints)+len(tmpl.Constraints))
	constraints = append(constraints, obj.Constraints...)
	for _, c := range tmpl.Constraints {
		if held[c.ID] {
			continue
		}
		held[c.ID] = true
		constraints = append(constraints, c)
	}
	obj.Constraints = constraints
	return obj
}

// shownObjective is sc.obj as every agent input carries it. A step context
// with no service has no registry to read a template from, so it shows the
// objective as it is.
func (sc *stepContext) shownObjective() objective.Objective {
	if sc.svc == nil {
		return sc.obj
	}
	return withTemplateConstraints(sc.svc.domReg, sc.obj)
}

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
		// An objective that names an agent gets that agent. This is what a
		// template's SuggestedAgents becomes at creation time, and it has to
		// be checked before the fallback: "the first agent the domain
		// declares" is an arbitrary answer that happened to be right while
		// packs were small.
		if obj.AgentID != "" {
			for _, d := range domains {
				pack, ok := domReg.Get(d)
				if !ok {
					continue
				}
				for _, def := range pack.AgentDefinitions() {
					if string(def.ID) == obj.AgentID {
						return def
					}
				}
			}
			// Named and not found: fall through to the default rather than
			// failing the objective, but say so — a typo in an agent name
			// should not silently run something else without a trace.
			slog.Warn("objective names an agent no enabled pack declares; falling back to the domain default",
				"objective", string(obj.ID), "agent", obj.AgentID)
		}
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
	buildCtx := environment.BuildContext{
		TwinID:          obj.TwinID,
		AdapterBindings: adapterBindings,
		// Nil unless bootstrap wired it, which is the ordinary case for every
		// pack except karakuri's.
		Telemetry: envReg.Telemetry(),
	}

	var envs []environment.Environment
	seen := make(map[string]bool)
	build := func(fac environment.Factory) {
		key := string(fac.EnvID)
		if seen[key] {
			return
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
			return
		}
		envs = append(envs, env)
	}

	for _, d := range obj.AllDomains() {
		for _, fac := range envReg.ListByDomain(d) {
			build(fac)
		}
	}

	// Tool sources, whatever the objective's domains are (Phase 28).
	//
	// An MCP server is not a pack and its tools belong to no subject matter, so
	// walking the objective's domains would never reach one — an operator who
	// bound a twin to a filesystem server would find its tools unreachable from
	// every objective they own. What confines these instead is the binding: the
	// factory refuses to build for a twin bound to another instance, so a twin
	// sees its own server and nobody else's (ADR 006, ADR 022).
	for _, fac := range envReg.ListByDomain(capability.MCPDomain) {
		build(fac)
	}
	return envs
}
