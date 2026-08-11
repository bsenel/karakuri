package quota

import (
	"context"
	"testing"
	"time"
)

var base = time.Date(2026, 8, 10, 14, 37, 11, 0, time.UTC)

// The full behavioural contract runs from quota/quotatest against this backend.
// What is here is the memory-specific machinery that contract cannot see:
// eviction, the logical clock, and the guards on inputs a well-behaved caller
// never produces.

func TestMemoryEvictsIdleKeys(t *testing.T) {
	b := NewMemoryBackend()
	ctx := context.Background()
	p := Policy{Algorithm: FixedWindow, Limit: 1, Window: time.Minute}

	for i := range 100 {
		if _, err := b.Take(ctx, Key(string(rune('a'+i%26)))+Key(string(rune('0'+i/26))), p, 1, base); err != nil {
			t.Fatalf("Take: %v", err)
		}
	}
	if got := b.Len(); got != 100 {
		t.Fatalf("tracked %d keys, want 100", got)
	}

	// One key stays warm; the rest go quiet. An in-memory limiter that never
	// forgets is a memory leak with a nice API — one key per twin, forever.
	warm := Key("warm")
	for _, at := range []time.Time{base, base.Add(time.Hour), base.Add(2 * time.Hour)} {
		if _, err := b.Take(ctx, warm, p, 1, at); err != nil {
			t.Fatalf("Take(warm): %v", err)
		}
	}
	if got := b.Len(); got != 1 {
		t.Errorf("after two idle hours %d keys remain, want only the warm one", got)
	}
}

func TestMemoryEvictionUsesTheLogicalClock(t *testing.T) {
	// Sweeping against wall time would wipe a backend the moment a test drove
	// a fake clock anywhere far from now, and would let a backend nobody is
	// calling decide its data is stale.
	b := NewMemoryBackend()
	ctx := context.Background()
	p := Policy{Algorithm: FixedWindow, Limit: 5, Window: time.Minute}

	long := time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := b.Take(ctx, "k", p, 1, long); err != nil {
		t.Fatalf("Take: %v", err)
	}
	if got := b.Len(); got != 1 {
		t.Fatalf("key from 1990 evicted against wall time: %d keys", got)
	}
	// A second call at the same logical instant must not sweep it either.
	if _, err := b.Take(ctx, "k", p, 1, long); err != nil {
		t.Fatalf("Take: %v", err)
	}
	if got := b.Len(); got != 1 {
		t.Errorf("%d keys, want 1", got)
	}
}

func TestMemoryClockRunningBackwardsDoesNotDrainTheBucket(t *testing.T) {
	// NTP steps, VM migrations and a caller passing a stale timestamp all look
	// the same from in here. None of them should cost anyone their budget.
	b := NewMemoryBackend()
	ctx := context.Background()
	p := Policy{Algorithm: TokenBucket, Limit: 10, Window: time.Minute, Rate: 1}

	if _, err := b.Take(ctx, "k", p, 1, base.Add(time.Hour)); err != nil {
		t.Fatalf("Take: %v", err)
	}
	d, err := b.Take(ctx, "k", p, 1, base) // an hour earlier
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if !d.Allowed {
		t.Error("a backwards clock refused a request")
	}
	if d.Remaining != 8 {
		t.Errorf("remaining = %d, want 8 — the bucket should be unchanged by the jump", d.Remaining)
	}
}

func TestMemoryKeyReusedUnderADifferentAlgorithm(t *testing.T) {
	// Not something a sane caller does, but the state shapes are not
	// interchangeable and reading a token count as a window counter would be a
	// silent wrong answer rather than a loud one.
	b := NewMemoryBackend()
	ctx := context.Background()

	bucket := Policy{Algorithm: TokenBucket, Limit: 2, Window: time.Minute, Rate: 1}
	window := Policy{Algorithm: FixedWindow, Limit: 2, Window: time.Minute}

	for range 2 {
		if _, err := b.Take(ctx, "k", bucket, 1, base); err != nil {
			t.Fatalf("Take: %v", err)
		}
	}
	d, err := b.Take(ctx, "k", window, 1, base)
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if !d.Allowed || d.Remaining != 1 {
		t.Errorf("switching algorithm gave allowed=%t remaining=%d, want a fresh counter", d.Allowed, d.Remaining)
	}
}

func TestMemoryPeekDoesNotCreateEntries(t *testing.T) {
	// A usage endpoint that mints a counter for every key it is asked about is
	// a way to grow the map without ever taking a request.
	b := NewMemoryBackend()
	p := Policy{Algorithm: FixedWindow, Limit: 5, Window: time.Minute}
	for i := range 50 {
		if _, err := b.Peek(context.Background(), Key(rune(i)), p, base); err != nil {
			t.Fatalf("Peek: %v", err)
		}
	}
	if got := b.Len(); got != 0 {
		t.Errorf("Peek created %d entries, want 0", got)
	}
}

func TestMemoryNegativeCostIsTreatedAsZero(t *testing.T) {
	b := NewMemoryBackend()
	p := Policy{Algorithm: FixedWindow, Limit: 1, Window: time.Minute}
	d, err := b.Take(context.Background(), "k", p, -5, base)
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	// Refunding budget through a negative cost would be a way to mint requests.
	if !d.Allowed || d.Remaining != 1 {
		t.Errorf("allowed=%t remaining=%d, want a no-op", d.Allowed, d.Remaining)
	}
}

func TestMemorySlidingLogCostBeyondTheLimit(t *testing.T) {
	// No amount of waiting makes room for a single call bigger than the whole
	// budget, so the wait has to be something other than zero or the caller
	// spins.
	b := NewMemoryBackend()
	p := Policy{Algorithm: SlidingLog, Limit: 3, Window: time.Minute}
	d, err := b.Take(context.Background(), "k", p, 10, base)
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if d.Allowed {
		t.Fatal("a request larger than the limit was allowed")
	}
	if d.RetryAfter != p.Window {
		t.Errorf("retry-after = %s, want the full window", d.RetryAfter)
	}
}

func TestMemorySlidingLogResetAtWithNoHistory(t *testing.T) {
	b := NewMemoryBackend()
	p := Policy{Algorithm: SlidingLog, Limit: 3, Window: time.Minute}
	d, err := b.Peek(context.Background(), "cold", p, base)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if !d.ResetAt.Equal(base) {
		t.Errorf("ResetAt = %s on an empty log, want now (%s)", d.ResetAt, base)
	}
}

func TestMemoryRejectsInvalidPolicyOnPeek(t *testing.T) {
	b := NewMemoryBackend()
	if _, err := b.Peek(context.Background(), "k", Policy{}, base); err == nil {
		t.Error("Peek accepted an empty policy")
	}
}
