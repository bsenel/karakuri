package observability

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
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

var errLokiNotImplemented = errors.New("loki: not implemented")

func (l *Loki) GetAlerts(ctx context.Context, env, service string, since time.Time, threshold string) ([]Alert, error) {
	return nil, errLokiNotImplemented
}

func (l *Loki) FetchLogs(ctx context.Context, q LogQuery) ([]LogLine, error) {
	return nil, errLokiNotImplemented
}

func (l *Loki) FetchMetrics(ctx context.Context, q MetricQuery) ([]MetricSeries, error) {
	return nil, errLokiNotImplemented
}
