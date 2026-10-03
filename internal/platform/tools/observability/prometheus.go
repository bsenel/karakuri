package observability

import (
	"context"
	"fmt"
	"net/http"
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
		url:         url,
		bearerToken: bearerToken,
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *Prometheus) Name() string { return "prometheus" }

func (p *Prometheus) Active() bool { return p.url != "" }

func (p *Prometheus) GetAlerts(ctx context.Context, env, service string, since time.Time, threshold string) ([]Alert, error) {
	return nil, fmt.Errorf("prometheus: GetAlerts not implemented")
}

func (p *Prometheus) FetchLogs(ctx context.Context, q LogQuery) ([]LogLine, error) {
	return nil, fmt.Errorf("prometheus: FetchLogs not implemented")
}

func (p *Prometheus) FetchMetrics(ctx context.Context, q MetricQuery) ([]MetricSeries, error) {
	return nil, fmt.Errorf("prometheus: FetchMetrics not implemented")
}
