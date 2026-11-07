package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	domainsw "github.com/bsenel/karakuri/domains/software"
	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
	featurememory "github.com/bsenel/karakuri/internal/feature/memory"
	featureobjective "github.com/bsenel/karakuri/internal/feature/objective"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

// docs/roadmap.md, Phase 41 step 11, startup half: "startup uses the median of
// N and rejects fewer than min_samples". One run of
// software.verify.cli_startup_ms is one action and one sample
// (internal/feature/loop/verify.go, qualifyingSamples), so N samples are N
// verifier actions in one plan. The whole loop runs here: a scripted planner
// plans the runs, the software pack's own verify environment times the
// fixture CLI, and the loop's verify step settles the criterion. A verifier
// that is not declared, not served or not measuring fails these tests.
//
// The fixture is told to sleep 40, 900, 45, 50 and 42 ms. Against <= 100 ms
// the median, about 45, is met; the mean, about 215, would not be.

const cliStartupVerifier = "software.verify.cli_startup_ms"

func cliStartupCriterion() coreobjective.Criterion {
	target := 100.0
	return coreobjective.Criterion{
		ID: "cli-starts-fast", Description: "the CLI starts in 100 ms or less",
		Kind: coreobjective.KindMeasured, Verifier: cliStartupVerifier,
		Metric: "cli_startup_ms", Op: coreobjective.OpLessEqual, Target: &target, Unit: "ms",
		Weight:  1.0,
		Params:  map[string]any{"binary": "bin/startupfixture"},
		Samples: 5, Reducer: coreobjective.ReducerMedian, MinSamples: 3,
	}
}

// buildCLIFixture builds the software pack's startup fixture into a fresh
// workdir as bin/startupfixture and returns the workdir.
func buildCLIFixture(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("..", "..", "domains", "software", "testdata", "startupfixture"))
	if err != nil {
		t.Fatal(err)
	}
	workdir := t.TempDir()
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", filepath.Join(workdir, "bin", "startupfixture"), ".") // #nosec G204 -- fixed arguments and a path this test chose; no shell
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build the startup fixture: %v\n%s", err, b)
	}
	return workdir
}

// planProvider is an LLM that always plans the same thing. It embeds the
// interface for the methods the agent runtime does not call on this path.
type planProvider struct {
	llm.ProviderAdapter
	reply string
}

func (p *planProvider) Name() string                   { return "cli-startup-plan" }
func (p *planProvider) Available(context.Context) bool { return true }

func (p *planProvider) Complete(context.Context, llm.CompletionRequest) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{Content: p.reply, TokensUsed: 10}, nil
}

func (p *planProvider) Stream(context.Context, llm.CompletionRequest) (<-chan llm.CompletionChunk, error) {
	ch := make(chan llm.CompletionChunk, 1)
	ch <- llm.CompletionChunk{Content: p.reply, Done: true}
	close(ch)
	return ch, nil
}

// startupPlan is one verifier action per sleep, each with the criterion's
// params plus the workdir and the args that make the fixture sleep.
func startupPlan(t *testing.T, envID, workdir string, sleeps ...string) string {
	t.Helper()
	type action struct {
		Capability string         `json:"capability"`
		EnvID      string         `json:"env_id"`
		Reason     string         `json:"reason"`
		Params     map[string]any `json:"params"`
	}
	var actions []action
	for _, s := range sleeps {
		actions = append(actions, action{
			Capability: cliStartupVerifier, EnvID: envID, Reason: "time the CLI",
			Params: map[string]any{"binary": "bin/startupfixture", "workdir": workdir, "args": []string{"-sleep", s}},
		})
	}
	b, err := json.Marshal(map[string]any{"actions": actions, "confidence": 0.95, "reasoning": "sample the startup"})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// settleCLIStartup creates the criterion through the production validator,
// runs one loop iteration whose plan times the fixture once per sleep, and
// returns the criterion as the verify step settled it.
func settleCLIStartup(t *testing.T, sleeps ...string) coreobjective.Criterion {
	t.Helper()
	workdir := buildCLIFixture(t)

	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "loop.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := storage.NewGORMStorage(db)

	pack := domainsw.New()
	capReg := capability.NewRegistry()
	for _, c := range pack.Capabilities() {
		if err := capReg.Register(c); err != nil {
			t.Fatalf("register %s: %v", c.ID, err)
		}
	}
	envReg := environment.NewRegistry()
	envID := ""
	for _, f := range pack.EnvironmentFactories() {
		for _, id := range f.Serves {
			if id == cliStartupVerifier {
				envID = string(f.EnvID)
				if err := envReg.Register(f); err != nil {
					t.Fatalf("register %s: %v", f.EnvID, err)
				}
			}
		}
	}
	if envID == "" {
		t.Fatalf("no environment of the software pack serves %s", cliStartupVerifier)
	}

	objSvc := featureobjective.NewService(store).WithVerifiers(capReg)
	obj, err := objSvc.Create(t.Context(), featureobjective.CreateRequest{
		Title: "keep the CLI fast", Domain: "software",
		SuccessCriteria: []coreobjective.Criterion{cliStartupCriterion()},
	})
	if err != nil {
		t.Fatalf("the production validator refused a cli startup criterion: %v", err)
	}

	hub := event.NewHub()
	otel := observability.NewOTel(nil)
	providers := llm.NewRegistry(nil)
	providers.Register(&planProvider{reply: startupPlan(t, envID, workdir, sleeps...)})
	svc := featureloop.NewService(store, platformagent.NewFactory(providers, hub, otel, nil), capReg, envReg,
		featurememory.NewService(store, 5), featurecp.NewService(store, hub), nil, nil, hub, otel, nil,
		karakuriquota.Deps{}, nil, 0)

	def := coreagent.Definition{
		ID: "cli-startup-agent", Name: "CLI Startup Agent", Domain: "software",
		ReasoningStrategy: coreagent.ReasoningChainOfThought,
		Authority:         coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions},
		LLMHints:          capability.LLMHints{PreferredProvider: "cli-startup-plan"},
	}
	started, err := svc.Run(t.Context(), coreloop.Request{Objective: obj, Agent: def, MaxIter: 1})
	if err != nil {
		t.Fatalf("run the loop: %v", err)
	}

	deadline := time.Now().Add(2 * time.Minute)
	for {
		state, err := store.GetLoopState(t.Context(), started.LoopID)
		if err == nil && state.Completed {
			for _, c := range state.Criteria {
				if c.ID == "cli-starts-fast" {
					return c
				}
			}
			t.Fatalf("the loop ended (outcome %q, error %q) with criteria %#v, none of them cli-starts-fast",
				state.Outcome, state.Error, state.Criteria)
		}
		if time.Now().After(deadline) {
			t.Fatalf("the loop did not end within 2 minutes (state %#v, err %v)", state, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Five runs, one of them an outlier: the median settles it met.
func TestACLIStartupCriterionIsMetOnTheMedianOfItsRuns(t *testing.T) {
	c := settleCLIStartup(t, "40ms", "900ms", "45ms", "50ms", "42ms")
	if c.State != coreobjective.CriterionMet {
		t.Errorf("state = %q (reason %q), want met: the median of runs sleeping 40, 900, 45, 50 and 42 ms is about 45 ms, under 100",
			c.State, c.Reason)
	}
}

// Two runs are fewer than min_samples 3: inconclusive, however fast they are.
func TestACLIStartupCriterionWithTooFewRunsIsInconclusive(t *testing.T) {
	c := settleCLIStartup(t, "40ms", "45ms")
	if c.State != coreobjective.CriterionInconclusive || c.Reason != coreobjective.ReasonTooFewSamples {
		t.Errorf("state = %q, reason = %q; want inconclusive: too_few_samples from two runs under min_samples 3",
			c.State, c.Reason)
	}
}
