package reconcile

// Phase 40 step 9a, the lead's review Blocker on pull request #272: the
// resume brief names the branch of the pass that was cut off, not of the pass
// before it.
//
// What the code does today, read before writing these (2026-11-21):
//   - plannedBranch (branch_commits.go) reads the branch from the objective's
//     newest LoopIteration whose Step is "learn".
//   - The loop saves that row in stepLearn only. A loop cut by the quota
//     returns right after stepAct (loop/runner.go, the outOfQuota exit comes
//     before stepLearn), and one cut by a restart never reaches learn either.
//   - So the cut pass's plan is never on a learn row, and the brief names the
//     previous pass that did reach learn: its branch, and its commits as
//     "must not plan again".

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	"github.com/bsenel/karakuri/internal/core/event"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	"github.com/bsenel/karakuri/internal/feature/gateway"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
	featurememory "github.com/bsenel/karakuri/internal/feature/memory"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

const (
	branchA = "karakuri/previous-pass"
	branchB = "karakuri/cut-off-pass"
)

// forgeByBranch answers CompareCommits with the commits of the branch asked
// about, so a brief that asks about the wrong branch reads the wrong commits.
type forgeByBranch struct {
	versioncontrol.VersionControlAdapter
	mu      sync.Mutex
	commits map[string][]versioncontrol.Commit
	asked   []string
}

func (g *forgeByBranch) Name() string { return "by-branch" }
func (g *forgeByBranch) Active() bool { return true }
func (g *forgeByBranch) CompareCommits(_ context.Context, _, _, head string) ([]versioncontrol.Commit, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.asked = append(g.asked, head)
	return g.commits[head], nil
}

// outOfQuotaEnv is the coding agent's environment refusing its first action.
type outOfQuotaEnv struct{}

func (outOfQuotaEnv) ID() environment.EnvironmentID { return "test.env.ok" }
func (outOfQuotaEnv) Domain() string                { return "test" }
func (outOfQuotaEnv) Observe(context.Context, environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{EnvID: "test.env.ok", Version: "v1", Timestamp: time.Now().UTC()}, nil
}
func (outOfQuotaEnv) Act(context.Context, environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{}, fmt.Errorf("delegate_to_cli: %w", &coreerrors.ProviderRateLimitError{
		Provider:   "claude_code",
		RetryAfter: 2 * time.Hour,
		Cause:      errors.New("claude_code: exit 1 (stderr: ); last output: You've hit your session limit"),
	})
}
func (outOfQuotaEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return make(chan environment.EnvironmentEvent), nil
}
func (outOfQuotaEnv) Snapshot(context.Context) (environment.EnvironmentSnapshot, error) {
	return environment.EnvironmentSnapshot{EnvID: "test.env.ok"}, nil
}

// plansBranchB is an LLM that always plans one write action on branchB.
type plansBranchB struct{ llm.ProviderAdapter }

const planOnBranchB = `{"actions":[{"capability":"test.act.ok","env_id":"test.env.ok",` +
	`"params":{"branch":"` + branchB + `","repo":"acme/api"},"reason":"delegate"}],"confidence":0.95,"reasoning":"r"}`

func (plansBranchB) Name() string                   { return "plans-b" }
func (plansBranchB) Available(context.Context) bool { return true }
func (plansBranchB) Complete(context.Context, llm.CompletionRequest) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{Content: planOnBranchB, TokensUsed: 10}, nil
}
func (plansBranchB) Stream(context.Context, llm.CompletionRequest) (<-chan llm.CompletionChunk, error) {
	ch := make(chan llm.CompletionChunk, 1)
	ch <- llm.CompletionChunk{Content: planOnBranchB, Done: true}
	close(ch)
	return ch, nil
}

// realCutLoop runs a real loop whose plan names branchB and whose coding
// agent is out of quota on its first action, and returns its id once ended.
func realCutLoop(t *testing.T, f *fixture, obj objective.Objective) string {
	t.Helper()
	hub := event.NewHub()
	otel := observability.NewOTel(nil)
	providers := llm.NewRegistry(nil)
	providers.Register(plansBranchB{})
	envReg := environment.NewRegistry()
	if err := envReg.Register(environment.Factory{
		EnvID: "test.env.ok", Domain: "test",
		Serves: []capability.CapabilityID{"test.act.ok"},
		Build:  func(environment.BuildContext) (environment.Environment, error) { return outOfQuotaEnv{}, nil },
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := featureloop.NewService(f.store, platformagent.NewFactory(providers, hub, otel, nil), nil, envReg,
		featurememory.NewService(f.store, 5), featurecp.NewService(f.store, hub), nil, nil, hub, otel, nil,
		karakuriquota.Deps{}, gateway.New(f.store, karakuriquota.Deps{}, nil), nil, 0)

	run, err := svc.Run(context.Background(), coreloop.Request{
		Objective: obj,
		Agent: coreagent.Definition{
			ID: "cut-agent", Name: "Cut Agent", Domain: "test",
			ReasoningStrategy: coreagent.ReasoningChainOfThought,
			Authority:         coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions},
			LLMHints:          capability.LLMHints{PreferredProvider: "plans-b"},
		},
		MaxIter: 3,
	})
	if err != nil {
		t.Fatalf("run the loop: %v", err)
	}
	deadline := time.Now().Add(stopWait)
	for {
		st, gerr := f.store.GetLoopState(context.Background(), run.LoopID)
		if gerr == nil && st.Completed {
			if st.Outcome != coreloop.RunWaitingForQuota {
				t.Fatalf("the loop ended %q, want %q: the fixture does not cut it in act", st.Outcome, coreloop.RunWaitingForQuota)
			}
			return run.LoopID
		}
		if time.Now().After(deadline) {
			t.Fatalf("the loop did not end within %s (state %+v, err %v)", stopWait, st, gerr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// (a) A previous pass finished learn on branch A; the next pass planned
// branch B and was cut in act by the quota. The brief is about B.
func TestTheBriefNamesTheCutPassBranchNotThePreviousPassBranch(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})
	ctx := context.Background()

	savePlan(t, f.store, obj.ID, 1, act("software.act.delegate_to_cli", map[string]any{"branch": branchA, "repo": "acme/api"}))
	forge := &forgeByBranch{commits: map[string][]versioncontrol.Commit{
		branchA: {{SHA: "aaa111", Message: "feat: previous pass"}},
		branchB: {{SHA: "bbb222", Message: "feat: cut pass first"}, {SHA: "ccc333", Message: "feat: cut pass second"}},
	}}
	f.svc.branchCommits = readerFor(f, forge)

	loopID := realCutLoop(t, f, obj)
	if err := f.store.SaveReconcileOutcome(ctx, reconcile.Outcome{
		ID: newOutcomeID(), ObjectiveID: obj.ID, Trigger: reconcile.TriggerManual, LoopID: loopID,
		Deferred: outcomeWaitingForQuota, DeferredUntil: f.clock.Add(2 * time.Hour),
		StartedAt: f.clock, EndedAt: f.clock,
	}); err != nil {
		t.Fatalf("record the cut pass: %v", err)
	}

	brief := f.svc.resumeBrief(ctx, obj)
	if brief == nil {
		t.Fatal("the pass after a pass cut off by the quota got no brief")
	}
	if brief.Branch == branchA {
		t.Errorf("the brief named the previous pass's branch %q, want the cut pass's %q", branchA, branchB)
	}
	if brief.Branch != branchB {
		t.Errorf("brief.Branch = %q, want %q: the branch the cut loop's plan named", brief.Branch, branchB)
	}
	if slices.Contains(brief.Commits, "aaa111") {
		t.Errorf("the brief named the previous pass's commits %v as must-not-plan-again", brief.Commits)
	}
	for _, want := range []string{"bbb222", "ccc333"} {
		if !slices.Contains(brief.Commits, want) {
			t.Errorf("brief.Commits = %v, want the cut loop's commit %s", brief.Commits, want)
		}
	}
	if brief.CommitsUnreadable != "" {
		t.Errorf("brief.CommitsUnreadable = %q, want none: the cut loop's plan names its branch", brief.CommitsUnreadable)
	}
}

// (b) A cut loop that saved no plan: its branch is unknown, which the brief
// says; it is not none, and it is not the previous pass's branch (AGENTS.md
// rule 10).
func TestTheBriefOfACutPassWithNoSavedPlanSaysItsBranchIsUnknown(t *testing.T) {
	r := newResumeFixture(t, nil, nil)
	ctx := context.Background()
	savePlan(t, r.store, r.obj.ID, 1, act("software.act.delegate_to_cli", map[string]any{"branch": branchA, "repo": "acme/api"}))
	forge := &forgeByBranch{commits: map[string][]versioncontrol.Commit{
		branchA: {{SHA: "aaa111", Message: "feat: previous pass"}},
	}}
	r.svc.branchCommits = readerFor(r.fixture, forge)

	// The fake loop saves no learn row, as a loop cut before learn saves none.
	endsWaitingForQuota(t, r)

	brief := r.svc.resumeBrief(ctx, r.obj)
	if brief == nil {
		t.Fatal("the pass after a pass cut off by the quota got no brief")
	}
	if brief.Branch == branchA {
		t.Errorf("the brief named the previous pass's branch %q for a cut pass that saved no plan", branchA)
	}
	if len(brief.Commits) != 0 {
		t.Errorf("the brief named the previous pass's commits %v as must-not-plan-again", brief.Commits)
	}
	if brief.CommitsUnreadable == "" {
		t.Error("brief.CommitsUnreadable is empty: the brief says the cut pass's branch holds none, not that it is unknown")
	}
	if strings.Contains(strings.ToLower(brief.CommitsUnreadable), "no commits") {
		t.Errorf("brief.CommitsUnreadable = %q, want it to say the branch is unknown, not empty", brief.CommitsUnreadable)
	}
	if len(forge.asked) != 0 {
		t.Errorf("the forge was asked about %v with no branch known for the cut pass", forge.asked)
	}
}
