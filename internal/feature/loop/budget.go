package loop

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	"github.com/bsenel/karakuri/internal/platform/llm"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota/cost"
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
	// objectiveID attributes the charge to the piece of work that incurred
	// it. The twin still pays — it is the subject on the ledger event — but
	// without this the expensive half of a loop's spend lands under
	// resourceType "twin" and there is no way to ask what one objective
	// cost. A per-objective ceiling has nothing to measure until this is set.
	objectiveID objective.ObjectiveID
	costs       *karakuriquota.Recorder
	now         func() time.Time

	// mu guards spent, which accumulates what this run has cost so far.
	//
	// Per run rather than per day, and the distinction is the point:
	// Budget.Daily bounds the month's bill and is answerable from the ledger
	// after the fact, while Budget.PerReconcile bounds the blast radius of one
	// pass that goes wrong — a loop that spends a day's allowance in a single
	// run has stayed inside its daily ceiling and still wants stopping. The
	// ledger cannot answer that in time, because the run is what is being
	// measured. A budgetedAgent is built once per runLoop, so its lifetime is
	// exactly the window the ceiling is about.
	mu    sync.Mutex
	spent float64
}

// Spent reports what this run has cost so far, priced.
func (a *budgetedAgent) Spent() float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.spent
}

var _ coreagent.Agent = (*budgetedAgent)(nil)

// withBudget wraps agent unless there is nothing to enforce.
func withBudget(agent coreagent.Agent, budget karakuriquota.TokenBudget, costs *karakuriquota.Recorder, twinID string, objectiveID objective.ObjectiveID) coreagent.Agent {
	if budget == nil || twinID == "" {
		// No twin means no subject to charge — an ad-hoc objective created
		// without one. Metering it against a shared bucket would be worse than
		// not metering it.
		return agent
	}
	return &budgetedAgent{
		inner: agent, budget: budget, costs: costs,
		twinID: twinID, objectiveID: objectiveID, now: time.Now,
	}
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
	// The budget says whether there is room left; the ledger says what it cost.
	// Both, because a token count is not a bill and a bill does not stop a
	// runaway loop.
	spend := karakuriquota.Spend{
		TwinID: a.twinID,
		// Attributed to the objective, matching how stepAct already
		// attributes adapter calls. Tokens are the expensive half, so
		// leaving them on the twin made per-objective spend answerable for
		// the cheap half only.
		ResourceType: resourceTypeFor(a.objectiveID),
		ResourceID:   string(a.objectiveID),
		Provider:     out.Provider,
		Model:        out.Model,
		Units:        float64(out.TokensUsed),
		UnitKind:     cost.UnitTokens,
	}
	a.costs.Record(ctx, spend)

	// Priced from the same recorder that writes the ledger, so the running
	// total and the eventual bill agree.
	a.mu.Lock()
	a.spent += a.costs.Price(spend)
	a.mu.Unlock()
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

// allowCapability charges the twin's daily allowance for one capability and
// reports whether the action may run.
//
// The tier has been configured, documented and defaulted since Phase 15 and was
// enforced nowhere — TakeCapability had no callers. This is that call site.
//
// It fails open, like every other limiter here: a counter that cannot be read
// must not stop work that is otherwise fine, because a quota backend outage
// turning into a halted loop does more damage than the calls it would have
// bounded.
func (s *serviceImpl) allowCapability(ctx context.Context, sc *stepContext, capabilityID string, index int) bool {
	if s.quota.Backend == nil || sc.twinID == "" || capabilityID == "" {
		return true
	}
	dec, err := s.quota.TakeCapability(ctx, sc.twinID, capabilityID, time.Now())
	if err != nil {
		slog.Error("capability quota could not be read; allowing the action",
			"twin", sc.twinID, "capability", capabilityID, "loop", sc.loopID, "err", err)
		return true
	}
	if dec.Allowed {
		return true
	}

	slog.Warn("capability quota exhausted; skipping the action",
		"twin", sc.twinID, "capability", capabilityID, "loop", sc.loopID,
		"limit", dec.Limit, "reset_at", dec.ResetAt)
	s.hub.Publish(ctx, event.Event{
		Type:        event.TypeQuotaPressure,
		ObjectiveID: string(sc.obj.ID),
		TwinID:      sc.twinID,
		Payload: map[string]any{
			"tier":       "capability",
			"capability": capabilityID,
			"limit":      dec.Limit,
			"remaining":  dec.Remaining,
			"reset_at":   dec.ResetAt,
			"action":     index,
		},
		Timestamp: time.Now().UTC(),
	})
	return false
}

// resourceTypeFor keeps an ad-hoc loop — one with no objective behind it —
// attributed the way it was before, rather than under an "objective" resource
// with an empty ID that no query could name.
func resourceTypeFor(id objective.ObjectiveID) string {
	if id == "" {
		return ""
	}
	return "objective"
}

// perPassCeilingReached reports whether this run has spent the objective's
// Budget.PerReconcile, with what it spent and what the ceiling was.
//
// Budget.Daily and Budget.PerReconcile answer different questions and are
// enforced in different places. Daily bounds the month's bill and is a
// pre-check the supervisor makes from the ledger before dispatching at all;
// PerReconcile bounds the blast radius of one pass that goes wrong, which the
// ledger cannot answer in time because the run is what is being measured.
//
// It was declared in Phase 23 and read by nothing until Phase 23's close-out —
// another field holding a plausible value that changed no behaviour, in the
// same line of work that found five others.
func perPassCeilingReached(metered *budgetedAgent, obj objective.Objective) (spent, ceiling float64, over bool) {
	if metered == nil {
		return 0, 0, false
	}
	ceiling = obj.BudgetDeclaration().PerReconcile
	if ceiling <= 0 {
		return 0, 0, false
	}
	spent = metered.Spent()
	// Reached, not passed, matching Budget.ExceedsDaily: a ledger is written
	// after the work, so a run sitting exactly on its ceiling has already
	// spent it. Treating that as room left is how a ceiling gets rounded up by
	// one iteration every pass.
	return spent, ceiling, spent >= ceiling
}
