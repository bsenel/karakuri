package email

import (
	"context"
	"log/slog"
)

type NoOp struct{}

func NewNoOp() *NoOp { return &NoOp{} }

func (n *NoOp) Name() string { return "noop" }

func (n *NoOp) Active() bool { return false }

func (n *NoOp) Send(ctx context.Context, msg Message) (string, error) {
	slog.WarnContext(ctx, "EmailAdapter not configured: send skipped",
		"to", msg.To, "subject", msg.Subject)
	return "", nil
}

func (n *NoOp) List(ctx context.Context, query string, _ int) ([]Message, error) {
	slog.WarnContext(ctx, "EmailAdapter not configured: list skipped", "query", query)
	return nil, nil
}
