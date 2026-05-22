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

// DatadogExporter ships metrics + logs to Datadog over their HTTP API.
// No third-party SDK; pure net/http so the dep footprint stays small.
//
// Configuration (all env-var driven):
//
//	DD_API_KEY   — required to enable the exporter; unset = inactive
//	DD_SITE      — datadoghq.com (default) | datadoghq.eu | us3.datadoghq.com | …
//	DD_HOSTNAME  — host label attached to every metric/log; defaults to os.Hostname()
//	DD_TAGS      — comma-separated key:value tags appended to every metric/log
//
// When DD_API_KEY is unset the exporter is inactive — `ExportMetrics`/`ExportLogs`
// silently return nil so the OTel flush loop keeps moving and the chain
// remains intact for other exporters.
type DatadogExporter struct {
	apiKey   string
	site     string
	hostname string
	tags     []string
	client   *http.Client
}

func NewDatadogExporter() *DatadogExporter {
	site := os.Getenv("DD_SITE")
	if site == "" {
		site = "datadoghq.com"
	}
	host := os.Getenv("DD_HOSTNAME")
	if host == "" {
		host, _ = os.Hostname()
	}
	var tags []string
	if v := os.Getenv("DD_TAGS"); v != "" {
		for _, t := range strings.Split(v, ",") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
	}
	return &DatadogExporter{
		apiKey:   os.Getenv("DD_API_KEY"),
		site:     site,
		hostname: host,
		tags:     tags,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (d *DatadogExporter) Name() string { return "datadog" }

func (d *DatadogExporter) Active() bool { return d.apiKey != "" }

func (d *DatadogExporter) ExportMetrics(ctx context.Context, records []MetricRecord) error {
	if !d.Active() || len(records) == 0 {
		return nil
	}
	type series struct {
		Metric string     `json:"metric"`
		Points [][]any    `json:"points"`
		Type   string     `json:"type"`
		Host   string     `json:"host,omitempty"`
		Tags   []string   `json:"tags,omitempty"`
	}
	body := struct {
		Series []series `json:"series"`
	}{}
	for _, r := range records {
		body.Series = append(body.Series, series{
			Metric: r.Name,
			Points: [][]any{{r.Timestamp.Unix(), r.Value}},
			Type:   "gauge",
			Host:   d.hostname,
			Tags:   append(append([]string{}, d.tags...), tagsFromLabels(r.Labels)...),
		})
	}
	return d.post(ctx, fmt.Sprintf("https://api.%s/api/v1/series", d.site), body)
}

func (d *DatadogExporter) ExportLogs(ctx context.Context, records []LogRecord) error {
	if !d.Active() || len(records) == 0 {
		return nil
	}
	// Datadog accepts a JSON array of log objects.
	type log struct {
		Message  string `json:"message"`
		Status   string `json:"status,omitempty"`
		Hostname string `json:"hostname,omitempty"`
		Service  string `json:"service,omitempty"`
		Ddsource string `json:"ddsource,omitempty"`
		Ddtags   string `json:"ddtags,omitempty"`
		Date     int64  `json:"date,omitempty"`
	}
	logs := make([]log, len(records))
	for i, r := range records {
		tagParts := append([]string{}, d.tags...)
		tagParts = append(tagParts, tagsFromLabels(r.Labels)...)
		logs[i] = log{
			Message:  r.Message,
			Status:   r.Level,
			Hostname: d.hostname,
			Service:  "karakuri",
			Ddsource: "karakuri",
			Ddtags:   strings.Join(tagParts, ","),
			Date:     r.Timestamp.UnixMilli(),
		}
	}
	return d.post(ctx, fmt.Sprintf("https://http-intake.logs.%s/api/v2/logs", d.site), logs)
}

func (d *DatadogExporter) post(ctx context.Context, url string, body any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", d.apiKey)

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("datadog: post %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("datadog: %s -> %d: %s", url, resp.StatusCode, string(body))
	}
	return nil
}

func (d *DatadogExporter) Flush(_ context.Context) error    { return nil }
func (d *DatadogExporter) Shutdown(_ context.Context) error { return nil }

// tagsFromLabels flattens a label map into Datadog's `key:value` tag form.
func tagsFromLabels(labels map[string]string) []string {
	out := make([]string, 0, len(labels))
	for k, v := range labels {
		out = append(out, k+":"+v)
	}
	return out
}
