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
	"strconv"
	"strings"
	"time"
)

// LokiExporter ships logs to Grafana Loki via its push API. Loki is the
// log half of the Grafana stack (Prometheus handles metrics on the other
// half), so this exporter implements `ExportLogs` only — `ExportMetrics`
// is a no-op. Operators routing both to the Grafana stack pair LokiExporter
// with PrometheusExporter.
//
// Configuration (env-var driven):
//
//	LOKI_URL          — required to enable (e.g. https://logs-prod.grafana.net)
//	LOKI_USERNAME     + LOKI_PASSWORD       — HTTP Basic auth (Grafana Cloud uses tenant ID as username)
//	LOKI_BEARER_TOKEN — alternative bearer auth; takes precedence over basic
//	LOKI_TENANT_ID    — for multi-tenant Loki deployments; sent as X-Scope-OrgID
//	LOKI_LABELS       — comma-separated default stream labels (k=v;k=v)
type LokiExporter struct {
	url      string
	username string
	password string
	token    string
	tenantID string
	labels   map[string]string
	client   *http.Client
}

func NewLokiExporter() *LokiExporter {
	labels := map[string]string{}
	if v := os.Getenv("LOKI_LABELS"); v != "" {
		for _, kv := range strings.Split(v, ",") {
			if k, val, ok := strings.Cut(strings.TrimSpace(kv), "="); ok {
				labels[k] = val
			}
		}
	}
	if _, ok := labels["service"]; !ok {
		labels["service"] = "karakuri"
	}
	return &LokiExporter{
		url:      strings.TrimRight(os.Getenv("LOKI_URL"), "/"),
		username: os.Getenv("LOKI_USERNAME"),
		password: os.Getenv("LOKI_PASSWORD"),
		token:    os.Getenv("LOKI_BEARER_TOKEN"),
		tenantID: os.Getenv("LOKI_TENANT_ID"),
		labels:   labels,
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (l *LokiExporter) Name() string { return "loki" }

func (l *LokiExporter) Active() bool { return l.url != "" }

// ExportMetrics is intentionally a no-op — Loki only ingests logs. Operators
// running the Grafana stack pair this exporter with PrometheusExporter for
// the metrics half.
func (l *LokiExporter) ExportMetrics(_ context.Context, _ []MetricRecord) error { return nil }

func (l *LokiExporter) ExportLogs(ctx context.Context, records []LogRecord) error {
	if !l.Active() || len(records) == 0 {
		return nil
	}
	// Loki accepts a single stream per push, but bucketing by level keeps
	// the label cardinality bounded (level is the only per-record label
	// we promote to a stream key — message labels stay in the line text).
	streams := map[string]*lokiStream{}
	for _, r := range records {
		key := r.Level
		s, ok := streams[key]
		if !ok {
			stream := mergedAttrs(l.labels, map[string]string{"level": orDefault(r.Level, "info")})
			s = &lokiStream{Stream: stream}
			streams[key] = s
		}
		line := r.Message
		if len(r.Labels) > 0 {
			lbl, _ := json.Marshal(r.Labels)
			line = r.Message + " " + string(lbl)
		}
		ts := strconv.FormatInt(r.Timestamp.UnixNano(), 10)
		s.Values = append(s.Values, [2]string{ts, line})
	}

	payload := struct {
		Streams []*lokiStream `json:"streams"`
	}{}
	for _, s := range streams {
		payload.Streams = append(payload.Streams, s)
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.url+"/loki/api/v1/push", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	l.applyAuth(req)
	if l.tenantID != "" {
		req.Header.Set("X-Scope-OrgID", l.tenantID)
	}

	resp, err := l.client.Do(req)
	if err != nil {
		return fmt.Errorf("loki: push: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return fmt.Errorf("%w: loki %d: %s", ErrPermanent, resp.StatusCode, string(respBody))
		}
		return fmt.Errorf("loki: push -> %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (l *LokiExporter) Flush(_ context.Context) error    { return nil }
func (l *LokiExporter) Shutdown(_ context.Context) error { return nil }

func (l *LokiExporter) applyAuth(req *http.Request) {
	if l.token != "" {
		req.Header.Set("Authorization", "Bearer "+l.token)
		return
	}
	if l.username != "" {
		creds := base64.StdEncoding.EncodeToString([]byte(l.username + ":" + l.password))
		req.Header.Set("Authorization", "Basic "+creds)
	}
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
