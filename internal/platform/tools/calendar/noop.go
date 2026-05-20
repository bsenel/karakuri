package calendar

import (
	"context"
	"log/slog"
	"time"
)

type NoOp struct{}

func NewNoOp() *NoOp { return &NoOp{} }

func (n *NoOp) Name() string { return "noop" }

func (n *NoOp) Active() bool { return false }

func (n *NoOp) ListEvents(ctx context.Context, calendarID string, _, _ time.Time) ([]Event, error) {
	slog.WarnContext(ctx, "CalendarAdapter not configured", "calendar", calendarID)
	return nil, nil
}

func (n *NoOp) CreateEvent(ctx context.Context, calendarID string, event Event) (string, error) {
	slog.WarnContext(ctx, "CalendarAdapter not configured: event creation skipped",
		"calendar", calendarID, "title", event.Title)
	return "", nil
}
