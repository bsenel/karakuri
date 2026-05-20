package email

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Gmail is an EmailAdapter backed by the Gmail API v1.
// Auth uses a Bearer OAuth 2.0 access token (scope gmail.send + gmail.readonly).
type Gmail struct {
	oauthToken  string
	fromAddress string // RFC 5322 From header value; required for Send
	client      *http.Client
}

func NewGmail(oauthToken, fromAddress string) *Gmail {
	return &Gmail{
		oauthToken:  oauthToken,
		fromAddress: fromAddress,
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *Gmail) Name() string { return "gmail" }

func (g *Gmail) Active() bool { return g.oauthToken != "" }

func (g *Gmail) Send(ctx context.Context, msg Message) (string, error) {
	from := msg.From
	if from == "" {
		from = g.fromAddress
	}
	if from == "" {
		return "", fmt.Errorf("gmail: from address required (set tools.gmail.from_address or pass Message.From)")
	}
	if len(msg.To) == 0 {
		return "", fmt.Errorf("gmail: at least one To address required")
	}

	raw := buildRFC2822(from, msg)
	body := map[string]string{
		"raw": base64.URLEncoding.EncodeToString([]byte(raw)),
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := g.do(ctx, "POST", "/gmail/v1/users/me/messages/send", body, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (g *Gmail) List(ctx context.Context, query string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 20
	}
	q := url.Values{}
	q.Set("maxResults", strconv.Itoa(limit))
	if query != "" {
		q.Set("q", query)
	}
	var list struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := g.do(ctx, "GET", "/gmail/v1/users/me/messages?"+q.Encode(), nil, &list); err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(list.Messages))
	for _, m := range list.Messages {
		msg, err := g.getMessage(ctx, m.ID)
		if err != nil {
			continue
		}
		out = append(out, msg)
	}
	return out, nil
}

func (g *Gmail) getMessage(ctx context.Context, id string) (Message, error) {
	var raw struct {
		ID           string `json:"id"`
		InternalDate string `json:"internalDate"`
		Payload      struct {
			Headers []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
		} `json:"payload"`
		Snippet string `json:"snippet"`
	}
	if err := g.do(ctx, "GET", "/gmail/v1/users/me/messages/"+id+"?format=metadata&metadataHeaders=From&metadataHeaders=To&metadataHeaders=Subject", nil, &raw); err != nil {
		return Message{}, err
	}
	msg := Message{ID: raw.ID, Body: raw.Snippet}
	for _, h := range raw.Payload.Headers {
		switch strings.ToLower(h.Name) {
		case "from":
			msg.From = h.Value
		case "to":
			msg.To = splitAddresses(h.Value)
		case "subject":
			msg.Subject = h.Value
		}
	}
	if ts, err := strconv.ParseInt(raw.InternalDate, 10, 64); err == nil {
		msg.SentAt = time.UnixMilli(ts).UTC()
	}
	return msg, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func (g *Gmail) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://gmail.googleapis.com"+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.oauthToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("gmail: %s %s -> %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}

func buildRFC2822(from string, m Message) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "From: %s\r\n", from)
	fmt.Fprintf(&sb, "To: %s\r\n", strings.Join(m.To, ", "))
	if len(m.Cc) > 0 {
		fmt.Fprintf(&sb, "Cc: %s\r\n", strings.Join(m.Cc, ", "))
	}
	fmt.Fprintf(&sb, "Subject: %s\r\n", m.Subject)
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(m.Body)
	return sb.String()
}

func splitAddresses(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
