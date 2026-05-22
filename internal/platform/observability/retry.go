package observability

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// RetryExporter wraps any Exporter and retries Export{Metrics,Logs} calls on
// error with exponential backoff. Transient network blips (a connection
// reset, a momentary 5xx) no longer drop a whole batch — the retry loop
// gives the downstream a few chances before the chain-isolation layer logs
// a final WARN.
//
// Local file writes don't need this layer (synchronous to disk), so
// bootstrap wraps remote exporters only.
type RetryExporter struct {
	inner    Exporter
	attempts int
	base     time.Duration
}

// RetryConfig sets the retry policy; zero values get sane defaults
// (3 attempts, 100ms base backoff, max ~700ms total wait).
type RetryConfig struct {
	Attempts    int           // total attempts including the first (default 3)
	BaseBackoff time.Duration // wait before retry #2 (default 100ms); doubled each subsequent retry
}

// NewRetryExporter constructs a retrying wrapper. The wrapped exporter's
// Name() is preserved so /health output and per-exporter logs still
// identify the underlying destination.
func NewRetryExporter(inner Exporter, cfg RetryConfig) *RetryExporter {
	if cfg.Attempts <= 0 {
		cfg.Attempts = 3
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = 100 * time.Millisecond
	}
	return &RetryExporter{inner: inner, attempts: cfg.Attempts, base: cfg.BaseBackoff}
}

func (r *RetryExporter) Name() string { return r.inner.Name() }

func (r *RetryExporter) ExportMetrics(ctx context.Context, records []MetricRecord) error {
	return r.retry(ctx, "ExportMetrics", func() error { return r.inner.ExportMetrics(ctx, records) })
}

func (r *RetryExporter) ExportLogs(ctx context.Context, records []LogRecord) error {
	return r.retry(ctx, "ExportLogs", func() error { return r.inner.ExportLogs(ctx, records) })
}

func (r *RetryExporter) Flush(ctx context.Context) error {
	return r.retry(ctx, "Flush", func() error { return r.inner.Flush(ctx) })
}

func (r *RetryExporter) Shutdown(ctx context.Context) error { return r.inner.Shutdown(ctx) }

// retry executes fn up to r.attempts times. Each retry waits base * 2^i,
// capped at 30s, so a 3-attempt policy waits 100ms + 200ms = 300ms total.
// Aborts immediately if the context is cancelled.
func (r *RetryExporter) retry(ctx context.Context, op string, fn func() error) error {
	var lastErr error
	for i := 0; i < r.attempts; i++ {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if i == r.attempts-1 {
			break
		}
		// Bail on permanent errors — no point retrying ErrPermanent.
		if errors.Is(err, ErrPermanent) {
			break
		}
		wait := r.base * (1 << i)
		if wait > 30*time.Second {
			wait = 30 * time.Second
		}
		slog.DebugContext(ctx, "exporter retry pending",
			"exporter", r.Name(), "op", op, "attempt", i+1, "wait_ms", wait.Milliseconds(), "err", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return lastErr
}

// ErrPermanent signals an unrecoverable error — bad credentials, malformed
// payload, dropped record. Exporters return this (wrapped) when retrying
// would be pointless; the RetryExporter short-circuits instead of burning
// attempts on guaranteed failures.
var ErrPermanent = errors.New("exporter: permanent error (no retry)")
