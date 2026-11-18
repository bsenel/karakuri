package reconcile

import (
	"strings"
	"testing"

	"github.com/bsenel/karakuri/domains/software"
	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 40 step 10 (ADR 030): at act_with_notice a proposal pass whose plan
// stays inside its template's limits runs to the pull request with no
// checkpoint; a plan that writes outside the declared paths is refused; and an
// objective naming the maintainer still escalates every action. The verdicts
// come from DecideActions, the policy stepDecide carries out unchanged.

func packDef(t *testing.T, id coreagent.AgentID) coreagent.Definition {
	t.Helper()
	for _, d := range software.New().AgentDefinitions() {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("the software pack declares no %s", id)
	return coreagent.Definition{}
}

func packTmpl(t *testing.T, id string) objective.Template {
	t.Helper()
	for _, tmpl := range software.New().ObjectiveTemplates() {
		if tmpl.ID == id {
			return tmpl
		}
	}
	t.Fatalf("the software pack declares no template %s", id)
	return objective.Template{}
}

type proposalPass struct {
	template string
	branch   string
	paths    []string
}

var proposalPasses = []proposalPass{
	{"software.objective.market_discovery", "karakuri/discovery-2026-11-18", []string{"docs/research/market-2026-11-18.md", "docs/roadmap.md"}},
	{"software.objective.engineering_backlog", "karakuri/tech-backlog-2026-11-18", []string{"docs/roadmap.md"}},
}

func proposalWrite(branch string, paths ...string) environment.Action {
	a := codeAction(briefTools, branch)
	list := make([]any, len(paths))
	for i, p := range paths {
		list[i] = p
	}
	a.Params["paths"] = list
	return a
}

func openPR(branch string) environment.Action {
	return environment.Action{CapabilityID: "software.act.create_pr", Params: map[string]any{
		"branch": branch, "base_branch": "main", "title": "docs(roadmap): findings",
	}}
}

// The whole pass, research written and pull request opened, at the rung the
// streams are meant to earn. The proposer's MaxAutonomousActions has to cover
// both actions.
func TestAProposalPassRunsToItsPullRequestWithNoCheckpoint(t *testing.T) {
	def := packDef(t, software.AgentProposer)
	for _, p := range proposalPasses {
		t.Run(p.template, func(t *testing.T) {
			tmpl := packTmpl(t, p.template)
			if len(tmpl.SuggestedAgents) == 0 || tmpl.SuggestedAgents[0].ID != software.AgentProposer {
				t.Fatalf("%s suggests %v, want %s: the pass runs as the agent the template names", p.template, tmpl.SuggestedAgents, software.AgentProposer)
			}
			bounds := effectiveAuthority(def, tmpl, objective.AutonomyActWithNotice)
			v := decide(bounds, proposalWrite(p.branch, p.paths...), openPR(p.branch))
			if v.Escalate || v.Refuse {
				t.Fatalf("a plan inside the limits stopped: escalate=%v refuse=%v %q", v.Escalate, v.Refuse, v.Reason)
			}
			if v.Allowed != 2 {
				t.Errorf("Allowed = %d, want both actions, the write and the pull request", v.Allowed)
			}
		})
	}
}

// At propose the streams still ask: the template does not lift the rung.
func TestAProposalPassStillAsksAtPropose(t *testing.T) {
	def := packDef(t, software.AgentProposer)
	for _, p := range proposalPasses {
		bounds := effectiveAuthority(def, packTmpl(t, p.template), objective.AutonomyPropose)
		if v := decide(bounds, proposalWrite(p.branch, p.paths...), openPR(p.branch)); !v.Escalate {
			t.Errorf("%s at propose: escalate=%v refuse=%v, want an escalation", p.template, v.Escalate, v.Refuse)
		}
	}
}

// Step 2: a write outside the declared paths is refused, with no checkpoint,
// and the reason names the paths limit.
func TestAProposalPlanOutsideItsPathsIsRefused(t *testing.T) {
	def := packDef(t, software.AgentProposer)
	for _, p := range proposalPasses {
		t.Run(p.template, func(t *testing.T) {
			bounds := effectiveAuthority(def, packTmpl(t, p.template), objective.AutonomyActWithNotice)
			v := decide(bounds, proposalWrite(p.branch, "docs/roadmap.md", "internal/feature/loop/runner.go"), openPR(p.branch))
			if !v.Refuse || v.Escalate {
				t.Fatalf("a write to internal/feature/loop/runner.go: refuse=%v escalate=%v %q; want a refusal: the template declares no paths limit",
					v.Refuse, v.Escalate, v.Reason)
			}
			if !strings.Contains(v.Reason, `"paths"`) || !strings.Contains(v.Reason, "internal/feature/loop/runner.go") {
				t.Errorf("Reason = %q, want it to name the paths parameter and the offending path", v.Reason)
			}
		})
	}
}

// An objective that names the maintainer escalates every action, inside the
// limits, at every rung that acts.
func TestAnObjectiveNamingTheMaintainerStillEscalatesEveryAction(t *testing.T) {
	def := packDef(t, "software.agent.maintainer")
	for _, p := range proposalPasses {
		tmpl := packTmpl(t, p.template)
		for _, level := range []objective.AutonomyLevel{objective.AutonomyActWithNotice, objective.AutonomyAct} {
			bounds := effectiveAuthority(def, tmpl, level)
			for name, plan := range map[string][]environment.Action{
				"the write":        {proposalWrite(p.branch, p.paths...)},
				"the pull request": {openPR(p.branch)},
				"the whole pass":   {proposalWrite(p.branch, p.paths...), openPR(p.branch)},
			} {
				v := decide(bounds, plan...)
				if !v.Escalate {
					t.Errorf("%s at %s, %s under the maintainer: escalate=%v refuse=%v %q; want an escalation",
						p.template, level, name, v.Escalate, v.Refuse, v.Reason)
				}
				// Allowed is not checked: on an escalation it counts the
				// actions shown to the reviewer, not actions that run.
			}
		}
	}
}
