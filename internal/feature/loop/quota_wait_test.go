package loop

// Phase 40 step 8, the loop's half: a delegated coding agent its provider
// refused for quota ends the run at once, waiting for quota, with the time the
// quota returns kept on the stored run.
//
// What the code does today, read before writing these (2026-11-17):
//   - stepAct (act.go) runs every action of the plan in turn. An action whose
//     environment returns an error is recorded as errored, its result's error
//     text kept, and the next action runs; nothing breaks out of the plan.
//     Since slice 1, the software cli_agent environment returns a quota
//     refusal as a *coreerrors.ProviderRateLimitError behind an Unwrap, so
//     the type reaches stepAct as the action's error, and is dropped there.
//   - runLoop (runner.go) then verifies and learns over the outcomes and goes
//     on to its next iteration. Only the planner's rate limit (reason.go,
//     plan.endReason) ends a run early, and it ends it errored, with the
//     error text reconcile matches as provider_rate_limited.
//   - finalizeLoop names the outcome: errored on a run error, refused_by_bounds,
//     converged, otherwise budget_spent. An objective with no success criteria
//     that runs out of iterations ends budget_spent with no error.
//   - State has no field for when a provider's quota returns; RetryAfter is the
//     scaffolding this step adds, and storage does not persist it yet.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/llm"
)

// quotaPlan is three actions on the one capability the fixture serves; the
// first is the coding agent's, told apart by its params.
const quotaPlan = `{"actions":[` +
	`{"capability":"test.act.ok","env_id":"test.env.ok","params":{"n":1},"reason":"delegate"},` +
	`{"capability":"test.act.ok","env_id":"test.env.ok","params":{"n":2},"reason":"second"},` +
	`{"capability":"test.act.ok","env_id":"test.env.ok","params":{"n":3},"reason":"third"}` +
	`],"confidence":0.95,"reasoning":"r"}`

const quotaRetryAfter = 2 * time.Hour

// quotaEnv fails the plan's first action with firstErr and lets the others
// succeed, counting every action that reaches it.
type quotaEnv struct {
	traceEnv
	firstErr error

	mu   sync.Mutex
	acts []string
}

func (e *quotaEnv) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	n := fmt.Sprint(a.Params["n"])
	e.mu.Lock()
	e.acts = append(e.acts, n)
	e.mu.Unlock()
	if n == "1" {
		return environment.ActionResult{}, e.firstErr
	}
	return environment.ActionResult{Success: true}, nil
}

func (e *quotaEnv) actCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.acts)
}

// quotaFixture is stopFixture's service with the three-action plan and env as
// the only environment. The objective has no success criteria, so a run that
// is not ended early runs every one of its maxIter iterations.
func quotaFixture(t *testing.T, env *quotaEnv, maxIter int) (*serviceImpl, coreloop.Request) {
	t.Helper()
	svc, req := stopFixture(t, newStopEnv(false), unlimited)
	providers := llm.NewRegistry(nil)
	providers.Register(&traceProvider{reply: quotaPlan})
	svc.factory = platformagent.NewFactory(providers, svc.hub, svc.otel, nil)
	reg := environment.NewRegistry()
	if err := reg.Register(environment.Factory{
		EnvID:  "test.env.ok",
		Domain: "test",
		Serves: []capability.CapabilityID{"test.act.ok"},
		Build: func(environment.BuildContext) (environment.Environment, error) {
			return env, nil
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc.envReg = reg
	req.MaxIter = maxIter
	return svc, req
}

// (a) The coding agent's quota refusal: one action reaches the executor, no
// further iteration runs, and the stored run ends waiting_for_quota with the
// provider's retry-after on it.
func TestACodingAgentOutOfQuotaEndsTheRunWaitingForQuota(t *testing.T) {
	// Wrapped as the software cli_agent environment returns it: the type is
	// reached through Unwrap, never by the text.
	refusal := fmt.Errorf("delegate_to_cli: %w", &coreerrors.ProviderRateLimitError{
		Provider:   "claude_code",
		RetryAfter: quotaRetryAfter,
		Cause:      errors.New("claude_code: exit 1 (stderr: ); last output: You've hit your session limit · resets 4:20pm (Europe/Paris)"),
	})
	env := &quotaEnv{traceEnv: traceEnv{id: "test.env.ok"}, firstErr: refusal}
	svc, req := quotaFixture(t, env, 5)

	run, err := svc.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	st := waitStoredCompleted(t, svc.store, run.LoopID)

	if n := env.actCount(); n != 1 {
		t.Errorf("%d actions reached the executor (%v), want 1: a pass out of quota stops spending actions at once", n, env.acts)
	}
	if st.Iteration != 0 {
		t.Errorf("stored iteration = %d, want 0: the run went on to further iterations after the coding agent was refused for quota", st.Iteration)
	}
	if st.Outcome != coreloop.RunWaitingForQuota {
		t.Errorf("stored outcome = %q, want %q: out of quota is a state, not %q", st.Outcome, coreloop.RunWaitingForQuota, st.Outcome)
	}
	if st.RetryAfter != quotaRetryAfter {
		t.Errorf("stored retry_after = %s, want %s: reconcile reads the time the quota returns from the stored run", st.RetryAfter, quotaRetryAfter)
	}
	got := waitRunEndOutcomes(t, svc.store, req.Objective.ID, run.LoopID)
	if len(got) != 1 || got[0] != string(coreloop.RunWaitingForQuota) {
		t.Errorf("run_end rows for the run name %v, want one naming %q", got, coreloop.RunWaitingForQuota)
	}
}

// (a') The same plan whose first action fails for any other reason behaves as
// today: the failed action is recorded and the plan goes on, every iteration
// runs, and the run ends budget_spent with no error, never waiting_for_quota.
// An adapter that cannot tell quota from another failure must not guess from
// an empty stderr.
func TestACodingAgentThatExitsOneWithNoReasonBehavesAsToday(t *testing.T) {
	const maxIter = 2
	env := &quotaEnv{traceEnv: traceEnv{id: "test.env.ok"}, firstErr: errors.New("claude_code: exit 1 (stderr: )")}
	svc, req := quotaFixture(t, env, maxIter)

	run, err := svc.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	st := waitStoredCompleted(t, svc.store, run.LoopID)

	if n := env.actCount(); n != 3*maxIter {
		t.Errorf("%d actions reached the executor (%v), want %d: every action of every iteration, as today", n, env.acts, 3*maxIter)
	}
	if st.Outcome == coreloop.RunWaitingForQuota {
		t.Fatal("a plain exit 1 ended the run waiting_for_quota; nothing in it says quota")
	}
	if st.Outcome != coreloop.RunBudgetSpent {
		t.Errorf("stored outcome = %q, want today's %q", st.Outcome, coreloop.RunBudgetSpent)
	}
	if st.Error != "" {
		t.Errorf("stored error = %q, want today's none: a failed action does not end the run", st.Error)
	}
	if st.RetryAfter != 0 {
		t.Errorf("stored retry_after = %s for a run that was not refused for quota", st.RetryAfter)
	}
	if strings.Contains(st.Error, coreerrors.ErrProviderRateLimited.Error()) {
		t.Errorf("stored error %q reads as a rate limit to reconcile", st.Error)
	}
}
