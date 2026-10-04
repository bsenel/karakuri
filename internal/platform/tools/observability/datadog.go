package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Datadog is an ObservabilityAdapter backed by the Datadog HTTP API.
//
// Monitors are the alert source: GetAlerts reads the monitors' current state.
// Datadog events are not read.
type Datadog struct {
	apiKey  string
	appKey  string
	baseURL string
	client  *http.Client
}

func NewDatadog(apiKey, appKey, site string) *Datadog {
	if site == "" {
		site = "datadoghq.com"
	}
	return &Datadog{
		apiKey:  apiKey,
		appKey:  appKey,
		baseURL: "https://api." + site,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (d *Datadog) Name() string { return "datadog" }

func (d *Datadog) Active() bool { return d.apiKey != "" && d.appKey != "" }

// GetAlerts returns the alerting monitors from /api/v1/monitor. Filters apply
// in this order:
//
//  1. state: overall_state "Alert" is a firing alert of severity critical and
//     "Warn" a firing alert of severity warning; every other state (OK, No
//     Data, Ignored, Skipped, Unknown) is skipped.
//  2. env: when non-empty, only monitors carrying the tag `env:<env>` are kept.
//  3. service: when non-empty it must equal the alert's Service (the
//     `service:<name>` tag, empty when absent).
//  4. since: when non-zero, alerts whose time (overall_state_modified, falling
//     back to modified) is before it are dropped.
//  5. threshold: a minimum severity in the order info < warning < error <
//     critical, case-insensitive. Empty means no filter. A non-empty threshold
//     not in that order is an error.
func (d *Datadog) GetAlerts(ctx context.Context, env, service string, since time.Time, threshold string) ([]Alert, error) {
	minRank := 0
	if threshold != "" {
		rank, ok := severityRank[strings.ToLower(threshold)]
		if !ok {
			return nil, fmt.Errorf("datadog: unknown severity threshold %q", threshold)
		}
		minRank = rank
	}

	var monitors []struct {
		ID                   int64    `json:"id"`
		Name                 string   `json:"name"`
		OverallState         string   `json:"overall_state"`
		Tags                 []string `json:"tags"`
		OverallStateModified string   `json:"overall_state_modified"`
		Modified             string   `json:"modified"`
	}
	if err := d.do(ctx, http.MethodGet, "/api/v1/monitor", nil, nil, &monitors); err != nil {
		return nil, err
	}

	alerts := make([]Alert, 0, len(monitors))
	for _, m := range monitors {
		var severity string
		switch m.OverallState {
		case "Alert":
			severity = "critical"
		case "Warn":
			severity = "warning"
		default:
			continue
		}
		var svc string
		inEnv := env == ""
		for _, tag := range m.Tags {
			if name, ok := strings.CutPrefix(tag, "service:"); ok && svc == "" {
				svc = name
			}
			if tag == "env:"+env {
				inEnv = true
			}
		}
		if !inEnv {
			continue
		}
		if service != "" && svc != service {
			continue
		}
		modified := m.OverallStateModified
		if modified == "" {
			modified = m.Modified
		}
		at, err := time.Parse(time.RFC3339Nano, modified)
		if err != nil {
			return nil, fmt.Errorf("datadog: monitor %d modified: %w", m.ID, err)
		}
		if !since.IsZero() && at.Before(since) {
			continue
		}
		if severityRank[severity] < minRank {
			continue
		}
		alerts = append(alerts, Alert{
			ID:       "datadog:monitor:" + strconv.FormatInt(m.ID, 10),
			Service:  svc,
			Severity: severity,
			Message:  m.Name,
			State:    AlertFiring,
			Time:     at,
		})
	}
	return alerts, nil
}

// FetchLogs searches /api/v2/logs/events/search from q.Since, newest first.
// The search is q.Query, or `service:<q.Service>` when q.Query is empty; one
// of the two is required. q.Limit defaults to 200 and is capped at 1000.
func (d *Datadog) FetchLogs(ctx context.Context, q LogQuery) ([]LogLine, error) {
	query := q.Query
	if query == "" && q.Service != "" {
		query = "service:" + q.Service
	}
	if query == "" {
		return nil, fmt.Errorf("datadog: logs: query or service is required")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	type filter struct {
		Query string `json:"query"`
		From  string `json:"from"`
	}
	type page struct {
		Limit int `json:"limit"`
	}
	reqBody, err := json.Marshal(struct {
		Filter filter `json:"filter"`
		Page   page   `json:"page"`
		Sort   string `json:"sort"`
	}{
		Filter: filter{Query: query, From: q.Since.Format(time.RFC3339)},
		Page:   page{Limit: limit},
		Sort:   "-timestamp",
	})
	if err != nil {
		return nil, fmt.Errorf("datadog: logs: encode request: %w", err)
	}

	var data struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Timestamp string `json:"timestamp"`
				Service   string `json:"service"`
				Message   string `json:"message"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := d.do(ctx, http.MethodPost, "/api/v2/logs/events/search", nil, reqBody, &data); err != nil {
		return nil, err
	}

	lines := make([]LogLine, 0, len(data.Data))
	for _, e := range data.Data {
		at, err := time.Parse(time.RFC3339Nano, e.Attributes.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("datadog: logs: event %q timestamp: %w", e.ID, err)
		}
		lines = append(lines, LogLine{Time: at, Service: e.Attributes.Service, Message: e.Attributes.Message})
	}
	return lines, nil
}

// FetchMetrics runs q.Query over /api/v1/query from q.Since until now. q.Step
// has no equivalent in this API, which picks the resolution itself, and is
// ignored.
func (d *Datadog) FetchMetrics(ctx context.Context, q MetricQuery) ([]MetricSeries, error) {
	if q.Query == "" {
		return nil, fmt.Errorf("datadog: metrics: query is required")
	}
	params := url.Values{
		"query": {q.Query},
		"from":  {strconv.FormatInt(q.Since.Unix(), 10)},
		"to":    {strconv.FormatInt(time.Now().Unix(), 10)},
	}

	var data struct {
		Series []struct {
			Metric string   `json:"metric"`
			TagSet []string `json:"tag_set"`
			// A point is [timestamp in milliseconds, value]; the value is null
			// where the series has a gap.
			Pointlist [][]*float64 `json:"pointlist"`
		} `json:"series"`
	}
	if err := d.do(ctx, http.MethodGet, "/api/v1/query", params, nil, &data); err != nil {
		return nil, err
	}

	series := make([]MetricSeries, 0, len(data.Series))
	for _, r := range data.Series {
		s := MetricSeries{
			Name:   r.Metric,
			Labels: make(map[string]string, len(r.TagSet)),
			Points: make([]MetricPoint, 0, len(r.Pointlist)),
		}
		for _, tag := range r.TagSet {
			if k, v, ok := strings.Cut(tag, ":"); ok {
				s.Labels[k] = v
			}
		}
		for _, p := range r.Pointlist {
			if len(p) != 2 || p[0] == nil {
				return nil, fmt.Errorf("datadog: metrics: series %q has a malformed point", s.Name)
			}
			if p[1] == nil {
				continue
			}
			s.Points = append(s.Points, MetricPoint{Time: time.UnixMilli(int64(*p[0])), Value: *p[1]})
		}
		series = append(series, s)
	}
	return series, nil
}

// do issues a request against the Datadog API, sending body as JSON when it is
// non-nil, and decodes the response into out.
func (d *Datadog) do(ctx context.Context, method, path string, params url.Values, body []byte, out any) error {
	target := d.baseURL + path
	if len(params) > 0 {
		target += "?" + params.Encode()
	}
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reqBody)
	if err != nil {
		return fmt.Errorf("datadog: %w", err)
	}
	req.Header.Set("DD-API-KEY", d.apiKey)
	req.Header.Set("DD-APPLICATION-KEY", d.appKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("datadog: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("datadog: %s: status %d", path, resp.StatusCode)
	}
	respBody, err := readBounded(resp.Body)
	if err != nil {
		return fmt.Errorf("datadog: %s: %w", path, err)
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("datadog: %s: decode response: %w", path, err)
	}
	return nil
}
