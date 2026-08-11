// Package quotatest holds the behavioural contract every quota.Backend must
// satisfy.
//
// It exists so the in-memory, SQL and Valkey backends cannot silently diverge:
// each runs the identical table, so a difference shows up as a failing test in
// the implementation that is wrong rather than as a production surprise months
// later. A new backend is finished when [Run] passes against it.
//
// Time is supplied to every case rather than slept through, so the suite is
// deterministic and takes milliseconds.
package quotatest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/quota"
)

// Base is the instant every case starts from. It is deliberately not a round
// number and not midnight: a backend that only works when the clock happens to
// sit on a window boundary is broken, and this is what catches it.
var Base = time.Date(2026, 8, 10, 14, 37, 11, 0, time.UTC)

// NewBackend builds a fresh, empty backend. Each case gets its own, so state
// cannot leak between them.
type NewBackend func(t *testing.T) quota.Backend

// Run executes the full contract. Call it from each backend's package.
func Run(t *testing.T, newBackend NewBackend) {
	t.Helper()
	cases := []struct {
		name string
		fn   func(*testing.T, quota.Backend)
	}{
		{"fixed window admits exactly the limit", fixedWindowLimit},
		{"fixed window resets on the boundary", fixedWindowReset},
		{"token bucket bursts then refills", tokenBucketRefill},
		{"sliding log is exact across the window", slidingLogExact},
		{"a refusal consumes nothing", refusalConsumesNothing},
		{"peek does not consume", peekDoesNotConsume},
		{"reset restores the full budget", resetRestores},
		{"cost above one is honoured", costAboveOne},
		{"keys are independent", keysAreIndependent},
		{"exhaustion is a decision, not an error", exhaustionIsNotAnError},
		{"an invalid policy is rejected", invalidPolicyRejected},
		{"concurrent takes never oversubscribe", concurrentTakesAreAtomic},
		{"a token bucket never outruns its rate", tokenBucketNeverOutrunsItsRate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.fn(t, newBackend(t))
		})
	}
}

func fixedWindowLimit(t *testing.T, b quota.Backend) {
	p := quota.Policy{Algorithm: quota.FixedWindow, Limit: 60, Window: time.Minute}
	allowed := 0
	for i := range 100 {
		d := take(t, b, "k", p, 1, Base.Add(time.Duration(i)*time.Millisecond))
		if d.Allowed {
			allowed++
		}
	}
	// The roadmap's acceptance case: 100 requests against 60/min is 60 through
	// and 40 refused, not 59 and not 61.
	if allowed != 60 {
		t.Errorf("allowed %d of 100 against a limit of 60", allowed)
	}
}

func fixedWindowReset(t *testing.T, b quota.Backend) {
	p := quota.Policy{Algorithm: quota.FixedWindow, Limit: 2, Window: time.Minute}
	for range 2 {
		mustAllow(t, take(t, b, "k", p, 1, Base))
	}
	mustRefuse(t, take(t, b, "k", p, 1, Base))

	// Windows are anchored to the clock, not to first use, so crossing the next
	// boundary is what refills — every replica agrees on where that falls.
	next := Base.Truncate(time.Minute).Add(time.Minute)
	d := take(t, b, "k", p, 1, next)
	mustAllow(t, d)
	if d.Remaining != 1 {
		t.Errorf("remaining after reset = %d, want 1", d.Remaining)
	}
}

func tokenBucketRefill(t *testing.T, b quota.Backend) {
	// 60/min bursting to 10: one token a second, ten in hand.
	p := quota.Policy{Algorithm: quota.TokenBucket, Limit: 10, Window: time.Minute, Rate: 1}

	for i := range 10 {
		mustAllow(t, take(t, b, "k", p, 1, Base.Add(time.Duration(i)*time.Millisecond)))
	}
	d := take(t, b, "k", p, 1, Base)
	mustRefuse(t, d)
	if d.RetryAfter <= 0 || d.RetryAfter > 2*time.Second {
		t.Errorf("retry-after = %s, want about a second", d.RetryAfter)
	}

	// One second buys exactly one token, not two.
	mustAllow(t, take(t, b, "k", p, 1, Base.Add(time.Second)))
	mustRefuse(t, take(t, b, "k", p, 1, Base.Add(time.Second)))

	// Idling long past capacity refills to the brim and no further.
	allowed := 0
	for range 20 {
		if take(t, b, "k", p, 1, Base.Add(time.Hour)).Allowed {
			allowed++
		}
	}
	if allowed != 10 {
		t.Errorf("after a long idle, burst was %d, want the capacity of 10", allowed)
	}
}

func slidingLogExact(t *testing.T, b quota.Backend) {
	p := quota.Policy{Algorithm: quota.SlidingLog, Limit: 3, Window: time.Minute}
	for i := range 3 {
		mustAllow(t, take(t, b, "k", p, 1, Base.Add(time.Duration(i)*time.Second)))
	}
	mustRefuse(t, take(t, b, "k", p, 1, Base.Add(3*time.Second)))

	// This is what separates a sliding log from a fixed window: the budget
	// returns one entry at a time as each ages out, rather than all at once on
	// a boundary. At Base+60.5s the first entry has expired and the other two
	// have not, so there is room for exactly one more.
	//
	// Half a second off the boundary on purpose: whether an entry exactly
	// Window old still counts as inside is not something the contract pins
	// down, and a case that depended on it would fail a backend that is merely
	// making the other reasonable choice.
	at := Base.Add(60500 * time.Millisecond)
	mustAllow(t, take(t, b, "k", p, 1, at))
	mustRefuse(t, take(t, b, "k", p, 1, at))
}

func refusalConsumesNothing(t *testing.T, b quota.Backend) {
	p := quota.Policy{Algorithm: quota.FixedWindow, Limit: 1, Window: time.Minute}
	mustAllow(t, take(t, b, "k", p, 1, Base))

	// Hammering a limiter must not push the reset further out, or a client in a
	// retry loop can never recover.
	var last quota.Decision
	for i := range 50 {
		last = take(t, b, "k", p, 1, Base.Add(time.Duration(i)*time.Millisecond))
		mustRefuse(t, last)
	}
	if last.Remaining != 0 {
		t.Errorf("remaining = %d after refusals, want 0", last.Remaining)
	}
	next := Base.Truncate(time.Minute).Add(time.Minute)
	mustAllow(t, take(t, b, "k", p, 1, next))
}

func peekDoesNotConsume(t *testing.T, b quota.Backend) {
	p := quota.Policy{Algorithm: quota.FixedWindow, Limit: 2, Window: time.Minute}
	for range 5 {
		d, err := b.Peek(context.Background(), "k", p, Base)
		if err != nil {
			t.Fatalf("Peek: %v", err)
		}
		if !d.Allowed || d.Remaining != 2 {
			t.Fatalf("Peek reported allowed=%t remaining=%d, want true/2", d.Allowed, d.Remaining)
		}
	}
	// A usage endpoint must not spend the budget it reports on.
	mustAllow(t, take(t, b, "k", p, 1, Base))
	mustAllow(t, take(t, b, "k", p, 1, Base))
	mustRefuse(t, take(t, b, "k", p, 1, Base))

	// Peek's Allowed means "would one more get through", not "would a
	// zero-cost request get through" — the latter is always yes and tells a
	// usage endpoint nothing.
	d, err := b.Peek(context.Background(), "k", p, Base)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if d.Allowed {
		t.Error("Peek reports room in an exhausted budget")
	}
	if d.Remaining != 0 {
		t.Errorf("Peek remaining = %d on an exhausted budget, want 0", d.Remaining)
	}
}

func resetRestores(t *testing.T, b quota.Backend) {
	p := quota.Policy{Algorithm: quota.FixedWindow, Limit: 1, Window: time.Minute}
	mustAllow(t, take(t, b, "k", p, 1, Base))
	mustRefuse(t, take(t, b, "k", p, 1, Base))

	if err := b.Reset(context.Background(), "k"); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	mustAllow(t, take(t, b, "k", p, 1, Base))

	// Resetting a key that was never used is a no-op, not a failure — an admin
	// override should not need to know whether there was anything to clear.
	if err := b.Reset(context.Background(), "never-seen"); err != nil {
		t.Errorf("Reset of an unknown key: %v", err)
	}
}

func costAboveOne(t *testing.T, b quota.Backend) {
	p := quota.Policy{Algorithm: quota.FixedWindow, Limit: 100, Window: time.Minute}
	d := take(t, b, "k", p, 30, Base)
	mustAllow(t, d)
	if d.Remaining != 70 {
		t.Errorf("remaining = %d after taking 30 of 100, want 70", d.Remaining)
	}
	mustAllow(t, take(t, b, "k", p, 70, Base))
	mustRefuse(t, take(t, b, "k", p, 1, Base))

	// A single call larger than the whole budget is refused rather than
	// partially applied.
	fresh := quota.Policy{Algorithm: quota.FixedWindow, Limit: 10, Window: time.Minute}
	mustRefuse(t, take(t, b, "big", fresh, 11, Base))
	mustAllow(t, take(t, b, "big", fresh, 10, Base))
}

func keysAreIndependent(t *testing.T, b quota.Backend) {
	p := quota.Policy{Algorithm: quota.FixedWindow, Limit: 1, Window: time.Minute}
	mustAllow(t, take(t, b, "alice", p, 1, Base))
	mustRefuse(t, take(t, b, "alice", p, 1, Base))
	mustAllow(t, take(t, b, "bob", p, 1, Base))
}

func exhaustionIsNotAnError(t *testing.T, b quota.Backend) {
	p := quota.Policy{Algorithm: quota.FixedWindow, Limit: 1, Window: time.Minute}
	mustAllow(t, take(t, b, "k", p, 1, Base))
	// Errors mean "I could not find out". A full bucket is an answer.
	if _, err := b.Take(context.Background(), "k", p, 1, Base); err != nil {
		t.Errorf("exhaustion surfaced as an error: %v", err)
	}
}

func invalidPolicyRejected(t *testing.T, b quota.Backend) {
	for _, p := range []quota.Policy{
		{Algorithm: quota.FixedWindow, Limit: 0, Window: time.Minute},
		{Algorithm: quota.FixedWindow, Limit: 1},
		{Algorithm: "nonsense", Limit: 1, Window: time.Minute},
	} {
		if _, err := b.Take(context.Background(), "k", p, 1, Base); !errors.Is(err, quota.ErrInvalidPolicy) {
			t.Errorf("Take(%+v) error = %v, want ErrInvalidPolicy", p, err)
		}
	}
}

func concurrentTakesAreAtomic(t *testing.T, b quota.Backend) {
	// The property the whole Backend interface exists to guarantee. A
	// check-then-write implementation passes every other case in this file and
	// fails this one.
	const limit, racers = 50, 200
	p := quota.Policy{Algorithm: quota.FixedWindow, Limit: limit, Window: time.Hour}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		allowed int
	)
	start := make(chan struct{})
	for range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			d, err := b.Take(context.Background(), "contended", p, 1, Base)
			if err != nil {
				return
			}
			if d.Allowed {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if allowed != limit {
		t.Errorf("%d of %d racers allowed against a limit of %d — the backend is not atomic per key",
			allowed, racers, limit)
	}
}

func tokenBucketNeverOutrunsItsRate(t *testing.T, b quota.Backend) {
	// The defining property of a token bucket: over any interval starting from
	// a full bucket, no more than rate×elapsed + burst can get through. That
	// bound is what an operator is actually buying, and it is the thing a
	// plausible-looking refill bug quietly breaks.
	//
	// Driven by a deterministic pseudo-random walk rather than a fixed cadence,
	// because a bug that only shows up under irregular arrival times is exactly
	// the bug a fixed cadence misses. The seed is fixed so a failure reproduces.
	const (
		burst = 10
		rate  = 4 // per second
		steps = 3000
	)
	p := quota.Policy{Algorithm: quota.TokenBucket, Limit: burst, Window: time.Second, Rate: rate}

	seed := uint64(0x9E3779B97F4A7C15)
	next := func(n uint64) uint64 {
		// xorshift64*: a few lines, no dependency, and good enough to produce
		// irregular gaps.
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		return seed % n
	}

	now, allowed := Base, 0
	for range steps {
		now = now.Add(time.Duration(next(400)) * time.Millisecond)
		if take(t, b, "walk", p, 1, now).Allowed {
			allowed++
		}
		elapsed := now.Sub(Base).Seconds()
		if ceiling := int(rate*elapsed) + burst; allowed > ceiling {
			t.Fatalf("after %.3fs the bucket admitted %d, above the rate×elapsed+burst ceiling of %d",
				elapsed, allowed, ceiling)
		}
	}

	// The bound is only interesting if it is tight — a backend that refused
	// everything would satisfy it too.
	if elapsed := now.Sub(Base).Seconds(); float64(allowed) < rate*elapsed*0.5 {
		t.Errorf("admitted %d over %.1fs at %d/s — far under the rate, so the bound proves nothing",
			allowed, elapsed, rate)
	}
}

func take(t *testing.T, b quota.Backend, key quota.Key, p quota.Policy, n int, now time.Time) quota.Decision {
	t.Helper()
	d, err := b.Take(context.Background(), key, p, n, now)
	if err != nil {
		t.Fatalf("Take(%s, %d): %v", key, n, err)
	}
	return d
}

func mustAllow(t *testing.T, d quota.Decision) {
	t.Helper()
	if !d.Allowed {
		t.Fatalf("refused, want allowed (remaining=%d retry_after=%s)", d.Remaining, d.RetryAfter)
	}
	if d.RetryAfter != 0 {
		t.Errorf("allowed decision carries retry-after %s, want 0", d.RetryAfter)
	}
}

func mustRefuse(t *testing.T, d quota.Decision) {
	t.Helper()
	if d.Allowed {
		t.Fatalf("allowed, want refused (remaining=%d)", d.Remaining)
	}
	if d.RetryAfter <= 0 {
		t.Errorf("refused decision carries retry-after %s, want a positive wait", d.RetryAfter)
	}
}
