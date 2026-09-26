package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// sessionHeader is how a streamable-HTTP server names the session it opened at
// initialize. A server that issues one rejects every later request that omits it.
const sessionHeader = "Mcp-Session-Id"

// maxHTTPBody caps what a single reply may be read as. A tool result is text
// bound for a planner prompt; a server answering with a gigabyte is a server
// this process should refuse rather than one it should hold in memory.
const maxHTTPBody = 8 << 20 // 8 MiB

// httpTransport speaks the streamable-HTTP transport: one POST per JSON-RPC
// message, answered either with a JSON body or with an SSE stream carrying the
// response as an event.
//
// Both answers are handled because both are legal and servers differ: the spec
// lets a server reply to a POST with `application/json` when it has the whole
// answer, or with `text/event-stream` when it wants to send progress
// notifications first. A client that accepted only one of them would work
// against half the deployments it was pointed at.
//
// The GET side of the transport — the server-to-client stream a server opens for
// its own requests — is not implemented. This client sends requests and reads
// their replies; sampling and elicitation are server-initiated, and neither has
// a caller here.
type httpTransport struct {
	url     string
	headers map[string]string
	client  *http.Client

	mu      sync.Mutex
	session string
}

func newHTTPTransport(cfg Config) *httpTransport {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &httpTransport{
		url:     cfg.URL,
		headers: cfg.Headers,
		// Per-call deadlines come from the context; the client timeout is the
		// backstop for a server that accepts a connection and never answers.
		client: &http.Client{Timeout: timeout + 5*time.Second},
	}
}

func (t *httpTransport) Kind() string { return TransportHTTP }

func (t *httpTransport) Send(ctx context.Context, req Request) (*Response, error) {
	resp, body, err := t.post(ctx, req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// A session id arrives on the initialize reply and is echoed on everything
	// after it.
	if sid := resp.Header.Get(sessionHeader); sid != "" {
		t.mu.Lock()
		t.session = sid
		t.mu.Unlock()
	}

	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return t.readSSE(body, req.ID)
	}

	raw, err := io.ReadAll(io.LimitReader(body, maxHTTPBody))
	if err != nil {
		return nil, fmt.Errorf("read %s reply: %w", req.Method, err)
	}
	var out Response
	if err := json.Unmarshal(bytes.TrimSpace(raw), &out); err != nil {
		return nil, fmt.Errorf("decode %s reply: %w", req.Method, err)
	}
	return &out, nil
}

func (t *httpTransport) Notify(ctx context.Context, req Request) error {
	resp, _, err := t.post(ctx, req)
	if err != nil {
		return err
	}
	// Nothing to read: a notification has no reply, and the body — 202 Accepted
	// with nothing in it, in practice — is drained so the connection can be
	// reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.Body.Close()
}

func (t *httpTransport) Close() error {
	t.client.CloseIdleConnections()
	return nil
}

// post sends one message and returns the response with its body still open.
func (t *httpTransport) post(ctx context.Context, req Request) (*http.Response, io.Reader, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("encode %s: %w", req.Method, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(payload))
	if err != nil {
		return nil, nil, fmt.Errorf("build %s request: %w", req.Method, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}
	t.mu.Lock()
	session := t.session
	t.mu.Unlock()
	if session != "" {
		httpReq.Header.Set(sessionHeader, session)
	}

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("post %s: %w", req.Method, err)
	}
	if resp.StatusCode >= 400 {
		// The body is where a server explains itself — an expired session, a
		// missing token — and a bare status code sends an operator guessing.
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		_ = resp.Body.Close()
		msg := strings.TrimSpace(string(detail))
		if msg != "" {
			msg = ": " + msg
		}
		return nil, nil, fmt.Errorf("%s returned %s%s", req.Method, resp.Status, msg)
	}
	return resp, resp.Body, nil
}

// readSSE pulls JSON-RPC responses out of an event stream and returns the one
// matching this request, ignoring everything else on it.
//
// Ignoring is the point: the reason a server chooses a stream is to send
// progress notifications before the result, and a reader that took the first
// event would report progress as a tool result.
func (t *httpTransport) readSSE(body io.Reader, id json.RawMessage) (*Response, error) {
	scanner := bufio.NewScanner(io.LimitReader(body, maxHTTPBody))
	scanner.Buffer(make([]byte, 0, 64*1024), maxHTTPBody)

	var data strings.Builder
	flush := func() (*Response, bool) {
		defer data.Reset()
		payload := strings.TrimSpace(data.String())
		if payload == "" || payload[0] != '{' {
			return nil, false
		}
		var resp Response
		if err := json.Unmarshal([]byte(payload), &resp); err != nil || len(resp.ID) == 0 {
			return nil, false
		}
		if !sameID(resp.ID, id) {
			return nil, false
		}
		return &resp, true
	}

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			// Blank line ends an event.
			if resp, ok := flush(); ok {
				return resp, nil
			}
		case strings.HasPrefix(line, "data:"):
			// Multi-line data fields concatenate, per the SSE grammar.
			if data.Len() > 0 {
				data.WriteString("\n")
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		default:
			// `event:`, `id:`, `retry:` and comments carry nothing this client
			// reads.
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read event stream: %w", err)
	}
	// A stream that ended without a blank line after its last event is still
	// carrying a result.
	if resp, ok := flush(); ok {
		return resp, nil
	}
	return nil, fmt.Errorf("event stream closed before answering the request")
}
