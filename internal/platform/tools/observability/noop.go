package observability

import (
	"context"
	"log/slog"
	"time"
)

type NoOp struct{}

func NewNoOp() *NoOp { return &NoOp{} }

func (n *NoOp) Active() bool { return false }

func (n *NoOp) GetAlerts(ctx context.Context, env, service string, since time.Time, threshold string) ([]Alert, error) {
	slog.WarnContext(ctx, "ObservabilityAdapter not configured", "env", env, "service", service)
	return nil, nil
}
