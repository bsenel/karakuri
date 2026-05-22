package observability

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ElasticsearchExporter ships metrics + logs to Elasticsearch's `_bulk` index
// API. One exporter covers the whole ELK stack: Logstash and Kibana sit on
// top of Elasticsearch, so writing to ES via _bulk is the universal entry
// point. Pure net/http — no Elastic SDK dep.
//
// Configuration (env-var driven):
//
//	ELASTICSEARCH_URL              — required to enable (e.g. https://es.example.com:9200)
//	ELASTICSEARCH_USERNAME + ELASTICSEARCH_PASSWORD — HTTP Basic auth
//	ELASTICSEARCH_API_KEY          — alternative auth (Elastic Cloud); takes precedence
//	ELASTICSEARCH_METRICS_INDEX    — defaults to "karakuri-metrics"
//	ELASTICSEARCH_LOGS_INDEX       — defaults to "karakuri-logs"
type ElasticsearchExporter struct {
	url           string
	username      string
	password      string
	apiKey        string
	metricsIndex  string
	logsIndex     string
	client        *http.Client
}

func NewElasticsearchExporter() *ElasticsearchExporter {
	url := strings.TrimRight(os.Getenv("ELASTICSEARCH_URL"), "/")
	return &ElasticsearchExporter{
		url:          url,
		username:     os.Getenv("ELASTICSEARCH_USERNAME"),
		password:     os.Getenv("ELASTICSEARCH_PASSWORD"),
		apiKey:       os.Getenv("ELASTICSEARCH_API_KEY"),
		metricsIndex: envDefault("ELASTICSEARCH_METRICS_INDEX", "karakuri-metrics"),
		logsIndex:    envDefault("ELASTICSEARCH_LOGS_INDEX", "karakuri-logs"),
		client:       &http.Client{Timeout: 15 * time.Second},
	}
}

func (e *ElasticsearchExporter) Name() string { return "elasticsearch" }

func (e *ElasticsearchExporter) Active() bool { return e.url != "" }

func (e *ElasticsearchExporter) ExportMetrics(ctx context.Context, records []MetricRecord) error {
	if !e.Active() || len(records) == 0 {
		return nil
	}
	rows := make([]any, len(records))
	for i, r := range records {
		rows[i] = map[string]any{
			"@timestamp": r.Timestamp.UTC(),
			"name":       r.Name,
			"value":      r.Value,
			"labels":     r.Labels,
		}
	}
	return e.bulk(ctx, e.metricsIndex, rows)
}

func (e *ElasticsearchExporter) ExportLogs(ctx context.Context, records []LogRecord) error {
	if !e.Active() || len(records) == 0 {
		return nil
	}
	rows := make([]any, len(records))
	for i, r := range records {
		rows[i] = map[string]any{
			"@timestamp": r.Timestamp.UTC(),
			"level":      r.Level,
			"message":    r.Message,
			"labels":     r.Labels,
		}
	}
	return e.bulk(ctx, e.logsIndex, rows)
}

func (e *ElasticsearchExporter) Flush(_ context.Context) error    { return nil }
func (e *ElasticsearchExporter) Shutdown(_ context.Context) error { return nil }

// bulk POSTs records to /_bulk in the Elasticsearch bulk format: alternating
// action lines (`{"index":{"_index":"..."}}`) and document lines, each
// newline-terminated, with a trailing newline.
func (e *ElasticsearchExporter) bulk(ctx context.Context, index string, rows []any) error {
	var buf bytes.Buffer
	for _, r := range rows {
		actionLine, _ := json.Marshal(map[string]map[string]string{"index": {"_index": index}})
		docLine, err := json.Marshal(r)
		if err != nil {
			return fmt.Errorf("elasticsearch: marshal row: %w", err)
		}
		buf.Write(actionLine)
		buf.WriteByte('\n')
		buf.Write(docLine)
		buf.WriteByte('\n')
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url+"/_bulk", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	e.applyAuth(req)

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("elasticsearch: post _bulk: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return fmt.Errorf("%w: elasticsearch %d: %s", ErrPermanent, resp.StatusCode, string(body))
		}
		return fmt.Errorf("elasticsearch: _bulk -> %d: %s", resp.StatusCode, string(body))
	}
	// _bulk returns 200 even with per-item errors. Parse `errors:true` and
	// surface a wrapped error so the retry layer can decide; per-document
	// errors are typically mapping problems (permanent).
	var br struct {
		Errors bool `json:"errors"`
	}
	if rb, err := io.ReadAll(resp.Body); err == nil {
		_ = json.Unmarshal(rb, &br)
		if br.Errors {
			return fmt.Errorf("%w: elasticsearch _bulk reported per-document errors", ErrPermanent)
		}
	}
	return nil
}

func (e *ElasticsearchExporter) applyAuth(req *http.Request) {
	if e.apiKey != "" {
		req.Header.Set("Authorization", "ApiKey "+e.apiKey)
		return
	}
	if e.username != "" {
		creds := base64.StdEncoding.EncodeToString([]byte(e.username + ":" + e.password))
		req.Header.Set("Authorization", "Basic "+creds)
	}
}

// envDefault returns the value of an environment variable, falling back to
// the provided default when unset. Shared helper across exporters in this
// package.
func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
