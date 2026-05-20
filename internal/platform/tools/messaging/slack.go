package messaging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Slack is a MessagingAdapter backed by the Slack Web API (chat.postMessage,
// conversations.history). Auth uses a Bot Token (xoxb-...).
type Slack struct {
	token          string
	defaultChannel string
	client         *http.Client
}

func NewSlack(token, defaultChannel string) *Slack {
	return &Slack{
		token:          token,
		defaultChannel: defaultChannel,
		client:         &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *Slack) Name() string { return "slack" }

func (s *Slack) Active() bool { return s.token != "" }

func (s *Slack) PostMessage(ctx context.Context, channel, text string) error {
	if channel == "" {
		channel = s.defaultChannel
	}
	if channel == "" {
		return fmt.Errorf("slack: no channel specified and no default configured")
	}
	body, _ := json.Marshal(map[string]string{"channel": channel, "text": text})
	req, err := http.NewRequestWithContext(ctx, "POST", "https://slack.com/api/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var sr struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	respBody, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(respBody, &sr); err != nil {
		return fmt.Errorf("slack: malformed response: %s", string(respBody))
	}
	if !sr.OK {
		return fmt.Errorf("slack: chat.postMessage failed: %s", sr.Error)
	}
	return nil
}

func (s *Slack) GetMessages(ctx context.Context, channel string, since time.Time) ([]Message, error) {
	if channel == "" {
		channel = s.defaultChannel
	}
	if channel == "" {
		return nil, fmt.Errorf("slack: no channel specified")
	}
	q := url.Values{}
	q.Set("channel", channel)
	q.Set("limit", "50")
	if !since.IsZero() {
		q.Set("oldest", strconv.FormatFloat(float64(since.Unix()), 'f', -1, 64))
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://slack.com/api/conversations.history?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var sr struct {
		OK       bool   `json:"ok"`
		Error    string `json:"error"`
		Messages []struct {
			Text string `json:"text"`
			User string `json:"user"`
			TS   string `json:"ts"`
		} `json:"messages"`
	}
	respBody, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(respBody, &sr); err != nil {
		return nil, fmt.Errorf("slack: malformed response: %s", string(respBody))
	}
	if !sr.OK {
		return nil, fmt.Errorf("slack: conversations.history failed: %s", sr.Error)
	}
	out := make([]Message, 0, len(sr.Messages))
	for _, m := range sr.Messages {
		secs, _ := strconv.ParseFloat(m.TS, 64)
		out = append(out, Message{
			Channel: channel,
			Text:    m.Text,
			User:    m.User,
			Time:    time.Unix(int64(secs), 0).UTC(),
		})
	}
	return out, nil
}
