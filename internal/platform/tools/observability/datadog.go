package observability

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Datadog is an ObservabilityAdapter backed by the Datadog HTTP API.
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

func (d *Datadog) Active() bool { return false }

func (d *Datadog) GetAlerts(ctx context.Context, env, service string, since time.Time, threshold string) ([]Alert, error) {
	return nil, fmt.Errorf("datadog: not implemented")
}

func (d *Datadog) FetchLogs(ctx context.Context, q LogQuery) ([]LogLine, error) {
	return nil, fmt.Errorf("datadog: not implemented")
}

func (d *Datadog) FetchMetrics(ctx context.Context, q MetricQuery) ([]MetricSeries, error) {
	return nil, fmt.Errorf("datadog: not implemented")
}
