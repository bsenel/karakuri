package email

import (
	"context"
	"time"
)

// Message is the cross-provider email model surfaced to capabilities.
type Message struct {
	ID      string
	From    string
	To      []string
	Cc      []string
	Subject string
	Body    string // plain text; HTML support TBD
	SentAt  time.Time
}

// EmailAdapter exposes the minimum operations needed by Karakuri:
// listing recent messages (for observation) and sending new messages
// (for the act step — e.g. notification, status digests).
type EmailAdapter interface {
	Name() string
	Active() bool

	Send(ctx context.Context, msg Message) (id string, err error)
	List(ctx context.Context, query string, limit int) ([]Message, error)
}
