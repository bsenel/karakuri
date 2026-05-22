package format

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/parquet-go/parquet-go"
)

// MetricRow is the columnar Parquet schema for a single MetricRecord.
// We mirror the in-memory MetricRecord shape but flatten the Labels map into
// a JSON-encoded string column so the file stays usable from DuckDB and
// pandas without needing nested-type support.
type MetricRow struct {
	Name      string  `parquet:"name,zstd"`
	Value     float64 `parquet:"value"`
	Labels    string  `parquet:"labels,zstd"`
	Timestamp int64   `parquet:"timestamp"`
}

// LogRow is the columnar Parquet schema for a single LogRecord.
type LogRow struct {
	Level     string `parquet:"level,dict,zstd"`
	Message   string `parquet:"message,zstd"`
	Labels    string `parquet:"labels,zstd"`
	Timestamp int64  `parquet:"timestamp"`
}

// WriteParquet writes records as a real Apache Parquet file. The caller must
// pass either []MetricRow or []LogRow (already flattened by the observability
// package). Other input types return an error so misuse is loud rather than
// silently writing JSON to a .parquet path.
func WriteParquet(path string, v any) error {
	parquetPath := ensureExt(path, ".parquet")
	f, err := os.Create(parquetPath)
	if err != nil {
		return err
	}
	defer f.Close()

	switch records := v.(type) {
	case []MetricRow:
		w := parquet.NewGenericWriter[MetricRow](f)
		if _, err := w.Write(records); err != nil {
			return err
		}
		return w.Close()
	case []LogRow:
		w := parquet.NewGenericWriter[LogRow](f)
		if _, err := w.Write(records); err != nil {
			return err
		}
		return w.Close()
	default:
		return fmt.Errorf("parquet: unsupported input type %T (expected []MetricRow or []LogRow)", v)
	}
}

func ensureExt(path, ext string) string {
	if strings.HasSuffix(path, ext) {
		return path
	}
	if i := strings.LastIndex(path, "."); i > 0 {
		return path[:i] + ext
	}
	return path + ext
}

// EpochMillis returns Unix milliseconds since epoch — the standard timestamp
// shape for tools that ingest the resulting Parquet files. Used by the
// observability package mapping MetricRecord → MetricRow / LogRecord → LogRow.
func EpochMillis(t time.Time) int64 { return t.UTC().UnixMilli() }
