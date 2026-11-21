package loop

// Phase 40 step 9a, acceptance: a coding agent out of quota after the second
// of four actions, and the pass that follows once the quota is back.
//
// What the code does, read before writing this (2026-11-21):
//   - A refused action ends the run waiting_for_quota (step 8); the actions
//     after it in the plan never reach the executor.
//   - reconcile gives the next pass coreloop.Request.Resume (slice A), and
//     stepReason renders it into the planner's task (buildResumeNotice).
//   - Nothing in the product can force a planner to obey it: a model reads
//     the notice and plans or does not. The scripted planner below obeys it
//     only when it is shown, so this test proves the brief reaches the
//     planner and that, planning from it, the second pass finishes the work
//     on the same branch with one pull request. It cannot prove a real model
//     complies; that is judged on the first real resumed pass.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/llm"
)

const resumeBranch = "karakuri/phase-40-step-09-quota-order"

// forgeEnv is a fake forge: actions 1 to 3 commit to the branch the action
// names, action 4 opens a pull request on it. The coding agent is out of
// quota when it is asked for action 3, once.
type forgeEnv struct {
	traceEnv

	mu       sync.Mutex
	quotaOut bool
	acts     []string
	commits  map[string][]string // branch -> commit subjects
	prs      []string            // branch of each pull request opened
}

var commitSubjects = map[string]string{"1": "feat: first action", "2": "feat: second action", "3": "feat: third action"}

func (e *forgeEnv) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	n := fmt.Sprint(a.Params["n"])
	branch := fmt.Sprint(a.Params["branch"])
	e.mu.Lock()
	defer e.mu.Unlock()
	e.acts = append(e.acts, n)
	if n == "3" && e.quotaOut {
		return environment.ActionResult{}, fmt.Errorf("delegate_to_cli: %w", &coreerrors.ProviderRateLimitError{
			Provider:   "claude_code",
			RetryAfter: quotaRetryAfter,
			Cause:      errors.New("claude_code: exit 1 (stderr: ); last output: You've hit your session limit"),
		})
	}
	if n == "4" {
		e.prs = append(e.prs, branch)
		return environment.ActionResult{Success: true}, nil
	}
	if e.commits == nil {
		e.commits = map[string][]string{}
	}
	e.commits[branch] = append(e.commits[branch], commitSubjects[n])
	return environment.ActionResult{Success: true}, nil
}

func (e *forgeEnv) branchCommits(branch string) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.commits[branch]...)
}

func forgeAction(n int, branch string) string {
	return fmt.Sprintf(`{"capability":"test.act.ok","env_id":"test.env.ok","params":{"n":%d,"branch":%q},"reason":"step %d"}`, n, branch, n)
}

// resumablePlanner plans all four actions on a fresh branch, unless the task it
// is shown says a pass was cut off; then it plans only what the notice leaves
// and on the branch the notice names.
type resumablePlanner struct {
	traceProvider
	mu      sync.Mutex
	prompts []string
}

func (p *resumablePlanner) Complete(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	var all strings.Builder
	all.WriteString(req.SystemPrompt)
	for _, m := range req.Messages {
		all.WriteString(m.Content)
	}
	prompt := all.String()
	p.mu.Lock()
	p.prompts = append(p.prompts, prompt)
	p.mu.Unlock()

	actions := []string{forgeAction(1, resumeBranch), forgeAction(2, resumeBranch), forgeAction(3, resumeBranch), forgeAction(4, resumeBranch)}
	if strings.Contains(prompt, "was cut off") {
		var left []string
		for n := 1; n <= 4; n++ {
			if n <= 3 && strings.Contains(prompt, commitSubjects[fmt.Sprint(n)]) {
				continue // the branch already holds it
			}
			left = append(left, forgeAction(n, resumeBranch))
		}
		actions = left
	}
	return llm.CompletionResponse{
		Content:    `{"actions":[` + strings.Join(actions, ",") + `],"confidence":0.95,"reasoning":"r"}`,
		TokensUsed: 10,
	}, nil
}

func TestThePassAfterAQuotaCutOffFinishesTheWorkOnTheSameBranchWithOnePullRequest(t *testing.T) {
	env := &forgeEnv{traceEnv: traceEnv{id: "test.env.ok"}, quotaOut: true}
	svc, req := quotaFixture(t, nil, 1)
	planner := &resumablePlanner{}
	providers := llm.NewRegistry(nil)
	providers.Register(planner)
	svc.factory = platformagent.NewFactory(providers, svc.hub, svc.otel, nil)
	reg := environment.NewRegistry()
	if err := reg.Register(environment.Factory{
		EnvID: "test.env.ok", Domain: "test",
		Serves: []capability.CapabilityID{"test.act.ok"},
		Build:  func(environment.BuildContext) (environment.Environment, error) { return env, nil },
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc.envReg = reg

	// First pass: out of quota after the second of four actions.
	first, err := svc.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	st := waitStoredCompleted(t, svc.store, first.LoopID)
	if st.Outcome != coreloop.RunWaitingForQuota {
		t.Fatalf("first pass outcome = %q, want %q", st.Outcome, coreloop.RunWaitingForQuota)
	}
	if got := strings.Join(env.acts, ","); got != "1,2,3" {
		t.Fatalf("first pass actions = %s, want 1,2,3 (the refused third is the last to reach the executor)", got)
	}
	held := env.branchCommits(resumeBranch)
	if len(held) != 2 {
		t.Fatalf("branch holds %v after the first pass, want its two commits", held)
	}

	// The quota is back. Reconcile's brief for the next pass, built from what
	// the branch holds (its seam is exercised in the reconcile package).
	env.mu.Lock()
	env.quotaOut = false
	env.acts = nil
	env.mu.Unlock()
	req.Resume = &coreloop.Resume{Branch: resumeBranch, Commits: held, Unsettled: []string{"pull-request"}}

	second, err := svc.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	waitStoredCompleted(t, svc.store, second.LoopID)

	if got := strings.Join(env.acts, ","); got != "3,4" {
		t.Errorf("second pass actions = %s, want 3,4: no action for what the branch already holds", got)
	}
	if got := env.branchCommits(resumeBranch); len(got) != 3 {
		t.Errorf("branch commits = %v, want each of the three commits once", got)
	}
	if len(env.commits) != 1 {
		t.Errorf("work went to %d branches (%v), want the one branch", len(env.commits), env.commits)
	}
	if len(env.prs) != 1 || env.prs[0] != resumeBranch {
		t.Errorf("pull requests opened on %v, want exactly one, on %s", env.prs, resumeBranch)
	}
	// The planner was shown the brief on the second pass only.
	planner.mu.Lock()
	defer planner.mu.Unlock()
	if len(planner.prompts) < 2 {
		t.Fatalf("planner called %d times, want at least once per pass", len(planner.prompts))
	}
	if strings.Contains(planner.prompts[0], "was cut off") {
		t.Error("the first pass was told it follows a cut-off pass")
	}
	last := planner.prompts[len(planner.prompts)-1]
	for _, want := range []string{resumeBranch, commitSubjects["1"], commitSubjects["2"], "pull-request"} {
		if !strings.Contains(last, want) {
			t.Errorf("the second pass's planner prompt does not contain %q", want)
		}
	}
}
