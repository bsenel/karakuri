package observability

import (
	"context"
	"sync"
	"time"
)

type ExportFormat string

const (
	FormatJSON    ExportFormat = "json"
	FormatNDJSON  ExportFormat = "ndjson"
	FormatParquet ExportFormat = "parquet"
	FormatCSV     ExportFormat = "csv"
)

type MetricRecord struct {
	Name      string
	Value     float64
	Labels    map[string]string
	Timestamp time.Time
}

type LogRecord struct {
	Level     string
	Message   string
	Labels    map[string]string
	Timestamp time.Time
}

type Exporter interface {
	Name() string
	ExportMetrics(ctx context.Context, records []MetricRecord) error
	ExportLogs(ctx context.Context, records []LogRecord) error
	Flush(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

type ExporterRegistry struct {
	mu        sync.RWMutex
	exporters map[string]Exporter
}

func NewExporterRegistry() *ExporterRegistry {
	return &ExporterRegistry{exporters: make(map[string]Exporter)}
}

func (r *ExporterRegistry) Register(e Exporter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exporters[e.Name()] = e
}

func (r *ExporterRegistry) Active() []Exporter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Exporter
	for _, e := range r.exporters {
		out = append(out, e)
	}
	return out
}

func (r *ExporterRegistry) Get(name string) (Exporter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.exporters[name]
	return e, ok
}

func (r *ExporterRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var names []string
	for n := range r.exporters {
		names = append(names, n)
	}
	return names
}
