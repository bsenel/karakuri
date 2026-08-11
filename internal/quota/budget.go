package quota

import (
	"context"
	"errors"
	"time"

	"github.com/bsenel/karakuri/quota"
)

// ErrBudgetExhausted is returned by a TokenBudget when a twin has spent its
// allowance for the period.
//
// It is a sentinel rather than a plain error because the loop treats it
// differently from a failure: exhaustion pauses on a checkpoint a human can
// approve, while a failure ends the iteration.
var ErrBudgetExhausted = errors.New("quota: llm token budget exhausted")

// TokenBudget bounds a twin's model spend.
//
// It is an interface with two implementations. The native one counts tokens
// through this module; the LiteLLM one delegates to a gateway that counts
// dollars. The loop knows about neither — it reserves, records and reports, and
// gets ErrBudgetExhausted either way.
type TokenBudget interface {
	// Reserve reports whether a call estimated at `estimate` tokens may
	// proceed. Returning ErrBudgetExhausted means the budget is spent; any
	// other error means the budget could not be consulted.
	Reserve(ctx context.Context, twinID string, estimate int, now time.Time) error

	// Record charges what a completed call actually used. It is separate from
	// Reserve because the estimate is always wrong: a provider reports its real
	// usage only after answering, and charging the estimate would drift in
	// whichever direction the estimate is biased.
	Record(ctx context.Context, twinID string, tokens int, now time.Time) error

	// Usage reports the current period without spending any of it.
	Usage(ctx context.Context, twinID string, now time.Time) (quota.Decision, error)
}

// NativeBudget counts tokens against the LLMTokens tier.
type NativeBudget struct {
	deps Deps
}

var _ TokenBudget = (*NativeBudget)(nil)

// Budget returns the native token budget over these deps.
func (d Deps) Budget() *NativeBudget { return &NativeBudget{deps: d} }

// Reserve charges nothing. It asks whether there is room left, which is the
// question that can be answered before a call — the answer to "how much will
// this cost" arrives with the response.
//
// The consequence is that a single call can overshoot the cap by whatever it
// happens to use. That is the right trade for a token budget: the alternative
// is charging an estimate up front and reconciling afterwards, which means
// either refusing calls that would have fit or holding a reservation across a
// request that may never return.
func (b *NativeBudget) Reserve(ctx context.Context, twinID string, _ int, now time.Time) error {
	dec, err := b.deps.Tiers.LLMTokens.Peek(ctx, b.deps.Backend, TwinKey(twinID), now)
	if err != nil {
		return err
	}
	if !dec.Allowed {
		return ErrBudgetExhausted
	}
	return nil
}

// Record charges actual usage, and publishes pressure once the twin is most of
// the way through its day.
func (b *NativeBudget) Record(ctx context.Context, twinID string, tokens int, now time.Time) error {
	if tokens <= 0 {
		// Providers that cannot report usage send zero — the CLI-based Gemini
		// path documents exactly that. Charging nothing is the honest reading;
		// inventing an estimate would make the budget a fiction.
		return nil
	}
	dec, err := b.deps.Tiers.LLMTokens.Take(ctx, b.deps.Backend, TwinKey(twinID), tokens, now)
	if err != nil {
		return err
	}
	if dec.Used() >= PressureThreshold {
		b.deps.publishPressure(ctx, TwinKey(twinID), "llm_tokens", dec)
	}
	return nil
}

func (b *NativeBudget) Usage(ctx context.Context, twinID string, now time.Time) (quota.Decision, error) {
	return b.deps.Tiers.LLMTokens.Peek(ctx, b.deps.Backend, TwinKey(twinID), now)
}

// UnlimitedBudget is the no-op used where no budget is configured, so callers
// never have to nil-check.
type UnlimitedBudget struct{}

var _ TokenBudget = UnlimitedBudget{}

func (UnlimitedBudget) Reserve(context.Context, string, int, time.Time) error { return nil }
func (UnlimitedBudget) Record(context.Context, string, int, time.Time) error  { return nil }
func (UnlimitedBudget) Usage(context.Context, string, time.Time) (quota.Decision, error) {
	return quota.Decision{Allowed: true}, nil
}
