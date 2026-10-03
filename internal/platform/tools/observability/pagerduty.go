package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// pagerDutyLimit is the page size asked of /incidents, and the most incidents
// one GetAlerts call can return.
const pagerDutyLimit = 100

// PagerDuty is an ObservabilityAdapter backed by the PagerDuty REST API. The
// roadmap names "PagerDuty or Opsgenie"; PagerDuty is the one implemented.
//
// It serves alerts only: PagerDuty holds no logs and no metrics.
type PagerDuty struct {
	token   string
	baseURL string
	client  *http.Client
}

func NewPagerDuty(token string) *PagerDuty {
	return &PagerDuty{
		token:   token,
		baseURL: "https://api.pagerduty.com",
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *PagerDuty) Name() string { return "pagerduty" }

func (p *PagerDuty) Active() bool { return p.token != "" }

// GetAlerts returns the open incidents from /incidents. Filters apply in this
// order:
//
//  1. state: only triggered and acknowledged incidents are asked for, mapped
//     to AlertFiring and AlertAcknowledged; resolved incidents are not read.
//  2. service: when non-empty it must equal the alert's Service (the
//     incident's service.summary).
//  3. since: when non-zero it is sent to PagerDuty, which drops incidents
//     created before it.
//  4. threshold: a minimum severity in the order info < warning < error <
//     critical, case-insensitive. Empty means no filter. Severity comes from
//     the incident's urgency: high is critical, low is warning. A non-empty
//     threshold not in that order is an error.
//
// env has no PagerDuty equivalent and is ignored.
//
// At most one page of 100 incidents is read. When PagerDuty reports more, the
// call is an error and returns no alerts: a partial list must not look complete.
func (p *PagerDuty) GetAlerts(ctx context.Context, env, service string, since time.Time, threshold string) ([]Alert, error) {
	minRank := 0
	if threshold != "" {
		rank, ok := severityRank[strings.ToLower(threshold)]
		if !ok {
			return nil, fmt.Errorf("pagerduty: unknown severity threshold %q", threshold)
		}
		minRank = rank
	}

	params := url.Values{
		"statuses[]": {"triggered", "acknowledged"},
		"limit":      {strconv.Itoa(pagerDutyLimit)},
	}
	if !since.IsZero() {
		params.Set("since", since.Format(time.RFC3339))
	}

	var data struct {
		Incidents []struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			Status    string `json:"status"`
			Urgency   string `json:"urgency"`
			CreatedAt string `json:"created_at"`
			Service   struct {
				Summary string `json:"summary"`
			} `json:"service"`
		} `json:"incidents"`
		More bool `json:"more"`
	}
	if err := p.get(ctx, "/incidents", params, &data); err != nil {
		return nil, err
	}
	if data.More {
		return nil, fmt.Errorf("pagerduty: /incidents: more than %d open incidents, refusing a partial list", pagerDutyLimit)
	}

	alerts := make([]Alert, 0, len(data.Incidents))
	for _, inc := range data.Incidents {
		var state string
		switch inc.Status {
		case "triggered":
			state = AlertFiring
		case "acknowledged":
			state = AlertAcknowledged
		default:
			continue
		}
		if service != "" && inc.Service.Summary != service {
			continue
		}
		createdAt, err := time.Parse(time.RFC3339, inc.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("pagerduty: incident %q created_at: %w", inc.ID, err)
		}
		severity := inc.Urgency
		switch inc.Urgency {
		case "high":
			severity = "critical"
		case "low":
			severity = "warning"
		}
		if rank, ok := severityRank[severity]; ok && rank < minRank {
			continue
		}
		alerts = append(alerts, Alert{
			ID:       "pagerduty:incident:" + inc.ID,
			Service:  inc.Service.Summary,
			Severity: severity,
			Message:  inc.Title,
			State:    state,
			Time:     createdAt,
		})
	}
	return alerts, nil
}

// FetchLogs is unsupported: PagerDuty holds no logs.
func (p *PagerDuty) FetchLogs(ctx context.Context, q LogQuery) ([]LogLine, error) {
	return nil, fmt.Errorf("pagerduty: logs: %w", ErrUnsupported)
}

// FetchMetrics is unsupported: PagerDuty holds no metrics.
func (p *PagerDuty) FetchMetrics(ctx context.Context, q MetricQuery) ([]MetricSeries, error) {
	return nil, fmt.Errorf("pagerduty: metrics: %w", ErrUnsupported)
}

// get issues a GET against the PagerDuty API and decodes the response into out.
func (p *PagerDuty) get(ctx context.Context, path string, params url.Values, out any) error {
	target := p.baseURL + path
	if len(params) > 0 {
		target += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("pagerduty: %w", err)
	}
	req.Header.Set("Authorization", "Token token="+p.token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("pagerduty: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("pagerduty: %s: status %d", path, resp.StatusCode)
	}
	body, err := readBounded(resp.Body)
	if err != nil {
		return fmt.Errorf("pagerduty: %s: %w", path, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("pagerduty: %s: decode response: %w", path, err)
	}
	return nil
}
