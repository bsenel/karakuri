package calendar

import (
	"context"
	"time"
)

// Event is the cross-provider event model surfaced to capabilities.
// Specific providers may carry richer fields in their own types.
type Event struct {
	ID          string
	Title       string
	Description string
	Start       time.Time
	End         time.Time
	Attendees   []string // email addresses
	Location    string
}

// CalendarAdapter exposes the minimum operations needed by Karakuri:
// listing upcoming events (for observation) and creating new events
// (for the act step — e.g. scheduling a review meeting).
type CalendarAdapter interface {
	Name() string
	Active() bool

	ListEvents(ctx context.Context, calendarID string, from, to time.Time) ([]Event, error)
	CreateEvent(ctx context.Context, calendarID string, event Event) (string, error)
}
