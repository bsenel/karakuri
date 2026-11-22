package software

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 40 step 10 (ADR 030): the proposal streams run as a new agent whose
// authority allows a few actions, and their template limits confine a pass to
// the stream's paths, its own branch and a pull request against main. The
// maintainer and the strategist keep zero.

// proposerMaxActions is the ceiling the step calls "a small number".
const proposerMaxActions = 5

func packAgent(t *testing.T, id agent.AgentID) (agent.Definition, bool) {
	t.Helper()
	for _, d := range New().AgentDefinitions() {
		if d.ID == id {
			return d, true
		}
	}
	return agent.Definition{}, false
}

func packTemplate(t *testing.T, id string) objective.Template {
	t.Helper()
	for _, tmpl := range New().ObjectiveTemplates() {
		if tmpl.ID == id {
			return tmpl
		}
	}
	t.Fatalf("the pack declares no template %s", id)
	return objective.Template{}
}

func TestTheProposerActsAFewTimesAndTheMaintainerAndStrategistDoNot(t *testing.T) {
	def, ok := packAgent(t, AgentProposer)
	if !ok {
		t.Fatalf("the pack declares no %s; the proposal streams need an agent whose authority allows a few actions", AgentProposer)
	}
	if n := def.Authority.MaxAutonomousActions; n <= 0 || n > proposerMaxActions {
		t.Errorf("%s MaxAutonomousActions = %d, want a small number in 1..%d", AgentProposer, n, proposerMaxActions)
	}
	if def.Domain != "software" {
		t.Errorf("%s Domain = %q, want software", AgentProposer, def.Domain)
	}
	for _, id := range []agent.AgentID{"software.agent.maintainer", "software.agent.strategist"} {
		d, ok := packAgent(t, id)
		if !ok {
			t.Fatalf("the pack no longer declares %s", id)
		}
		if d.Authority.MaxAutonomousActions != 0 {
			t.Errorf("%s MaxAutonomousActions = %d, want 0: ADR 018 rests on it", id, d.Authority.MaxAutonomousActions)
		}
	}
}

// The proposer never merges: none of the capabilities that would let it act
// on main or on the owner's behalf is in its list.
func TestTheProposerHoldsNoMergeCapability(t *testing.T) {
	def, ok := packAgent(t, AgentProposer)
	if !ok {
		t.Fatalf("the pack declares no %s", AgentProposer)
	}
	for _, c := range def.Capabilities {
		if c == CapMarkReady || c == CapRunRemediation || c == "software.act.shell_exec" {
			t.Errorf("%s holds %s, which no proposal pass needs", AgentProposer, c)
		}
	}
}

func TestProposalStreamsNameTheProposer(t *testing.T) {
	for _, id := range []string{marketDiscoveryTemplateID, engineeringBacklogTemplateID} {
		tmpl := packTemplate(t, id)
		if len(tmpl.SuggestedAgents) == 0 || tmpl.SuggestedAgents[0].ID != AgentProposer {
			t.Errorf("%s suggests %v, want %s first", id, tmpl.SuggestedAgents, AgentProposer)
		}
	}
}

// proposalCase is one proposal stream, its own branch, the paths a pass may
// write and one path it may not.
type proposalCase struct {
	template string
	branch   string
	other    string // another stream's branch
	inside   []string
	outside  string
}

var proposalCases = []proposalCase{
	{
		template: marketDiscoveryTemplateID,
		branch:   "karakuri/discovery-2026-11-18",
		other:    "karakuri/tech-backlog-2026-11-18",
		inside:   []string{"docs/roadmap.md", "docs/research/market-2026-11-18.md"},
		outside:  "cli/main.go",
	},
	{
		template: engineeringBacklogTemplateID,
		branch:   "karakuri/tech-backlog-2026-11-18",
		other:    "karakuri/discovery-2026-11-18",
		inside:   []string{"docs/roadmap.md"},
		// Discovery's research path is not the backlog's.
		outside: "docs/research/market-2026-11-18.md",
	},
}

func strs(ss ...string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// proposalWrite is a coding action as a proposal pass plans it: the stream's
// branch, base tools, and the paths it will write in params.paths.
func proposalWrite(branch string, paths ...string) environment.Action {
	return environment.Action{CapabilityID: "software.act.write_code", Params: map[string]any{
		"prompt":        "record the findings",
		"allowed_tools": strs("Read", "Edit", "Write", "Bash(git:*)", "Bash(gh pr create:*)"),
		"branch":        branch,
		"paths":         strs(paths...),
	}}
}

func proposalPR(branch, base string) environment.Action {
	return environment.Action{CapabilityID: "software.act.create_pr", Params: map[string]any{
		"branch": branch, "base_branch": base, "title": "docs(roadmap): findings",
	}}
}

func violations(tmpl objective.Template, actions ...environment.Action) []agent.ActionLimitViolation {
	return agent.AuthorityBounds{ActionLimits: tmpl.ActionLimits}.CheckActionLimits(actions)
}

func TestProposalStreamLimitsAdmitAPassInsideThem(t *testing.T) {
	for _, c := range proposalCases {
		t.Run(c.template, func(t *testing.T) {
			tmpl := packTemplate(t, c.template)
			if vs := violations(tmpl, proposalWrite(c.branch, c.inside...), proposalPR(c.branch, "main")); len(vs) != 0 {
				t.Errorf("a pass inside the declared paths, on the stream's branch, opening a pull request against main is outside the limits: %+v", vs)
			}
		})
	}
}

// Each plan is outside the stream's limits in exactly one way.
func TestProposalStreamLimitsConfineAPass(t *testing.T) {
	for _, c := range proposalCases {
		t.Run(c.template, func(t *testing.T) {
			tmpl := packTemplate(t, c.template)
			plans := map[string]struct {
				action environment.Action
				param  string
			}{
				"a path outside the declared ones":      {proposalWrite(c.branch, "docs/roadmap.md", c.outside), "paths"},
				"no declared paths":                     {withoutParam(proposalWrite(c.branch, "docs/roadmap.md"), "paths"), "paths"},
				"another stream's branch":               {proposalWrite(c.other, "docs/roadmap.md"), "branch"},
				"main":                                  {proposalWrite("main", "docs/roadmap.md"), "branch"},
				"a pull request against another branch": {proposalPR(c.branch, c.branch), "base_branch"},
			}
			for name, p := range plans {
				vs := violations(tmpl, p.action)
				if len(vs) == 0 {
					t.Errorf("%s: inside the template's limits; want a violation on %s", name, p.param)
					continue
				}
				if !slices.ContainsFunc(vs, func(v agent.ActionLimitViolation) bool { return v.Parameter == p.param }) {
					t.Errorf("%s: violations %+v, want one on parameter %s", name, vs, p.param)
				}
			}
		})
	}
}

// Every coding capability a proposal pass could use carries the path limit,
// or delegate_to_cli would write around write_code's.
func TestProposalStreamPathLimitCoversEveryCodingCapability(t *testing.T) {
	for _, c := range proposalCases {
		tmpl := packTemplate(t, c.template)
		for _, capID := range streamCodingCapabilities {
			has := slices.ContainsFunc(tmpl.ActionLimits, func(l agent.ActionLimit) bool {
				return l.Capability == capID && l.Parameter == "paths"
			})
			if !has {
				t.Errorf("%s: no paths limit on %s", c.template, capID)
			}
		}
	}
}

func withoutParam(a environment.Action, name string) environment.Action {
	params := map[string]any{}
	for k, v := range a.Params {
		if k != name {
			params[k] = v
		}
	}
	a.Params = params
	return a
}

// Step 3: what a delegated proposal action pushed is checked after it ran. A
// push to another stream's branch is caught by the refs check that exists.
func TestAProposalActionThatPushedToAnotherStreamsBranchIsCaught(t *testing.T) {
	f := newPushedRefsFixture(t)
	action := salvageAction(f.worktree)
	action.Params["branch"] = "karakuri/discovery-2026-11-18"
	res := actWith(t, f.worktree, action, func(t *testing.T, wt string) error {
		commitIn(t, wt, "findings.md")
		gitIn(t, wt, "push", "origin", "HEAD:refs/heads/karakuri/tech-backlog-2026-11-18")
		return nil
	})
	if res.Success || res.BoundsViolation == nil {
		t.Fatalf("a discovery action that pushed to the backlog's branch passed (success=%v, violation=%+v)", res.Success, res.BoundsViolation)
	}
	if !slices.Contains(res.BoundsViolation.Refs, "refs/heads/karakuri/tech-backlog-2026-11-18") {
		t.Errorf("BoundsViolation.Refs = %v, want the backlog's branch", res.BoundsViolation.Refs)
	}
}

// Step 3 for paths: an action that slipped past the decide step and pushed,
// to its own branch, a file outside its declared paths fails after it ran
// (checkPushedPaths), not only later at the paths criterion.
func TestAProposalActionThatPushedOutsideItsPathsIsCaught(t *testing.T) {
	const branch = "karakuri/tech-backlog-2026-11-18"
	f := newPushedRefsFixture(t)
	action := salvageAction(f.worktree)
	action.Params["branch"] = branch
	action.Params["paths"] = strs("docs/roadmap.md")
	res := actWith(t, f.worktree, action, func(t *testing.T, wt string) error {
		commitIn(t, wt, "outside.go")
		gitIn(t, wt, "push", "origin", "HEAD:refs/heads/"+branch)
		return nil
	})
	if res.Success || res.BoundsViolation == nil {
		t.Fatalf("an action declaring paths [docs/roadmap.md] pushed outside.go to its own branch and passed "+
			"(success=%v, violation=%+v): the step 3 check reads refs only, not the paths a push changed", res.Success, res.BoundsViolation)
	}
	if !strings.Contains(res.Error, "outside.go") {
		t.Errorf("Error = %q, want it to name the path outside.go", res.Error)
	}
}

// The check after the run passes an action whose push to its own branch
// changed only its declared paths.
func TestAProposalActionThatPushedInsideItsPathsPasses(t *testing.T) {
	const branch = "karakuri/tech-backlog-2026-11-18"
	f := newPushedRefsFixture(t)
	action := salvageAction(f.worktree)
	action.Params["branch"] = branch
	// item.txt is the fixture's own commit on the worktree, ahead of main.
	action.Params["paths"] = strs("item.txt", "docs/*")
	res := actWith(t, f.worktree, action, func(t *testing.T, wt string) error {
		if err := os.MkdirAll(filepath.Join(wt, "docs"), 0o750); err != nil {
			t.Fatal(err)
		}
		commitIn(t, wt, "docs/roadmap.md")
		gitIn(t, wt, "push", "origin", "HEAD:refs/heads/"+branch)
		return nil
	})
	if !res.Success || res.BoundsViolation != nil {
		t.Fatalf("an action that pushed only its declared paths failed: error %q, violation %+v", res.Error, res.BoundsViolation)
	}
}
