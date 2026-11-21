package loop

// Phase 40 step 6a, acceptance item (i), the whole pass: for each of the four
// stream templates, with an open pull request of that stream that conflicts
// with main in docs/roadmap.md, the planner is told to repair it first, its
// first action is a write action on that pull request's branch that the
// template's limits admit, and before any other action runs the remote branch
// holds a merge commit keeping both sides with the previous tip as an
// ancestor. The runner, the software pack's environments and the template's
// limits are the real ones; the forge is a fake, the remote a local bare
// repository, and the planner and the coding agent are scripts.

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/domains/software"
	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	featurememory "github.com/bsenel/karakuri/internal/feature/memory"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	platformgit "github.com/bsenel/karakuri/internal/platform/git"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/cliagent"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

const (
	pass6aMainSide   = "main side"
	pass6aBranchSide = "branch side"
)

func pass6aGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := pass6aGitErr(dir, args...)
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return out
}

func pass6aGitErr(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func pass6aWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// pass6aForge reports one open pull request as conflicting with main and
// records what it was asked.
type pass6aForge struct {
	versioncontrol.VersionControlAdapter
	pr versioncontrol.PRSummary
}

func (*pass6aForge) Name() string { return "pass6a-forge" }
func (*pass6aForge) Active() bool { return true }
func (f *pass6aForge) ListPRs(context.Context, string, time.Time) ([]versioncontrol.PRSummary, error) {
	return []versioncontrol.PRSummary{f.pr}, nil
}
func (*pass6aForge) MergeState(context.Context, string, string) (string, error) {
	return "conflicting", nil
}
func (*pass6aForge) ListBranches(context.Context, string, string) ([]versioncontrol.Branch, error) {
	return nil, nil
}
func (*pass6aForge) ListChecks(context.Context, string, string) ([]versioncontrol.CheckRun, error) {
	return nil, nil
}

// pass6aCLI is the coding agent. It does what the repair task says, in the
// worktree the loop gave it, and records the state of the remote branch each
// time it is called, so the test can say what held before the next action.
type pass6aCLI struct {
	t      *testing.T
	origin string
	branch string

	calls []pass6aCall
}

type pass6aCall struct {
	prompt        string
	tools         []string
	worktree      string
	worktreeOn    string
	remoteTipSeen string
}

func (*pass6aCLI) Name() string { return "pass6a-cli" }
func (*pass6aCLI) Active() bool { return true }
func (c *pass6aCLI) Delegate(_ context.Context, in cliagent.DelegateInput) (cliagent.DelegateOutput, error) {
	on, _ := pass6aGitErr(in.WorktreePath, "rev-parse", "--abbrev-ref", "HEAD")
	tip, _ := pass6aGitErr(c.origin, "rev-parse", "refs/heads/"+c.branch)
	c.calls = append(c.calls, pass6aCall{
		prompt: in.Prompt, tools: in.AllowedTools, worktree: in.WorktreePath, worktreeOn: on, remoteTipSeen: tip,
	})
	if len(c.calls) > 1 || !strings.Contains(strings.ToLower(in.Prompt), "merge") {
		return cliagent.DelegateOutput{Summary: "nothing to do"}, nil
	}
	// The repair the task asks for: merge main into the branch keeping both
	// sides, push it to the branch. The checkout is by name, so what follows
	// does not depend on which branch the loop's worktree was on.
	g := func(args ...string) { pass6aGit(c.t, in.WorktreePath, args...) }
	g("fetch", "origin")
	g("checkout", "-B", c.branch, "origin/"+c.branch)
	if out, err := pass6aGitErr(in.WorktreePath, "merge", "--no-edit", "origin/main"); err == nil {
		c.t.Fatalf("the fixture's merge did not conflict: %s", out)
	}
	pass6aWrite(c.t, filepath.Join(in.WorktreePath, "docs", "roadmap.md"),
		"intro\n"+pass6aMainSide+"\n"+pass6aBranchSide+"\noutro\n")
	g("add", "docs/roadmap.md")
	g("commit", "--no-edit")
	g("push", "origin", "HEAD:refs/heads/"+c.branch)
	return cliagent.DelegateOutput{Summary: "merged main into " + c.branch, RawOutput: "merged"}, nil
}
func (*pass6aCLI) Stream(context.Context, cliagent.DelegateInput) (<-chan cliagent.DelegateChunk, error) {
	ch := make(chan cliagent.DelegateChunk)
	close(ch)
	return ch, nil
}

// pass6aPlanner is the model: it reads the task it is given (the objective's
// constraints and the git observation, as the prompt carries them) and plans
// what they say. It plans the repair of a pull request only when the prompt
// tells it to repair unmergeable pull requests first and lists one; it then
// adds an action of new work after it, to show the order.
type pass6aPlanner struct {
	llm.ProviderAdapter
	t       *testing.T
	prompts []string
	// tools is what the planner puts in params.allowed_tools: the tools every
	// stream's brief gives its coding agent.
	tools     []any
	newBranch string
	// outOfBand is the branch the planner plans the repair of when the prompt
	// lists none. Empty in the whole-pass test; the diagnostic variant sets
	// it, to see what the rest of the chain does when the list is known.
	outOfBand string
}

func (*pass6aPlanner) Name() string                   { return "pass6a" }
func (*pass6aPlanner) Available(context.Context) bool { return true }

var pass6aPRLine = regexp.MustCompile(`Pull request #(\d+) on branch (\S+) of stream`)

func (p *pass6aPlanner) Complete(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	var b strings.Builder
	b.WriteString(req.SystemPrompt)
	for _, m := range req.Messages {
		b.WriteString("\n")
		b.WriteString(m.Content)
	}
	prompt := b.String()
	p.prompts = append(p.prompts, prompt)

	reply := `{"actions":[],"confidence":0.95,"reasoning":"nothing to do"}`
	told := strings.Contains(prompt, "unmergeable_prs")
	m := pass6aPRLine.FindStringSubmatch(prompt)
	if told && m == nil && p.outOfBand != "" {
		m = []string{"", "260", p.outOfBand}
	}
	if told && m != nil {
		branch := m[2]
		actions := []map[string]any{
			{
				"capability": "software.act.write_code",
				"env_id":     "software.env.cli_agent",
				"reason":     "pull request #" + m[1] + " conflicts with main: merge main into " + branch + ", keeping both sides, never rebase",
				"params": map[string]any{
					"prompt":        "Merge origin/main into " + branch + ", keeping both sides of docs/roadmap.md. Never rebase, never force-push. Push the merge to " + branch + ".",
					"branch":        branch,
					"allowed_tools": p.tools,
				},
			},
			{
				"capability": "software.act.write_code",
				"env_id":     "software.env.cli_agent",
				"reason":     "new work, after the repair",
				"params": map[string]any{
					"prompt":        "Start the next item.",
					"branch":        p.newBranch,
					"allowed_tools": p.tools,
				},
			},
		}
		raw, err := json.Marshal(map[string]any{"actions": actions, "confidence": 0.95, "reasoning": "repair first"})
		if err != nil {
			p.t.Fatal(err)
		}
		reply = string(raw)
	}
	return llm.CompletionResponse{Content: reply, TokensUsed: 10}, nil
}

func (p *pass6aPlanner) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.CompletionChunk, error) {
	resp, err := p.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	ch := make(chan llm.CompletionChunk, 1)
	ch <- llm.CompletionChunk{Content: resp.Content, Done: true}
	close(ch)
	return ch, nil
}

func TestAWholePassRepairsAnUnmergeablePullRequestFirstInEveryTemplate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	cases := []struct {
		template  string
		branch    string
		newBranch string
	}{
		{"software.objective.market_discovery", "karakuri/discovery-2026-11-20", "karakuri/discovery-2026-11-21"},
		{"software.objective.engineering_backlog", "karakuri/tech-backlog-2026-11-20", "karakuri/tech-backlog-2026-11-21"},
		{"software.objective.ux_improvement", "karakuri/ux-2026-11-20", "karakuri/ux-2026-11-21"},
		{deliveryTemplateID, "karakuri/phase-99-repair", "karakuri/phase-99-next"},
	}
	for _, tc := range cases {
		t.Run(tc.template, func(t *testing.T) {
			runUnmergeablePass(t, tc.template, tc.branch, tc.newBranch, false)
		})
	}
}

func runUnmergeablePass(t *testing.T, templateID, branch, newBranch string, listOutOfBand bool) {
	ctx := context.Background()
	// The host may hand git config through the environment (core.hooksPath).
	t.Setenv("GIT_CONFIG_PARAMETERS", "")
	t.Setenv("GIT_CONFIG_COUNT", "0")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(k+"_NAME", "Test")
		t.Setenv(k+"_EMAIL", "test@example.invalid")
	}

	// A bare origin whose main and the stream branch change one line of
	// docs/roadmap.md, and a clone of it for the worktree manager.
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	seed := filepath.Join(root, "seed")
	work := filepath.Join(root, "work")
	roadmap := filepath.Join(seed, "docs", "roadmap.md")
	pass6aGit(t, root, "init", "--bare", "-b", "main", origin)
	pass6aGit(t, root, "clone", origin, seed)
	pass6aWrite(t, roadmap, "intro\nshared line\noutro\n")
	pass6aGit(t, seed, "add", ".")
	pass6aGit(t, seed, "commit", "-m", "base")
	pass6aGit(t, seed, "push", "origin", "HEAD:refs/heads/main")
	pass6aGit(t, seed, "checkout", "-b", branch)
	pass6aWrite(t, roadmap, "intro\n"+pass6aBranchSide+"\noutro\n")
	pass6aGit(t, seed, "commit", "-am", "branch edit")
	prevTip := pass6aGit(t, seed, "rev-parse", "HEAD")
	pass6aGit(t, seed, "push", "origin", "HEAD:refs/heads/"+branch)
	pass6aGit(t, seed, "checkout", "main")
	pass6aWrite(t, roadmap, "intro\n"+pass6aMainSide+"\noutro\n")
	pass6aGit(t, seed, "commit", "-am", "main edit")
	pass6aGit(t, seed, "push", "origin", "HEAD:refs/heads/main")
	pass6aGit(t, root, "clone", origin, work)

	wtm, err := platformgit.NewGoGitWorktreeManager(config.GitConfig{
		RepoPath: work, WorktreeBase: ".worktrees", BaseBranch: "main", BranchPrefix: "karakuri",
	})
	if err != nil {
		t.Fatalf("worktree manager: %v", err)
	}

	forge := &pass6aForge{VersionControlAdapter: versioncontrol.NewNoOp(), pr: versioncontrol.PRSummary{
		ID: "260", Number: 260, Title: "a stream pull request", HeadBranch: branch, BaseBranch: "main",
		OwnAuthor: true, Draft: true, UpdatedAt: time.Now().UTC(),
	}}
	cli := &pass6aCLI{t: t, origin: origin, branch: branch}
	toolsReg := tools.NewRegistry()
	toolsReg.VC.Set("forge", "fake", versioncontrol.VersionControlAdapter(forge))
	toolsReg.CLIAgents.Set("cli", "fake", cliagent.CLIAgentAdapter(cli))

	pack := software.NewWithTools(toolsReg)
	svc, _, store := newResumeFixture(t)
	svc.otel = observability.NewOTel(nil)
	svc.memSvc = featurememory.NewService(store, 5)
	svc.wt = wtm
	svc.domReg = softwareRegistryFor(t, pack)

	planner := &pass6aPlanner{t: t, newBranch: newBranch}
	if listOutOfBand {
		planner.outOfBand = branch
	}
	for _, tool := range []string{"Read", "Edit", "Write", "Glob", "Grep", "Bash(git:*)"} {
		planner.tools = append(planner.tools, tool)
	}
	providers := llm.NewRegistry(nil)
	providers.Register(planner)
	svc.factory = platformagent.NewFactory(providers, svc.hub, svc.otel, nil)

	envReg := environment.NewRegistry()
	for _, f := range pack.EnvironmentFactories() {
		if err := envReg.Register(f); err != nil {
			t.Fatalf("register %s: %v", f.EnvID, err)
		}
	}
	svc.envReg = envReg
	capReg := capability.NewRegistry()
	for _, c := range pack.Capabilities() {
		if err := capReg.Register(c); err != nil {
			t.Fatalf("register capability %s: %v", c.ID, err)
		}
	}
	svc.capReg = capReg

	tmpl := softwareTemplate(t, templateID)
	obj := objective.Objective{
		ID:    "obj-6a-" + objective.ObjectiveID(strings.NewReplacer("software.objective.", "", "_", "-").Replace(templateID)),
		Title: tmpl.Title, Domain: "software", TemplateID: templateID, Constraints: tmpl.Constraints,
	}
	if err := store.SaveObjective(ctx, obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}
	agentDef := coreagent.Definition{
		ID: "pass6a-agent", Name: "Pass 6a agent", Domain: "software",
		ReasoningStrategy: coreagent.ReasoningChainOfThought,
		LLMHints:          capability.LLMHints{PreferredProvider: "pass6a"},
		Authority: coreagent.AuthorityBounds{
			MaxAutonomousActions: coreagent.UnlimitedActions,
			ConfidenceThreshold:  0,
			ActionLimits:         tmpl.ActionLimits,
		},
	}
	runSync(svc, "loop-6a-0001", loop.Request{Objective: obj, Agent: agentDef, MaxIter: 1}, nil)

	// 1. The planner was told, by the task it received, to repair that pull
	// request first.
	if len(planner.prompts) == 0 {
		t.Fatal("the planner was never called")
	}
	told := planner.prompts[0]
	repair, n := constraintByID(tmpl.Constraints, "unmergeable-prs")
	if n != 1 {
		t.Fatalf("%s holds %d unmergeable-prs constraints, want 1", templateID, n)
	}
	if !strings.Contains(told, repair.Description) {
		t.Fatalf("the planner's task does not carry the template's unmergeable-prs constraint %q", repair.Description)
	}
	if !listOutOfBand && !pass6aPRLine.MatchString(told) {
		t.Fatalf("the planner's task does not list pull request #260 on %s as one that cannot be merged: "+
			"the git observation's unmergeable_prs_lines never reach the prompt (the agent runtime renders the objective, the task and a memory count, "+
			"not WorldState; buildProvenanceNotice adds only third-party sources and blind environments), "+
			"so the planner is told to repair \"the listed\" pull requests and is shown none", branch)
	}
	if !listOutOfBand && !strings.Contains(told, "conflicts with main") {
		t.Fatalf("the planner's task does not say that pull request #260 on %s conflicts with main", branch)
	}
	if strings.Index(told, "unmergeable") > strings.Index(told, "branches_without_pr") && strings.Contains(told, "branches_without_pr") {
		t.Errorf("the planner is told about branches without a pull request before it is told to repair unmergeable ones")
	}

	// 2. The first action is a write action on that branch, admitted by the
	// template's limits: it ran, and the audit log holds no bounds row.
	if len(cli.calls) == 0 {
		t.Fatalf("no write action ran: the planner's plan was refused or never acted on (planner replied to %d prompts)", len(planner.prompts))
	}
	rows, err := store.ListToolEvents(ctx, storage.ToolEventFilter{ObjectiveID: string(obj.ID)})
	if err != nil {
		t.Fatalf("list tool events: %v", err)
	}
	for _, r := range rows {
		if r.BoundsViolation || r.Kind == storage.ToolEventEscalation {
			t.Errorf("the repair was refused or paused by a bounds row (the limits before it ran, or step 3's pushed-ref check after it): %q", r.EscalationReason)
		}
	}
	first := cli.calls[0]
	if !strings.Contains(strings.ToLower(first.prompt), "merge") {
		t.Errorf("the first write action is not the repair: its task is %q", first.prompt)
	}
	// The write must happen on the pull request's branch. stepAct provisions
	// a worktree on a branch of its own and writes it into params.branch,
	// replacing the branch the planner named; this says whether that is so.
	if first.worktreeOn != branch {
		t.Errorf("the first write action ran in a worktree on %q, not on the pull request's branch %q: "+
			"the loop provisions its own branch for a write action and overwrites params.branch, so the repair is not made on the branch the plan named",
			first.worktreeOn, branch)
	}

	// 3. The remote branch holds a merge commit keeping both sides, with the
	// previous tip as an ancestor, before any other action ran.
	if len(cli.calls) < 2 {
		t.Fatalf("the plan's second action never ran (%d write actions); nothing shows the order", len(cli.calls))
	}
	if cli.calls[1].remoteTipSeen == prevTip {
		t.Fatalf("a second action started while %s was still at its previous tip %s: the repair was not pushed before it", branch, prevTip)
	}
	tip := pass6aGit(t, origin, "rev-parse", "refs/heads/"+branch)
	if tip != cli.calls[1].remoteTipSeen {
		t.Errorf("the remote branch moved again after the repair: %s then %s", cli.calls[1].remoteTipSeen, tip)
	}
	parents := strings.Fields(pass6aGit(t, origin, "rev-list", "--parents", "-n", "1", tip))
	if len(parents) != 3 {
		t.Fatalf("the remote branch's tip %s has %d parents, want a merge commit with 2", tip, len(parents)-1)
	}
	if _, err := pass6aGitErr(origin, "merge-base", "--is-ancestor", prevTip, tip); err != nil {
		t.Errorf("the previous tip %s is not an ancestor of the new tip %s", prevTip, tip)
	}
	mainTip := pass6aGit(t, origin, "rev-parse", "refs/heads/main")
	if _, err := pass6aGitErr(origin, "merge-base", "--is-ancestor", mainTip, tip); err != nil {
		t.Errorf("main's tip %s is not an ancestor of the new tip %s", mainTip, tip)
	}
	content := pass6aGit(t, origin, "show", tip+":docs/roadmap.md")
	if !strings.Contains(content, pass6aMainSide) || !strings.Contains(content, pass6aBranchSide) {
		t.Errorf("the merged docs/roadmap.md does not keep both sides:\n%s", content)
	}
	if mainAfter := pass6aGit(t, origin, "rev-parse", "refs/heads/main"); mainAfter != mainTip {
		t.Errorf("main moved from %s to %s", mainTip, mainAfter)
	}
}

// Diagnostic, not the acceptance item: the same pass with the planner handed
// the conflicting branch outside its prompt. It separates "the planner is not
// told" from the rest of the chain (the limits admit the repair, the write
// happens on the branch, the merge is pushed before the next action).
func TestAWholePassRepairsWhenThePlannerKnowsTheBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	runUnmergeablePass(t, deliveryTemplateID, "karakuri/phase-99-repair", "karakuri/phase-99-next", true)
}

// softwareRegistryFor registers the given pack, so a template's lookup and
// the agent selection read the same pack whose environments the run uses.
func softwareRegistryFor(t *testing.T, pack *software.Pack) *domain.Registry {
	t.Helper()
	reg := domain.NewRegistry()
	if err := reg.Register(context.Background(), pack, domain.Config{}); err != nil {
		t.Fatalf("register software: %v", err)
	}
	return reg
}
