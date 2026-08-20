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
		checkAgentBoundsBehave,
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

// checkAgentBoundsBehave runs each agent's declared bounds through the real
// decision policy and asserts the mechanism does what the declaration says.
//
// This phase exists because of a specific bug. `MaxAutonomousActions: 0` —
// written by four packs to mean "plans but never acts", with healthcare saying
// so in a comment on the line — was read by the decide step as "no cap at all".
// None of those agents were bounded. It survived three phases and this suite
// because **every test asserted the field *was* zero and none asserted what
// zero *did***.
//
// So the checks below never read a field back to itself. They call
// agent.AuthorityBounds.Decide — the same function stepDecide calls — and
// assert the outcome. A `> 0` guard reintroduced in the policy fails this in
// every pack that declares a zero cap, rather than in one hand-written test
// somebody remembered to write.
//
// The ladder is asserted whole, so the fix cannot be "escalate everything":
// a cap must trim to exactly the cap and proceed, and UnlimitedActions must
// not be trimmed at all.
func checkAgentBoundsBehave(_ context.Context, p domain.Pack) Result {
	const name = "agent_bounds_behave"

	declared := make(map[capability.CapabilityID]bool, len(p.Capabilities()))
	for _, c := range p.Capabilities() {
		declared[c.ID] = true
	}

	fail := func(format string, args ...any) Result {
		return Result{Check: name, Passed: false, Message: fmt.Sprintf(format, args...)}
	}

	// A capability every agent is guaranteed not to have listed in
	// RequiresApprovalFor, so a plan built from it isolates the cap rules from
	// the approval rules.
	const neutral = capability.CapabilityID("conformance.probe.neutral")

	for _, def := range p.AgentDefinitions() {
		b := def.Authority

		// Confidence is pinned above any declared threshold so the cap rules
		// are what decide, not the threshold. A threshold of 1.0 is the
		// "escalate always" bound and is checked on its own below.
		confident := 1.0

		plan := func(n int) []capability.CapabilityID {
			out := make([]capability.CapabilityID, n)
			for i := range out {
				out[i] = neutral
			}
			return out
		}

		switch {
		case b.MaxAutonomousActions == 0:
			// "Draft and ask". Three actions must escalate, and must survive
			// intact — an approval falls straight through to act, so trimming
			// here would discard the work the operator approved.
			v := b.Decide(confident, 0, plan(3))
			if !v.Escalate {
				return fail("agent %q declares MaxAutonomousActions: 0 but a 3-action plan runs without asking", def.ID)
			}
			if v.Allowed != 3 {
				return fail("agent %q escalated but its plan was trimmed to %d of 3; an approved plan would lose actions", def.ID, v.Allowed)
			}

		case b.MaxAutonomousActions > 0:
			// A cap trims to exactly the cap, and does not escalate for it.
			over := b.MaxAutonomousActions + 2
			v := b.Decide(confident, 0, plan(over))
			if v.Allowed != b.MaxAutonomousActions {
				return fail("agent %q declares a cap of %d but a %d-action plan left %d actions",
					def.ID, b.MaxAutonomousActions, over, v.Allowed)
			}
			// And a plan within the cap is untouched.
			under := b.Decide(confident, 0, plan(b.MaxAutonomousActions))
			if under.Allowed != b.MaxAutonomousActions {
				return fail("agent %q trimmed a plan already within its cap of %d to %d",
					def.ID, b.MaxAutonomousActions, under.Allowed)
			}
			if under.Escalate {
				return fail("agent %q declares a cap of %d and escalates a plan inside it: %s",
					def.ID, b.MaxAutonomousActions, under.Reason)
			}

		default:
			// agent.UnlimitedActions. Nothing is trimmed, however long.
			v := b.Decide(confident, 0, plan(50))
			if v.Allowed != 50 {
				return fail("agent %q declares UnlimitedActions and a 50-action plan was trimmed to %d", def.ID, v.Allowed)
			}
			if v.Escalate {
				return fail("agent %q declares UnlimitedActions and escalated anyway: %s", def.ID, v.Reason)
			}
		}

		// A capability listed for approval must escalate when planned, and
		// must name something the pack declares — a list nobody reads is the
		// same defect one field over.
		for _, c := range b.RequiresApprovalFor {
			if !declared[c] {
				return fail("agent %q requires approval for %q, which this pack does not declare", def.ID, c)
			}
			v := b.Decide(confident, 0, []capability.CapabilityID{c})
			if !v.Escalate {
				return fail("agent %q lists %q under RequiresApprovalFor and plans it without asking", def.ID, c)
			}
		}

		// A declared threshold must bite. Below it, escalate; at or above it,
		// do not — a threshold that escalates everything is not a threshold.
		if b.ConfidenceThreshold > 0 {
			low := b.Decide(b.ConfidenceThreshold-0.01, b.ConfidenceThreshold, plan(1))
			if !low.Escalate {
				return fail("agent %q declares a confidence threshold of %.2f and ran a plan below it",
					def.ID, b.ConfidenceThreshold)
			}
			if b.MaxAutonomousActions != 0 {
				high := b.Decide(1.0, b.ConfidenceThreshold, plan(1))
				if high.Escalate && strings.Contains(high.Reason, "threshold") {
					return fail("agent %q escalates a fully confident plan on its %.2f threshold: %s",
						def.ID, b.ConfidenceThreshold, high.Reason)
				}
			}
		}
	}

	return Result{
		Check:   name,
		Passed:  true,
		Message: fmt.Sprintf("all %d agent definitions behave as their bounds declare", len(p.AgentDefinitions())),
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

// CheckDanglingVerifiers finds criteria whose verifier no enabled pack
// exports. Run it at boot against the packs that are actually active.
//
// The per-pack check deliberately does not resolve foreign domains: a pack is
// valid on its own (ADR 017), and making one pack's validity depend on which
// others happen to be enabled would be wrong. But that leaves nobody at all
// checking a criterion that declares `Domain: "healthcare"` on a deployment
// where healthcare is switched off. The registry is where that question has an
// answer, and the answer is worth a loud line in the log rather than a
// surprise when an objective reaches verify and scores zero for a criterion
// nothing could ever satisfy.
//
// It reports rather than refuses. An operator may be mid-rollout, and a
// deployment that will not start because one template names a pack that is not
// enabled yet is worse than one that says so.
func CheckDanglingVerifiers(packs ...domain.Pack) []Result {
	const name = "dangling_verifier"

	exported := make(map[capability.CapabilityID]bool)
	enabled := make(map[string]bool, len(packs))
	for _, p := range packs {
		enabled[p.ID()] = true
		for _, c := range p.Capabilities() {
			exported[c.ID] = true
		}
	}

	var results []Result
	for _, p := range packs {
		for _, tmpl := range p.ObjectiveTemplates() {
			for _, crit := range tmpl.SuccessCriteria {
				if crit.Verifier == "" || exported[crit.Verifier] {
					continue
				}
				// Distinguish "the pack that owns it is switched off" from
				// "nothing anywhere exports it": the first is a deployment
				// decision, the second is a bug in the pack.
				detail := "no enabled pack exports it"
				if crit.Domain != "" && !enabled[crit.Domain] {
					detail = fmt.Sprintf("pack %q is not enabled on this deployment", crit.Domain)
				}
				results = append(results, Result{
					Check:  name,
					Passed: false,
					Message: fmt.Sprintf("template %q criterion %q verifies with %q: %s",
						tmpl.ID, crit.ID, crit.Verifier, detail),
				})
			}
		}
	}

	if len(results) == 0 {
		return []Result{{
			Check:   name,
			Passed:  true,
			Message: fmt.Sprintf("every criterion verifier across %d enabled packs resolves", len(packs)),
		}}
	}
	return results
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
