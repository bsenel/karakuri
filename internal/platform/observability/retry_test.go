package observability

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type countingExporter struct {
	noopExporter
	metricsCalls int32
	failUntil    int32   // succeed once metricsCalls > failUntil
	err          error
	permanent    bool
}

type noopExporter struct{ name string }

func (n *noopExporter) Name() string                                              { return n.name }
func (n *noopExporter) ExportMetrics(_ context.Context, _ []MetricRecord) error  { return nil }
func (n *noopExporter) ExportLogs(_ context.Context, _ []LogRecord) error        { return nil }
func (n *noopExporter) Flush(_ context.Context) error                            { return nil }
func (n *noopExporter) Shutdown(_ context.Context) error                         { return nil }

func (c *countingExporter) ExportMetrics(_ context.Context, _ []MetricRecord) error {
	n := atomic.AddInt32(&c.metricsCalls, 1)
	if n > c.failUntil {
		return nil
	}
	if c.permanent {
		return fmt.Errorf("%w: hard fail", ErrPermanent)
	}
	return c.err
}

func TestRetryExporter_SucceedsAfterTransientFailures(t *testing.T) {
	inner := &countingExporter{noopExporter: noopExporter{name: "test"}, failUntil: 2, err: errors.New("transient")}
	r := NewRetryExporter(inner, RetryConfig{Attempts: 3, BaseBackoff: time.Millisecond})

	if err := r.ExportMetrics(context.Background(), nil); err != nil {
		t.Errorf("expected success after 2 retries, got %v", err)
	}
	if got := atomic.LoadInt32(&inner.metricsCalls); got != 3 {
		t.Errorf("expected 3 calls (1 + 2 retries), got %d", got)
	}
}

func TestRetryExporter_GivesUpAfterMaxAttempts(t *testing.T) {
	inner := &countingExporter{noopExporter: noopExporter{name: "test"}, failUntil: 99, err: errors.New("always fails")}
	r := NewRetryExporter(inner, RetryConfig{Attempts: 3, BaseBackoff: time.Millisecond})

	err := r.ExportMetrics(context.Background(), nil)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if got := atomic.LoadInt32(&inner.metricsCalls); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestRetryExporter_ShortCircuitsOnPermanent(t *testing.T) {
	inner := &countingExporter{noopExporter: noopExporter{name: "test"}, failUntil: 99, permanent: true}
	r := NewRetryExporter(inner, RetryConfig{Attempts: 5, BaseBackoff: time.Millisecond})

	err := r.ExportMetrics(context.Background(), nil)
	if err == nil || !errors.Is(err, ErrPermanent) {
		t.Errorf("expected ErrPermanent, got %v", err)
	}
	if got := atomic.LoadInt32(&inner.metricsCalls); got != 1 {
		t.Errorf("expected single attempt for permanent error, got %d", got)
	}
}

func TestRetryExporter_RespectsContextCancel(t *testing.T) {
	inner := &countingExporter{noopExporter: noopExporter{name: "test"}, failUntil: 99, err: errors.New("transient")}
	r := NewRetryExporter(inner, RetryConfig{Attempts: 5, BaseBackoff: time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediate cancel
	err := r.ExportMetrics(ctx, nil)
	if err == nil {
		t.Errorf("expected error on cancelled context")
	}
}

func TestRetryExporter_NameDelegates(t *testing.T) {
	inner := &noopExporter{name: "underlying"}
	r := NewRetryExporter(inner, RetryConfig{})
	if r.Name() != "underlying" {
		t.Errorf("expected delegated name, got %s", r.Name())
	}
}
