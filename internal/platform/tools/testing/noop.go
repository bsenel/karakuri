package testing

import (
	"context"
	"log/slog"
)

type NoOp struct{}

func NewNoOp() *NoOp { return &NoOp{} }

func (n *NoOp) Active() bool { return false }

func (n *NoOp) RunTests(ctx context.Context, path string) ([]TestResult, error) {
	slog.WarnContext(ctx, "TestingAdapter not configured: test run skipped", "path", path)
	return nil, nil
}
