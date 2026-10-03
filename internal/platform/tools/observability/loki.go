package observability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	lokiQueryRangePath = "/loki/api/v1/query_range"
	lokiDefaultLimit   = 200
	lokiMaxLimit       = 1000
)

// Loki is an ObservabilityAdapter backed by the Loki HTTP API.
type Loki struct {
	url         string
	bearerToken string
	tenant      string
	client      *http.Client
}

func NewLoki(url, bearerToken, tenant string) *Loki {
	return &Loki{
		url:         strings.TrimSuffix(url, "/"),
		bearerToken: bearerToken,
		tenant:      tenant,
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (l *Loki) Name() string { return "loki" }

func (l *Loki) Active() bool { return l.url != "" }

// GetAlerts is unsupported: Loki holds log lines, not alerts.
func (l *Loki) GetAlerts(ctx context.Context, env, service string, since time.Time, threshold string) ([]Alert, error) {
	return nil, fmt.Errorf("loki: alerts: %w", ErrUnsupported)
}

// FetchLogs runs q.Query as LogQL, or selects q.Service's streams when no
// query is given, and returns the lines newest first across streams.
func (l *Loki) FetchLogs(ctx context.Context, q LogQuery) ([]LogLine, error) {
	logQL := q.Query
	if logQL == "" {
		if q.Service == "" {
			// Never fall back to a selector that matches everything.
			return nil, errors.New("loki: logs: a query or a service is required")
		}
		logQL = `{service=` + strconv.Quote(q.Service) + `}`
	}
	limit := q.Limit
	if limit <= 0 {
		limit = lokiDefaultLimit
	}
	if limit > lokiMaxLimit {
		limit = lokiMaxLimit
	}

	params := url.Values{}
	params.Set("query", logQL)
	params.Set("start", strconv.FormatInt(q.Since.UnixNano(), 10))
	params.Set("limit", strconv.Itoa(limit))
	params.Set("direction", "backward")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.url+lokiQueryRangePath+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("loki: %w", err)
	}
	if l.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+l.bearerToken)
	}
	if l.tenant != "" {
		req.Header.Set("X-Scope-OrgID", l.tenant)
	}

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("loki: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("loki: %s: status %d", lokiQueryRangePath, resp.StatusCode)
	}
	body, err := readBounded(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("loki: %s: %w", lokiQueryRangePath, err)
	}

	var envelope struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Stream map[string]string `json:"stream"`
				Values [][2]string       `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("loki: %s: decode response: %w", lokiQueryRangePath, err)
	}
	if envelope.Status != "success" {
		return nil, fmt.Errorf("loki: %s: status %q", lokiQueryRangePath, envelope.Status)
	}

	lines := []LogLine{}
	for _, stream := range envelope.Data.Result {
		service := stream.Stream["service"]
		if service == "" {
			service = q.Service
		}
		for _, value := range stream.Values {
			ns, err := strconv.ParseInt(value[0], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("loki: %s: timestamp %q: %w", lokiQueryRangePath, value[0], err)
			}
			lines = append(lines, LogLine{Time: time.Unix(0, ns), Service: service, Message: value[1]})
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].Time.After(lines[j].Time) })
	return lines, nil
}

// FetchMetrics is unsupported: metric queries belong to a metrics backend.
func (l *Loki) FetchMetrics(ctx context.Context, q MetricQuery) ([]MetricSeries, error) {
	return nil, fmt.Errorf("loki: metrics: %w", ErrUnsupported)
}
