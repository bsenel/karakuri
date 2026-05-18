package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bsenel/karakuri/internal/platform/observability/format"
)

type LocalFileExporter struct {
	path          string
	metricsFormat ExportFormat
	logsFormat    ExportFormat
	mu            sync.Mutex
}

func NewLocalFileExporter(path string, metricsFmt, logsFmt string) *LocalFileExporter {
	return &LocalFileExporter{
		path: path, metricsFormat: ExportFormat(metricsFmt), logsFormat: ExportFormat(logsFmt),
	}
}

func (l *LocalFileExporter) Name() string { return "local" }

func (l *LocalFileExporter) ExportMetrics(ctx context.Context, records []MetricRecord) error {
	return l.write(ctx, "metrics", l.metricsFormat, records)
}

func (l *LocalFileExporter) ExportLogs(ctx context.Context, records []LogRecord) error {
	return l.write(ctx, "logs", l.logsFormat, records)
}

func (l *LocalFileExporter) write(ctx context.Context, kind string, exportFmt ExportFormat, records any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	date := time.Now().Format("2006-01-02")
	dir := filepath.Join(l.path, kind, date)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	filename := filepath.Join(dir, fmt.Sprintf("%s-00001.%s", kind, string(exportFmt)))
	switch exportFmt {
	case FormatNDJSON:
		return format.WriteNDJSON(filename, records)
	case FormatCSV:
		return format.WriteCSV(filename, records)
	case FormatParquet:
		return format.WriteParquet(filename, records)
	default:
		data, err := json.MarshalIndent(records, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(filename, data, 0o644)
	}
}

func (l *LocalFileExporter) Flush(_ context.Context) error   { return nil }
func (l *LocalFileExporter) Shutdown(_ context.Context) error { return nil }
