package observability

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// PrometheusExporter supports both scrape (pull) and push modes:
//
//   - Scrape (always available when registered). The exporter is itself an
//     http.Handler; the API server mounts it at GET /metrics outside the
//     bearer-auth scope so Prometheus scrapers reach it without a token.
//     ExportMetrics records the latest value per (metric_name, sorted-labels).
//
//   - Push (optional). When PROMETHEUS_PUSHGATEWAY_URL is set, ExportMetrics
//     also POSTs the current snapshot to the pushgateway. Useful for
//     short-lived workloads that scrapers might miss.
//
// Logs are not Prometheus's concern — ExportLogs is a no-op. Pair this
// exporter with LokiExporter for the Grafana stack's log half.
type PrometheusExporter struct {
	// Pushgateway config (optional)
	pushURL string
	jobName string

	client *http.Client

	mu     sync.RWMutex
	series map[string]*promSeries // key: name + "{" + sorted-labels + "}"
}

type promSeries struct {
	name   string
	labels map[string]string
	value  float64
	when   time.Time
}

func NewPrometheusExporter() *PrometheusExporter {
	return &PrometheusExporter{
		pushURL: strings.TrimRight(os.Getenv("PROMETHEUS_PUSHGATEWAY_URL"), "/"),
		jobName: envDefault("PROMETHEUS_JOB_NAME", "karakuri"),
		client:  &http.Client{Timeout: 10 * time.Second},
		series:  make(map[string]*promSeries),
	}
}

func (p *PrometheusExporter) Name() string { return "prometheus" }

// Active always returns true once registered — scrape mode has no
// credential requirement. Push mode is opt-in via env, but a scrape-only
// exporter is still a valid Active exporter.
func (p *PrometheusExporter) Active() bool { return true }

func (p *PrometheusExporter) ExportMetrics(ctx context.Context, records []MetricRecord) error {
	if len(records) == 0 {
		return nil
	}
	p.mu.Lock()
	for _, r := range records {
		key := seriesKey(r.Name, r.Labels)
		p.series[key] = &promSeries{name: r.Name, labels: r.Labels, value: r.Value, when: r.Timestamp}
	}
	p.mu.Unlock()

	if p.pushURL != "" {
		return p.push(ctx)
	}
	return nil
}

// ExportLogs is a no-op — Prometheus doesn't ingest logs. See LokiExporter
// for the log half of the Grafana stack.
func (p *PrometheusExporter) ExportLogs(_ context.Context, _ []LogRecord) error { return nil }

func (p *PrometheusExporter) Flush(_ context.Context) error    { return nil }
func (p *PrometheusExporter) Shutdown(_ context.Context) error { return nil }

// ServeHTTP renders the current snapshot in Prometheus text exposition format.
// Suitable for direct mounting at /metrics.
func (p *PrometheusExporter) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Group by metric name to emit one HELP/TYPE block per metric.
	byName := map[string][]*promSeries{}
	for _, s := range p.series {
		byName[s.name] = append(byName[s.name], s)
	}
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		fmt.Fprintf(w, "# HELP %s Karakuri metric\n", name)
		fmt.Fprintf(w, "# TYPE %s gauge\n", name)
		for _, s := range byName[name] {
			fmt.Fprintf(w, "%s %g %d\n", renderSeries(s.name, s.labels), s.value, s.when.UnixMilli())
		}
	}
}

// push POSTs the current snapshot to the configured pushgateway in the
// Prometheus text format. The job name appears in the URL path; per-instance
// dimensions are encoded as labels on each series.
func (p *PrometheusExporter) push(ctx context.Context) error {
	var buf bytes.Buffer
	p.ServeHTTP(&memWriter{Buffer: &buf}, nil)

	url := fmt.Sprintf("%s/metrics/job/%s", p.pushURL, p.jobName)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain; version=0.0.4")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("prometheus pushgateway: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("prometheus pushgateway %s -> %d: %s", url, resp.StatusCode, string(body))
	}
	return nil
}

// memWriter satisfies http.ResponseWriter against a bytes.Buffer for push
// rendering. The pushgateway only reads the body so Header()/WriteHeader
// are no-ops.
type memWriter struct {
	*bytes.Buffer
}

func (m *memWriter) Header() http.Header        { return http.Header{} }
func (m *memWriter) WriteHeader(_ int)          {}
func (m *memWriter) Write(b []byte) (int, error) { return m.Buffer.Write(b) }

// seriesKey builds a stable map key for `(metric_name, labels)` so updates
// to the same series overwrite the prior value rather than accumulating.
func seriesKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(labels[k])
	}
	sb.WriteByte('}')
	return sb.String()
}

// renderSeries emits the canonical Prometheus exposition form for one row:
//
//	metric_name{label_a="value",label_b="value"}
func renderSeries(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(k)
		sb.WriteString(`="`)
		sb.WriteString(escapeLabelValue(labels[k]))
		sb.WriteByte('"')
	}
	sb.WriteByte('}')
	return sb.String()
}

// escapeLabelValue quotes label values per Prometheus exposition format:
// backslashes, double-quotes, and newlines must be escaped.
func escapeLabelValue(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return r.Replace(v)
}
