package reconcile

import (
	"context"
	"reflect"
	"testing"

	"github.com/bsenel/karakuri/domains/software"
	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// Phase 40 step 2: a stream's template declares its limits, effectiveAuthority
// adds them to the agent definition's bounds, and a plan outside them is
// refused, not escalated, at the rungs that act. The verdicts below come from
// DecideActions, the policy stepDecide carries out unchanged (step 1's
// TestActionLimitsAreCheckedInTheExistingBoundsCheck pins that); what stepDecide
// does with a refusal is pinned in internal/feature/loop.

const writeCode = capability.CapabilityID("software.act.write_code")

// streamCase is one of the four stream templates, the agent its objective
// runs as, and a branch and tool list its brief gives a coding action today.
type streamCase struct {
	tmpl   objective.Template
	def    coreagent.Definition
	branch string
	tools  []string
	paths  []string
}

// briefTools is the part of the four briefs' allowed_tools lists every
// stream shares; the whole lists are pinned in domains/software.
var briefTools = []string{"Read", "Edit", "Write", "Glob", "Grep", "Bash(git:*)", "Bash(gh pr list:*)",
	"Bash(gh pr view:*)", "Bash(gh pr create:*)", "Bash(gh pr edit:*)", "Bash(gh label create:*)",
	"Bash(ls:*)", "Bash(cat:*)", "Bash(grep:*)", "Bash(find:*)", "Bash(date:*)"}

func streamCases(t *testing.T) []streamCase {
	t.Helper()
	branches := map[string]string{
		"software.objective.market_discovery":    "karakuri/discovery-2026-11-15",
		"software.objective.engineering_backlog": "karakuri/tech-backlog-2026-11-15",
		"software.objective.ux_improvement":      "karakuri/ux-2026-11-15",
		"software.objective.roadmap_delivery":    "karakuri/phase-40-action-limits",
	}
	// The proposal streams' coding actions declare the files they write
	// (Phase 40 step 10).
	paths := map[string][]string{
		"software.objective.market_discovery":    {"docs/research/discovery-2026-11-15.md", "docs/roadmap.md"},
		"software.objective.engineering_backlog": {"docs/roadmap.md"},
	}
	pack := software.New()
	defs := map[coreagent.AgentID]coreagent.Definition{}
	for _, d := range pack.AgentDefinitions() {
		defs[d.ID] = d
	}
	var out []streamCase
	for _, tmpl := range pack.ObjectiveTemplates() {
		branch, ok := branches[tmpl.ID]
		if !ok {
			continue
		}
		if len(tmpl.SuggestedAgents) == 0 {
			t.Fatalf("%s suggests no agent", tmpl.ID)
		}
		def, ok := defs[tmpl.SuggestedAgents[0].ID]
		if !ok {
			t.Fatalf("%s suggests %s, which the pack does not declare", tmpl.ID, tmpl.SuggestedAgents[0].ID)
		}
		out = append(out, streamCase{tmpl: tmpl, def: def, branch: branch, tools: briefTools, paths: paths[tmpl.ID]})
	}
	if len(out) != len(branches) {
		t.Fatalf("found %d of the %d stream templates", len(out), len(branches))
	}
	return out
}

func codeAction(tools []string, branch string, paths ...string) environment.Action {
	list := make([]any, len(tools))
	for i, s := range tools {
		list[i] = s
	}
	a := environment.Action{CapabilityID: writeCode, Params: map[string]any{
		"prompt": "do the slice", "allowed_tools": list, "branch": branch,
	}}
	if len(paths) > 0 {
		a.Params["paths"] = paths
	}
	return a
}

// outsidePlans are the four plans the roadmap names, each one write_code
// action outside the stream's limits in exactly one way.
func outsidePlans(c streamCase, all []streamCase) map[string]environment.Action {
	other := ""
	for _, o := range all {
		if o.tmpl.ID != c.tmpl.ID {
			other = o.branch
			break
		}
	}
	return map[string]environment.Action{
		"a merge tool":            codeAction(append(append([]string{}, c.tools...), "Bash(gh pr merge:*)"), c.branch, c.paths...),
		"an unrestricted shell":   codeAction(append(append([]string{}, c.tools...), "Bash"), c.branch, c.paths...),
		"a push to main":          codeAction(c.tools, "main", c.paths...),
		"another stream's branch": codeAction(c.tools, other, c.paths...),
	}
}

// decide runs the bounds policy the way stepDecide does, at a confidence no
// stream agent's threshold refuses and with no third-party evidence, so a
// verdict is the bounds' and the limits' doing alone.
func decide(b coreagent.AuthorityBounds, actions ...environment.Action) coreagent.Verdict {
	return b.DecideActions(0.99, b.ConfidenceThreshold, actions, nil, coreagent.Evidence{})
}

// (1) At act_with_notice, each plan outside a stream's limits is refused, not
// escalated; a plan inside them is treated exactly as the agent's own bounds
// treat it, which for the implementer's streams means it runs.
func TestStreamTemplateLimitsRefuseAtActWithNotice(t *testing.T) {
	cases := streamCases(t)
	for _, c := range cases {
		t.Run(c.tmpl.ID, func(t *testing.T) {
			bounds := effectiveAuthority(c.def, c.tmpl, objective.AutonomyActWithNotice)
			for name, a := range outsidePlans(c, cases) {
				v := decide(bounds, a)
				if !v.Refuse {
					t.Errorf("%s: not refused (escalate=%v, reason %q)", name, v.Escalate, v.Reason)
				}
				if v.Escalate {
					t.Errorf("%s: escalated; a refusal must not hand the review back to a person", name)
				}
				if v.Reason == "" {
					t.Errorf("%s: refused with no reason for the audit row", name)
				}
			}

			inside := codeAction(c.tools, c.branch, c.paths...)
			if vs := bounds.CheckActionLimits([]environment.Action{inside}); len(vs) != 0 {
				t.Fatalf("the brief's own plan is outside the limits: %+v", vs)
			}
			got := decide(bounds, inside)
			today := decide(effectiveAuthority(c.def, objective.Template{}, objective.AutonomyActWithNotice), inside)
			if got.Refuse {
				t.Errorf("a plan inside the limits was refused: %q", got.Reason)
			}
			if got.Escalate != today.Escalate || got.Reason != today.Reason || got.Allowed != today.Allowed {
				t.Errorf("a plan inside the limits is decided %+v, the agent's own bounds decide %+v", got, today)
			}
		})
	}
}

// The implementer's streams (ux, delivery) are the ones that act at
// act_with_notice today: for them a plan inside the limits runs with no
// checkpoint at all. Strategist and maintainer declare MaxAutonomousActions 0,
// so their plans escalate on that bound whatever the template says; a template
// narrows and cannot lift it.
func TestAPlanInsideTheLimitsRunsWithoutACheckpoint(t *testing.T) {
	for _, c := range streamCases(t) {
		if c.def.Authority.MaxAutonomousActions == 0 {
			continue
		}
		t.Run(c.tmpl.ID, func(t *testing.T) {
			bounds := effectiveAuthority(c.def, c.tmpl, objective.AutonomyActWithNotice)
			if v := decide(bounds, codeAction(c.tools, c.branch, c.paths...)); v.Escalate || v.Refuse {
				t.Errorf("a plan inside the limits stopped: escalate=%v refuse=%v %q", v.Escalate, v.Refuse, v.Reason)
			}
		})
	}
}

// (1) At propose nothing changes: every plan escalates, inside the limits or
// outside them, and none is refused.
func TestStreamTemplateLimitsStillEscalateAtPropose(t *testing.T) {
	cases := streamCases(t)
	for _, c := range cases {
		t.Run(c.tmpl.ID, func(t *testing.T) {
			bounds := effectiveAuthority(c.def, c.tmpl, objective.AutonomyPropose)
			plans := outsidePlans(c, cases)
			plans["inside the limits"] = codeAction(c.tools, c.branch, c.paths...)
			for name, a := range plans {
				v := decide(bounds, a)
				today := decide(effectiveAuthority(c.def, objective.Template{}, objective.AutonomyPropose), a)
				if !v.Escalate || v.Refuse {
					t.Errorf("%s: escalate=%v refuse=%v at propose, want an escalation", name, v.Escalate, v.Refuse)
				}
				if v.Allowed != today.Allowed {
					t.Errorf("%s: %d actions shown to the reviewer, %d today", name, v.Allowed, today.Allowed)
				}
			}
		})
	}
}

// Refusing is what the rungs that act do; the rungs that ask keep step 1's
// escalation, so a person reviewing a propose-rung plan still sees it.
func TestOnlyTheRungsThatActRefuseOutsideTheLimits(t *testing.T) {
	c := streamCases(t)[0]
	for level, want := range map[objective.AutonomyLevel]bool{
		objective.AutonomySense:         false,
		objective.AutonomyPropose:       false,
		objective.AutonomyActWithNotice: true,
		objective.AutonomyAct:           true,
		"typo":                          false,
	} {
		if got := effectiveAuthority(c.def, c.tmpl, level).RefuseOutsideLimits; got != want {
			t.Errorf("RefuseOutsideLimits at %q = %v, want %v", level, got, want)
		}
	}
}

// (2) A template's limits intersect the agent definition's: they narrow it
// and never widen it.
func TestTemplateLimitsNarrowTheAgentsAndNeverWiden(t *testing.T) {
	c := streamCases(t)[3] // roadmap_delivery, run by the implementer
	if c.tmpl.ID != "software.objective.roadmap_delivery" {
		t.Fatalf("case 3 is %s", c.tmpl.ID)
	}

	t.Run("the agent's narrower limit still holds", func(t *testing.T) {
		def := c.def
		def.Authority.ActionLimits = []coreagent.ActionLimit{{Capability: writeCode, Parameter: "allowed_tools", Patterns: []string{"Read"}}}
		bounds := effectiveAuthority(def, c.tmpl, objective.AutonomyActWithNotice)
		// Edit is in the template's list and not in the agent's.
		if v := decide(bounds, codeAction([]string{"Read", "Edit"}, c.branch)); !v.Refuse {
			t.Errorf("the template widened the agent's allow-list: %+v", v)
		}
		if v := decide(bounds, codeAction([]string{"Read"}, c.branch)); v.Refuse || v.Escalate {
			t.Errorf("inside both limits but stopped: %+v", v)
		}
	})

	t.Run("the agent's wider limit does not lift the template's", func(t *testing.T) {
		def := c.def
		def.Authority.ActionLimits = []coreagent.ActionLimit{
			{Capability: writeCode, Parameter: "allowed_tools", Patterns: []string{"*", "Bash"}},
			// path.Match's * stops at '/', so the second pattern is what
			// admits every karakuri/ branch.
			{Capability: writeCode, Parameter: "branch", Patterns: []string{"*", "karakuri/*"}},
		}
		for _, a := range []environment.Action{codeAction([]string{"Read", "Bash"}, "main"), codeAction(nil, c.branch)} {
			if vs := def.Authority.CheckActionLimits([]environment.Action{a}); len(vs) != 0 {
				t.Fatalf("the agent's limit is not wider than the template's: %+v", vs)
			}
		}
		bounds := effectiveAuthority(def, c.tmpl, objective.AutonomyActWithNotice)
		for name, a := range map[string]environment.Action{
			"Bash":           codeAction([]string{"Read", "Bash"}, c.branch),
			"branch main":    codeAction(c.tools, "main"),
			"gh pr merge":    codeAction([]string{"Bash(gh pr merge:*)"}, c.branch),
			"ux's branch":    codeAction(c.tools, "karakuri/ux-2026-11-15"),
			"no branch left": {CapabilityID: writeCode, Params: map[string]any{"prompt": "p", "allowed_tools": []any{"Read"}}},
		} {
			if v := decide(bounds, a); !v.Refuse {
				t.Errorf("%s: the agent's wider limit lifted the template's: %+v", name, v)
			}
		}
	})

	t.Run("every other bound is the agent's, at every rung", func(t *testing.T) {
		for _, level := range []objective.AutonomyLevel{
			objective.AutonomySense, objective.AutonomyPropose, objective.AutonomyActWithNotice, objective.AutonomyAct, "typo",
		} {
			with := effectiveAuthority(c.def, c.tmpl, level)
			without := effectiveAuthority(c.def, objective.Template{}, level)
			with.ActionLimits, without.ActionLimits = nil, nil
			with.RefuseOutsideLimits, without.RefuseOutsideLimits = false, false
			if !reflect.DeepEqual(with, without) {
				t.Errorf("at %q the template changed bounds other than the limits:\n with %+v\n without %+v", level, with, without)
			}
		}
	})

	t.Run("the result holds both declarations", func(t *testing.T) {
		mine := coreagent.ActionLimit{Capability: writeCode, Parameter: "allowed_tools", Patterns: []string{"Read"}}
		def := c.def
		def.Authority.ActionLimits = []coreagent.ActionLimit{mine}
		got := effectiveAuthority(def, c.tmpl, objective.AutonomyActWithNotice).ActionLimits
		if !containsLimit(got, mine) {
			t.Errorf("the agent's own limit is gone: %+v", got)
		}
		if len(c.tmpl.ActionLimits) == 0 {
			t.Fatal("the template declares no limits")
		}
		for _, l := range c.tmpl.ActionLimits {
			if !containsLimit(got, l) {
				t.Errorf("the template's limit %+v is missing: %+v", l, got)
			}
		}
		if len(def.Authority.ActionLimits) != 1 {
			t.Errorf("effectiveAuthority wrote into the definition's own limits: %+v", def.Authority.ActionLimits)
		}
	})
}

// The supervisor finds the objective's template and hands its limits to the
// loop with the rest of the bounds. Without this the template's limits exist
// and nothing reads them.
//
// The limits are read from the template at run time and the constraints were
// copied onto the objective at creation, so the objective created before #252,
// holding only no-merge, is held to the same limits as one created after: a
// write action with no params.branch is refused for both. The loop is what
// tells its planner the template's constraints (internal/feature/loop); the
// stored objective is never rewritten to carry them.
func TestTheSupervisorHandsTheTemplatesLimitsToTheLoop(t *testing.T) {
	c := streamCases(t)[3]
	if len(c.tmpl.ActionLimits) == 0 {
		t.Fatal("the delivery template declares no limits")
	}
	byID := map[string]objective.Constraint{}
	for _, k := range c.tmpl.Constraints {
		byID[k.ID] = k
	}
	if _, ok := byID["no-merge"]; !ok {
		t.Fatal("the delivery template holds no no-merge constraint")
	}
	if _, ok := byID["action-limits"]; !ok {
		t.Fatal("the delivery template holds no action-limits constraint")
	}

	for name, constraints := range map[string][]objective.Constraint{
		"declared with no constraints":         nil,
		"created before #252, only no-merge":   {byID["no-merge"]},
		"created after #252, every constraint": append([]objective.Constraint(nil), c.tmpl.Constraints...),
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, Config{})
			reg := domain.NewRegistry()
			if err := reg.Register(context.Background(), software.New(), domain.Config{}); err != nil {
				t.Fatalf("register software: %v", err)
			}
			f.svc.domReg = reg
			f.use(t, map[string]string{"git": "aaa"})
			f.loops.criteriaMet = 1.0
			obj := f.declare(t, objective.Objective{
				AdditionalDomains: []string{"software"},
				TemplateID:        c.tmpl.ID,
				AgentID:           string(c.def.ID),
				Constraints:       constraints,
				Cadence:           &objective.Cadence{Every: "1h"},
				Autonomy:          &objective.Autonomy{Level: objective.AutonomyActWithNotice, Ceiling: objective.AutonomyActWithNotice},
			})

			f.pass(t, obj.ID, reconcile.TriggerManual)

			if f.loops.count() != 1 {
				t.Fatalf("the loop ran %d times, want 1", f.loops.count())
			}
			bounds := f.loops.bounds()
			for _, l := range c.tmpl.ActionLimits {
				if !containsLimit(bounds.ActionLimits, l) {
					t.Errorf("the loop was not given the template's limit %+v", l)
				}
			}
			if !bounds.RefuseOutsideLimits {
				t.Error("the loop was not told to refuse at act_with_notice")
			}

			noBranch := environment.Action{CapabilityID: writeCode, Params: map[string]any{
				"prompt": "do the slice", "allowed_tools": []any{"Read", "Edit"},
			}}
			if v := decide(bounds, noBranch); !v.Refuse || v.Escalate {
				t.Errorf("a write action with no params.branch: refuse=%v escalate=%v %q, want refused", v.Refuse, v.Escalate, v.Reason)
			}
			if v := decide(bounds, codeAction(c.tools, c.branch, c.paths...)); v.Refuse || v.Escalate {
				t.Errorf("the brief's own plan stopped: refuse=%v escalate=%v %q", v.Refuse, v.Escalate, v.Reason)
			}

			stored, err := f.store.GetObjective(context.Background(), obj.ID)
			if err != nil {
				t.Fatalf("read the objective back: %v", err)
			}
			if !reflect.DeepEqual(constraintIDs(stored.Constraints), constraintIDs(constraints)) {
				t.Errorf("the stored objective now holds %v, want %v; the template's constraints are read at run time, never written",
					constraintIDs(stored.Constraints), constraintIDs(constraints))
			}
		})
	}
}

func constraintIDs(cs []objective.Constraint) []string {
	ids := []string{}
	for _, k := range cs {
		ids = append(ids, k.ID)
	}
	return ids
}

func containsLimit(ls []coreagent.ActionLimit, want coreagent.ActionLimit) bool {
	for _, l := range ls {
		if reflect.DeepEqual(l, want) {
			return true
		}
	}
	return false
}
