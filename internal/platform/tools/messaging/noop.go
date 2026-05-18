package messaging

import (
	"context"
	"log/slog"
	"time"
)

type NoOp struct{}

func NewNoOp() *NoOp { return &NoOp{} }

func (n *NoOp) Active() bool { return false }

func (n *NoOp) GetMessages(ctx context.Context, channel string, since time.Time) ([]Message, error) {
	slog.WarnContext(ctx, "MessagingAdapter not configured", "channel", channel)
	return nil, nil
}

func (n *NoOp) PostMessage(ctx context.Context, channel, text string) error {
	slog.WarnContext(ctx, "MessagingAdapter not configured: post skipped", "channel", channel, "text", text)
	return nil
}
