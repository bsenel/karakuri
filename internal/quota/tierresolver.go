package quota

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bsenel/karakuri/quota"
)

// TierResolver answers what the limits are right now.
//
// It is the same shape as quota.Resolver and for the same reasons: the request
// tier is consulted on every API call, so a store read per request would make
// the limiter a second hot-path dependency and one that fails when that store
// is slow. The cache bounds how stale an edit can be; whoever writes calls
// Invalidate, which turns the TTL from a delay into an upper bound on how stale
// *another replica* can be.
//
// A nil *TierResolver resolves to configuration, so a deployment with no
// database never branches.
type TierResolver struct {
	configured Tiers
	store      TierStore
	ttl        time.Duration

	mu     sync.RWMutex
	cached []Tier
	readAt time.Time
	// failed records that the last read errored, so the log line about it is
	// written once per outage rather than once per request.
	failed bool
}

// NewTierResolver returns a resolver over a store. A nil store is valid and
// resolves everything to configuration.
func NewTierResolver(configured Tiers, store TierStore) *TierResolver {
	return &TierResolver{configured: configured, store: store, ttl: quota.DefaultCacheTTL}
}

// WithTTL sets how long a read is reused. A negative duration disables caching,
// which is what a test wanting determinism sets.
func (r *TierResolver) WithTTL(d time.Duration) *TierResolver {
	if r != nil {
		r.ttl = d
	}
	return r
}

// Configured returns the limits as configuration shipped them, which is what a
// stored tier is diffed against and what "reset" returns to.
func (r *TierResolver) Configured() Tiers {
	if r == nil {
		return Tiers{}
	}
	return r.configured
}

// Tiers returns the limits in force: configuration, with any stored tier
// replacing its ceiling.
func (r *TierResolver) Tiers(ctx context.Context) Tiers {
	if r == nil {
		return Tiers{}
	}
	out := r.configured
	for _, t := range r.stored(ctx) {
		switch t.Name {
		case TierRequest:
			out.Request = applyPolicy(t, out.Request)
		case TierCapability:
			out.Capability.Cap = t.Cap
		case TierLLMTokens:
			out.LLMTokens.Cap = t.Cap
		case TierAdapter:
			out.Adapter.Cap = t.Cap
		}
	}
	// A stored row that does not validate as a whole policy is dropped rather
	// than enforced. Writing goes through Tier.Validate, so this can only be a
	// row somebody edited in the database by hand — and the safe answer to that
	// is the limit the operator wrote in the file.
	if out.Validate() != nil {
		return r.configured
	}
	return out
}

// Stored returns the tiers an operator has set, for a UI that shows what is
// configured beside what is in force. It reads through the same cache.
func (r *TierResolver) Stored(ctx context.Context) []Tier {
	if r == nil {
		return nil
	}
	return r.stored(ctx)
}

// Invalidate drops the cache so the next read goes to the store. Whoever writes
// a tier calls it, which is what makes an edit visible in the writing process
// immediately rather than a TTL later.
func (r *TierResolver) Invalidate() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cached, r.readAt = nil, time.Time{}
}

func (r *TierResolver) stored(ctx context.Context) []Tier {
	if r.store == nil {
		return nil
	}
	now := time.Now()
	if r.ttl >= 0 {
		r.mu.RLock()
		cached, readAt := r.cached, r.readAt
		r.mu.RUnlock()
		if !readAt.IsZero() && now.Sub(readAt) < r.effectiveTTL() {
			return cached
		}
	}

	tiers, err := r.store.Tiers(ctx)
	if err != nil {
		// Fall back to configuration rather than to whatever was cached, and do
		// not cache the failure — the same trade quota.Resolver makes. A store
		// that cannot be read must not be able to raise a limit, and recovery
		// should be immediate rather than a TTL away.
		r.mu.Lock()
		alreadyLogged := r.failed
		r.failed = true
		r.mu.Unlock()
		if !alreadyLogged {
			slog.Error("stored quota tiers could not be read; configured limits apply", "err", err)
		}
		return nil
	}

	r.mu.Lock()
	r.cached, r.readAt, r.failed = tiers, now, false
	r.mu.Unlock()
	return tiers
}

func (r *TierResolver) effectiveTTL() time.Duration {
	if r.ttl == 0 {
		return quota.DefaultCacheTTL
	}
	return r.ttl
}

// applyPolicy replaces a rate limit's ceiling and window, keeping its algorithm.
func applyPolicy(t Tier, base quota.Policy) quota.Policy {
	base.Limit = t.Cap
	if t.Window > 0 {
		base.Window = t.Window
	}
	// Rate is set explicitly rather than conditionally: zero is a meaningful
	// value here — "derive the refill from cap and window" — and an operator
	// lowering a burst back to plain n-per-window has to be able to say so.
	base.Rate = t.Rate
	return base
}

// LogTierDivergence writes one line per tier whose stored value differs from
// the configured one.
//
// This exists because of what the database being authoritative costs: an
// operator reads llm_tokens_per_day in a YAML file and believes it. Without
// this line, the only way to discover the real limit is to ask the API. It runs
// at startup, where somebody is already reading output.
func LogTierDivergence(ctx context.Context, r *TierResolver) {
	if r == nil {
		return
	}
	stored := r.Stored(ctx)
	if len(stored) == 0 {
		return
	}
	configured := r.configured
	for _, t := range stored {
		var was int
		switch t.Name {
		case TierRequest:
			was = configured.Request.Limit
		case TierCapability:
			was = configured.Capability.Cap
		case TierLLMTokens:
			was = configured.LLMTokens.Cap
		case TierAdapter:
			was = configured.Adapter.Cap
		}
		if was == t.Cap {
			continue
		}
		slog.Warn("a stored limit overrides configuration",
			"tier", t.Name, "configured", was, "in_force", t.Cap,
			"set_by", t.UpdatedBy, "reason", t.Reason,
			"note", "the configuration file is the seed, not the limit; `krk quota unset` returns to it")
	}
}

// SetTier stores a limit and makes it visible to this process at once.
//
// The Invalidate is not an optimisation. An operator who raises a limit and
// watches `krk quota config` report the old one for another half-minute
// concludes the write failed and does it again — the same reasoning that put an
// Invalidate on the override path in Phase 18.
func (d Deps) SetTier(ctx context.Context, t Tier) error {
	if d.TierStore == nil {
		return errNoTierStore
	}
	if err := t.Validate(); err != nil {
		return err
	}
	// Refuse a stored tier that would not survive validation as a whole limit,
	// at write time rather than at read time. The alternative is a row that is
	// silently ignored on every request, which looks exactly like the write
	// having been lost.
	probe := d.TierSet.Configured()
	switch t.Name {
	case TierRequest:
		probe.Request = applyPolicy(t, probe.Request)
	case TierCapability:
		probe.Capability.Cap = t.Cap
	case TierLLMTokens:
		probe.LLMTokens.Cap = t.Cap
	case TierAdapter:
		probe.Adapter.Cap = t.Cap
	}
	if err := probe.Validate(); err != nil {
		return err
	}

	if err := d.TierStore.PutTier(ctx, t); err != nil {
		return err
	}
	d.TierSet.Invalidate()
	return nil
}

// ResetTier drops a stored limit, returning that tier to configuration.
func (d Deps) ResetTier(ctx context.Context, name string) error {
	if d.TierStore == nil {
		return errNoTierStore
	}
	if !storableTier[name] {
		return fmt.Errorf("unknown tier %q; one of %s", name, strings.Join(TierNames(), ", "))
	}
	if err := d.TierStore.DeleteTier(ctx, name); err != nil {
		return err
	}
	d.TierSet.Invalidate()
	return nil
}

// errNoTierStore is returned when there is nowhere to keep an edited limit. The
// API turns it into a 501: the request is fine and the deployment is what
// cannot honour it.
var errNoTierStore = errors.New("no tier store is configured, so an edited limit would not survive a restart")
