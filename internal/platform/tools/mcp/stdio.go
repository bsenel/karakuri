package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/bsenel/karakuri/internal/platform/procenv"
)

// stdioTransport talks to a server this process launched, over its stdin and
// stdout, one JSON message per line.
//
// The subprocess starts when the transport is built rather than on first use, so
// a command that does not exist is a boot-time failure an operator sees in
// /health beside the instance that names it — not a surprise on the first action
// somebody approved.
type stdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	// stderr is drained into a bounded buffer. A server that logs to stderr and
	// is never read from fills the pipe and blocks, and the tail is exactly what
	// an operator wants when discovery failed.
	stderr *tailBuffer

	mu     sync.Mutex
	broken error
}

func newStdioTransport(cfg Config) (*stdioTransport, error) {
	cmd := exec.Command(cfg.Command, cfg.Args...)
	cmd.Dir = cfg.WorkDir
	// Scrubbed for the same reason a delegated CLI is: a child of this process
	// is a fresh run, not a continuation of whatever session launched the
	// server.
	cmd.Env = procenv.ScrubNestedSession(procenv.Merge(cfg.Env))

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", cfg.Command, err)
	}

	t := &stdioTransport{
		cmd:   cmd,
		stdin: stdin,
		// A tool result can be a whole file, so the line limit is generous for
		// the same reason the CLI agent adapter's is.
		stdout: bufio.NewReaderSize(stdout, 64*1024),
		stderr: &tailBuffer{limit: 8 * 1024},
	}
	go func() { _, _ = io.Copy(t.stderr, stderrPipe) }()
	return t, nil
}

func (t *stdioTransport) Kind() string { return TransportStdio }

func (t *stdioTransport) Send(ctx context.Context, req Request) (*Response, error) {
	if err := t.write(req); err != nil {
		return nil, err
	}
	return t.read(ctx, req.ID)
}

func (t *stdioTransport) Notify(_ context.Context, req Request) error {
	return t.write(req)
}

func (t *stdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.shutdown(nil)
}

func (t *stdioTransport) write(req Request) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.broken != nil {
		return t.broken
	}
	line, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encode %s: %w", req.Method, err)
	}
	if _, err := t.stdin.Write(append(line, '\n')); err != nil {
		return t.shutdown(fmt.Errorf("write %s: %w%s", req.Method, err, t.stderrTail()))
	}
	return nil
}

// read returns the response whose ID matches, skipping anything else on the
// stream.
//
// Skipping is deliberate rather than lenient. A server may write a notification
// — a progress update, a log line — between the request and its reply, and a
// reader that treated the first line as the answer would return a notification
// as a tool result. Anything that is not JSON at all is also skipped: servers
// that print a banner to stdout exist, and one line of noise should not take the
// instance down.
func (t *stdioTransport) read(ctx context.Context, id json.RawMessage) (*Response, error) {
	type lineResult struct {
		resp *Response
		err  error
	}
	done := make(chan lineResult, 1)

	go func() {
		for {
			line, err := t.stdout.ReadBytes('\n')
			if err != nil {
				if len(bytes.TrimSpace(line)) == 0 {
					done <- lineResult{err: err}
					return
				}
				// A final line with no trailing newline is still a message.
			}
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) == 0 || trimmed[0] != '{' {
				if err != nil {
					done <- lineResult{err: err}
					return
				}
				continue
			}
			var resp Response
			if jsonErr := json.Unmarshal(trimmed, &resp); jsonErr != nil || len(resp.ID) == 0 {
				if err != nil {
					done <- lineResult{err: err}
					return
				}
				continue
			}
			if !sameID(resp.ID, id) {
				if err != nil {
					done <- lineResult{err: err}
					return
				}
				continue
			}
			done <- lineResult{resp: &resp}
			return
		}
	}()

	select {
	case <-ctx.Done():
		// The reply is still in flight, so the stream is now one message out of
		// step with every future request. There is no way back to a known state
		// from here, and answering the next call with this one's result would be
		// worse than failing: the transport is closed and the instance reports
		// unreachable.
		t.mu.Lock()
		defer t.mu.Unlock()
		return nil, t.shutdown(fmt.Errorf("server did not reply before the deadline: %w", ctx.Err()))
	case res := <-done:
		if res.err != nil {
			t.mu.Lock()
			defer t.mu.Unlock()
			return nil, t.shutdown(fmt.Errorf("read reply: %w%s", res.err, t.stderrTail()))
		}
		return res.resp, nil
	}
}

// shutdown kills the subprocess and records why the transport is unusable.
// Called with t.mu held. A nil cause means an ordinary Close.
func (t *stdioTransport) shutdown(cause error) error {
	if t.broken == nil {
		t.broken = cause
		if t.broken == nil {
			t.broken = fmt.Errorf("transport closed")
		}
		_ = t.stdin.Close()
		if t.cmd.Process != nil {
			_ = t.cmd.Process.Kill()
		}
		go func() { _ = t.cmd.Wait() }()
	}
	return cause
}

// stderrTail renders what the server said on its way out, or nothing.
func (t *stdioTransport) stderrTail() string {
	tail := strings.TrimSpace(t.stderr.String())
	if tail == "" {
		return ""
	}
	return " (stderr: " + tail + ")"
}

// tailBuffer keeps the last `limit` bytes written to it and discards the rest,
// so a chatty server cannot grow this process's memory.
type tailBuffer struct {
	mu    sync.Mutex
	buf   []byte
	limit int
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if over := len(b.buf) - b.limit; over > 0 {
		b.buf = b.buf[over:]
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

// sameID compares two JSON-RPC ids by value, so a server answering 1 where the
// client sent 1 matches whether either side wrote it as a number or a string.
func sameID(a, b json.RawMessage) bool {
	return string(bytes.TrimSpace(a)) == string(bytes.TrimSpace(b)) ||
		strings.Trim(string(bytes.TrimSpace(a)), `"`) == strings.Trim(string(bytes.TrimSpace(b)), `"`)
}
