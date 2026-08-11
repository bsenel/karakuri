package valkey_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bsenel/karakuri/quota"
	"github.com/bsenel/karakuri/quota/quotatest"
	quotavalkey "github.com/bsenel/karakuri/quota/valkey"
)

// addr is where the tests expect a Valkey-compatible server. CI runs one as a
// service container; locally, any redis-server or valkey-server will do, since
// the protocol and the scripting engine are the same.
//
// Skipping when it is unset keeps `go test ./...` working on a laptop with
// nothing running, at the cost of a suite that silently covers nothing there —
// which is why CI sets it rather than leaving it optional.
func addr(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("VALKEY_ADDR"); v != "" {
		return v
	}
	t.Skip("VALKEY_ADDR not set; start a Valkey or Redis and set it to host:port")
	return ""
}

var keyspace atomic.Uint64

// runID is random per process, and the per-case counter alone is not enough
// without it: a second `go test` run would start counting from one again and
// inherit the previous run's keys, which is exactly how this suite failed the
// first time it was run twice. The server is long-lived; the test process is
// not.
var runID = func() string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("quota/valkey test: " + err.Error())
	}
	return hex.EncodeToString(buf[:])
}()

// connect returns a backend on its own key prefix, so cases cannot see each
// other's counters without a FLUSHALL that would trample a developer's own
// database. Keys carry a TTL of twice their window, so nothing is left behind
// for long.
func connect(t *testing.T) *quotavalkey.Backend {
	t.Helper()
	c, err := dial(addr(t))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	prefix := "quotatest:" + runID + ":" + strconv.FormatUint(keyspace.Add(1), 10) + ":"
	b, err := quotavalkey.New(c, quotavalkey.Options{KeyPrefix: prefix})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return b
}

// The point of the shared suite: three algorithms reimplemented in Lua have to
// come out behaving exactly like the Go ones, or a limit means something
// different depending on which backend is configured.
func TestSatisfiesContract(t *testing.T) {
	quotatest.Run(t, func(t *testing.T) quota.Backend { return connect(t) })
}

func TestScriptIsCachedAndSurvivesAFlush(t *testing.T) {
	// The first call registers the script and every later one runs it by digest.
	// A SCRIPT FLUSH — or a server restart, which looks the same from here —
	// must not turn the limiter into a source of errors.
	ctx := context.Background()
	c, err := dial(addr(t))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	var loads, evalshas, evals, noscript atomic.Int64
	counting := quotavalkey.DoerFunc(func(ctx context.Context, args ...string) (any, error) {
		switch args[0] {
		case "SCRIPT":
			loads.Add(1)
		case "EVALSHA":
			evalshas.Add(1)
		case "EVAL":
			evals.Add(1)
		}
		reply, err := c.Do(ctx, args...)
		if err != nil && args[0] == "EVALSHA" {
			noscript.Add(1)
		}
		return reply, err
	})

	b, err := quotavalkey.New(counting, quotavalkey.Options{KeyPrefix: "quotatest:" + runID + ":flush:"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := quota.Policy{Algorithm: quota.FixedWindow, Limit: 100, Window: time.Minute}

	for range 3 {
		if _, err := b.Take(ctx, "k", p, 1, quotatest.Base); err != nil {
			t.Fatalf("Take: %v", err)
		}
	}
	if loads.Load() != 1 {
		t.Errorf("SCRIPT LOAD called %d times, want once — the digest is not being reused", loads.Load())
	}
	if evalshas.Load() != 3 {
		t.Errorf("EVALSHA called %d times, want three", evalshas.Load())
	}
	if evals.Load() != 0 {
		t.Errorf("EVAL called %d times; the digest path should have covered every take", evals.Load())
	}

	if _, err := c.Do(ctx, "SCRIPT", "FLUSH"); err != nil {
		t.Fatalf("SCRIPT FLUSH: %v", err)
	}
	if _, err := b.Take(ctx, "k", p, 1, quotatest.Base); err != nil {
		t.Fatalf("Take after SCRIPT FLUSH: %v", err)
	}
	if noscript.Load() == 0 {
		t.Error("no NOSCRIPT was observed, so the recovery path did not run")
	}
	if loads.Load() != 2 {
		t.Errorf("SCRIPT LOAD called %d times, want a second one after the flush", loads.Load())
	}
}

func TestKeyPrefixIsolatesKeyspaces(t *testing.T) {
	// Valkey instances are routinely shared. A bare quota key is one collision
	// away from counting somebody else's traffic.
	ctx := context.Background()
	c, err := dial(addr(t))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	p := quota.Policy{Algorithm: quota.FixedWindow, Limit: 1, Window: time.Minute}
	one, _ := quotavalkey.New(c, quotavalkey.Options{KeyPrefix: "quotatest:" + runID + ":tenant-a:"})
	two, _ := quotavalkey.New(c, quotavalkey.Options{KeyPrefix: "quotatest:" + runID + ":tenant-b:"})
	t.Cleanup(func() {
		_, _ = one.Reset(ctx, "shared"), two.Reset(ctx, "shared")
	})

	if d, _ := one.Take(ctx, "shared", p, 1, quotatest.Base); !d.Allowed {
		t.Fatal("first tenant refused")
	}
	if d, _ := one.Take(ctx, "shared", p, 1, quotatest.Base); d.Allowed {
		t.Fatal("first tenant allowed past its limit")
	}
	if d, _ := two.Take(ctx, "shared", p, 1, quotatest.Base); !d.Allowed {
		t.Error("the second tenant was charged for the first tenant's traffic")
	}
}

func TestSlidingLogCostsSurviveTheRoundTrip(t *testing.T) {
	// A sorted set has no payload, so the cost rides in the member name. If
	// that encoding were wrong every entry would read as 1 and a limit would be
	// wildly too generous — silently.
	ctx := context.Background()
	b := connect(t)
	p := quota.Policy{Algorithm: quota.SlidingLog, Limit: 10, Window: time.Minute}

	d, err := b.Take(ctx, "k", p, 7, quotatest.Base)
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if !d.Allowed || d.Remaining != 3 {
		t.Fatalf("allowed=%t remaining=%d, want true/3", d.Allowed, d.Remaining)
	}
	if d, _ := b.Take(ctx, "k", p, 4, quotatest.Base); d.Allowed {
		t.Error("4 more were allowed against 3 remaining — the cost did not survive")
	}
}

func TestSlidingLogEntriesInTheSameMillisecondAreDistinct(t *testing.T) {
	// Members are unique by name, so two takes at the same instant would
	// collapse into one entry if the id were derived only from the timestamp —
	// and the second unit would be free.
	ctx := context.Background()
	b := connect(t)
	p := quota.Policy{Algorithm: quota.SlidingLog, Limit: 3, Window: time.Minute}

	for i := range 3 {
		if d, err := b.Take(ctx, "k", p, 1, quotatest.Base); err != nil || !d.Allowed {
			t.Fatalf("take %d: err=%v allowed=%t", i, err, d.Allowed)
		}
	}
	if d, _ := b.Take(ctx, "k", p, 1, quotatest.Base); d.Allowed {
		t.Error("a fourth take at the same instant was allowed")
	}
}

func TestNewValidatesItsArguments(t *testing.T) {
	if _, err := quotavalkey.New(nil, quotavalkey.Options{}); err == nil {
		t.Error("New accepted a nil Doer")
	}
	noop := quotavalkey.DoerFunc(func(context.Context, ...string) (any, error) { return nil, nil })
	if _, err := quotavalkey.New(noop, quotavalkey.Options{}); err != nil {
		t.Errorf("New with default options: %v", err)
	}
}

func TestConnectionErrorsSurface(t *testing.T) {
	// The contract reserves errors for "I could not find out". A limiter that
	// answered "allowed" when its server was unreachable would have silently
	// stopped limiting — the middleware decides what to do about that, and it
	// cannot decide if it is never told.
	ctx := context.Background()
	boom := errors.New("connection refused")
	failing := quotavalkey.DoerFunc(func(context.Context, ...string) (any, error) { return nil, boom })

	b, err := quotavalkey.New(failing, quotavalkey.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := quota.Policy{Algorithm: quota.TokenBucket, Limit: 5, Window: time.Minute}

	d, err := b.Take(ctx, "k", p, 1, quotatest.Base)
	if !errors.Is(err, boom) {
		t.Errorf("Take error = %v, want the connection error", err)
	}
	if d.Allowed {
		t.Error("a failed Take reported the request as allowed")
	}
	if _, err := b.Peek(ctx, "k", p, quotatest.Base); !errors.Is(err, boom) {
		t.Errorf("Peek error = %v", err)
	}
	if err := b.Reset(ctx, "k"); !errors.Is(err, boom) {
		t.Errorf("Reset error = %v", err)
	}
}

func TestMalformedReplyIsAnError(t *testing.T) {
	// A Doer that does not follow the documented mapping should produce a clear
	// error rather than a decision built from nonsense.
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		reply any
	}{
		{"not an array", "OK"},
		{"wrong length", []any{int64(1), int64(2)}},
		{"non-numeric element", []any{int64(1), "many", int64(0), int64(0)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := quotavalkey.New(
				quotavalkey.DoerFunc(func(context.Context, ...string) (any, error) { return tc.reply, nil }),
				quotavalkey.Options{})
			p := quota.Policy{Algorithm: quota.FixedWindow, Limit: 5, Window: time.Minute}
			if _, err := b.Take(ctx, "k", p, 1, quotatest.Base); !errors.Is(err, quotavalkey.ErrUnexpectedReply) {
				t.Errorf("error = %v, want ErrUnexpectedReply", err)
			}
		})
	}
}

func TestInvalidPolicyIsRejectedBeforeTheServer(t *testing.T) {
	ctx := context.Background()
	called := false
	b, _ := quotavalkey.New(
		quotavalkey.DoerFunc(func(context.Context, ...string) (any, error) {
			called = true
			return nil, nil
		}), quotavalkey.Options{})

	bad := quota.Policy{Algorithm: quota.FixedWindow, Limit: 1}
	if _, err := b.Take(ctx, "k", bad, 1, quotatest.Base); !errors.Is(err, quota.ErrInvalidPolicy) {
		t.Errorf("error = %v, want ErrInvalidPolicy", err)
	}
	if called {
		t.Error("an invalid policy reached the server")
	}
}
