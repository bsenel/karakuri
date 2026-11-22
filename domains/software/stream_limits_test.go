package software

import (
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 40 step 2: a stream's template declares the limits its actions must
// stay inside. These tests read the template alone, through the same
// CheckActionLimits the decide step runs; the refusal itself and the
// intersection with the agent's bounds are tested in internal/feature/reconcile.

// codingCapabilities are the actions that hand a coding agent a tool
// allow-list in params.allowed_tools (see the hint in hints.go). Every one of
// them can run `gh pr merge` if its list lets it, so each must be limited:
// delegate_to_cli is not in the design's list, but leaving it open would let a
// plan route a merge around the write_code limit.
var codingCapabilities = []capability.CapabilityID{
	"software.act.write_code",
	"software.act.write_test",
	"software.act.delegate_to_cli",
}

// streamBrief is what the four stream briefs tell the planner today. The
// briefs are the standing objectives' descriptions, which live with the
// deployment rather than in this repository, so the lists are copied here as
// they stood on 2026-11-15. A template whose limits refused one of these would
// refuse every plan the streams make today.
type streamBrief struct {
	templateID string
	// tools is the brief's "Allowed tools" list for its write actions.
	tools []string
	// branches are branch names the brief tells the stream to push to.
	branches []string
	// paths are the files a proposal stream's action declares it writes
	// (Phase 40 step 10); none for a stream with no paths limit.
	paths []string
	// bases are the branches its pull request may open onto, beyond main.
	bases []string
}

// krk is the operator's krk binary. The tech brief names it by its absolute
// path on the deployment, which a pack cannot know; this path has the same
// depth as the live one without being anyone's home directory. path.Match's *
// does not cross '/', so a limit has to admit it some other way than one star.
const krk = "/home/operator/Documents/GitHub/karakuri/bin/krk"

func streamBriefs() []streamBrief {
	return []streamBrief{
		{
			templateID: marketDiscoveryTemplateID,
			tools: []string{"Read", "Edit", "Write", "Glob", "Grep", "Bash(git:*)", "Bash(gh pr list:*)", "Bash(gh pr view:*)",
				"Bash(gh pr create:*)", "Bash(gh pr edit:*)", "Bash(gh label create:*)", "Bash(ls:*)", "Bash(cat:*)",
				"Bash(grep:*)", "Bash(find:*)", "Bash(date:*)", "WebSearch", "WebFetch"},
			branches: []string{"karakuri/discovery-2026-11-15"},
			paths:    []string{"docs/research/discovery-2026-11-15.md", "docs/roadmap.md"},
		},
		{
			templateID: engineeringBacklogTemplateID,
			tools: []string{"Read", "Edit", "Write", "Glob", "Grep", "Bash(git:*)", "Bash(gh pr list:*)", "Bash(gh pr view:*)",
				"Bash(gh pr create:*)", "Bash(gh pr edit:*)", "Bash(gh label create:*)", "Bash(ls:*)", "Bash(cat:*)",
				"Bash(grep:*)", "Bash(find:*)", "Bash(date:*)",
				"Bash(" + krk + " audit:*)", "Bash(" + krk + " --output json audit:*)",
				"Bash(" + krk + " cost report:*)", "Bash(" + krk + " --output json cost report:*)",
				"Bash(" + krk + " eval:*)", "Bash(" + krk + " --output json eval:*)",
				"Bash(" + krk + " objective list:*)", "Bash(" + krk + " --output json objective list:*)",
				"Bash(" + krk + " objective get:*)", "Bash(" + krk + " --output json objective get:*)",
				"Bash(" + krk + " objective reconcile-status:*)", "Bash(" + krk + " --output json objective reconcile-status:*)",
				"Bash(" + krk + " checkpoint list:*)", "Bash(" + krk + " --output json checkpoint list:*)",
				"Bash(" + krk + " checkpoint get:*)", "Bash(" + krk + " --output json checkpoint get:*)",
				"Bash(" + krk + " twin list:*)", "Bash(" + krk + " twin get:*)", "Bash(" + krk + " domain list:*)",
				"Bash(tail:*)", "Bash(wc:*)", "Bash(sort:*)", "Bash(uniq:*)", "Bash(head:*)"},
			branches: []string{"karakuri/tech-backlog-2026-11-15"},
			paths:    []string{"docs/roadmap.md"},
		},
		{
			templateID: uxImprovementTemplateID,
			tools: []string{"Read", "Edit", "Write", "Glob", "Grep", "Bash(git:*)", "Bash(gh pr list:*)", "Bash(gh pr view:*)",
				"Bash(gh pr create:*)", "Bash(gh pr edit:*)", "Bash(gh pr checks:*)", "Bash(gh run view:*)",
				"Bash(gh label create:*)", "Bash(ls:*)", "Bash(cat:*)", "Bash(grep:*)", "Bash(find:*)", "Bash(date:*)",
				"Bash(go:*)", "Bash(gofmt:*)", "Bash(sed:*)"},
			branches: []string{"karakuri/ux-2026-11-15"},
			bases:    []string{"karakuri/ux-2026-11-15"},
		},
		{
			templateID: roadmapDeliveryTemplateID,
			tools: []string{"Read", "Edit", "Write", "Glob", "Grep", "Bash(git:*)", "Bash(gh pr list:*)", "Bash(gh pr view:*)",
				"Bash(gh pr create:*)", "Bash(gh pr edit:*)", "Bash(gh pr comment:*)", "Bash(gh pr ready:*)",
				"Bash(gh pr checks:*)", "Bash(gh run view:*)", "Bash(gh label create:*)", "Bash(ls:*)", "Bash(cat:*)",
				"Bash(grep:*)", "Bash(find:*)", "Bash(date:*)", "Bash(go:*)", "Bash(gofmt:*)",
				"Bash(scripts/check_langchaingo_imports.sh:*)", "Bash(sed:*)"},
			// A phase's first group, a later group, and a backlog entry.
			branches: []string{"karakuri/phase-40-action-limits", "karakuri/phase-41-step-10-coverage", "karakuri/eb-011-unbound-instance"},
			bases:    []string{"karakuri/phase-40-action-limits", "karakuri/phase-41-step-10-coverage", "karakuri/eb-011-unbound-instance"},
		},
	}
}

// mergeOrShell are tool entries that must never reach a stream's coding
// agent: a merge, a gh or shell pattern wide enough to hold one, and the
// unrestricted shell.
var mergeOrShell = []string{
	"Bash(gh pr merge:*)",
	"Bash(gh pr merge 245 --squash --admin)",
	"Bash(gh pr:*)",
	"Bash(gh:*)",
	"Bash(gh api:*)",
	"Bash",
	"Bash(*)",
	"Bash(bash:*)",
	"Bash(sh:*)",
}

func codingAction(id capability.CapabilityID, tools []string, branch string, paths ...string) environment.Action {
	list := make([]any, len(tools))
	for i, s := range tools {
		list[i] = s
	}
	a := environment.Action{CapabilityID: id, Params: map[string]any{
		"prompt":        "do the slice",
		"allowed_tools": list,
		"branch":        branch,
	}}
	if len(paths) > 0 {
		a.Params["paths"] = paths
	}
	return a
}

func templateBounds(tmpl objective.Template) agent.AuthorityBounds {
	return agent.AuthorityBounds{ActionLimits: tmpl.ActionLimits}
}

// hasLimit reports whether the template limits parameter on capability. A
// limit that is not declared cannot refuse anything, and every refusal test
// below would pass on a template that declares none.
func hasLimit(tmpl objective.Template, id capability.CapabilityID, parameter string) bool {
	for _, l := range tmpl.ActionLimits {
		if l.Capability == id && l.Parameter == parameter && len(l.Patterns) > 0 {
			return true
		}
	}
	return false
}

func TestStreamTemplatesDeclareCodingLimits(t *testing.T) {
	for _, b := range streamBriefs() {
		tmpl := streamTemplate(t, b.templateID)
		for _, id := range codingCapabilities {
			for _, parameter := range []string{"allowed_tools", "branch"} {
				if !hasLimit(tmpl, id, parameter) {
					t.Errorf("%s declares no limit on %s params.%s", b.templateID, id, parameter)
				}
			}
		}
	}
}

// (4) Today's plans are not refused: the brief's whole list, on the stream's
// own branch, is inside the template's limits for every coding action.
func TestStreamTemplatesAdmitTheToolsTheirBriefsGive(t *testing.T) {
	for _, b := range streamBriefs() {
		t.Run(b.templateID, func(t *testing.T) {
			bounds := templateBounds(streamTemplate(t, b.templateID))
			for _, id := range codingCapabilities {
				for _, branch := range b.branches {
					if v := bounds.CheckActionLimits([]environment.Action{codingAction(id, b.tools, branch, b.paths...)}); len(v) != 0 {
						for _, viol := range v {
							t.Errorf("%s on %s: the brief's %s %q is refused by [%s]", id, branch,
								viol.Parameter, viol.Value, strings.Join(viol.Limit.Patterns, ", "))
						}
					}
				}
			}
		})
	}
}

// (1) and (3) A merge tool and an unrestricted shell are outside every
// stream's limits, alone or hidden in the brief's otherwise valid list.
func TestStreamTemplatesRefuseMergeAndUnrestrictedShell(t *testing.T) {
	for _, b := range streamBriefs() {
		t.Run(b.templateID, func(t *testing.T) {
			bounds := templateBounds(streamTemplate(t, b.templateID))
			for _, id := range codingCapabilities {
				for _, tool := range mergeOrShell {
					for _, tools := range [][]string{{tool}, append(append([]string{}, b.tools...), tool)} {
						a := codingAction(id, tools, b.branches[0], b.paths...)
						if !violates(bounds.CheckActionLimits([]environment.Action{a}), "allowed_tools", tool) {
							t.Errorf("%s with %q in allowed_tools is inside the limits", id, tool)
						}
					}
				}
			}
		})
	}
}

// (1) A coding action pushes only to its own stream's branches: never main,
// never another stream's branch, never a name that only starts like its own.
func TestStreamTemplatesConfineTheBranchACodingActionPushes(t *testing.T) {
	briefs := streamBriefs()
	for _, b := range briefs {
		t.Run(b.templateID, func(t *testing.T) {
			bounds := templateBounds(streamTemplate(t, b.templateID))
			refused := []string{"main", "refs/heads/main", "origin/main", ""}
			for _, other := range briefs {
				if other.templateID != b.templateID {
					refused = append(refused, other.branches...)
				}
			}
			// path.Match's * stops at '/': a pattern like karakuri/ux-* must
			// not be widened to admit a nested ref under it.
			refused = append(refused, b.branches[0]+"/main")
			for _, id := range codingCapabilities {
				for _, branch := range refused {
					a := codingAction(id, b.tools, branch, b.paths...)
					if !violates(bounds.CheckActionLimits([]environment.Action{a}), "branch", branch) {
						t.Errorf("%s may push to %q", id, branch)
					}
				}
			}
		})
	}
}

// A pull request opens from the stream's own branch onto main, or onto the
// stream's own branch, which is how delivery stacks a later group on an
// earlier group's head. It never opens from main or onto another stream.
func TestStreamTemplatesConfineThePullRequestsTheyOpen(t *testing.T) {
	const createPR = capability.CapabilityID("software.act.create_pr")
	briefs := streamBriefs()
	pr := func(head, base string) environment.Action {
		return environment.Action{CapabilityID: createPR, Params: map[string]any{
			"title": "feat(loop): add a thing (phase 40 step 2)", "branch": head, "base_branch": base,
		}}
	}
	for _, b := range briefs {
		t.Run(b.templateID, func(t *testing.T) {
			tmpl := streamTemplate(t, b.templateID)
			for _, parameter := range []string{"branch", "base_branch"} {
				if !hasLimit(tmpl, createPR, parameter) {
					t.Errorf("no limit on %s params.%s", createPR, parameter)
				}
			}
			bounds := templateBounds(tmpl)
			own := b.branches[0]
			for _, base := range append([]string{"main"}, b.bases...) {
				if v := bounds.CheckActionLimits([]environment.Action{pr(own, base)}); len(v) != 0 {
					t.Errorf("a pull request from %s onto %s is refused: %+v", own, base, v)
				}
			}
			// A proposal stream opens its pull request only against main.
			if len(b.bases) == 0 && !violates(bounds.CheckActionLimits([]environment.Action{pr(own, own)}), "base_branch", own) {
				t.Errorf("a pull request may be opened onto the stream's own branch %s", own)
			}
			if !violates(bounds.CheckActionLimits([]environment.Action{pr("main", "main")}), "branch", "main") {
				t.Error("a pull request may be opened from main")
			}
			for _, other := range briefs {
				if other.templateID == b.templateID {
					continue
				}
				if !violates(bounds.CheckActionLimits([]environment.Action{pr(own, other.branches[0])}), "base_branch", other.branches[0]) {
					t.Errorf("a pull request may be opened onto another stream's branch %s", other.branches[0])
				}
				if !violates(bounds.CheckActionLimits([]environment.Action{pr(other.branches[0], "main")}), "branch", other.branches[0]) {
					t.Errorf("a pull request may be opened from another stream's branch %s", other.branches[0])
				}
			}
		})
	}
}

// (3) The no_merge constraint is a sentence the planner reads, and nothing
// evaluates it. Every template that carries it, in this pack, must also
// declare limits that refuse a merge on every coding action, so the sentence
// is never again the only thing standing there.
func TestEveryNoMergeTemplateDeclaresLimitsThatExcludeMerge(t *testing.T) {
	carriers := 0
	for _, tmpl := range New().ObjectiveTemplates() {
		if !carriesNoMerge(tmpl) {
			continue
		}
		carriers++
		bounds := templateBounds(tmpl)
		for _, id := range codingCapabilities {
			for _, tool := range []string{"Bash(gh pr merge:*)", "Bash(gh pr merge 245 --squash --admin)", "Bash(gh:*)", "Bash"} {
				a := codingAction(id, []string{"Read", tool}, "karakuri/phase-40-action-limits")
				if !violates(bounds.CheckActionLimits([]environment.Action{a}), "allowed_tools", tool) {
					t.Errorf("%s carries no_merge but lets %s hold %q", tmpl.ID, id, tool)
				}
			}
		}
	}
	if carriers < 4 {
		t.Fatalf("%d templates carry no_merge, want at least the four streams: the test would pass on none", carriers)
	}
}

// The planner must set params.branch on every coding action, or the branch
// limit refuses the action for leaving it out. The planner reads the objective
// it is given, whose constraints come from the template (the template's own
// description does not reach it), so a constraint has to say so, and name the
// stream's branch.
func TestStreamTemplatesTellThePlannerToSetTheBranch(t *testing.T) {
	prefixes := map[string]string{
		marketDiscoveryTemplateID:    "karakuri/discovery-",
		engineeringBacklogTemplateID: "karakuri/tech-backlog-",
		uxImprovementTemplateID:      "karakuri/ux-",
		roadmapDeliveryTemplateID:    "karakuri/phase-",
	}
	for id, prefix := range prefixes {
		t.Run(id, func(t *testing.T) {
			tmpl := streamTemplate(t, id)
			for _, c := range tmpl.Constraints {
				d := c.Description
				if strings.Contains(d, "params.branch") && strings.Contains(d, "software.act.write_code") && strings.Contains(d, prefix) {
					return
				}
			}
			t.Errorf("no constraint tells the planner to set params.branch on software.act.write_code to a %s* branch", prefix)
		})
	}
}

func carriesNoMerge(tmpl objective.Template) bool {
	for _, c := range tmpl.Constraints {
		if c.Expression == "no_merge" {
			return true
		}
	}
	return false
}

// violates reports whether the violations hold one on parameter with value.
func violates(vs []agent.ActionLimitViolation, parameter, value string) bool {
	for _, v := range vs {
		if v.Parameter == parameter && v.Value == value {
			return true
		}
	}
	return false
}
