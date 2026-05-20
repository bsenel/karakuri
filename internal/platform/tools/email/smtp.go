package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"time"
)

// SMTP is a send-only EmailAdapter using net/smtp. Works with iCloud, Fastmail,
// ProtonMail Bridge, corporate SMTP servers, or any RFC 5321 endpoint.
// Port chooses TLS strategy: 465 = implicit TLS, 587 = STARTTLS, anything else = plain.
type SMTP struct {
	host        string
	port        int
	username    string
	password    string
	fromAddress string
}

func NewSMTP(host string, port int, username, password, fromAddress string) *SMTP {
	if port == 0 {
		port = 587
	}
	return &SMTP{host: host, port: port, username: username, password: password, fromAddress: fromAddress}
}

func (s *SMTP) Name() string { return "smtp" }

func (s *SMTP) Active() bool { return s.host != "" && s.username != "" }

func (s *SMTP) Send(ctx context.Context, msg Message) (string, error) {
	from := msg.From
	if from == "" {
		from = s.fromAddress
	}
	if from == "" {
		from = s.username
	}
	if len(msg.To) == 0 {
		return "", fmt.Errorf("smtp: at least one To address required")
	}

	raw := buildRFC2822(from, msg)
	addr := fmt.Sprintf("%s:%d", s.host, s.port)

	switch s.port {
	case 465:
		// Implicit TLS (SMTPS).
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.host})
		if err != nil {
			return "", fmt.Errorf("smtp: tls dial: %w", err)
		}
		client, err := smtp.NewClient(conn, s.host)
		if err != nil {
			return "", fmt.Errorf("smtp: client: %w", err)
		}
		defer client.Quit()
		return "", sendVia(client, s.host, s.username, s.password, from, msg.To, raw)
	case 587:
		// STARTTLS.
		dialer := &net.Dialer{Timeout: 30 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return "", fmt.Errorf("smtp: dial: %w", err)
		}
		client, err := smtp.NewClient(conn, s.host)
		if err != nil {
			return "", fmt.Errorf("smtp: client: %w", err)
		}
		defer client.Quit()
		if err := client.StartTLS(&tls.Config{ServerName: s.host}); err != nil {
			return "", fmt.Errorf("smtp: starttls: %w", err)
		}
		return "", sendVia(client, s.host, s.username, s.password, from, msg.To, raw)
	default:
		// Plain — use stdlib SendMail.
		auth := smtp.PlainAuth("", s.username, s.password, s.host)
		if err := smtp.SendMail(addr, auth, from, msg.To, []byte(raw)); err != nil {
			return "", fmt.Errorf("smtp: send: %w", err)
		}
		return "", nil
	}
}

func (s *SMTP) List(_ context.Context, _ string, _ int) ([]Message, error) {
	return nil, fmt.Errorf("smtp: List requires IMAP; use a different EmailAdapter for read operations")
}

func sendVia(client *smtp.Client, host, username, password, from string, to []string, raw string) error {
	auth := smtp.PlainAuth("", username, password, host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp: auth: %w", err)
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp: MAIL FROM: %w", err)
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp: RCPT TO %s: %w", rcpt, err)
		}
	}
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp: DATA: %w", err)
	}
	if _, err := wc.Write([]byte(raw)); err != nil {
		return fmt.Errorf("smtp: write: %w", err)
	}
	return wc.Close()
}
