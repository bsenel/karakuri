package observability

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLocalFileExporter_RollsOverSizeLimit writes many metric batches in
// NDJSON to a 1MB-limited exporter and verifies a second file appears.
func TestLocalFileExporter_RollsOverSizeLimit(t *testing.T) {
	dir := t.TempDir()
	e := NewLocalFileExporter(dir, "ndjson", "ndjson").WithRotation(1, 0)

	// 50 batches of 1000 metric records each — enough to overflow 1 MiB.
	for batch := 0; batch < 50; batch++ {
		recs := make([]MetricRecord, 1000)
		for i := range recs {
			recs[i] = MetricRecord{
				Name:      "test.metric",
				Value:     float64(i),
				Labels:    map[string]string{"batch": "abcdefghij"},
				Timestamp: time.Now().UTC(),
			}
		}
		if err := e.ExportMetrics(context.Background(), recs); err != nil {
			t.Fatalf("export batch %d: %v", batch, err)
		}
	}

	// Today's directory should contain ≥ 2 metrics-*.ndjson files.
	today := time.Now().Format("2006-01-02")
	entries, err := os.ReadDir(filepath.Join(dir, "metrics", today))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	count := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "metrics-") && strings.HasSuffix(e.Name(), ".ndjson") {
			count++
		}
	}
	if count < 2 {
		t.Errorf("expected ≥ 2 rolled files, got %d", count)
	}
}

// TestLocalFileExporter_ParquetRollsEveryWrite verifies that parquet output
// produces a new sequence index on every call (Parquet footers are closed and
// not appendable).
func TestLocalFileExporter_ParquetRollsEveryWrite(t *testing.T) {
	dir := t.TempDir()
	e := NewLocalFileExporter(dir, "parquet", "parquet")

	for i := 0; i < 3; i++ {
		if err := e.ExportMetrics(context.Background(), []MetricRecord{{Name: "x", Value: float64(i), Timestamp: time.Now().UTC()}}); err != nil {
			t.Fatalf("export %d: %v", i, err)
		}
	}
	today := time.Now().Format("2006-01-02")
	entries, _ := os.ReadDir(filepath.Join(dir, "metrics", today))
	parquetCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".parquet") {
			parquetCount++
		}
	}
	if parquetCount != 3 {
		t.Errorf("expected 3 parquet files (one per export call), got %d", parquetCount)
	}
}

// TestLocalFileExporter_PrunesOldDirs creates a fake old-dated dir and
// confirms it's removed when WithRotation maxAgeDays is set.
func TestLocalFileExporter_PrunesOldDirs(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "metrics", "2020-01-01")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatalf("mkdir old: %v", err)
	}
	if err := os.WriteFile(filepath.Join(old, "metrics-00001.ndjson"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("seed old: %v", err)
	}

	e := NewLocalFileExporter(dir, "ndjson", "ndjson").WithRotation(0, 7)
	if err := e.ExportMetrics(context.Background(), []MetricRecord{{Name: "x", Value: 1, Timestamp: time.Now().UTC()}}); err != nil {
		t.Fatalf("export: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("expected old dir to be pruned, still exists: %v", err)
	}
}
