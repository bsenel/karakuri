package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bsenel/karakuri/internal/platform/observability/format"
)

// LocalFileExporter writes metrics/logs to date-partitioned files under
// `<path>/<kind>/<YYYY-MM-DD>/<kind>-NNNNN.<ext>`. When a file exceeds
// MaxSizeMB it rolls to the next sequence number; files older than
// MaxAgeDays are pruned on each write so the disk footprint stays bounded.
type LocalFileExporter struct {
	path          string
	metricsFormat ExportFormat
	logsFormat    ExportFormat
	maxSizeMB     int
	maxAgeDays    int
	mu            sync.Mutex
}

func NewLocalFileExporter(path string, metricsFmt, logsFmt string) *LocalFileExporter {
	return &LocalFileExporter{
		path:          path,
		metricsFormat: ExportFormat(metricsFmt),
		logsFormat:    ExportFormat(logsFmt),
	}
}

// WithRotation sets size + age limits. 0 on either field disables that
// dimension (no size rotation / no age pruning).
func (l *LocalFileExporter) WithRotation(maxSizeMB, maxAgeDays int) *LocalFileExporter {
	l.maxSizeMB = maxSizeMB
	l.maxAgeDays = maxAgeDays
	return l
}

func (l *LocalFileExporter) Name() string { return "local" }

func (l *LocalFileExporter) ExportMetrics(ctx context.Context, records []MetricRecord) error {
	return l.write(ctx, "metrics", l.metricsFormat, records)
}

func (l *LocalFileExporter) ExportLogs(ctx context.Context, records []LogRecord) error {
	return l.write(ctx, "logs", l.logsFormat, records)
}

func (l *LocalFileExporter) write(_ context.Context, kind string, exportFmt ExportFormat, records any) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	date := time.Now().Format("2006-01-02")
	dir := filepath.Join(l.path, kind, date)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	filename, err := l.nextFile(dir, kind, string(exportFmt))
	if err != nil {
		return err
	}

	if err := writeFormat(filename, exportFmt, kind, records); err != nil {
		return err
	}

	if l.maxAgeDays > 0 {
		l.prune(kind)
	}
	return nil
}

// nextFile finds the next file index in `dir` for the given kind+format. For
// appendable formats (ndjson, json, csv) it returns the highest existing
// index when that file is still under MaxSizeMB; for Parquet it always rolls
// to a new index because Parquet files have closed footers and can't be
// appended.
func (l *LocalFileExporter) nextFile(dir, kind, ext string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	maxIdx := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), kind+"-") || !strings.HasSuffix(e.Name(), "."+ext) {
			continue
		}
		var idx int
		if _, err := fmt.Sscanf(e.Name(), kind+"-%05d."+ext, &idx); err == nil && idx > maxIdx {
			maxIdx = idx
		}
	}

	if l.maxSizeMB > 0 && ext != string(FormatParquet) && maxIdx > 0 {
		cur := filepath.Join(dir, fmt.Sprintf("%s-%05d.%s", kind, maxIdx, ext))
		if info, err := os.Stat(cur); err == nil {
			if info.Size() < int64(l.maxSizeMB)*1024*1024 {
				return cur, nil
			}
			maxIdx++
		}
	}
	if ext == string(FormatParquet) || maxIdx == 0 {
		maxIdx++
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%05d.%s", kind, maxIdx, ext)), nil
}

// prune removes per-kind date directories older than maxAgeDays.
func (l *LocalFileExporter) prune(kind string) {
	root := filepath.Join(l.path, kind)
	cutoff := time.Now().AddDate(0, 0, -l.maxAgeDays)
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t, err := time.Parse("2006-01-02", e.Name())
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
}

// writeFormat dispatches to the right format writer; Parquet inputs are
// pre-flattened into format.{Metric,Log}Row slices since the columnar writer
// needs typed structs.
func writeFormat(filename string, exportFmt ExportFormat, kind string, records any) error {
	switch exportFmt {
	case FormatNDJSON:
		return format.WriteNDJSON(filename, records)
	case FormatCSV:
		return format.WriteCSV(filename, records)
	case FormatParquet:
		return writeParquet(filename, kind, records)
	default:
		data, err := json.MarshalIndent(records, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(filename, data, 0o644)
	}
}

func writeParquet(filename, kind string, records any) error {
	switch kind {
	case "metrics":
		mr, ok := records.([]MetricRecord)
		if !ok {
			return fmt.Errorf("parquet: expected []MetricRecord for kind=metrics, got %T", records)
		}
		rows := make([]format.MetricRow, len(mr))
		for i, r := range mr {
			lbl, _ := json.Marshal(r.Labels)
			rows[i] = format.MetricRow{
				Name:      r.Name,
				Value:     r.Value,
				Labels:    string(lbl),
				Timestamp: format.EpochMillis(r.Timestamp),
			}
		}
		return format.WriteParquet(filename, rows)
	case "logs":
		lr, ok := records.([]LogRecord)
		if !ok {
			return fmt.Errorf("parquet: expected []LogRecord for kind=logs, got %T", records)
		}
		rows := make([]format.LogRow, len(lr))
		for i, r := range lr {
			lbl, _ := json.Marshal(r.Labels)
			rows[i] = format.LogRow{
				Level:     r.Level,
				Message:   r.Message,
				Labels:    string(lbl),
				Timestamp: format.EpochMillis(r.Timestamp),
			}
		}
		return format.WriteParquet(filename, rows)
	default:
		return fmt.Errorf("parquet: unknown kind %q", kind)
	}
}

func (l *LocalFileExporter) Flush(_ context.Context) error    { return nil }
func (l *LocalFileExporter) Shutdown(_ context.Context) error { return nil }
