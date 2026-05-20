package email

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

// Outlook is an EmailAdapter backed by Microsoft Graph (Outlook / M365 mailbox).
// Auth: OAuth 2.0 Bearer token with Mail.Send + Mail.Read scopes.
type Outlook struct {
	oauthToken  string
	fromAddress string
	client      *http.Client
}

func NewOutlook(oauthToken, fromAddress string) *Outlook {
	return &Outlook{
		oauthToken:  oauthToken,
		fromAddress: fromAddress,
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (o *Outlook) Name() string { return "outlook" }

func (o *Outlook) Active() bool { return o.oauthToken != "" }

func (o *Outlook) Send(ctx context.Context, msg Message) (string, error) {
	if len(msg.To) == 0 {
		return "", fmt.Errorf("outlook: at least one To address required")
	}
	body := map[string]any{
		"message": map[string]any{
			"subject":       msg.Subject,
			"body":          map[string]string{"contentType": "Text", "content": msg.Body},
			"toRecipients":  toRecipients(msg.To),
			"ccRecipients":  toRecipients(msg.Cc),
		},
		"saveToSentItems": true,
	}
	// Graph sendMail returns 202 Accepted with no body and no message id;
	// we report success without an id.
	if err := o.do(ctx, "POST", "/v1.0/me/sendMail", body, nil); err != nil {
		return "", err
	}
	return "", nil
}

func (o *Outlook) List(ctx context.Context, query string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 20
	}
	q := url.Values{}
	q.Set("$top", strconv.Itoa(limit))
	q.Set("$select", "id,subject,from,toRecipients,receivedDateTime,bodyPreview")
	if query != "" {
		q.Set("$search", `"`+query+`"`)
	}
	var raw struct {
		Value []struct {
			ID      string `json:"id"`
			Subject string `json:"subject"`
			From    struct {
				EmailAddress struct {
					Address string `json:"address"`
				} `json:"emailAddress"`
			} `json:"from"`
			ToRecipients     []graphRecipient `json:"toRecipients"`
			ReceivedDateTime time.Time        `json:"receivedDateTime"`
			BodyPreview      string           `json:"bodyPreview"`
		} `json:"value"`
	}
	if err := o.do(ctx, "GET", "/v1.0/me/messages?"+q.Encode(), nil, &raw); err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(raw.Value))
	for _, m := range raw.Value {
		to := make([]string, 0, len(m.ToRecipients))
		for _, r := range m.ToRecipients {
			to = append(to, r.EmailAddress.Address)
		}
		out = append(out, Message{
			ID:      m.ID,
			From:    m.From.EmailAddress.Address,
			To:      to,
			Subject: m.Subject,
			Body:    m.BodyPreview,
			SentAt:  m.ReceivedDateTime,
		})
	}
	return out, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

type graphRecipient struct {
	EmailAddress struct {
		Address string `json:"address"`
	} `json:"emailAddress"`
}

func toRecipients(addrs []string) []map[string]map[string]string {
	out := make([]map[string]map[string]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, map[string]map[string]string{"emailAddress": {"address": a}})
	}
	return out
}

func (o *Outlook) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://graph.microsoft.com"+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+o.oauthToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("outlook: %s %s -> %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}
