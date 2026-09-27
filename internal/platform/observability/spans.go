package observability

import (
	"context"
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

// Start is not yet implemented and records nothing.
func (o *OTel) Start(ctx context.Context, name string, attrs ...telemetry.Attribute) (context.Context, telemetry.Span) {
	return telemetry.NoopTracer().Start(ctx, name, attrs...)
}

// ExportSpans is not yet implemented.
func (r *RetryExporter) ExportSpans(ctx context.Context, spans []SpanRecord) error { return nil }

// SupportsSpans is not yet implemented.
func (r *RetryExporter) SupportsSpans() bool { return false }

// ExportSpans is not yet implemented.
func (o *OTLPExporter) ExportSpans(ctx context.Context, spans []SpanRecord) error { return nil }
