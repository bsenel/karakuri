package quota

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/quota"
)

// Deps is everything the API needs to enforce quotas, built by
// internal/app.BuildQuota.
type Deps struct {
	Backend quota.Backend
	Tiers   Tiers

	// Hub receives quota_pressure events. Optional: without it, pressure is
	// logged and nothing else.
	Hub *event.Hub

	// TokenBudget bounds model spend. It is the native implementation unless
	// a gateway is configured; never nil, so callers do not have to check.
	TokenBudget TokenBudget

	// Close releases whatever the backend holds — a Valkey pool, say. Never
	// nil, so callers do not have to check.
	Close func() error
}

// RequestKey is the key the request-rate tier counts against.
//
// It is the principal, always. The plan for this phase said to key twin routes
// on the twin, and that is wrong: a caller could then spend a full budget
// against every twin it can see, so the limit would bound nothing. Whoever is
// making requests is who you throttle.
//
// The twin dimension is not lost — it belongs to the per-capability quota,
// which is genuinely per twin because it bounds a twin's work rather than a
// caller's traffic.
//
// An empty key exempts the request. That happens only before authentication
// has run, which for the routes this wraps means it cannot happen at all.
func RequestKey(r *http.Request) quota.Key {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.ID == "" {
		return ""
	}
	return quota.JoinKey("principal", p.ID)
}

// CapabilityKey is the subject of the per-capability daily quota.
func CapabilityKey(twinID, capability string) quota.Key {
	return quota.JoinKey("twin", twinID, capability)
}

// TwinKey is the subject of the per-twin quotas — the LLM token budget in
// particular.
func TwinKey(twinID string) quota.Key {
	return quota.JoinKey("twin", twinID)
}

// Limiter returns the middleware the API mounts on its authenticated routes.
//
// It goes after Authenticate, because the key needs a principal, and before
// RequirePermission, because refusing on rate is cheaper than resolving a
// policy — and a caller who is over their limit should get the same answer
// whether or not they would have been allowed.
func (d Deps) Limiter() func(http.Handler) http.Handler {
	return quota.Limit(d.Backend, d.Tiers.Request, RequestKey,
		quota.OnLimited(func(r *http.Request, key quota.Key, dec quota.Decision) {
			slog.Info("rate limit exceeded",
				"key", string(key),
				"path", karakuriauth.SanitizeLogValue(r.URL.Path),
				"retry_after", dec.RetryAfter.Round(time.Second))
		}),
		quota.OnPressure(PressureThreshold, func(r *http.Request, key quota.Key, dec quota.Decision) {
			d.publishPressure(r.Context(), key, "request", dec)
		}),
		quota.OnError(func(r *http.Request, key quota.Key, err error) {
			// The request is allowed through — see quota.Limit's fail-open
			// rationale — so this log line is the only trace that the limiter
			// was not actually limiting.
			slog.Error("quota backend unavailable; request allowed unlimited",
				"key", string(key), "path", karakuriauth.SanitizeLogValue(r.URL.Path), "err", err)
		}),
	)
}

// publishPressure emits a quota_pressure event so the SPA and `krk` can show a
// twin approaching its ceiling before anything is refused.
func (d Deps) publishPressure(ctx context.Context, key quota.Key, tier string, dec quota.Decision) {
	if d.Hub == nil {
		return
	}
	d.Hub.Publish(ctx, event.Event{
		Type: event.TypeQuotaPressure,
		Payload: map[string]any{
			"tier":      tier,
			"key":       string(key),
			"used":      dec.Used(),
			"limit":     dec.Limit,
			"remaining": dec.Remaining,
			"reset_at":  dec.ResetAt,
		},
	})
}

// TakeCapability charges one invocation of a capability against a twin's daily
// allowance, returning whether it may proceed.
func (d Deps) TakeCapability(ctx context.Context, twinID, capability string, now time.Time) (quota.Decision, error) {
	dec, err := d.Tiers.Capability.Take(ctx, d.Backend, CapabilityKey(twinID, capability), 1, now)
	if err != nil {
		return dec, err
	}
	if dec.Allowed && dec.Used() >= PressureThreshold {
		d.publishPressure(ctx, CapabilityKey(twinID, capability), "capability", dec)
	}
	return dec, nil
}

// Usage reports every tier's current state for a twin, without spending any of
// it. It backs `krk quota show`.
func (d Deps) Usage(ctx context.Context, twinID string, now time.Time) (map[string]quota.Decision, error) {
	out := map[string]quota.Decision{}
	for name, q := range map[string]quota.Quota{
		"llm_tokens": d.Tiers.LLMTokens,
		"adapter":    d.Tiers.Adapter,
	} {
		dec, err := q.Peek(ctx, d.Backend, TwinKey(twinID), now)
		if err != nil {
			return nil, fmt.Errorf("peek %s: %w", name, err)
		}
		out[name] = dec
	}
	return out, nil
}

// Reset clears a twin's counters for the current period. An empty capability
// resets the twin-wide tiers; naming one resets that capability's daily count.
//
// It affects the period containing now and nothing else, so an override today
// cannot hand back yesterday's budget.
func (d Deps) Reset(ctx context.Context, twinID, capability string, now time.Time) error {
	if capability != "" {
		return d.Tiers.Capability.Reset(ctx, d.Backend, CapabilityKey(twinID, capability), now)
	}
	for _, q := range []quota.Quota{d.Tiers.LLMTokens, d.Tiers.Adapter} {
		if err := q.Reset(ctx, d.Backend, TwinKey(twinID), now); err != nil {
			return err
		}
	}
	return nil
}
