package loop

import (
	"context"
	"errors"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota"
)

// greedyAgent reports a large token count, the way a real provider does after a
// long completion.
type greedyAgent struct {
	calls  int
	tokens int
	err    error
}

func (a *greedyAgent) Run(context.Context, coreagent.Input) (coreagent.Output, error) {
	a.calls++
	if a.err != nil {
		return coreagent.Output{}, a.err
	}
	return coreagent.Output{Content: "{}", TokensUsed: a.tokens}, nil
}

func (a *greedyAgent) Stream(ctx context.Context, in coreagent.Input) (<-chan coreagent.OutputChunk, error) {
	ch := make(chan coreagent.OutputChunk, 1)
	out, err := a.Run(ctx, in)
	ch <- coreagent.OutputChunk{Content: out.Content, Done: true, Err: err}
	close(ch)
	return ch, nil
}

// recordingBudget captures what the decorator charged.
type recordingBudget struct {
	recorded   []int
	reserveErr error
	usageErr   error
	spent      int
	limit      int
}

func (b *recordingBudget) Reserve(context.Context, string, int, time.Time) error {
	return b.reserveErr
}

func (b *recordingBudget) Record(_ context.Context, _ string, tokens int, _ time.Time) error {
	b.recorded = append(b.recorded, tokens)
	b.spent += tokens
	return nil
}

func (b *recordingBudget) Usage(context.Context, string, time.Time) (quota.Decision, error) {
	if b.usageErr != nil {
		return quota.Decision{}, b.usageErr
	}
	return quota.Decision{
		Allowed:   b.spent < b.limit,
		Limit:     b.limit,
		Remaining: max(b.limit-b.spent, 0),
	}, nil
}

func TestBudgetedAgentChargesEveryCall(t *testing.T) {
	// The reason for wrapping the agent rather than checking inside stepReason:
	// the reflexion path calls Run four times, and three of them would go
	// unmetered otherwise.
	inner := &greedyAgent{tokens: 250}
	budget := &recordingBudget{limit: 10000}
	agent := withBudget(inner, budget, nil, "twin-1")

	for range 4 {
		if _, err := agent.Run(context.Background(), coreagent.Input{}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	}
	if len(budget.recorded) != 4 {
		t.Errorf("charged %d calls, want 4 (%v)", len(budget.recorded), budget.recorded)
	}
	if budget.spent != 1000 {
		t.Errorf("charged %d tokens, want 1000", budget.spent)
	}
}

func TestBudgetedAgentRefusesWhenExhausted(t *testing.T) {
	inner := &greedyAgent{tokens: 10}
	budget := &recordingBudget{limit: 100, reserveErr: karakuriquota.ErrBudgetExhausted}
	agent := withBudget(inner, budget, nil, "twin-1")

	_, err := agent.Run(context.Background(), coreagent.Input{})
	if !errors.Is(err, karakuriquota.ErrBudgetExhausted) {
		t.Errorf("Run error = %v, want ErrBudgetExhausted", err)
	}
	if inner.calls != 0 {
		t.Error("the model was called despite an exhausted budget")
	}
}

func TestBudgetedAgentFailsOpenWhenTheCounterIsUnreadable(t *testing.T) {
	// A counter that cannot be read must not stop a loop that is otherwise
	// healthy — the same trade the HTTP limiter makes.
	inner := &greedyAgent{tokens: 10}
	budget := &recordingBudget{limit: 100, reserveErr: errors.New("store unreachable")}
	agent := withBudget(inner, budget, nil, "twin-1")

	if _, err := agent.Run(context.Background(), coreagent.Input{}); err != nil {
		t.Errorf("Run: %v", err)
	}
	if inner.calls != 1 {
		t.Error("the call did not go through")
	}
}

func TestBudgetedAgentDoesNotChargeFailedCalls(t *testing.T) {
	// Nothing reports what a failed call spent, so there is nothing honest to
	// charge for it.
	boom := errors.New("provider exploded")
	inner := &greedyAgent{tokens: 500, err: boom}
	budget := &recordingBudget{limit: 10000}
	agent := withBudget(inner, budget, nil, "twin-1")

	if _, err := agent.Run(context.Background(), coreagent.Input{}); !errors.Is(err, boom) {
		t.Fatalf("Run error = %v", err)
	}
	if len(budget.recorded) != 0 {
		t.Errorf("charged %v for a failed call", budget.recorded)
	}
}

func TestWithBudgetIsANoOpWithoutATwin(t *testing.T) {
	// An ad-hoc objective with no twin has no subject to charge. Metering it
	// against a shared bucket would be worse than not metering it.
	inner := &greedyAgent{tokens: 10}
	if got := withBudget(inner, &recordingBudget{limit: 1}, nil, ""); got != coreagent.Agent(inner) {
		t.Error("an objective with no twin was wrapped")
	}
	if got := withBudget(inner, nil, nil, "twin-1"); got != coreagent.Agent(inner) {
		t.Error("a nil budget produced a wrapper")
	}
}
