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

type entry struct {
	state State

	// expiresAt is when this entry may be evicted, on the logical clock.
	expiresAt time.Time
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

// eval is the single implementation behind Take and Peek. The mutex is what
// makes this backend satisfy the contract's atomicity rule.
func (m *MemoryBackend) eval(key Key, p Policy, n int, now time.Time, commit bool) (Decision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if now.After(m.clock) {
		m.clock = now
	}
	m.sweep()

	// Work on a copy so a Peek — or a Take that turns out to be refused —
	// cannot leave a half-applied mutation behind.
	var work State
	if e := m.entries[key]; e != nil {
		work = e.state
		work.Log = append(work.Log[:0:0], e.state.Log...)
	}

	d, err := Apply(&work, p, n, now)
	if err != nil {
		return Decision{}, err
	}
	if !commit {
		if d.Allowed {
			// The unit was subtracted from a copy that is about to be
			// discarded, so hand it back before reporting.
			d.Remaining++
		}
		// Peek never stores. Recomputing an untouched budget is trivial, and a
		// usage endpoint that mints a counter for every key it is asked about
		// is a way to grow the map without ever taking a request.
		return d, nil
	}

	// A key is kept for two windows past its last use: one so an in-flight
	// window is never dropped mid-flight, one as slack for a caller whose clock
	// runs behind ours.
	m.entries[key] = &entry{state: work, expiresAt: now.Add(2 * p.Window)}
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
