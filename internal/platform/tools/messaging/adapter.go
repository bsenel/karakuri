package messaging

import (
	"context"
	"time"
)

type Message struct {
	Channel string
	Text    string
	User    string
	Time    time.Time
}

type MessagingAdapter interface {
	GetMessages(ctx context.Context, channel string, since time.Time) ([]Message, error)
	PostMessage(ctx context.Context, channel, text string) error
	Active() bool
}
