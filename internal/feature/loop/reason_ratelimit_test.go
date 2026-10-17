package loop

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	"github.com/bsenel/karakuri/internal/core/loop"
)

// rateLimitAgent returns errs in order, then out.
type rateLimitAgent struct {
	errs  []error
	out   coreagent.Output
	calls int
}

func (f *rateLimitAgent) Run(context.Context, coreagent.Input) (coreagent.Output, error) {
	f.calls++
	if f.calls <= len(f.errs) {
		return coreagent.Output{}, f.errs[f.calls-1]
	}
	return f.out, nil
}

func (f *rateLimitAgent) Stream(context.Context, coreagent.Input) (<-chan coreagent.OutputChunk, error) {
	return nil, nil
}

func rateLimited() error {
	return &coreerrors.ProviderRateLimitError{Provider: "claude", RetryAfter: 2 * time.Second}
}

func recordWaits(sc *stepContext) *[]time.Duration {
	waits := &[]time.Duration{}
	sc.sleep = func(_ context.Context, d time.Duration) { *waits = append(*waits, d) }
	return waits
}

// A provider's refusal is not a plan: two rate limits end the iteration with
// the reason, and leave nothing for stepDecide to raise a checkpoint from.
func TestStepReasonEndsTheIterationWhenRateLimitedTwice(t *testing.T) {
	agent := &rateLimitAgent{errs: []error{rateLimited(), rateLimited()}, out: coreagent.Output{Content: goodPlan}}
	sc := scriptedReasonContext(t, agent)
	waits := recordWaits(sc)

	p := stepReason(context.Background(), sc, loop.WorldState{})

	if agent.calls != 2 {
		t.Errorf("made %d calls, want 2: one retry, not a loop", agent.calls)
	}
	if len(*waits) != 1 {
		t.Fatalf("waited %d times, want exactly once", len(*waits))
	}
	if w := (*waits)[0]; w <= 0 || w > 30*time.Second {
		t.Errorf("wait = %v, want a bounded positive wait", w)
	}
	for _, a := range p.Actions {
		if a.CapabilityID == "reason.plan" {
			t.Error("built a reason.plan placeholder from a rate limit")
		}
	}
	// With no actions there is nothing to approve; the placeholder at 0.3 is
	// what trips AuthorityBounds.Decide's confidence rule into a checkpoint.
	if len(p.Actions) != 0 {
		t.Errorf("plan has %d actions, want none", len(p.Actions))
	}
	if !strings.Contains(p.endReason, "claude") || !strings.Contains(p.endReason, "rate limit") {
		t.Errorf("endReason = %q, want it to name the provider and the limit", p.endReason)
	}
}

func TestStepReasonRetriesOnceAfterARateLimitAndPlans(t *testing.T) {
	agent := &rateLimitAgent{errs: []error{rateLimited()}, out: coreagent.Output{Content: goodPlan, Confidence: 0.8}}
	sc := scriptedReasonContext(t, agent)
	waits := recordWaits(sc)

	p := stepReason(context.Background(), sc, loop.WorldState{})

	if len(*waits) != 1 {
		t.Errorf("waited %d times, want exactly once", len(*waits))
	}
	if len(p.Actions) != 1 || p.Actions[0].CapabilityID != "software.act.shell_exec" {
		t.Fatalf("expected the normal plan after the retry, got %+v", p.Actions)
	}
	if p.endReason != "" {
		t.Errorf("endReason = %q, want none on a plan that landed", p.endReason)
	}
}

// Any other failure keeps today's behaviour: no wait, no retry, the
// reason.plan placeholder at confidence 0.3 holding the error text.
func TestStepReasonOtherErrorsStillBecomeThePlaceholder(t *testing.T) {
	agent := &rateLimitAgent{errs: []error{errors.New("connection reset"), errors.New("unused")}}
	sc := scriptedReasonContext(t, agent)
	waits := recordWaits(sc)

	p := stepReason(context.Background(), sc, loop.WorldState{})

	if agent.calls != 1 || len(*waits) != 0 {
		t.Errorf("calls = %d, waits = %d; want 1 and 0", agent.calls, len(*waits))
	}
	if len(p.Actions) != 1 || p.Actions[0].CapabilityID != "reason.plan" {
		t.Fatalf("expected the reason.plan placeholder, got %+v", p.Actions)
	}
	if p.Actions[0].Params["error"] != "connection reset" || p.Confidence != 0.3 {
		t.Errorf("placeholder = %+v at %v, want the error text at 0.3", p.Actions[0].Params, p.Confidence)
	}
	if p.Reasoning != "Agent call failed: connection reset" || p.endReason != "" {
		t.Errorf("reasoning = %q, endReason = %q", p.Reasoning, p.endReason)
	}
}
