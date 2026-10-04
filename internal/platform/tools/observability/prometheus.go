package observability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Prometheus is an ObservabilityAdapter backed by the Prometheus HTTP API.
type Prometheus struct {
	url         string
	bearerToken string
	client      *http.Client
}

func NewPrometheus(url, bearerToken string) *Prometheus {
	return &Prometheus{
		url:         strings.TrimSuffix(url, "/"),
		bearerToken: bearerToken,
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *Prometheus) Name() string { return "prometheus" }

func (p *Prometheus) Active() bool { return p.url != "" }

// severityRank orders the severities a threshold can name.
var severityRank = map[string]int{"info": 0, "warning": 1, "error": 2, "critical": 3}

// GetAlerts returns the firing alerts from /api/v1/alerts. Filters apply in
// this order:
//
//  1. state: only firing alerts are kept; pending and inactive are skipped.
//  2. service: when non-empty it must equal the alert's Service (the `service`
//     label, falling back to `job`).
//  3. since: when non-zero, alerts whose activeAt is before it are dropped.
//  4. threshold: a minimum severity in the order info < warning < error <
//     critical, case-insensitive. Empty means no filter. An alert whose
//     severity is missing or not in that order is kept. A non-empty threshold
//     not in that order is an error.
//
// env has no Prometheus equivalent and is ignored: no label is assumed to
// carry it.
func (p *Prometheus) GetAlerts(ctx context.Context, env, service string, since time.Time, threshold string) ([]Alert, error) {
	minRank := 0
	if threshold != "" {
		rank, ok := severityRank[strings.ToLower(threshold)]
		if !ok {
			return nil, fmt.Errorf("prometheus: unknown severity threshold %q", threshold)
		}
		minRank = rank
	}

	var data struct {
		Alerts *[]struct {
			Labels      map[string]string `json:"labels"`
			Annotations map[string]string `json:"annotations"`
			State       string            `json:"state"`
			ActiveAt    string            `json:"activeAt"`
		} `json:"alerts"`
	}
	if err := p.get(ctx, "/api/v1/alerts", nil, &data); err != nil {
		return nil, err
	}
	if data.Alerts == nil {
		return nil, fmt.Errorf("prometheus: alerts response has no alerts list")
	}

	alerts := make([]Alert, 0, len(*data.Alerts))
	for _, a := range *data.Alerts {
		if a.State != "firing" {
			continue
		}
		svc := a.Labels["service"]
		if svc == "" {
			svc = a.Labels["job"]
		}
		if service != "" && svc != service {
			continue
		}
		activeAt, err := time.Parse(time.RFC3339Nano, a.ActiveAt)
		if err != nil {
			return nil, fmt.Errorf("prometheus: alert %q activeAt: %w", a.Labels["alertname"], err)
		}
		if !since.IsZero() && activeAt.Before(since) {
			continue
		}
		severity := a.Labels["severity"]
		if rank, ok := severityRank[strings.ToLower(severity)]; ok && rank < minRank {
			continue
		}
		message := a.Annotations["summary"]
		if message == "" {
			message = a.Annotations["description"]
		}
		if message == "" {
			message = a.Labels["alertname"]
		}
		alerts = append(alerts, Alert{
			ID:       labelsID(a.Labels),
			Service:  svc,
			Severity: severity,
			Message:  message,
			State:    AlertFiring,
			Time:     activeAt,
		})
	}
	return alerts, nil
}

// FetchLogs is unsupported: Prometheus holds no logs.
func (p *Prometheus) FetchLogs(ctx context.Context, q LogQuery) ([]LogLine, error) {
	return nil, fmt.Errorf("prometheus: logs: %w", ErrUnsupported)
}

// FetchMetrics runs q.Query over /api/v1/query_range from q.Since until now,
// at q.Step resolution (60 seconds when zero).
func (p *Prometheus) FetchMetrics(ctx context.Context, q MetricQuery) ([]MetricSeries, error) {
	if q.Query == "" {
		return nil, fmt.Errorf("prometheus: metrics: query is required")
	}
	step := q.Step
	if step == 0 {
		step = 60 * time.Second
	}
	params := url.Values{
		"query": {q.Query},
		"start": {strconv.FormatInt(q.Since.Unix(), 10)},
		"end":   {strconv.FormatInt(time.Now().Unix(), 10)},
		"step":  {strconv.FormatFloat(step.Seconds(), 'f', -1, 64)},
	}

	var data struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string    `json:"metric"`
			Values [][2]json.RawMessage `json:"values"`
		} `json:"result"`
	}
	if err := p.get(ctx, "/api/v1/query_range", params, &data); err != nil {
		return nil, err
	}
	if data.ResultType != "matrix" {
		return nil, fmt.Errorf("prometheus: metrics: unexpected result type %q, want matrix", data.ResultType)
	}

	series := make([]MetricSeries, 0, len(data.Result))
	for _, r := range data.Result {
		s := MetricSeries{
			Name:   r.Metric["__name__"],
			Labels: make(map[string]string, len(r.Metric)),
			Points: make([]MetricPoint, 0, len(r.Values)),
		}
		for k, v := range r.Metric {
			if k != "__name__" {
				s.Labels[k] = v
			}
		}
		for _, pair := range r.Values {
			var ts float64
			if err := json.Unmarshal(pair[0], &ts); err != nil {
				return nil, fmt.Errorf("prometheus: metrics: series %q timestamp %s: %w", s.Name, pair[0], err)
			}
			var raw string
			if err := json.Unmarshal(pair[1], &raw); err != nil {
				return nil, fmt.Errorf("prometheus: metrics: series %q value %s: %w", s.Name, pair[1], err)
			}
			value, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return nil, fmt.Errorf("prometheus: metrics: series %q value %q: %w", s.Name, raw, err)
			}
			sec, frac := math.Modf(ts)
			s.Points = append(s.Points, MetricPoint{Time: time.Unix(int64(sec), int64(frac*1e9)), Value: value})
		}
		series = append(series, s)
	}
	return series, nil
}

// labelsID is the lowercase hex sha256 of the labels sorted by key.
func labelsID(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(labels[k]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// get issues a GET against the Prometheus API and decodes the envelope's data
// into out.
func (p *Prometheus) get(ctx context.Context, path string, params url.Values, out any) error {
	target := p.url + path
	if len(params) > 0 {
		target += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("prometheus: %w", err)
	}
	if p.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.bearerToken)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("prometheus: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("prometheus: %s: status %d", path, resp.StatusCode)
	}
	body, err := readBounded(resp.Body)
	if err != nil {
		return fmt.Errorf("prometheus: %s: %w", path, err)
	}

	var envelope struct {
		Status    string          `json:"status"`
		ErrorType string          `json:"errorType"`
		Error     string          `json:"error"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("prometheus: %s: decode response: %w", path, err)
	}
	if envelope.Status != "success" {
		return fmt.Errorf("prometheus: %s: status %q: %s: %s", path, envelope.Status, envelope.ErrorType, envelope.Error)
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("prometheus: %s: decode data: %w", path, err)
	}
	return nil
}
