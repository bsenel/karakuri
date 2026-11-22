package loop

// Phase 40 step 14b, the loop's half: when the request carries a blocker, the
// planner's prompt shows it and says to diagnose before acting. The loop
// decides nothing from it (AGENTS.md rule 8); it only shows it. With no blocker
// the prompt is the one it has always been.

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/llm"
)

type promptKeeper struct {
	traceProvider
	mu      sync.Mutex
	prompts []string
}

func (p *promptKeeper) Complete(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	var all strings.Builder
	all.WriteString(req.SystemPrompt)
	for _, m := range req.Messages {
		all.WriteString(m.Content)
	}
	p.mu.Lock()
	p.prompts = append(p.prompts, all.String())
	p.mu.Unlock()
	return llm.CompletionResponse{Content: `{"actions":[],"confidence":0.95,"reasoning":"r"}`, TokensUsed: 10}, nil
}

// firstPrompt runs one pass with the request as given and returns what the
// planner was shown.
func firstPrompt(t *testing.T, mutate func(*coreloop.Request)) string {
	t.Helper()
	svc, req := quotaFixture(t, &quotaEnv{traceEnv: traceEnv{id: "test.env.ok"}}, 1)
	planner := &promptKeeper{}
	providers := llm.NewRegistry(nil)
	providers.Register(planner)
	svc.factory = platformagent.NewFactory(providers, svc.hub, svc.otel, nil)
	mutate(&req)
	run, err := svc.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitStoredCompleted(t, svc.store, run.LoopID)
	planner.mu.Lock()
	defer planner.mu.Unlock()
	if len(planner.prompts) == 0 {
		t.Fatal("the planner was never called")
	}
	return planner.prompts[0]
}

func TestThePlannerIsShownTheBlockerAndToldToDiagnoseBeforeActing(t *testing.T) {
	at := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
	prompt := firstPrompt(t, func(r *coreloop.Request) {
		r.Unblock = &coreloop.Unblock{
			Kind: "unmergeable_pr", Subject: "karakuri/phase-40-step-14b", Count: 3,
			FirstSeen: at, LastSeen: at.Add(2 * time.Hour),
			Remedy: "merge main into the branch keeping both sides and push it",
		}
	})
	lower := strings.ToLower(prompt)
	for _, want := range []string{
		"unmergeable_pr", "karakuri/phase-40-step-14b", "3",
		"merge main into the branch keeping both sides and push it",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt does not show %q", want)
		}
	}
	// Diagnose first: what to read, to write the finding down, then the
	// smallest bounded fix, inside limits it never widens.
	for _, want := range []string{
		"diagnos", "pull request", "checks", "audit log", "branch",
		"write", "smallest", "failing test", "limits", "never widen",
	} {
		if !strings.Contains(lower, want) {
			t.Errorf("the prompt does not tell the planner about %q", want)
		}
	}
}

func TestABlockerWithNoRemedyIsStillShown(t *testing.T) {
	prompt := firstPrompt(t, func(r *coreloop.Request) {
		r.Unblock = &coreloop.Unblock{Kind: "verify_failed", Subject: "tests-pass", Count: 2}
	})
	for _, want := range []string{"verify_failed", "tests-pass"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt does not show %q", want)
		}
	}
	if !strings.Contains(strings.ToLower(prompt), "diagnos") {
		t.Error("the prompt does not tell the planner to diagnose")
	}
}

// With no blocker the prompt carries none of it.
func TestWithNoBlockerThePromptIsUnchanged(t *testing.T) {
	prompt := strings.ToLower(firstPrompt(t, func(*coreloop.Request) {}))
	for _, banned := range []string{"diagnos", "blocker", "never widen"} {
		if strings.Contains(prompt, banned) {
			t.Errorf("a pass with no blocker was shown %q", banned)
		}
	}
}
