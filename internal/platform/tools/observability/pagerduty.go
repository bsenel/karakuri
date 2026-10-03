package observability

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// PagerDuty is an ObservabilityAdapter backed by the PagerDuty REST API.
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

func (p *PagerDuty) GetAlerts(ctx context.Context, env, service string, since time.Time, threshold string) ([]Alert, error) {
	return nil, errors.New("pagerduty: not implemented")
}

func (p *PagerDuty) FetchLogs(ctx context.Context, q LogQuery) ([]LogLine, error) {
	return nil, errors.New("pagerduty: not implemented")
}

func (p *PagerDuty) FetchMetrics(ctx context.Context, q MetricQuery) ([]MetricSeries, error) {
	return nil, errors.New("pagerduty: not implemented")
}
