package observability

import (
	"context"

	"github.com/bsenel/karakuri/internal/core/errors"
)

type AWSExporter struct{}

func NewAWSExporter() *AWSExporter { return &AWSExporter{} }

func (a *AWSExporter) Name() string { return "aws" }

func (a *AWSExporter) ExportMetrics(_ context.Context, _ []MetricRecord) error {
	return errors.ErrNotImplemented
}

func (a *AWSExporter) ExportLogs(_ context.Context, _ []LogRecord) error {
	return errors.ErrNotImplemented
}

func (a *AWSExporter) Flush(_ context.Context) error    { return errors.ErrNotImplemented }
func (a *AWSExporter) Shutdown(_ context.Context) error { return errors.ErrNotImplemented }
