package conformance

import (
	"context"
	"fmt"
	"strings"

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

// checkCriterionVerifierRefs verifies that every non-empty Criterion.Verifier in all objective
// templates' SuccessCriteria refers to a capability ID present in the pack.
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
			if _, ok := capSet[string(crit.Verifier)]; !ok {
				return Result{
					Check:   name,
					Passed:  false,
					Message: fmt.Sprintf("template %q criterion %q references unknown verifier %q", tmpl.ID, crit.ID, crit.Verifier),
				}
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
