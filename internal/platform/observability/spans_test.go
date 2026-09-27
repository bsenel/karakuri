package observability

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/telemetry"
)

// spanCapture is an Exporter that also takes spans. It records every span it
// is handed, and returns err from ExportSpans when set.
type spanCapture struct {
	noopExporter
	mu    sync.Mutex
	calls int
	spans []SpanRecord
	err   error
}

func (c *spanCapture) ExportSpans(_ context.Context, spans []SpanRecord) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	c.spans = append(c.spans, spans...)
	return c.err
}

// take returns the spans captured so far and forgets them.
func (c *spanCapture) take() []SpanRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.spans
	c.spans = nil
	return out
}

func newSpanOTel(exporters ...Exporter) *OTel {
	reg := NewExporterRegistry()
	for _, e := range exporters {
		reg.Register(e)
	}
	return NewOTel(reg)
}

func flush(t *testing.T, o *OTel) {
	t.Helper()
	if err := o.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}
}

func spansNamed(spans []SpanRecord, name string) []SpanRecord {
	var out []SpanRecord
	for _, s := range spans {
		if s.Name == name {
			out = append(out, s)
		}
	}
	return out
}

func requireSpan(t *testing.T, spans []SpanRecord, name string) SpanRecord {
	t.Helper()
	got := spansNamed(spans, name)
	if len(got) != 1 {
		t.Fatalf("want exactly one %q span, got %d in %+v", name, len(got), spans)
	}
	return got[0]
}

var (
	traceIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	spanIDPattern  = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

func requireID(t *testing.T, what, id string, pattern *regexp.Regexp) {
	t.Helper()
	if !pattern.MatchString(id) {
		t.Errorf("%s %q does not match %s", what, id, pattern)
	}
	if strings.Trim(id, "0") == "" {
		t.Errorf("%s %q is all zeros", what, id)
	}
}

func TestSpanParentChildThroughContext(t *testing.T) {
	c := &spanCapture{noopExporter: noopExporter{name: "spans"}}
	o := newSpanOTel(c)

	ctx, parent := o.Start(context.Background(), telemetry.OpInvokeAgent)
	_, child := o.Start(ctx, telemetry.OpChat)
	child.End()
	parent.End()
	_, other := o.Start(context.Background(), "other_root")
	other.End()
	flush(t, o)

	spans := c.take()
	p := requireSpan(t, spans, telemetry.OpInvokeAgent)
	ch := requireSpan(t, spans, telemetry.OpChat)
	r := requireSpan(t, spans, "other_root")

	requireID(t, "parent TraceID", p.TraceID, traceIDPattern)
	requireID(t, "parent SpanID", p.SpanID, spanIDPattern)
	requireID(t, "child SpanID", ch.SpanID, spanIDPattern)
	requireID(t, "second root TraceID", r.TraceID, traceIDPattern)

	if p.ParentSpanID != "" {
		t.Errorf("root ParentSpanID: want empty, got %q", p.ParentSpanID)
	}
	if r.ParentSpanID != "" {
		t.Errorf("second root ParentSpanID: want empty, got %q", r.ParentSpanID)
	}
	if ch.TraceID != p.TraceID {
		t.Errorf("child TraceID %q != parent TraceID %q", ch.TraceID, p.TraceID)
	}
	if ch.ParentSpanID != p.SpanID {
		t.Errorf("child ParentSpanID %q != parent SpanID %q", ch.ParentSpanID, p.SpanID)
	}
	if ch.SpanID == p.SpanID {
		t.Errorf("child and parent share SpanID %q", ch.SpanID)
	}
	if r.TraceID == p.TraceID {
		t.Errorf("two root spans share TraceID %q", r.TraceID)
	}
}

func TestSpanBufferAndFlush(t *testing.T) {
	c := &spanCapture{noopExporter: noopExporter{name: "spans"}}
	o := newSpanOTel(c)

	_, s := o.Start(context.Background(), telemetry.OpExecuteTool)
	s.SetAttributes(telemetry.Attribute{Key: telemetry.GenAIToolName, Value: "git.commit"})
	s.SetError("boom")
	s.End()
	s.End()
	flush(t, o)

	spans := c.take()
	if len(spans) != 1 {
		t.Fatalf("first flush: want 1 span, got %d: %+v", len(spans), spans)
	}
	got := spans[0]
	if got.Name != telemetry.OpExecuteTool {
		t.Errorf("Name: want %q, got %q", telemetry.OpExecuteTool, got.Name)
	}
	if !hasAttr(got.Attributes, telemetry.GenAIToolName, "git.commit") {
		t.Errorf("Attributes: want %s=git.commit in %+v", telemetry.GenAIToolName, got.Attributes)
	}
	if want := (SpanStatus{Code: StatusError, Description: "boom"}); got.Status != want {
		t.Errorf("Status: want %+v, got %+v", want, got.Status)
	}
	if got.Start.IsZero() || got.End.Before(got.Start) {
		t.Errorf("want End (%v) at or after a set Start (%v)", got.End, got.Start)
	}

	flush(t, o)
	if spans := c.take(); len(spans) != 0 {
		t.Errorf("second flush: want 0 spans, got %d: %+v", len(spans), spans)
	}

	s.End()
	flush(t, o)
	if spans := c.take(); len(spans) != 0 {
		t.Errorf("End after export: want 0 spans, got %d: %+v", len(spans), spans)
	}
}

func hasAttr(attrs []telemetry.Attribute, key string, value any) bool {
	for _, a := range attrs {
		if a.Key == key && a.Value == value {
			return true
		}
	}
	return false
}

func TestSpanKindFromOperation(t *testing.T) {
	cases := []struct {
		op   string
		want SpanKind
	}{
		{telemetry.OpChat, SpanKindClient},
		{telemetry.OpInvokeAgent, SpanKindInternal},
		{telemetry.OpExecuteTool, SpanKindInternal},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			c := &spanCapture{noopExporter: noopExporter{name: "spans"}}
			o := newSpanOTel(c)
			_, s := o.Start(context.Background(), tc.op,
				telemetry.Attribute{Key: telemetry.GenAIOperationName, Value: tc.op})
			s.End()
			flush(t, o)

			got := requireSpan(t, c.take(), tc.op)
			if got.Kind != tc.want {
				t.Errorf("Kind: want %d, got %d", tc.want, got.Kind)
			}
		})
	}
}

// One exporter failing must not stop the others receiving spans, the same
// isolation Flush gives metrics and logs.
func TestSpanExporterIsolation(t *testing.T) {
	failing := &spanCapture{noopExporter: noopExporter{name: "failing"}, err: errors.New("collector down")}
	healthy := &spanCapture{noopExporter: noopExporter{name: "healthy"}}
	o := newSpanOTel(failing, healthy)

	names := []string{"a", "b", "c"}
	for _, n := range names {
		_, s := o.Start(context.Background(), n)
		s.End()
	}
	flush(t, o)

	if failing.calls == 0 {
		t.Errorf("failing exporter was never called")
	}
	if healthy.calls == 0 {
		t.Fatalf("healthy exporter was never called")
	}
	got := healthy.take()
	if len(got) != len(names) {
		t.Fatalf("healthy exporter: want %d spans, got %d: %+v", len(names), len(got), got)
	}
	for _, n := range names {
		requireSpan(t, got, n)
	}
}

// countingSpanExporter fails ExportSpans until spanCalls exceeds failUntil.
type countingSpanExporter struct {
	noopExporter
	spanCalls int32
	failUntil int32
	permanent bool
}

func (c *countingSpanExporter) ExportSpans(context.Context, []SpanRecord) error {
	n := atomic.AddInt32(&c.spanCalls, 1)
	if n > c.failUntil {
		return nil
	}
	if c.permanent {
		return fmt.Errorf("%w: hard fail", ErrPermanent)
	}
	return errors.New("transient")
}

func TestRetryExporterSpans(t *testing.T) {
	spans := []SpanRecord{{Name: "chat"}}

	t.Run("transient then success", func(t *testing.T) {
		inner := &countingSpanExporter{noopExporter: noopExporter{name: "test"}, failUntil: 1}
		r := NewRetryExporter(inner, RetryConfig{Attempts: 3, BaseBackoff: time.Millisecond})
		if err := r.ExportSpans(context.Background(), spans); err != nil {
			t.Errorf("want nil after one retry, got %v", err)
		}
		if got := atomic.LoadInt32(&inner.spanCalls); got != 2 {
			t.Errorf("want 2 attempts, got %d", got)
		}
	})

	t.Run("permanent short-circuits", func(t *testing.T) {
		inner := &countingSpanExporter{noopExporter: noopExporter{name: "test"}, failUntil: 99, permanent: true}
		r := NewRetryExporter(inner, RetryConfig{Attempts: 5, BaseBackoff: time.Millisecond})
		if err := r.ExportSpans(context.Background(), spans); err == nil {
			t.Errorf("want an error for a permanent failure, got nil")
		}
		if got := atomic.LoadInt32(&inner.spanCalls); got != 1 {
			t.Errorf("want 1 attempt, got %d", got)
		}
	})

	t.Run("spanExporterFor", func(t *testing.T) {
		cases := []struct {
			name string
			e    Exporter
			want bool
		}{
			{"retry around metrics-only", NewRetryExporter(&noopExporter{name: "m"}, RetryConfig{}), false},
			{"retry around span exporter", NewRetryExporter(&countingSpanExporter{noopExporter: noopExporter{name: "s"}}, RetryConfig{}), true},
			{"bare metrics-only", &noopExporter{name: "m"}, false},
			{"bare span exporter", &countingSpanExporter{noopExporter: noopExporter{name: "s"}}, true},
		}
		for _, tc := range cases {
			if _, got := spanExporterFor(tc.e); got != tc.want {
				t.Errorf("%s: want %v, got %v", tc.name, tc.want, got)
			}
		}
	})
}

// An exporter that cannot take spans says so once, rather than dropping every
// batch quietly or logging on every flush.
func TestUnsupportedSpanExporterWarnsOnce(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	reg := NewExporterRegistry()
	reg.Register(&noopExporter{name: "metrics-only"})
	o := NewOTel(reg)

	emit := func(n int) {
		for i := 0; i < n; i++ {
			_, s := o.Start(context.Background(), "work")
			s.End()
		}
		flush(t, o)
	}
	emit(2)
	emit(3)

	var warns []string
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, "level=WARN") {
			warns = append(warns, line)
		}
	}
	if len(warns) != 1 {
		t.Fatalf("want exactly 1 WARN line, got %d:\n%s", len(warns), buf.String())
	}
	if !strings.Contains(warns[0], "metrics-only") {
		t.Errorf("WARN does not name the exporter: %s", warns[0])
	}
	if !regexp.MustCompile(`=2(\s|$)`).MatchString(warns[0]) {
		t.Errorf("WARN does not carry the dropped span count 2: %s", warns[0])
	}

	// The spans no exporter could take were drained, not kept for later.
	late := &spanCapture{noopExporter: noopExporter{name: "late"}}
	reg.Register(late)
	flush(t, o)
	if got := late.take(); len(got) != 0 {
		t.Errorf("want the span buffer drained after flush, got %d stale spans", len(got))
	}
}
