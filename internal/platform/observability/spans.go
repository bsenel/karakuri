package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/bsenel/karakuri/internal/core/telemetry"
)

// SpanKind mirrors the OTLP SpanKind enum values it uses.
type SpanKind int

const (
	SpanKindInternal SpanKind = 1
	SpanKindClient   SpanKind = 3
)

// StatusCode mirrors the OTLP Status.code enum.
type StatusCode int

const (
	StatusUnset StatusCode = 0
	StatusOK    StatusCode = 1
	StatusError StatusCode = 2
)

type SpanStatus struct {
	Code        StatusCode
	Description string
}

// SpanRecord is one ended span as buffered for export. IDs are lowercase hex:
// 32 characters for TraceID, 16 for SpanID and ParentSpanID.
type SpanRecord struct {
	TraceID, SpanID, ParentSpanID string
	Name                          string
	Kind                          SpanKind
	Start, End                    time.Time
	Attributes                    []telemetry.Attribute
	Status                        SpanStatus
}

// SpanExporter is implemented by an Exporter that can ship spans.
type SpanExporter interface {
	ExportSpans(ctx context.Context, spans []SpanRecord) error
}

// spanExporterFor reports whether e can take spans. A wrapper that implements
// ExportSpans for any inner exporter says whether its inner one can through
// SupportsSpans.
func spanExporterFor(e Exporter) (SpanExporter, bool) {
	se, ok := e.(SpanExporter)
	if !ok {
		return nil, false
	}
	if s, ok := e.(interface{ SupportsSpans() bool }); ok && !s.SupportsSpans() {
		return nil, false
	}
	return se, true
}

var _ telemetry.Tracer = (*OTel)(nil)

// spanKey carries the open span in a context so a child can find its parent.
type spanKey struct{}

// Start opens a span as a child of the span ctx carries, if any, and returns a
// ctx carrying the new span.
func (o *OTel) Start(ctx context.Context, name string, attrs ...telemetry.Attribute) (context.Context, telemetry.Span) {
	s := &span{o: o}
	s.rec.Name = name
	s.rec.SpanID = newID(8)
	if parent, ok := ctx.Value(spanKey{}).(*span); ok {
		s.rec.TraceID = parent.rec.TraceID
		s.rec.ParentSpanID = parent.rec.SpanID
	} else {
		s.rec.TraceID = newID(16)
	}
	s.rec.Start = time.Now().UTC()
	s.SetAttributes(attrs...)
	return context.WithValue(ctx, spanKey{}, s), s
}

// newID returns n random bytes hex-encoded, never all zeros: OTLP treats an
// all-zero ID as invalid.
func newID(n int) string {
	b := make([]byte, n)
	for {
		if _, err := rand.Read(b); err != nil {
			panic(fmt.Sprintf("observability: crypto/rand: %v", err))
		}
		for _, c := range b {
			if c != 0 {
				return hex.EncodeToString(b)
			}
		}
	}
}

// span is an open span. Its IDs are fixed at Start, so a child reads them
// without taking the lock.
type span struct {
	o     *OTel
	mu    sync.Mutex
	rec   SpanRecord
	ended bool
}

// SetAttributes adds attrs, replacing any attribute with the same key.
func (s *span) SetAttributes(attrs ...telemetry.Attribute) {
	s.mu.Lock()
	defer s.mu.Unlock()
next:
	for _, a := range attrs {
		for i := range s.rec.Attributes {
			if s.rec.Attributes[i].Key == a.Key {
				s.rec.Attributes[i] = a
				continue next
			}
		}
		s.rec.Attributes = append(s.rec.Attributes, a)
	}
}

func (s *span) SetError(msg string) {
	if msg == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.Status = SpanStatus{Code: StatusError, Description: msg}
}

// End buffers a copy of the span for the next Flush. Later calls do nothing.
func (s *span) End() {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	s.ended = true
	s.rec.End = time.Now().UTC()
	s.rec.Kind = SpanKindInternal
	for _, a := range s.rec.Attributes {
		if a.Key == telemetry.GenAIOperationName && a.Value == telemetry.OpChat {
			s.rec.Kind = SpanKindClient
		}
	}
	rec := s.rec
	rec.Attributes = append([]telemetry.Attribute(nil), s.rec.Attributes...)
	s.mu.Unlock()

	s.o.mu.Lock()
	s.o.spans = append(s.o.spans, rec)
	s.o.mu.Unlock()
}

// ExportSpans retries the inner exporter's ExportSpans under the same policy
// as metrics. An inner exporter without span support is a permanent error.
func (r *RetryExporter) ExportSpans(ctx context.Context, spans []SpanRecord) error {
	se, ok := spanExporterFor(r.inner)
	if !ok {
		return fmt.Errorf("%w: %s: spans unsupported", ErrPermanent, r.Name())
	}
	return r.retry(ctx, "ExportSpans", func() error { return se.ExportSpans(ctx, spans) })
}

// SupportsSpans reports whether the inner exporter can take spans.
func (r *RetryExporter) SupportsSpans() bool {
	_, ok := spanExporterFor(r.inner)
	return ok
}

// ExportSpans posts spans to <endpoint>/v1/traces as OTLP/JSON.
func (o *OTLPExporter) ExportSpans(ctx context.Context, spans []SpanRecord) error {
	if !o.Active() || len(spans) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(spans))
	for _, s := range spans {
		status := map[string]any{"code": int(s.Status.Code)}
		if s.Status.Description != "" {
			status["message"] = s.Status.Description
		}
		span := map[string]any{
			"traceId":           s.TraceID,
			"spanId":            s.SpanID,
			"name":              s.Name,
			"kind":              int(s.Kind),
			"startTimeUnixNano": fmt.Sprintf("%d", s.Start.UnixNano()),
			"endTimeUnixNano":   fmt.Sprintf("%d", s.End.UnixNano()),
			"attributes":        otlpAttributes(spanAttributes(s.Attributes)),
			"status":            status,
		}
		if s.ParentSpanID != "" {
			span["parentSpanId"] = s.ParentSpanID
		}
		out = append(out, span)
	}
	body := map[string]any{
		"resourceSpans": []map[string]any{
			{
				"resource": map[string]any{"attributes": otlpAttributes(map[string]string{"service.name": o.service})},
				"scopeSpans": []map[string]any{
					{
						"scope": map[string]any{"name": "karakuri"},
						"spans": out,
					},
				},
			},
		},
	}
	return o.post(ctx, o.endpoint+"/v1/traces", body)
}

// spanAttributes flattens span attributes into the string map otlpAttributes
// takes. A later attribute with the same key wins.
func spanAttributes(attrs []telemetry.Attribute) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		switch v := a.Value.(type) {
		case string:
			m[a.Key] = v
		case bool:
			m[a.Key] = strconv.FormatBool(v)
		case int:
			m[a.Key] = strconv.Itoa(v)
		case int64:
			m[a.Key] = strconv.FormatInt(v, 10)
		case float64:
			m[a.Key] = strconv.FormatFloat(v, 'g', -1, 64)
		default:
			m[a.Key] = fmt.Sprint(v)
		}
	}
	return m
}
