package quota

import (
	"context"
	"sync"
	"time"
)

// MemoryBackend keeps every counter in this process.
//
// It is the reference implementation — the one the shared contract suite
// defines correct behaviour against — and the right choice for a single-replica
// deployment. It is the wrong choice behind a load balancer: each replica keeps
// its own counters, so a limit of 60/min across three replicas admits 180.
//
// Zero value is not usable; call [NewMemoryBackend].
type MemoryBackend struct {
	mu      sync.Mutex
	entries map[Key]*entry

	// clock is logical, not wall: it tracks the largest now any caller has
	// passed in. Eviction is measured against it so that a test driving a fake
	// clock evicts on that clock, and so a backend nobody is calling never
	// decides its data is stale.
	clock     time.Time
	lastSweep time.Time
}

// sweepEvery is how much logical time passes between eviction sweeps. Sweeping
// happens inside Take, under the lock it already holds, rather than from a
// background goroutine — a library that starts a goroutine you cannot see is a
// library you have to remember to shut down.
const sweepEvery = time.Minute

// logEntry is one recorded consumption for SlidingLog. Costs are folded into a
// count rather than repeated, so a single expensive call does not allocate a
// timestamp per unit.
type logEntry struct {
	at time.Time
	n  int
}

type entry struct {
	// algorithm guards against a key being reused under a different policy —
	// the state shapes are not interchangeable, so the entry starts over.
	algorithm Algorithm

	// expiresAt is when this entry may be evicted, on the logical clock.
	expiresAt time.Time

	// TokenBucket
	tokens float64
	last   time.Time

	// FixedWindow
	windowStart time.Time
	count       int

	// SlidingLog
	log []logEntry
}

// NewMemoryBackend returns an empty in-process backend.
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{entries: make(map[Key]*entry)}
}

var _ Backend = (*MemoryBackend)(nil)

// Len reports how many keys are currently tracked. Exported for tests and for
// exposing a gauge — an unbounded key space is the failure mode of any
// in-memory limiter, and this is how you watch for it.
func (m *MemoryBackend) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}

func (m *MemoryBackend) Take(_ context.Context, key Key, p Policy, n int, now time.Time) (Decision, error) {
	return m.eval(key, p, n, now, true)
}

func (m *MemoryBackend) Peek(_ context.Context, key Key, p Policy, now time.Time) (Decision, error) {
	// Costed at one, not zero: "is there room for another request" is the
	// question worth answering. Asking whether a zero-cost request fits is
	// always yes, even against an exhausted budget.
	return m.eval(key, p, 1, now, false)
}

func (m *MemoryBackend) Reset(_ context.Context, key Key) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, key)
	return nil
}

// eval is the single implementation behind Take and Peek. commit=false leaves
// the entry untouched, which is what keeps a usage endpoint from spending the
// budget it is reporting on.
func (m *MemoryBackend) eval(key Key, p Policy, n int, now time.Time, commit bool) (Decision, error) {
	if err := p.Validate(); err != nil {
		return Decision{}, err
	}
	if n < 0 {
		n = 0
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if now.After(m.clock) {
		m.clock = now
	}
	m.sweep()

	e := m.entries[key]
	if e == nil || e.algorithm != p.Algorithm {
		e = &entry{algorithm: p.Algorithm}
	}

	// Work on a copy so a refused Peek — or a Take that turns out to be
	// refused — cannot leave a half-applied mutation behind.
	work := *e
	work.log = append(work.log[:0:0], e.log...)

	var d Decision
	switch p.Algorithm {
	case TokenBucket:
		d = takeTokenBucket(&work, p, n, now)
	case FixedWindow:
		d = takeFixedWindow(&work, p, n, now)
	case SlidingLog:
		d = takeSlidingLog(&work, p, n, now)
	}

	if !commit && d.Allowed {
		// The unit was subtracted from a copy that is about to be discarded, so
		// hand it back before reporting.
		d.Remaining++
	}
	d = d.Normalize()

	// A key is kept for two windows past its last use: one so an in-flight
	// window is never dropped mid-flight, one as slack for a caller whose clock
	// runs behind ours.
	work.expiresAt = now.Add(2 * p.Window)
	if commit {
		// Peek never stores. Recomputing an untouched budget is trivial, and a
		// usage endpoint that mints a counter for every key it is asked about is
		// a way to grow the map without ever taking a request.
		m.entries[key] = &work
	}
	return d, nil
}

// sweep drops entries nothing has touched for two windows. Amortised: it does
// real work at most once per sweepEvery of logical time.
func (m *MemoryBackend) sweep() {
	if m.clock.Sub(m.lastSweep) < sweepEvery {
		return
	}
	m.lastSweep = m.clock
	for k, e := range m.entries {
		if e.expiresAt.Before(m.clock) {
			delete(m.entries, k)
		}
	}
}

func takeTokenBucket(e *entry, p Policy, n int, now time.Time) Decision {
	rate := p.refillRate()
	capacity := float64(p.Limit)

	if e.last.IsZero() {
		e.tokens = capacity
		e.last = now
	}
	// Only ever refill forward. A clock that jumps backwards must not drain the
	// bucket, and must not be recorded as the new baseline either.
	if elapsed := now.Sub(e.last); elapsed > 0 {
		e.tokens = min(capacity, e.tokens+elapsed.Seconds()*rate)
		e.last = now
	}

	d := Decision{Limit: p.Limit}
	// tokenEpsilon absorbs floating-point rounding. Refilling for exactly the
	// time one token takes can land a hair under it, which would refuse a
	// request that arrived precisely on schedule. A nanotoken of slack costs
	// nothing real and makes the boundary behave the way the arithmetic says
	// it should.
	const tokenEpsilon = 1e-9
	if e.tokens+tokenEpsilon >= float64(n) {
		e.tokens -= float64(n)
		d.Allowed = true
	} else {
		d.RetryAfter = seconds((float64(n) - e.tokens) / rate)
	}
	d.Remaining = int(e.tokens)
	d.ResetAt = now.Add(seconds((capacity - e.tokens) / rate))
	return d
}

func takeFixedWindow(e *entry, p Policy, n int, now time.Time) Decision {
	// Windows are anchored to the epoch rather than to first use, so every
	// replica and every restart agrees on where the boundary falls. A 24h
	// window lands on midnight UTC because the zero time is midnight UTC.
	start := now.Truncate(p.Window)
	if !e.windowStart.Equal(start) {
		e.windowStart, e.count = start, 0
	}
	end := start.Add(p.Window)

	d := Decision{Limit: p.Limit, ResetAt: end}
	if e.count+n <= p.Limit {
		e.count += n
		d.Allowed = true
	} else {
		d.RetryAfter = end.Sub(now)
	}
	d.Remaining = max(p.Limit-e.count, 0)
	return d
}

func takeSlidingLog(e *entry, p Policy, n int, now time.Time) Decision {
	cutoff := now.Add(-p.Window)
	kept := e.log[:0]
	used := 0
	for _, l := range e.log {
		if l.at.After(cutoff) {
			kept = append(kept, l)
			used += l.n
		}
	}
	e.log = kept

	d := Decision{Limit: p.Limit, Remaining: max(p.Limit-used, 0)}
	if len(e.log) > 0 {
		d.ResetAt = e.log[0].at.Add(p.Window)
	} else {
		d.ResetAt = now
	}

	if used+n <= p.Limit {
		if n > 0 {
			e.log = append(e.log, logEntry{at: now, n: n})
			d.Remaining = max(p.Limit-(used+n), 0)
		}
		d.Allowed = true
		return d
	}

	// Refused: wait until enough of the oldest entries have aged out to make
	// room for n. Walking from the oldest is what makes this the *earliest*
	// time the request would succeed rather than a conservative guess.
	need := used + n - p.Limit
	freed := 0
	for _, l := range e.log {
		freed += l.n
		if freed >= need {
			d.RetryAfter = l.at.Add(p.Window).Sub(now)
			break
		}
	}
	if d.RetryAfter <= 0 {
		// n alone exceeds the limit, so no amount of waiting helps. Report the
		// full window rather than zero, which would invite an instant retry.
		d.RetryAfter = p.Window
	}
	return d
}

// seconds converts a float number of seconds into a Duration, clamping the
// negative and absurd cases that arithmetic on a caller-supplied rate can
// produce — a Duration is nanoseconds in an int64, so a large enough float
// silently wraps to a negative wait.
func seconds(s float64) time.Duration {
	if s <= 0 || s != s { // NaN compares false against itself
		return 0
	}
	// A Duration tops out at about 292 years. Clamping in *seconds* has to
	// stay well under that, because the multiply below is what overflows.
	const maxSeconds = float64(1 << 33) // ~272 years
	return time.Duration(min(s, maxSeconds) * float64(time.Second))
}
