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

// NewRelicExporter ships metrics + logs to New Relic over their HTTP API.
// Pure net/http; no SDK so the dep footprint stays small.
//
// Configuration (env-var driven):
//
//	NEW_RELIC_LICENSE_KEY  — required to enable; unset = inactive
//	NEW_RELIC_REGION       — "us" (default) | "eu"
//	NEW_RELIC_TAGS         — comma-separated key:value tags applied to all data
type NewRelicExporter struct {
	apiKey  string
	region  string
	tags    map[string]string
	client  *http.Client
}

func NewNewRelicExporter() *NewRelicExporter {
	region := strings.ToLower(os.Getenv("NEW_RELIC_REGION"))
	if region == "" {
		region = "us"
	}
	tags := map[string]string{}
	if v := os.Getenv("NEW_RELIC_TAGS"); v != "" {
		for _, kv := range strings.Split(v, ",") {
			if k, val, ok := strings.Cut(strings.TrimSpace(kv), ":"); ok {
				tags[k] = val
			}
		}
	}
	return &NewRelicExporter{
		apiKey: os.Getenv("NEW_RELIC_LICENSE_KEY"),
		region: region,
		tags:   tags,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *NewRelicExporter) Name() string { return "newrelic" }

func (n *NewRelicExporter) Active() bool { return n.apiKey != "" }

func (n *NewRelicExporter) ExportMetrics(ctx context.Context, records []MetricRecord) error {
	if !n.Active() || len(records) == 0 {
		return nil
	}
	type metric struct {
		Name       string            `json:"name"`
		Type       string            `json:"type"`
		Value      float64           `json:"value"`
		Timestamp  int64             `json:"timestamp"`
		Attributes map[string]string `json:"attributes,omitempty"`
	}
	metrics := make([]metric, 0, len(records))
	for _, r := range records {
		attrs := mergedAttrs(n.tags, r.Labels)
		metrics = append(metrics, metric{
			Name: r.Name, Type: "gauge", Value: r.Value,
			Timestamp: r.Timestamp.UnixMilli(), Attributes: attrs,
		})
	}
	body := []map[string]any{{"metrics": metrics}}
	return n.post(ctx, regionURL(n.region, "metric-api", "/metric/v1"), body)
}

func (n *NewRelicExporter) ExportLogs(ctx context.Context, records []LogRecord) error {
	if !n.Active() || len(records) == 0 {
		return nil
	}
	type logEntry struct {
		Message    string            `json:"message"`
		Level      string            `json:"level,omitempty"`
		Timestamp  int64             `json:"timestamp"`
		Attributes map[string]string `json:"attributes,omitempty"`
	}
	logs := make([]logEntry, 0, len(records))
	for _, r := range records {
		attrs := mergedAttrs(n.tags, r.Labels)
		logs = append(logs, logEntry{
			Message: r.Message, Level: r.Level,
			Timestamp: r.Timestamp.UnixMilli(), Attributes: attrs,
		})
	}
	body := []map[string]any{{"logs": logs}}
	return n.post(ctx, regionURL(n.region, "log-api", "/log/v1"), body)
}

func (n *NewRelicExporter) Flush(_ context.Context) error    { return nil }
func (n *NewRelicExporter) Shutdown(_ context.Context) error { return nil }

func (n *NewRelicExporter) post(ctx context.Context, url string, body any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", n.apiKey)

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("newrelic: post %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		// 401/403 are permanent — bad key. Wrap so RetryExporter short-circuits.
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return fmt.Errorf("%w: newrelic %d: %s", ErrPermanent, resp.StatusCode, string(respBody))
		}
		return fmt.Errorf("newrelic: %s -> %d: %s", url, resp.StatusCode, string(respBody))
	}
	return nil
}

// regionURL builds the full New Relic API URL for a given host prefix +
// path. US uses bare `metric-api.newrelic.com`; EU/staging use the regional
// subdomain `metric-api.eu.newrelic.com`.
func regionURL(region, hostPrefix, path string) string {
	switch region {
	case "eu":
		return fmt.Sprintf("https://%s.eu.newrelic.com%s", hostPrefix, path)
	case "staging":
		return fmt.Sprintf("https://%s.staging.newrelic.com%s", hostPrefix, path)
	default: // "us" or anything else
		return fmt.Sprintf("https://%s.newrelic.com%s", hostPrefix, path)
	}
}

// mergedAttrs combines static exporter-level tags with per-record labels;
// per-record labels override on key collision.
func mergedAttrs(static, perRecord map[string]string) map[string]string {
	if len(static) == 0 && len(perRecord) == 0 {
		return nil
	}
	out := make(map[string]string, len(static)+len(perRecord))
	for k, v := range static {
		out[k] = v
	}
	for k, v := range perRecord {
		out[k] = v
	}
	return out
}
