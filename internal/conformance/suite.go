package conformance

import (
	"context"
	"fmt"
	"strings"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// Result holds the outcome of a single conformance check.
type Result struct {
	Check   string `json:"check"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

// Suite runs conformance checks against a domain.Pack.
type Suite struct{}

// New returns a ready-to-use Suite.
func New() *Suite { return &Suite{} }

// Run executes all conformance checks against p and returns one Result per check.
func (s *Suite) Run(ctx context.Context, p domain.Pack) []Result {
	var results []Result
	checks := []func(context.Context, domain.Pack) Result{
		checkIDFormat,
		checkCapabilitySchemas,
		checkEnvironmentFactories,
		checkCapabilityRouting,
		checkAgentCapabilityRefs,
		checkCriterionVerifierRefs,
		checkNoCapabilityIDCollision,
		checkTeardownNoPanic,
	}
	for _, check := range checks {
		results = append(results, check(ctx, p))
	}
	return results
}

// checkIDFormat verifies the pack ID is non-empty, lowercase, and contains no spaces.
func checkIDFormat(_ context.Context, p domain.Pack) Result {
	const name = "id_format"
	id := p.ID()
	if id == "" {
		return Result{Check: name, Passed: false, Message: "pack ID must not be empty"}
	}
	if strings.ToLower(id) != id {
		return Result{Check: name, Passed: false, Message: fmt.Sprintf("pack ID %q must be lowercase", id)}
	}
	if strings.ContainsAny(id, " \t\n\r") {
		return Result{Check: name, Passed: false, Message: fmt.Sprintf("pack ID %q must not contain whitespace", id)}
	}
	return Result{Check: name, Passed: true, Message: fmt.Sprintf("pack ID %q is valid", id)}
}

// checkCapabilitySchemas verifies every capability has non-empty InputSchema.Type and OutputSchema.Type.
func checkCapabilitySchemas(_ context.Context, p domain.Pack) Result {
	const name = "capability_schemas"
	for _, cap := range p.Capabilities() {
		if cap.InputSchema.Type == "" {
			return Result{
				Check:   name,
				Passed:  false,
				Message: fmt.Sprintf("capability %q has empty InputSchema.Type", cap.ID),
			}
		}
		if cap.OutputSchema.Type == "" {
			return Result{
				Check:   name,
				Passed:  false,
				Message: fmt.Sprintf("capability %q has empty OutputSchema.Type", cap.ID),
			}
		}
	}
	return Result{Check: name, Passed: true, Message: fmt.Sprintf("all %d capabilities have valid schemas", len(p.Capabilities()))}
}

// checkEnvironmentFactories verifies every factory's Build returns a non-nil
// environment without error when called with a zero-value BuildContext.
func checkEnvironmentFactories(_ context.Context, p domain.Pack) Result {
	const name = "environment_factories"
	for _, f := range p.EnvironmentFactories() {
		env, err := f.Build(environment.BuildContext{})
		if err != nil {
			return Result{
				Check:   name,
				Passed:  false,
				Message: fmt.Sprintf("factory %q Build returned error: %v", f.EnvID, err),
			}
		}
		if env == nil {
			return Result{
				Check:   name,
				Passed:  false,
				Message: fmt.Sprintf("factory %q Build returned nil environment", f.EnvID),
			}
		}
	}
	return Result{Check: name, Passed: true, Message: fmt.Sprintf("all %d environment factories build successfully", len(p.EnvironmentFactories()))}
}

// checkCapabilityRouting verifies the pack's Serves declarations can actually
// route, which is now what the loop resolves actions through.
//
// Three ways a declaration fails to be one:
//
//   - It names a capability the pack does not declare. A route to nothing is a
//     typo that surfaces as an action reaching noopEnv.
//   - Two of the pack's environments claim the same capability. The registry
//     refuses to pick between them and falls back to the plan's env_id, so an
//     ambiguous declaration silently returns routing to the model it was built
//     to take it away from.
//   - A capability declares NeedsWorkspace and nothing serves it. That is the
//     exact shape of the bug this phase exists for: write_code was given a git
//     worktree and had no environment behind it, so every plan that used it
//     created a branch and returned "unimplemented". A capability that writes
//     and routes nowhere is inert in the most expensive way available.
//
// A capability with no route is otherwise fine and common: most are reasoned
// about or verified rather than executed, and packs with no environments at all
// pass this trivially.
func checkCapabilityRouting(_ context.Context, p domain.Pack) Result {
	const name = "capability_routing"

	declared := make(map[capability.CapabilityID]capability.Capability, len(p.Capabilities()))
	for _, c := range p.Capabilities() {
		declared[c.ID] = c
	}

	servedBy := make(map[capability.CapabilityID]environment.EnvironmentID)
	for _, f := range p.EnvironmentFactories() {
		for _, capID := range f.Serves {
			if _, ok := declared[capID]; !ok {
				return Result{
					Check:  name,
					Passed: false,
					Message: fmt.Sprintf("environment %q serves %q, which this pack does not declare",
						f.EnvID, capID),
				}
			}
			if other, dup := servedBy[capID]; dup {
				return Result{
					Check:  name,
					Passed: false,
					Message: fmt.Sprintf("capability %q is served by both %q and %q; routing cannot choose between them",
						capID, other, f.EnvID),
				}
			}
			servedBy[capID] = f.EnvID
		}
	}

	for _, c := range p.Capabilities() {
		if c.NeedsWorkspace && servedBy[c.ID] == "" {
			return Result{
				Check:  name,
				Passed: false,
				Message: fmt.Sprintf("capability %q declares NeedsWorkspace but no environment serves it: it would be given a worktree and then fail", c.ID),
			}
		}
	}

	return Result{
		Check:   name,
		Passed:  true,
		Message: fmt.Sprintf("%d capabilities route to an environment that serves them", len(servedBy)),
	}
}

// checkAgentCapabilityRefs verifies that all capability IDs referenced by each agent definition
// appear in the pack's capability list.
func checkAgentCapabilityRefs(_ context.Context, p domain.Pack) Result {
	const name = "agent_capability_refs"

	capSet := make(map[string]struct{})
	for _, c := range p.Capabilities() {
		capSet[string(c.ID)] = struct{}{}
	}

	for _, def := range p.AgentDefinitions() {
		for _, capID := range def.Capabilities {
			if _, ok := capSet[string(capID)]; !ok {
				return Result{
					Check:   name,
					Passed:  false,
					Message: fmt.Sprintf("agent %q references unknown capability %q", def.ID, capID),
				}
			}
		}
	}
	return Result{Check: name, Passed: true, Message: fmt.Sprintf("all agent capability references are valid across %d agents", len(p.AgentDefinitions()))}
}

// checkCriterionVerifierRefs verifies that every non-empty Criterion.Verifier
// resolves — locally by default, or in another pack when the criterion says so.
//
// A criterion may name a capability this pack does not own, which is what
// Phase 13's cross-domain objectives are for: a template can require a step
// only another pack can perform, and be verified by that pack. What it must
// not do is name one silently. Criterion.Domain is the declaration, and
// requiring it here means a typo in a local verifier still fails — it would
// otherwise be indistinguishable from a deliberate cross-pack reference.
//
// The named domain is not resolved. A pack is validated on its own, and
// checking that another one exists and exports the capability is the
// registry's job at boot (CheckCrossPackCollisions and the environment
// registry), not a conformance check that would make every pack's validity
// depend on which other packs happen to be enabled.
func checkCriterionVerifierRefs(_ context.Context, p domain.Pack) Result {
	const name = "criterion_verifier_refs"

	capSet := make(map[string]struct{})
	for _, c := range p.Capabilities() {
		capSet[string(c.ID)] = struct{}{}
	}

	for _, tmpl := range p.ObjectiveTemplates() {
		for _, crit := range tmpl.SuccessCriteria {
			if crit.Verifier == "" {
				continue
			}
			if _, ok := capSet[string(crit.Verifier)]; ok {
				continue
			}
			if crit.Domain != "" && crit.Domain != p.ID() {
				continue // declared as another pack's, which is allowed
			}
			return Result{
				Check:  name,
				Passed: false,
				Message: fmt.Sprintf(
					"template %q criterion %q references unknown verifier %q (set Criterion.Domain if it belongs to another pack)",
					tmpl.ID, crit.ID, crit.Verifier),
			}
		}
	}
	return Result{Check: name, Passed: true, Message: fmt.Sprintf("all criterion verifier references are valid across %d templates", len(p.ObjectiveTemplates()))}
}

// checkNoCapabilityIDCollision verifies no two capabilities share the same ID.
func checkNoCapabilityIDCollision(_ context.Context, p domain.Pack) Result {
	const name = "no_capability_id_collision"
	seen := make(map[string]struct{})
	for _, c := range p.Capabilities() {
		id := string(c.ID)
		if _, exists := seen[id]; exists {
			return Result{
				Check:   name,
				Passed:  false,
				Message: fmt.Sprintf("duplicate capability ID %q", id),
			}
		}
		seen[id] = struct{}{}
	}
	return Result{Check: name, Passed: true, Message: fmt.Sprintf("no ID collisions among %d capabilities", len(p.Capabilities()))}
}

// checkTeardownNoPanic calls p.Teardown inside a deferred recover and fails if it panics.
func checkTeardownNoPanic(ctx context.Context, p domain.Pack) (res Result) {
	const name = "teardown_no_panic"
	res = Result{Check: name, Passed: true, Message: "Teardown completed without panic"}
	defer func() {
		if r := recover(); r != nil {
			res = Result{
				Check:   name,
				Passed:  false,
				Message: fmt.Sprintf("Teardown panicked: %v", r),
			}
		}
	}()
	_ = p.Teardown(ctx)
	return res
}

// CheckCrossPackCollisions verifies no two packs share the same capability ID,
// environment ID, or agent ID. Run this against the set of packs that will be
// active simultaneously — at minimum the union of domains referenced by any
// cross-domain objective. Returns a slice of Result so a single audit pass can
// surface every collision instead of stopping at the first.
//
// Each check is independent of the per-pack Run() — Run() rejects collisions
// within one pack; this rejects them across packs.
func CheckCrossPackCollisions(packs ...domain.Pack) []Result {
	if len(packs) < 2 {
		return []Result{{
			Check:   "cross_pack_capability_collision",
			Passed:  true,
			Message: "fewer than two packs supplied; nothing to compare",
		}}
	}

	var results []Result

	results = append(results, collisionCheck(
		"cross_pack_capability_collision",
		packs,
		func(p domain.Pack) []string {
			out := make([]string, 0, len(p.Capabilities()))
			for _, c := range p.Capabilities() {
				out = append(out, string(c.ID))
			}
			return out
		},
	))
	results = append(results, collisionCheck(
		"cross_pack_environment_collision",
		packs,
		func(p domain.Pack) []string {
			facs := p.EnvironmentFactories()
			out := make([]string, 0, len(facs))
			for _, f := range facs {
				out = append(out, string(f.EnvID))
			}
			return out
		},
	))
	results = append(results, collisionCheck(
		"cross_pack_agent_collision",
		packs,
		func(p domain.Pack) []string {
			defs := p.AgentDefinitions()
			out := make([]string, 0, len(defs))
			for _, d := range defs {
				out = append(out, string(d.ID))
			}
			return out
		},
	))

	return results
}

// collisionCheck builds a {ID → packs that declare it} map and reports any
// ID claimed by more than one pack. Pack IDs in the failure message are
// sorted for stable, diffable output.
func collisionCheck(name string, packs []domain.Pack, extract func(domain.Pack) []string) Result {
	owners := make(map[string][]string)
	for _, p := range packs {
		for _, id := range extract(p) {
			if id == "" {
				continue
			}
			owners[id] = append(owners[id], p.ID())
		}
	}
	for id, ps := range owners {
		if len(ps) > 1 {
			return Result{
				Check:   name,
				Passed:  false,
				Message: fmt.Sprintf("id %q declared by multiple packs: %s", id, strings.Join(ps, ", ")),
			}
		}
	}
	return Result{Check: name, Passed: true, Message: fmt.Sprintf("no collisions across %d packs", len(packs))}
}
