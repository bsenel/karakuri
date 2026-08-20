package software

import (
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// Every capability that acts on the world is served by exactly one
// environment.
//
// This is the general form of the bug Phase 26 exists for, and it would have
// caught all of it. `write_code` and `write_test` were declared in Phase 2 and
// served by nothing until Phase 26 — planned by models, given a git worktree,
// and answered "unimplemented". `delegate_to_cli` was served and never routed
// to. `write_design_doc` was on two agents' capability lists and required by a
// priority-9 hint before any write_code action, and no environment ran it for
// four phases.
//
// A capability whose ID begins with "software.act." is one a plan can execute.
// If nothing serves it, it is inert, and the only way anyone finds out is a
// failed action in production.
func TestEveryActCapabilityIsServed(t *testing.T) {
	pack := New()

	served := map[capability.CapabilityID]environment.EnvironmentID{}
	for _, f := range pack.EnvironmentFactories() {
		for _, capID := range f.Serves {
			if prev, dup := served[capID]; dup {
				t.Errorf("capability %q is served by both %q and %q; routing cannot choose", capID, prev, f.EnvID)
			}
			served[capID] = f.EnvID
		}
	}

	for _, c := range pack.Capabilities() {
		if !strings.HasPrefix(string(c.ID), "software.act.") {
			continue
		}
		if served[c.ID] == "" {
			t.Errorf("capability %q acts on the world and no environment serves it: every plan that uses it fails", c.ID)
		}
	}
}

// The reverse direction. A Serves entry naming a capability the pack does not
// declare routes nothing anywhere and reads as though it does.
func TestNothingIsServedThatIsNotDeclared(t *testing.T) {
	pack := New()

	declared := map[capability.CapabilityID]bool{}
	for _, c := range pack.Capabilities() {
		declared[c.ID] = true
	}

	for _, f := range pack.EnvironmentFactories() {
		for _, capID := range f.Serves {
			if !declared[capID] {
				t.Errorf("environment %q serves %q, which this pack does not declare", f.EnvID, capID)
			}
		}
	}
}

// Every agent's declared capabilities are ones a plan can actually carry out.
//
// The tech-lead and strategist both list write_design_doc; before Phase 26 an
// agent could be given a capability list whose steps all failed, and the only
// symptom was a low score with no explanation of why.
func TestAgentActCapabilitiesAreRunnable(t *testing.T) {
	pack := New()

	served := map[capability.CapabilityID]bool{}
	for _, f := range pack.EnvironmentFactories() {
		for _, capID := range f.Serves {
			served[capID] = true
		}
	}

	for _, def := range pack.AgentDefinitions() {
		for _, capID := range def.Capabilities {
			if !strings.HasPrefix(string(capID), "software.act.") {
				continue
			}
			if !served[capID] {
				t.Errorf("agent %q may plan %q, which no environment serves", def.ID, capID)
			}
		}
	}
}

// The design document is a draft, recorded like the other two, and refuses to
// report success on an empty one — a capability that succeeds on nothing feeds
// a perfect success rate into procedural memory.
func TestWriteDesignDocIsServedAndRefusesEmptyInput(t *testing.T) {
	env := &gitEnv{id: EnvGit}

	res, err := env.Act(t.Context(), environment.Action{
		CapabilityID: "software.act.write_design_doc",
		Params:       map[string]any{"design": "one supervisor, one lease, one due-wheel"},
	})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if !res.Success {
		t.Fatalf("write_design_doc refused a real design: %s", res.Error)
	}

	res, err = env.Act(t.Context(), environment.Action{
		CapabilityID: "software.act.write_design_doc",
		Params:       map[string]any{},
	})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if res.Success {
		t.Error("write_design_doc reported success on an empty design")
	}
	if !strings.Contains(res.Error, "design") {
		t.Errorf("refusal %q does not name the missing parameter", res.Error)
	}
}

// A criterion's verifier must be a capability whose success *answers* the
// criterion, not one that supplies the material for answering it.
//
// The distinction was invisible until the verify step started reading action
// results. Before that every criterion was effectively judged by a model shown
// nothing, so naming a verifier that could not decide the question cost
// nothing — and four criteria in this pack named analyse_usage, which produces
// telemetry rather than deciding anything. Once a verifier settles a criterion
// deterministically, that spelling means "met whenever the analysis ran",
// which is every time, including on a deployment with no telemetry at all.
//
// analyse_usage in particular succeeds even with no reader wired, deliberately
// — so it can never be a verifier for anything.
func TestNoCriterionIsVerifiedByEvidenceGathering(t *testing.T) {
	// Capabilities that report on the world rather than change it. Their
	// success says "I looked", never "the thing you asked about is true".
	reporting := map[capability.CapabilityID]bool{
		CapAnalyseUsage: true,
	}

	checked := 0
	for _, tpl := range New().ObjectiveTemplates() {
		for _, crit := range tpl.SuccessCriteria {
			if crit.Verifier == "" {
				continue // judged from the results, which is the other correct answer
			}
			checked++
			if reporting[crit.Verifier] {
				t.Errorf("template %q criterion %q is verified by %q, which succeeds whenever it runs — "+
					"the criterion would be met on a deployment with no telemetry at all",
					tpl.ID, crit.ID, crit.Verifier)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no criterion in this pack declares a verifier; this test is checking nothing")
	}
}
