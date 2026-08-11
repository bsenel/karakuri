package loop

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/event"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	"github.com/bsenel/karakuri/internal/platform/llm"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

// budgetedAgent charges a twin's LLM token budget for every model call.
//
// It wraps the agent rather than sitting in the reason step, because the
// reflexion path calls Run four separate times — the initial reasoning, the
// critique, the revision, and the modify-driven revision. A check in one of
// those would leave three unmetered, and the ones it would miss are exactly the
// ones a loop that will not converge keeps making.
type budgetedAgent struct {
	inner  coreagent.Agent
	budget karakuriquota.TokenBudget
	twinID string
	now    func() time.Time
}

var _ coreagent.Agent = (*budgetedAgent)(nil)

// withBudget wraps agent unless there is nothing to enforce.
func withBudget(agent coreagent.Agent, budget karakuriquota.TokenBudget, twinID string) coreagent.Agent {
	if budget == nil || twinID == "" {
		// No twin means no subject to charge — an ad-hoc objective created
		// without one. Metering it against a shared bucket would be worse than
		// not metering it.
		return agent
	}
	return &budgetedAgent{inner: agent, budget: budget, twinID: twinID, now: time.Now}
}

func (a *budgetedAgent) Run(ctx context.Context, input coreagent.Input) (coreagent.Output, error) {
	now := a.now()
	// Mark the call as this twin's so a gateway can bill it. Harmless when no
	// gateway is configured — nothing reads it.
	ctx = llm.WithTwin(ctx, a.twinID)
	if err := a.budget.Reserve(ctx, a.twinID, 0, now); err != nil {
		if errors.Is(err, karakuriquota.ErrBudgetExhausted) {
			return coreagent.Output{}, err
		}
		// The budget could not be consulted. Allow the call and carry on: an
		// unreachable counter should not stop a loop that is otherwise
		// healthy, for the same reason the HTTP limiter fails open.
		return a.inner.Run(ctx, input)
	}

	out, err := a.inner.Run(ctx, input)
	if err != nil {
		// A failed call still costs whatever the provider spent before failing,
		// but nothing reports that, so there is nothing honest to charge.
		return out, err
	}
	// Recording is best-effort: the work is already done and paid for
	// upstream, and losing the record is better than discarding the result.
	_ = a.budget.Record(ctx, a.twinID, out.TokensUsed, now)
	return out, nil
}

func (a *budgetedAgent) Stream(ctx context.Context, input coreagent.Input) (<-chan coreagent.OutputChunk, error) {
	// Stream is implemented over Run everywhere in this codebase, so the
	// charge lands there. Wrapping it again would double-count.
	return a.inner.Stream(ctx, input)
}

// budgetExhaustedReason is the checkpoint reason an operator sees, and what
// `krk audit --kind escalation` filters on.
const budgetExhaustedReason = "llm_budget_exhausted"

// pauseIfBudgetExhausted checks the twin's LLM allowance and, when it is spent,
// pauses the loop on a checkpoint instead of failing it.
//
// The check is at the iteration boundary rather than around each model call.
// Pausing mid-iteration would leave a half-applied plan — actions taken, none
// verified — and a human resuming it would have no way to tell what had already
// happened. The cost is that one iteration can overshoot the cap by whatever it
// spends; the alternative costs correctness.
//
// This is deliberately not an error. A budget is a business limit, not a fault:
// the right response is to ask somebody whether to keep going, which is exactly
// what the authority-bounds machinery from Phase 13 already does.
func (s *serviceImpl) pauseIfBudgetExhausted(ctx context.Context, sc *stepContext) bool {
	if s.budget == nil || sc.twinID == "" {
		return false
	}
	usage, err := s.budget.Usage(ctx, sc.twinID, time.Now())
	if err != nil {
		// Same reasoning as the HTTP limiter: a counter that cannot be read
		// must not stop work that is otherwise fine.
		slog.Error("llm budget could not be read; continuing unmetered",
			"twin", sc.twinID, "loop", sc.loopID, "err", err)
		return false
	}
	if usage.Allowed {
		return false
	}

	summary := fmt.Sprintf(
		"Loop %s has spent twin %s's LLM budget for the period (%d of %d tokens). "+
			"Approve to continue for one more iteration, or reject to stop here.",
		sc.loopID, sc.twinID, usage.Limit-usage.Remaining, usage.Limit)

	cp, err := s.cpSvc.Create(ctx, sc.obj.ID, sc.twinID,
		budgetExhaustedReason, summary,
		[]string{"approve", "reject"},
		featurecp.CreateOptions{},
	)
	cpID := ""
	if err != nil {
		slog.Error("could not create the budget checkpoint; pausing anyway",
			"loop", sc.loopID, "err", err)
	} else {
		cpID = cp.ID
	}

	sc.state.mu.Lock()
	sc.state.status.Paused = true
	idCopy := cpID
	sc.state.result.CheckpointID = &idCopy
	sc.state.mu.Unlock()

	s.hub.Publish(ctx, event.Event{
		Type:        event.TypeCheckpoint,
		ObjectiveID: string(sc.obj.ID),
		TwinID:      sc.twinID,
		Payload: map[string]any{
			"checkpoint_id": cpID,
			"reason":        budgetExhaustedReason,
			"loop_id":       sc.loopID,
			"limit":         usage.Limit,
			"reset_at":      usage.ResetAt,
		},
	})
	return true
}
