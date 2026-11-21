package software

import (
	"slices"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 40 step 11: a review that builds and runs the branch. The template is
// a fifth standing stream with an agent of its own, so the reviewer is never
// the author's context, and its limits let it build, test, lint, run binaries
// and post one review, and push nothing. Authority is written into the
// agent's bounds and the template's limits (ADR 015), never a second gate.
//
// Whether the objective is standing is the mode of the objective built from
// the template, not a field of the template (objective.Template has none), so
// it is not asserted here; the wake-up and the fixture-repository acceptance
// are later slices.

const reviewTemplateID = "software.objective.pull_request_review"

// reviewMayRun are the capabilities the reviewer holds: build and test, lint,
// a command in its worktree, and one review.
var reviewMayRun = []capability.CapabilityID{
	"software.verify.run_tests",
	"software.verify.lint",
	"software.act.shell_exec",
	CapPostReview,
}

// reviewMayNotRun are the capabilities that push or change the repository.
var reviewMayNotRun = []capability.CapabilityID{
	"software.act.write_code",
	"software.act.write_test",
	"software.act.delegate_to_cli",
	"software.act.create_pr",
	CapMarkReady,
}

func reviewTemplate(t *testing.T) objective.Template {
	t.Helper()
	return streamTemplate(t, reviewTemplateID)
}

// reviewAgent is the agent definition the template names, looked up in the
// pack: a name the pack does not declare is an agent nobody can run.
func reviewAgent(t *testing.T) agent.Definition {
	t.Helper()
	tmpl := reviewTemplate(t)
	if len(tmpl.SuggestedAgents) != 1 {
		t.Fatalf("%s suggests %d agents, want exactly its own", reviewTemplateID, len(tmpl.SuggestedAgents))
	}
	id := tmpl.SuggestedAgents[0].ID
	for _, def := range New().AgentDefinitions() {
		if def.ID == id {
			return def
		}
	}
	t.Fatalf("%s names agent %q, which the pack does not declare", reviewTemplateID, id)
	return agent.Definition{}
}

// reviewBounds is what the reconcile supervisor runs a pass under at a rung
// that acts: the agent's limits and the template's, intersected, with a plan
// outside them refused (effectiveAuthority in internal/feature/reconcile).
func reviewBounds(t *testing.T) agent.AuthorityBounds {
	t.Helper()
	bounds := reviewAgent(t).Authority
	bounds.ActionLimits = slices.Concat(bounds.ActionLimits, reviewTemplate(t).ActionLimits)
	bounds.RefuseOutsideLimits = true
	return bounds
}

func reviewAction(id capability.CapabilityID, params map[string]any) environment.Action {
	return environment.Action{CapabilityID: id, Params: params}
}

func decideReview(b agent.AuthorityBounds, a environment.Action) agent.Verdict {
	return b.DecideActions(1.0, 0, []environment.Action{a}, nil, agent.Evidence{})
}

func TestPullRequestReviewTemplateIsDeclaredByThePack(t *testing.T) {
	n := 0
	for _, tpl := range New().ObjectiveTemplates() {
		if tpl.ID == reviewTemplateID {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("the pack returns %s %d times, want 1", reviewTemplateID, n)
	}
	tmpl := reviewTemplate(t)
	if tmpl.Domain != "software" {
		t.Errorf("Domain = %q, want software", tmpl.Domain)
	}
	if !tmpl.Risk.Valid() || tmpl.Risk == objective.RiskUnclassified {
		t.Errorf("Risk = %q, want a classified risk", tmpl.Risk)
	}
}

// The requirement is separation of context: an agent shared with a stream that
// writes is the author's context, however the objective is worded.
func TestPullRequestReviewRunsUnderItsOwnAgent(t *testing.T) {
	def := reviewAgent(t)
	for _, notMine := range []string{
		"software.agent.implementer", "software.agent.maintainer", "software.agent.strategist",
		"software.agent.sre", "software.agent.watcher",
	} {
		if string(def.ID) == notMine {
			t.Errorf("%s runs under %s, which is another stream's agent", reviewTemplateID, notMine)
		}
	}
	for _, tc := range streamCases {
		if tc.agent == string(def.ID) {
			t.Errorf("%s runs under %s, which %s also names", reviewTemplateID, def.ID, tc.id)
		}
	}
	if def.Domain != "software" {
		t.Errorf("agent Domain = %q, want software", def.Domain)
	}
}

func TestPullRequestReviewAgentHoldsTheReviewCapabilitiesAndNoPushingOne(t *testing.T) {
	held := reviewAgent(t).Capabilities
	for _, id := range reviewMayRun {
		if !slices.Contains(held, id) {
			t.Errorf("the reviewer does not hold %s", id)
		}
	}
	for _, id := range reviewMayNotRun {
		if slices.Contains(held, id) {
			t.Errorf("the reviewer holds %s, which pushes or changes the repository", id)
		}
	}
}

// A plan holding any capability that pushes or changes the repository is
// refused by the limits, whatever its parameters, and the refusal is not an
// escalation a person could approve past. The parameter sets include a
// stream-shaped branch, so a limit that only checked the branch would admit
// the action and this test would say so.
func TestPullRequestReviewRefusesAnyPush(t *testing.T) {
	bounds := reviewBounds(t)
	paramSets := map[string]map[string]any{
		"no params": {},
		"a stream branch": {
			"branch": "karakuri/phase-40-step-11-review-builds-branch", "base_branch": "main",
			"prompt": "fix it", "title": "fix(software): x", "allowed_tools": []any{"Read"},
		},
		"main": {"branch": "main", "base_branch": "main", "allowed_tools": []any{"Bash(git:*)"}},
	}
	for _, id := range reviewMayNotRun {
		for name, params := range paramSets {
			t.Run(string(id)+"/"+name, func(t *testing.T) {
				v := decideReview(bounds, reviewAction(id, params))
				if !v.Refuse {
					t.Fatalf("a plan holding %s with %s is not refused: %+v", id, name, v)
				}
				if v.Escalate || v.Allowed != 0 {
					t.Errorf("verdict = %+v, want a refusal that neither escalates nor allows", v)
				}
				if !strings.Contains(v.Reason, string(id)) {
					t.Errorf("reason %q does not name %s", v.Reason, id)
				}
			})
		}
	}
}

// The shell is how a reviewer runs binaries and also how a branch is pushed,
// so the limit on its command has to refuse the pushing commands.
func TestPullRequestReviewShellCannotPushOrMerge(t *testing.T) {
	bounds := reviewBounds(t)
	bounded := slices.ContainsFunc(bounds.ActionLimits, func(l agent.ActionLimit) bool {
		return l.Capability == "software.act.shell_exec" && l.Parameter == "cmd" && len(l.Patterns) > 0
	})
	if !bounded {
		t.Fatal("no limit on software.act.shell_exec params.cmd: the reviewer's shell is unbounded")
	}
	for _, cmd := range []string{
		"git push origin HEAD", "git push origin HEAD:main", "git push --force origin HEAD",
		"git commit -am fix", "git checkout -b other", "gh pr merge 273 --squash", "gh pr create --fill",
		"gh pr ready 273", "rm -rf .", "sh -c 'git push'", "curl -s example.com",
	} {
		a := reviewAction("software.act.shell_exec", map[string]any{"cmd": cmd, "workdir": "."})
		if v := decideReview(bounds, a); !v.Refuse {
			t.Errorf("shell command %q is not refused: %+v", cmd, v)
		}
	}
}

// The limits are not a blanket refusal: the review's own actions pass.
func TestPullRequestReviewAdmitsTestLintAndPostReview(t *testing.T) {
	bounds := reviewBounds(t)
	branch := "karakuri/phase-40-step-11-review-builds-branch"
	for _, a := range []environment.Action{
		reviewAction("software.verify.run_tests", map[string]any{"branch": branch}),
		reviewAction("software.verify.lint", map[string]any{"branch": branch}),
		reviewAction(CapPostReview, map[string]any{"branch": branch, "body": "what I ran, and what I did not"}),
	} {
		if v := decideReview(bounds, a); v.Refuse {
			t.Errorf("%s is refused: %s", a.CapabilityID, v.Reason)
		}
	}
}

// The reviewer must be able to post without a person: the template's limits
// are what confine it. Zero autonomous actions would escalate every review.
func TestPullRequestReviewMayActWithoutAPerson(t *testing.T) {
	b := reviewAgent(t).Authority
	if b.MaxAutonomousActions == 0 {
		t.Error("MaxAutonomousActions = 0, so every review escalates and none is ever posted")
	}
	for _, id := range reviewMayRun {
		if slices.Contains(b.RequiresApprovalFor, id) {
			t.Errorf("%s requires approval, so the reviewer cannot run it alone", id)
		}
	}
	if b.CanDelegate || b.CanModifyObjective {
		t.Error("the reviewer may delegate or modify its objective")
	}
}

// The roadmap's wording: each review says what it ran and what it did not,
// and lists what it cannot verify.
func TestPullRequestReviewDescriptionStatesWhatRanAndWhatDidNot(t *testing.T) {
	desc := strings.ToLower(reviewTemplate(t).Description)
	for _, want := range []string{
		"what it ran",
		"what it did not",
		"cannot verify",
		"software.act.post_review",
		"push nothing",
	} {
		if !strings.Contains(desc, want) {
			t.Errorf("the description does not say %q:\n%s", want, desc)
		}
	}
	for _, cannot := range []string{"not installed", "web page", "proposal is wanted"} {
		if !strings.Contains(desc, cannot) {
			t.Errorf("the description does not list %q among what it cannot verify", cannot)
		}
	}
}

// On a forge where author and reviewer are one account the review is a
// comment review; approval stays the owner's merge.
func TestPullRequestReviewDescriptionLeavesApprovalToTheOwner(t *testing.T) {
	desc := strings.ToLower(reviewTemplate(t).Description)
	if !strings.Contains(desc, "comment review") || !strings.Contains(desc, "merge") {
		t.Errorf("the description does not say the review is a comment and approval stays the owner's merge:\n%s", desc)
	}
}

func TestPullRequestReviewForbidsMerging(t *testing.T) {
	if !carriesNoMerge(reviewTemplate(t)) {
		t.Errorf("%s carries no no_merge constraint", reviewTemplateID)
	}
}

// Phase 40 step 11: the pass is woken by stream pull requests whose head has
// no review, so the planner is told where the list is and what to do with it.
func TestPullRequestReviewTemplateTellsThePlannerPRsWithoutReview(t *testing.T) {
	var got *objective.Constraint
	for _, c := range reviewTemplate(t).Constraints {
		if c.ID == "prs-without-review" {
			c := c
			got = &c
		}
	}
	if got == nil {
		t.Fatalf("%s declares no prs-without-review constraint", reviewTemplateID)
	}
	if !got.Hard {
		t.Errorf("prs-without-review is not a hard constraint")
	}
	for _, want := range []string{"prs_without_review", "prs_without_review_error", "software.act.post_review"} {
		if !strings.Contains(got.Description, want) {
			t.Errorf("description %q does not name %s", got.Description, want)
		}
	}
}
