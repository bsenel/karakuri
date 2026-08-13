package quota

import (
	"context"
	"sync"
	"time"
)

// DefaultCacheTTL is how long a Resolver reuses what it read.
//
// Thirty seconds is a deliberate compromise between two costs. The request tier
// is consulted on every API call, so reading a store each time turns a limiter
// into a second database dependency on the hot path — and one that fails the
// moment that store is slow. At the other end, an operator who approves a raise
// and watches nothing happen for five minutes will approve it again.
//
// Half a minute means an approval is live well inside the minute, and a busy
// process reads the store twice a minute per subject instead of thousands of
// times.
const DefaultCacheTTL = 30 * time.Second

// Resolver applies overrides to the limits a caller configured.
//
// It sits between the configuration ("everybody gets 60 a minute") and the
// backend ("this key, this policy, right now"), and it exists because those two
// answer to different clocks: configuration changes at deploy time, an override
// changes when somebody approves it.
//
// A nil *Resolver resolves to the base limit, so a deployment with no override
// store never has to branch — call the methods on nil and get configuration
// back.
type Resolver struct {
	store OverrideStore

	// TTL is how long an entry is reused. Zero means DefaultCacheTTL; negative
	// disables caching entirely, which is what a test wanting determinism sets.
	ttl time.Duration

	// OnError, when set, is called when the store could not be read. The
	// resolution still returns the base limit — see resolve.
	onError func(subject Key, err error)

	mu     sync.RWMutex
	cached map[Key]cacheEntry
}

type cacheEntry struct {
	overrides []Override
	readAt    time.Time
}

// ResolverOption configures a Resolver.
type ResolverOption func(*Resolver)

// CacheTTL sets how long the resolver reuses a read. A negative duration
// disables caching, which costs a store read per resolution.
func CacheTTL(d time.Duration) ResolverOption {
	return func(r *Resolver) { r.ttl = d }
}

// OnResolveError reports a store that could not be read. Without it the failure
// is silent, and a silent failure here means limits quietly revert to
// configuration — worth a log line.
func OnResolveError(fn func(subject Key, err error)) ResolverOption {
	return func(r *Resolver) { r.onError = fn }
}

// NewResolver returns a resolver over a store. A nil store is valid and resolves
// everything to its base.
func NewResolver(store OverrideStore, opts ...ResolverOption) *Resolver {
	r := &Resolver{store: store, ttl: DefaultCacheTTL, cached: map[Key]cacheEntry{}}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Policy returns the policy in force for a subject: base, unless an override
// replaces its ceiling.
func (r *Resolver) Policy(ctx context.Context, subject Key, name string, base Policy, now time.Time) Policy {
	if o, ok := r.lookup(ctx, subject, name, now); ok {
		return o.Apply(base)
	}
	return base
}

// Quota returns the quota in force for a subject. The name is taken from the
// quota itself, which is the one limit in this package that carries one.
func (r *Resolver) Quota(ctx context.Context, subject Key, base Quota, now time.Time) Quota {
	if o, ok := r.lookup(ctx, subject, base.Name, now); ok {
		return o.ApplyQuota(base)
	}
	return base
}

// Invalidate drops a subject's cached overrides, so the next resolution reads
// through.
//
// Whoever writes an override should call it: within one process that turns the
// TTL from a delay into an upper bound on how stale another replica can be.
func (r *Resolver) Invalidate(subject Key) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cached, subject)
}

// lookup finds the active override for one named limit.
func (r *Resolver) lookup(ctx context.Context, subject Key, name string, now time.Time) (Override, bool) {
	if r == nil || r.store == nil || subject == "" || name == "" {
		return Override{}, false
	}
	for _, o := range r.overridesFor(ctx, subject, now) {
		if o.Name == name && o.Active(now) {
			return o, true
		}
	}
	return Override{}, false
}

func (r *Resolver) overridesFor(ctx context.Context, subject Key, now time.Time) []Override {
	if r.ttl >= 0 {
		r.mu.RLock()
		entry, ok := r.cached[subject]
		r.mu.RUnlock()
		if ok && now.Sub(entry.readAt) < r.effectiveTTL() {
			return entry.overrides
		}
	}

	overrides, err := r.store.Overrides(ctx, subject)
	if err != nil {
		if r.onError != nil {
			r.onError(subject, err)
		}
		// Fall back to the base limit rather than to whatever was cached, and
		// do not cache the failure. This is the same fail-open reasoning the
		// middleware uses: an override store that is down must not become an
		// outage, and configuration is the safe answer because it is what the
		// operator wrote. The one thing it must not do is *raise* a limit it
		// cannot verify.
		return nil
	}

	if r.ttl >= 0 {
		r.mu.Lock()
		r.cached[subject] = cacheEntry{overrides: overrides, readAt: now}
		r.mu.Unlock()
	}
	return overrides
}

func (r *Resolver) effectiveTTL() time.Duration {
	if r.ttl == 0 {
		return DefaultCacheTTL
	}
	return r.ttl
}
