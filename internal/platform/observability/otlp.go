package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OTLPExporter ships metrics + logs to an OpenTelemetry Collector via the
// OTLP/HTTP+JSON transport. The OTel Collector is the canonical OSS routing
// hub for telemetry — it can fan back out to any backend (Tempo, Loki,
// Mimir, vendor APMs) without Karakuri having to know about each one.
//
// Configuration (env-var driven; follows OTel SDK conventions):
//
//	OTEL_EXPORTER_OTLP_ENDPOINT     — required to enable (e.g. http://otelcol:4318)
//	OTEL_EXPORTER_OTLP_HEADERS      — comma-separated key=value headers, e.g.
//	                                  "Authorization=Bearer abc,X-Tenant=acme"
//	OTEL_SERVICE_NAME               — resource service.name attribute; default "karakuri"
type OTLPExporter struct {
	endpoint  string
	headers   map[string]string
	service   string
	client    *http.Client
}

func NewOTLPExporter() *OTLPExporter {
	headers := map[string]string{}
	if v := os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"); v != "" {
		for _, kv := range strings.Split(v, ",") {
			if k, val, ok := strings.Cut(strings.TrimSpace(kv), "="); ok {
				headers[k] = val
			}
		}
	}
	return &OTLPExporter{
		endpoint: strings.TrimRight(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), "/"),
		headers:  headers,
		service:  envDefault("OTEL_SERVICE_NAME", "karakuri"),
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (o *OTLPExporter) Name() string { return "otlp" }

func (o *OTLPExporter) Active() bool { return o.endpoint != "" }

func (o *OTLPExporter) ExportMetrics(ctx context.Context, records []MetricRecord) error {
	if !o.Active() || len(records) == 0 {
		return nil
	}
	dataPoints := make([]map[string]any, 0, len(records))
	groupedByName := map[string][]map[string]any{}
	for _, r := range records {
		dp := map[string]any{
			"timeUnixNano": fmt.Sprintf("%d", r.Timestamp.UnixNano()),
			"asDouble":     r.Value,
			"attributes":   otlpAttributes(r.Labels),
		}
		groupedByName[r.Name] = append(groupedByName[r.Name], dp)
	}
	metrics := make([]map[string]any, 0, len(groupedByName))
	for name, points := range groupedByName {
		metrics = append(metrics, map[string]any{
			"name":  name,
			"gauge": map[string]any{"dataPoints": points},
		})
	}
	body := map[string]any{
		"resourceMetrics": []map[string]any{
			{
				"resource": map[string]any{"attributes": otlpAttributes(map[string]string{"service.name": o.service})},
				"scopeMetrics": []map[string]any{
					{
						"scope":   map[string]any{"name": "karakuri"},
						"metrics": metrics,
					},
				},
			},
		},
	}
	_ = dataPoints // silence unused if compiler peeks ahead
	return o.post(ctx, o.endpoint+"/v1/metrics", body)
}

func (o *OTLPExporter) ExportLogs(ctx context.Context, records []LogRecord) error {
	if !o.Active() || len(records) == 0 {
		return nil
	}
	logs := make([]map[string]any, 0, len(records))
	for _, r := range records {
		logs = append(logs, map[string]any{
			"timeUnixNano":   fmt.Sprintf("%d", r.Timestamp.UnixNano()),
			"severityText":   r.Level,
			"severityNumber": severityNumber(r.Level),
			"body":           map[string]string{"stringValue": r.Message},
			"attributes":     otlpAttributes(r.Labels),
		})
	}
	body := map[string]any{
		"resourceLogs": []map[string]any{
			{
				"resource": map[string]any{"attributes": otlpAttributes(map[string]string{"service.name": o.service})},
				"scopeLogs": []map[string]any{
					{
						"scope":      map[string]any{"name": "karakuri"},
						"logRecords": logs,
					},
				},
			},
		},
	}
	return o.post(ctx, o.endpoint+"/v1/logs", body)
}

func (o *OTLPExporter) Flush(_ context.Context) error    { return nil }
func (o *OTLPExporter) Shutdown(_ context.Context) error { return nil }

func (o *OTLPExporter) post(ctx context.Context, url string, body any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range o.headers {
		req.Header.Set(k, v)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("otlp: post %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return fmt.Errorf("%w: otlp %d: %s", ErrPermanent, resp.StatusCode, string(respBody))
		}
		return fmt.Errorf("otlp: %s -> %d: %s", url, resp.StatusCode, string(respBody))
	}
	return nil
}

// otlpAttributes converts a Go map into OTLP's verbose
// [{"key":"k","value":{"stringValue":"v"}}] attribute array form.
func otlpAttributes(m map[string]string) []map[string]any {
	out := make([]map[string]any, 0, len(m))
	for k, v := range m {
		out = append(out, map[string]any{
			"key":   k,
			"value": map[string]any{"stringValue": v},
		})
	}
	return out
}

// severityNumber maps a textual log level to OTel's numeric severity enum.
// OTel SeverityNumber values: TRACE=1..4, DEBUG=5..8, INFO=9..12,
// WARN=13..16, ERROR=17..20, FATAL=21..24. We pick the middle of each band.
func severityNumber(level string) int {
	switch strings.ToLower(level) {
	case "trace":
		return 2
	case "debug":
		return 6
	case "info", "":
		return 10
	case "warn", "warning":
		return 14
	case "error", "err":
		return 18
	case "fatal", "panic", "critical":
		return 22
	}
	return 10
}
