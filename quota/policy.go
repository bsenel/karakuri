// Package quota is a rate-limiting and quota engine for net/http and chi
// servers.
//
// It has no external dependencies and knows nothing about the application
// consuming it: keys are opaque strings produced by a caller-supplied
// KeyExtractor, and every limit is described by a Policy the caller composes.
//
// Three pieces fit together:
//
//   - A [Policy] says what the limit is — algorithm, ceiling, window.
//   - A [Backend] enforces it for one key, atomically, and returns a [Decision]
//     explaining the outcome.
//   - [Limit] turns those into middleware; [Quota] turns them into hard caps
//     over calendar periods.
//
// The in-memory backend ships here. Persistent and cross-replica backends are
// separate modules, so a caller pulls a database or a Valkey client only if
// they use one.
package quota

import (
	"fmt"
	"time"
)

// Algorithm selects how a Policy counts.
//
// The three differ in what they optimise for, and the choice is a real one:
//
//   - [TokenBucket] smooths bursts. It admits a burst up to Limit and then
//     settles to Rate, which is what an API client's retry loop expects.
//   - [FixedWindow] is the cheapest to store — one counter and a timestamp —
//     but allows up to 2×Limit across a window boundary, since a client can
//     spend a full window at the end of one and again at the start of the next.
//   - [SlidingLog] is exact over any window at the cost of remembering one
//     timestamp per request, so it suits low limits over long windows rather
//     than high-frequency traffic.
type Algorithm string

const (
	TokenBucket Algorithm = "token_bucket"
	FixedWindow Algorithm = "fixed_window"
	SlidingLog  Algorithm = "sliding_log"
)

// Policy describes one limit. It is a value: copy it, compare it, store it in a
// table beside your routes.
type Policy struct {
	Algorithm Algorithm

	// Limit is the ceiling. For TokenBucket it is the bucket's capacity — the
	// largest burst admitted from a standing start. For the window algorithms
	// it is the number of requests permitted per Window.
	Limit int

	// Window is the period the limit applies to. Required by every algorithm:
	// TokenBucket uses it to derive Rate, and it doubles as the lifetime of a
	// key's state, which is what lets a backend expire idle keys.
	Window time.Duration

	// Rate is the TokenBucket refill rate in units per second. Leave it zero to
	// derive Limit/Window, which is what "60 per minute, bursting to 60" means.
	// Set it explicitly to decouple the two — Limit 20 with Rate 1 is "one per
	// second, forgiving a burst of twenty".
	//
	// Ignored by the window algorithms.
	Rate float64
}

// Validate reports whether the policy is usable.
//
// Call it at startup, not per request. A policy that is wrong is wrong for the
// process's whole life, and a limiter that silently admits everything because
// its window is zero is worse than one that refuses to boot.
func (p Policy) Validate() error {
	switch p.Algorithm {
	case TokenBucket, FixedWindow, SlidingLog:
	case "":
		return fmt.Errorf("%w: algorithm is empty", ErrInvalidPolicy)
	default:
		return fmt.Errorf("%w: unknown algorithm %q", ErrInvalidPolicy, p.Algorithm)
	}
	if p.Limit <= 0 {
		return fmt.Errorf("%w: limit must be positive, got %d", ErrInvalidPolicy, p.Limit)
	}
	if p.Window <= 0 {
		return fmt.Errorf("%w: window must be positive, got %s", ErrInvalidPolicy, p.Window)
	}
	if p.Rate < 0 {
		return fmt.Errorf("%w: rate must not be negative, got %v", ErrInvalidPolicy, p.Rate)
	}
	if p.Rate > 0 && p.Algorithm != TokenBucket {
		return fmt.Errorf("%w: rate applies to %s only, not %s", ErrInvalidPolicy, TokenBucket, p.Algorithm)
	}
	return nil
}

// RatePerSecond is the rate a TokenBucket policy replenishes at: Rate when it
// is set, and Limit/Window when it is not.
//
// Exported for backends that have to reproduce the refill somewhere this
// package's arithmetic cannot reach — quota/valkey computes it inside a Lua
// script — so that "60 a minute" means the same number in both places.
func (p Policy) RatePerSecond() float64 {
	if p.Rate > 0 {
		return p.Rate
	}
	return float64(p.Limit) / p.Window.Seconds()
}

// PerMinute is shorthand for a token bucket of n per minute bursting to n.
func PerMinute(n int) Policy {
	return Policy{Algorithm: TokenBucket, Limit: n, Window: time.Minute}
}

// PerSecond is shorthand for a token bucket of n per second bursting to n.
func PerSecond(n int) Policy {
	return Policy{Algorithm: TokenBucket, Limit: n, Window: time.Second}
}

// Burst returns p with its burst capacity raised to n, keeping the refill rate
// p already implies. It is the readable way to say "60 a minute, but tolerate
// twenty arriving at once".
func (p Policy) Burst(n int) Policy {
	p.Rate = p.RatePerSecond()
	p.Limit = n
	return p
}
