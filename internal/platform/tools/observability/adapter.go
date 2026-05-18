package observability

import (
	"context"
	"time"
)

type Alert struct {
	Service  string
	Severity string
	Message  string
	Time     time.Time
}

type ObservabilityAdapter interface {
	GetAlerts(ctx context.Context, env, service string, since time.Time, threshold string) ([]Alert, error)
	Active() bool
}
