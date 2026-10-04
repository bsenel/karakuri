package observability

import (
	"context"
	"errors"
	"time"
)

// Alert states, normalised across providers.
const (
	AlertFiring       = "firing"
	AlertAcknowledged = "acknowledged"
	AlertResolved     = "resolved"
)

// ErrUnsupported is what an adapter returns, wrapped with its own name, for a
// kind of signal its backend does not have.
//
// An adapter must never return an empty result for something it cannot see:
// empty means it looked and found nothing.
var ErrUnsupported = errors.New("signal not supported by this observability backend")

type Alert struct {
	// ID is a stable identity for the alert: the provider's fingerprint when it
	// has one, otherwise derived deterministically from the alert's name and
	// sorted labels, never from a timestamp or a value.
	ID       string
	Service  string
	Severity string
	Message  string
	// State is one of AlertFiring, AlertAcknowledged or AlertResolved.
	State string
	Time  time.Time
}

type LogQuery struct {
	Service string
	Query   string
	Since   time.Time
	Limit   int
}

type LogLine struct {
	Time    time.Time
	Service string
	Message string
}

type MetricQuery struct {
	Query string
	Since time.Time
	Step  time.Duration
}

type MetricSeries struct {
	Name   string
	Labels map[string]string
	Points []MetricPoint
}

type MetricPoint struct {
	Time  time.Time
	Value float64
}

type ObservabilityAdapter interface {
	GetAlerts(ctx context.Context, env, service string, since time.Time, threshold string) ([]Alert, error)
	FetchLogs(ctx context.Context, q LogQuery) ([]LogLine, error)
	FetchMetrics(ctx context.Context, q MetricQuery) ([]MetricSeries, error)
	Active() bool
}
