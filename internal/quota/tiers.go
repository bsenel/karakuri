// Package quota wires the standalone github.com/bsenel/karakuri/quota module
// into Karakuri. The module knows nothing about twins, principals or
// capabilities; this package is where those concepts meet it — the canonical
// tiers, the key extractor, and the middleware the API mounts.
//
// See ADR 008 for why the engine lives outside the main module.
package quota

import (
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/quota"
)

// Tiers are the four limits Karakuri ships with.
//
// They are separate types because they answer different questions. A rate limit
// refuses you and expects you back in a moment; a quota refuses you until
// tomorrow. Conflating them would mean one message and one alert for two very
// different operational situations.
type Tiers struct {
	// Request caps how fast one caller can drive the API.
	Request quota.Policy

	// Capability caps how many times a twin may invoke one capability in a day.
	// Most capabilities are not model calls — they open worktrees, or reach
	// GitHub and Linear — so this is the tier that stops a misconfigured
	// watcher hammering an external service.
	Capability quota.Quota

	// LLMTokens caps a twin's model spend for the day, counted in tokens.
	// Defined here and enforced by the loop rather than by middleware: the
	// spend happens inside a reasoning step, not on an HTTP request.
	LLMTokens quota.Quota

	// Adapter caps calls to one external adapter, sized to match the rates
	// those services publish. Enforced where an adapter is invoked.
	Adapter quota.Quota
}

// DefaultTiers returns the shipped limits, overridden by anything set in
// configuration.
//
// The defaults are deliberately generous. A limiter that fires during ordinary
// use trains operators to raise it without reading it, and the first tier to
// matter should be one that is protecting something.
func DefaultTiers(cfg config.QuotaConfig) Tiers {
	t := Tiers{
		// 60 a minute sustained, tolerating twenty arriving at once — a page
		// load that fires several requests should never be throttled.
		Request:    quota.PerMinute(60).Burst(20),
		Capability: quota.Quota{Name: "capability", Cap: 1000, Period: quota.Daily},
		LLMTokens:  quota.Quota{Name: "llm-tokens", Cap: 1_000_000, Period: quota.Daily},
		Adapter:    quota.Quota{Name: "adapter", Cap: 5000, Period: quota.Daily},
	}

	if cfg.RequestsPerMinute > 0 {
		burst := cfg.RequestBurst
		if burst <= 0 {
			burst = cfg.RequestsPerMinute
		}
		t.Request = quota.PerMinute(cfg.RequestsPerMinute).Burst(burst)
	}
	if cfg.CapabilityPerDay > 0 {
		t.Capability.Cap = cfg.CapabilityPerDay
	}
	if cfg.LLMTokensPerDay > 0 {
		t.LLMTokens.Cap = cfg.LLMTokensPerDay
	}
	if cfg.AdapterPerDay > 0 {
		t.Adapter.Cap = cfg.AdapterPerDay
	}
	return t
}

// Validate reports whether every tier is usable. Called at boot, so a
// misconfigured limit fails the process rather than silently admitting
// everything.
func (t Tiers) Validate() error {
	if err := t.Request.Validate(); err != nil {
		return err
	}
	for _, q := range []quota.Quota{t.Capability, t.LLMTokens, t.Adapter} {
		if err := q.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// PressureThreshold is the fraction of a limit at which a quota_pressure event
// is published. Eighty percent leaves enough room to react before anything is
// actually refused.
const PressureThreshold = 0.8

// sweepInterval is how often the SQL backend's housekeeping runs, when that
// backend is in use.
const sweepInterval = time.Hour
