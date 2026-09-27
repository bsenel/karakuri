package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/platform/observability"
)

// countingExporter remembers how many metric records it was handed.
type countingExporter struct {
	mu      sync.Mutex
	metrics int
}

func (e *countingExporter) Name() string { return "counting" }

func (e *countingExporter) ExportMetrics(_ context.Context, records []observability.MetricRecord) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.metrics += len(records)
	return nil
}

func (e *countingExporter) ExportLogs(context.Context, []observability.LogRecord) error { return nil }
func (e *countingExporter) Flush(context.Context) error                                 { return nil }
func (e *countingExporter) Shutdown(context.Context) error                              { return nil }

func (e *countingExporter) received() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.metrics
}

func newCountingOTel() (*observability.OTel, *countingExporter) {
	exp := &countingExporter{}
	reg := observability.NewExporterRegistry()
	reg.Register(exp)
	return observability.NewOTel(reg), exp
}

// waitReceived polls for up to 2s until exp has been handed want records.
func waitReceived(t *testing.T, exp *countingExporter, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for exp.received() < want {
		if time.Now().After(deadline) {
			t.Fatalf("exporter received %d metrics within 2s, want %d", exp.received(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Shutdown flushes whatever is still buffered. The interval is far longer
// than the test, so only the final flush can deliver the metric.
func TestTelemetryFlushFlushesOnShutdown(t *testing.T) {
	o, exp := newCountingOTel()
	o.RecordMetric("m", 1, nil)

	ctx, cancel := context.WithCancel(context.Background())
	startTelemetryFlush(ctx, o, time.Hour)
	cancel()

	waitReceived(t, exp, 1)
}

func TestTelemetryFlushFlushesOnTick(t *testing.T) {
	o, exp := newCountingOTel()
	o.RecordMetric("m", 1, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startTelemetryFlush(ctx, o, 10*time.Millisecond)

	waitReceived(t, exp, 1)
}

func TestTelemetryFlushIgnoresNilOTel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startTelemetryFlush(ctx, nil, time.Millisecond)
}
