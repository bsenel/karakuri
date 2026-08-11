package valkey_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

// A minimal RESP client, in the tests only.
//
// The package under test brings no client on purpose, so its own tests cannot
// borrow one either without contradicting that — and a test-only file adds no
// entry to go.mod. Ninety lines of stdlib is the whole cost of proving the
// point, and writing it is also a check on the [valkey.Doer] contract: if a
// naive implementation cannot satisfy it, the interface is too clever.
type respClient struct {
	mu   sync.Mutex
	conn net.Conn
	r    *bufio.Reader
}

func dial(addr string) (*respClient, error) {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return nil, err
	}
	return &respClient{conn: conn, r: bufio.NewReader(conn)}, nil
}

func (c *respClient) Close() error { return c.conn.Close() }

// Do serialises one command and decodes one reply. The mutex is what makes it
// safe for the contract's concurrency case: RESP has no request ids, so replies
// are matched to commands purely by order on the wire.
func (c *respClient) Do(_ context.Context, args ...string) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return nil, err
	}
	var b []byte
	b = fmt.Appendf(b, "*%d\r\n", len(args))
	for _, a := range args {
		b = fmt.Appendf(b, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := c.conn.Write(b); err != nil {
		return nil, err
	}
	return readReply(c.r)
}

func readReply(r *bufio.Reader) (any, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 3 {
		return nil, fmt.Errorf("short reply %q", line)
	}
	body := line[1 : len(line)-2] // strip the type byte and CRLF

	switch line[0] {
	case '+':
		return body, nil
	case '-':
		return nil, fmt.Errorf("%s", body)
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
		out := make([]any, n)
		for i := range out {
			if out[i], err = readReply(r); err != nil {
				return nil, err
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported reply type %q", line[0])
	}
}
