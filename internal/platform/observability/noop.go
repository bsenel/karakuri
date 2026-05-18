package observability

import "context"

type NoOpExporter struct{}

func NewNoOpExporter() *NoOpExporter { return &NoOpExporter{} }

func (n *NoOpExporter) Name() string { return "noop" }

func (n *NoOpExporter) ExportMetrics(_ context.Context, _ []MetricRecord) error { return nil }
func (n *NoOpExporter) ExportLogs(_ context.Context, _ []LogRecord) error       { return nil }
func (n *NoOpExporter) Flush(_ context.Context) error                           { return nil }
func (n *NoOpExporter) Shutdown(_ context.Context) error                        { return nil }
