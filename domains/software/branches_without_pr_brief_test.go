package software

import (
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// Phase 40 step 6: the git observation lists each stream's branches ahead of
// main with no pull request in state["branches_without_pr"], or says in
// state["branches_without_pr_error"] that it could not read them. The roadmap
// asks that every stream's pass opens those first, before new work, as one
// action inside the stream's own limits, and that a pass whose remote was
// unreachable reports it could not see rather than reading the list as empty.
//
// The planner reads the objective's constraints, which come from the template
// (withTemplateConstraints in internal/feature/loop/resolve.go); the
// template's description does not reach it. So the rule has to be a
// constraint, as branchConstraint in stream_limits.go is.

// branchesWithoutPRRule is every constraint of the template that names
// branches_without_pr, joined, so the rule may be one sentence or several.
func branchesWithoutPRRule(t *testing.T, templateID string) string {
	t.Helper()
	var parts []string
	for _, c := range streamTemplate(t, templateID).Constraints {
		if strings.Contains(c.Description, "branches_without_pr") {
			parts = append(parts, c.Description)
		}
	}
	return strings.Join(parts, "\n")
}

// (a) Each stream's planner is told about branches_without_pr.
func TestStreamTemplatesNameBranchesWithoutPR(t *testing.T) {
	for _, b := range streamBriefs() {
		t.Run(b.templateID, func(t *testing.T) {
			if branchesWithoutPRRule(t, b.templateID) == "" {
				t.Errorf("no constraint of %s names branches_without_pr: the planner is never told to open the stream's branches that have no pull request", b.templateID)
			}
		})
	}
}

// (b) The rule says to open a pull request for each listed branch of this
// stream first, before any other new work, with software.act.create_pr
// setting params.branch and params.base_branch.
func TestStreamTemplatesOpenBranchesWithoutPRFirst(t *testing.T) {
	for _, b := range streamBriefs() {
		t.Run(b.templateID, func(t *testing.T) {
			rule := branchesWithoutPRRule(t, b.templateID)
			if rule == "" {
				t.Fatalf("no constraint of %s names branches_without_pr", b.templateID)
			}
			for _, want := range []struct{ phrase, why string }{
				{"for each branch of this stream", "which branches to open: each listed branch of this stream, not another stream's"},
				{"first, before any other new work", "that opening them comes before any other new work"},
				{"software.act.create_pr", "the action that opens the pull request"},
				{"params.branch", "that the action sets params.branch to the listed branch"},
				{"params.base_branch", "that the action sets params.base_branch"},
			} {
				if !strings.Contains(rule, want.phrase) {
					t.Errorf("the branches_without_pr rule does not say %s (missing %q):\n%s", want.why, want.phrase, rule)
				}
			}
		})
	}
}

// (b) The action the rule names is inside the template's own limits for the
// stream's own branch, through the same CheckActionLimits the decide step
// runs: a rule whose action the limits refused would leave the branch where
// it is on every pass.
func TestStreamTemplatesAdmitTheActionThatOpensABranchWithoutPR(t *testing.T) {
	const createPR = capability.CapabilityID("software.act.create_pr")
	for _, b := range streamBriefs() {
		t.Run(b.templateID, func(t *testing.T) {
			tmpl := streamTemplate(t, b.templateID)
			if !strings.Contains(branchesWithoutPRRule(t, b.templateID), string(createPR)) {
				t.Fatalf("the branches_without_pr rule names no %s action to check against the limits", createPR)
			}
			// A limit that is not declared refuses nothing, and the check
			// below would pass on a template with none.
			for _, parameter := range []string{"branch", "base_branch"} {
				if !hasLimit(tmpl, createPR, parameter) {
					t.Fatalf("%s declares no limit on %s params.%s", b.templateID, createPR, parameter)
				}
			}
			bounds := templateBounds(tmpl)
			for _, branch := range b.branches {
				a := environment.Action{CapabilityID: createPR, Params: map[string]any{
					"title":       "feat(software): open a branch that had no pull request (phase 40 step 6)",
					"branch":      branch,
					"base_branch": "main",
				}}
				for _, v := range bounds.CheckActionLimits([]environment.Action{a}) {
					t.Errorf("opening %s onto main with %s is refused: params.%s %q is outside [%s]",
						branch, createPR, v.Parameter, v.Value, strings.Join(v.Limit.Patterns, ", "))
				}
			}
		})
	}
}

// (c) With branches_without_pr_error set, the pass reports that it could not
// see the branches and does not read the missing list as empty (AGENTS.md
// rule 10).
func TestStreamTemplatesReportBranchesWithoutPRBlindness(t *testing.T) {
	for _, b := range streamBriefs() {
		t.Run(b.templateID, func(t *testing.T) {
			rule := branchesWithoutPRRule(t, b.templateID)
			if rule == "" {
				t.Fatalf("no constraint of %s names branches_without_pr", b.templateID)
			}
			for _, want := range []struct{ phrase, why string }{
				{"branches_without_pr_error", "which state key means the branches could not be read"},
				{"could not see the branches", "that the pass then reports it could not see the branches"},
				{"does not treat the list as empty", "that the pass then does not read the missing list as no branches"},
			} {
				if !strings.Contains(rule, want.phrase) {
					t.Errorf("the branches_without_pr rule does not say %s (missing %q):\n%s", want.why, want.phrase, rule)
				}
			}
		})
	}
}
