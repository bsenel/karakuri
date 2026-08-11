// Package valkey is a small RESP client for the Valkey/Redis-compatible
// servers Karakuri talks to.
//
// It exists because two things now need one — the quota limiter and the Celery
// executor's broker — and because neither justifies a third-party client. The
// commands between them are RPUSH, GET, DEL, SCRIPT LOAD, EVAL and EVALSHA;
// what a full client would add is cluster support and a connection pool, and
// only the pool matters here.
//
// The pool does matter. The Celery executor dialled per task submission, which
// is fine at its rate and hopeless for a limiter that runs on every request.
package valkey

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Reply size limits. A header states how much is coming, and sizing an
// allocation from it lets a broken or hostile server decide how much memory
// this process reserves. Nothing Karakuri issues comes close to either bound,
// so a header claiming more is a protocol error — cheaper to treat as one than
// to discover as an out-of-memory kill.
const (
	maxBulkLen  = 8 << 20 // 8 MiB
	maxArrayLen = 1 << 20
)

// ErrClosed is returned by a Client used after Close.
var ErrClosed = errors.New("valkey: client closed")

// Client is a connection pool over one server. It is safe for concurrent use.
type Client struct {
	addr     string
	password string
	db       string
	dialer   net.Dialer
	timeout  time.Duration

	mu     sync.Mutex
	idle   []*conn
	closed bool

	// maxIdle bounds the pool. Connections beyond it are closed on return
	// rather than kept, so a burst does not leave the process holding sockets
	// it will not use again.
	maxIdle int
}

// Options configures a Client.
type Options struct {
	// MaxIdle connections to keep. Defaults to 8.
	MaxIdle int

	// Timeout bounds a single command, dial included. Defaults to 5s. A
	// limiter that blocks indefinitely on an unreachable server turns every
	// request into a hung request, which is worse than not limiting.
	Timeout time.Duration
}

// New parses a redis:// or valkey:// URL and returns a pool. Nothing is dialled
// until the first command, so a server that is not up yet does not stop the
// process from starting.
func New(rawURL string, opts Options) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("valkey: %w", err)
	}
	host := u.Host
	if host == "" {
		return nil, fmt.Errorf("valkey: %q has no host", rawURL)
	}
	if !strings.Contains(host, ":") {
		host += ":6379"
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	maxIdle := opts.MaxIdle
	if maxIdle <= 0 {
		maxIdle = 8
	}

	c := &Client{
		addr:    host,
		dialer:  net.Dialer{Timeout: timeout},
		timeout: timeout,
		maxIdle: maxIdle,
	}
	if pw, ok := u.User.Password(); ok {
		c.password = pw
	}
	if db := strings.TrimPrefix(u.Path, "/"); db != "" {
		if _, err := strconv.Atoi(db); err == nil {
			c.db = db
		}
	}
	return c, nil
}

// Do executes one command and returns the decoded reply, satisfying
// quota/valkey.Doer.
func (c *Client) Do(ctx context.Context, args ...string) (any, error) {
	cn, err := c.take(ctx)
	if err != nil {
		return nil, err
	}
	reply, err := cn.do(ctx, c.timeout, args)
	if err != nil {
		// A connection that has failed mid-conversation is out of sync with
		// the server — the next reply on it would belong to this command — so
		// it is closed rather than returned to the pool.
		cn.Close()
		return nil, err
	}
	c.put(cn)
	return reply, nil
}

// Close shuts the pool. In-flight commands finish on their own connections.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	for _, cn := range c.idle {
		cn.Close()
	}
	c.idle = nil
	return nil
}

func (c *Client) take(ctx context.Context) (*conn, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	if n := len(c.idle); n > 0 {
		cn := c.idle[n-1]
		c.idle = c.idle[:n-1]
		c.mu.Unlock()
		return cn, nil
	}
	c.mu.Unlock()
	return c.dial(ctx)
}

func (c *Client) put(cn *conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || len(c.idle) >= c.maxIdle {
		cn.Close()
		return
	}
	c.idle = append(c.idle, cn)
}

func (c *Client) dial(ctx context.Context) (*conn, error) {
	nc, err := c.dialer.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return nil, err
	}
	cn := &conn{nc: nc, r: bufio.NewReader(nc)}

	// AUTH and SELECT are per-connection state, so they belong here rather than
	// anywhere a pooled connection could be handed out without them.
	if c.password != "" {
		if _, err := cn.do(ctx, c.timeout, []string{"AUTH", c.password}); err != nil {
			cn.Close()
			return nil, fmt.Errorf("valkey: auth: %w", err)
		}
	}
	if c.db != "" {
		if _, err := cn.do(ctx, c.timeout, []string{"SELECT", c.db}); err != nil {
			cn.Close()
			return nil, fmt.Errorf("valkey: select %s: %w", c.db, err)
		}
	}
	return cn, nil
}

type conn struct {
	nc net.Conn
	r  *bufio.Reader
}

func (c *conn) Close() { _ = c.nc.Close() }

func (c *conn) do(ctx context.Context, timeout time.Duration, args []string) (any, error) {
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := c.nc.SetDeadline(deadline); err != nil {
		return nil, err
	}

	var b []byte
	b = fmt.Appendf(b, "*%d\r\n", len(args))
	for _, a := range args {
		b = fmt.Appendf(b, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := c.nc.Write(b); err != nil {
		return nil, err
	}
	return readReply(c.r)
}

// readReply decodes one RESP value into the shape quota/valkey.Doer documents:
// string, int64, []any, or nil.
func readReply(r *bufio.Reader) (any, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 3 {
		return nil, fmt.Errorf("valkey: short reply %q", line)
	}
	body := line[1 : len(line)-2] // strip the type byte and the CRLF

	switch line[0] {
	case '+':
		return body, nil
	case '-':
		// A server-side error, including NOSCRIPT, which quota/valkey matches
		// on to reload a flushed script.
		return nil, errors.New(body)
	case ':':
		return strconv.ParseInt(body, 10, 64)
	case '$':
		n, err := strconv.Atoi(body)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil // RESP nil bulk string
		}
		if n > maxBulkLen {
			return nil, fmt.Errorf("valkey: bulk string of %d bytes exceeds the %d-byte limit", n, maxBulkLen)
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	case '*':
		n, err := strconv.Atoi(body)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		if n > maxArrayLen {
			return nil, fmt.Errorf("valkey: array of %d elements exceeds the %d-element limit", n, maxArrayLen)
		}
		out := make([]any, n)
		for i := range out {
			if out[i], err = readReply(r); err != nil {
				return nil, err
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("valkey: unsupported reply type %q", line[0])
	}
}
