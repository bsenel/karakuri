package valkey

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bsenel/karakuri/quota"
)

// Options configures a Backend.
type Options struct {
	// KeyPrefix namespaces every key, e.g. "krk:quota:". Valkey instances are
	// routinely shared, and a bare quota key is one collision away from
	// counting somebody else's traffic. Defaults to "quota:".
	KeyPrefix string
}

// Backend implements quota.Backend against a Valkey-compatible server.
type Backend struct {
	doer   Doer
	prefix string

	// mu guards the loaded digests. Script loading is idempotent, so the worst
	// a race could do is load twice, but a map written from many request
	// goroutines still needs protecting.
	mu      sync.RWMutex
	digests map[string]string

	// seq disambiguates two sliding-log members created in the same
	// millisecond. Combined with the random suffix below it makes a member id
	// unique without the script having to generate anything.
	seqMu sync.Mutex
	seq   uint64
	nonce string
}

var _ quota.Backend = (*Backend)(nil)

// New wraps a connection. The caller keeps ownership of it — Backend never
// closes anything.
func New(d Doer, opts Options) (*Backend, error) {
	if d == nil {
		return nil, errors.New("quota/valkey: nil Doer")
	}
	prefix := opts.KeyPrefix
	if prefix == "" {
		prefix = "quota:"
	}
	// The nonce makes member ids unique across processes as well as within one,
	// so two replicas taking against the same key in the same millisecond do
	// not write the same member and lose a unit.
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("quota/valkey: %w", err)
	}
	return &Backend{
		doer:    d,
		prefix:  prefix,
		digests: make(map[string]string),
		nonce:   hex.EncodeToString(buf),
	}, nil
}

func (b *Backend) key(k quota.Key) string { return b.prefix + string(k) }

func (b *Backend) Take(ctx context.Context, key quota.Key, p quota.Policy, n int, now time.Time) (quota.Decision, error) {
	return b.eval(ctx, key, p, n, now, true)
}

func (b *Backend) Peek(ctx context.Context, key quota.Key, p quota.Policy, now time.Time) (quota.Decision, error) {
	// Costed at one, not zero: "is there room for another request" is the
	// question worth answering. The script is told not to commit, so nothing is
	// spent and the unit is handed back below.
	d, err := b.eval(ctx, key, p, 1, now, false)
	if err != nil {
		return quota.Decision{}, err
	}
	if d.Allowed {
		d.Remaining++
	}
	return d, nil
}

func (b *Backend) Reset(ctx context.Context, key quota.Key) error {
	_, err := b.doer.Do(ctx, "DEL", b.key(key))
	return err
}

func (b *Backend) eval(
	ctx context.Context, key quota.Key, p quota.Policy, n int, now time.Time, commit bool,
) (quota.Decision, error) {
	if err := p.Validate(); err != nil {
		return quota.Decision{}, err
	}
	if n < 0 {
		n = 0
	}

	nowMS := now.UnixMilli()
	windowMS := p.Window.Milliseconds()
	if windowMS <= 0 {
		// A sub-millisecond window cannot be represented server-side, and
		// rounding it to zero would make the fixed window divide by zero.
		windowMS = 1
	}

	var (
		script string
		argv   []string
	)
	common := []string{
		strconv.FormatInt(nowMS, 10),
		strconv.Itoa(p.Limit),
		strconv.FormatInt(windowMS, 10),
	}
	switch p.Algorithm {
	case quota.TokenBucket:
		script = tokenBucketScript
		// Per millisecond, because the whole script works in milliseconds.
		argv = append(common,
			strconv.FormatFloat(p.RatePerSecond()/1000, 'g', 17, 64),
			strconv.Itoa(n), boolArg(commit))
	case quota.FixedWindow:
		script = fixedWindowScript
		argv = append(common, strconv.Itoa(n), boolArg(commit))
	case quota.SlidingLog:
		script = slidingLogScript
		argv = append(common, strconv.Itoa(n), boolArg(commit), b.memberID())
	}

	reply, err := b.run(ctx, script, b.key(key), argv)
	if err != nil {
		return quota.Decision{}, err
	}
	out, err := toInts(reply, 4)
	if err != nil {
		return quota.Decision{}, err
	}
	return quota.Decision{
		Allowed:    out[0] == 1,
		Limit:      p.Limit,
		Remaining:  remainingWithin(out[1], p.Limit),
		ResetAt:    time.UnixMilli(out[2]).UTC(),
		RetryAfter: time.Duration(out[3]) * time.Millisecond,
	}.Normalize(), nil
}

// remainingWithin narrows the reply's remaining count to int.
//
// The bound is a real invariant rather than a formality: what is left can never
// exceed the limit the policy set, so anything above it is a broken reply. The
// check also has to be here rather than left to the conversion, because int is
// 32 bits on some platforms and the value arrives from another process — an
// unchecked narrowing would wrap a large number into a negative remaining,
// which reads as an exhausted budget and would refuse every request.
func remainingWithin(v int64, limit int) int {
	if v <= 0 {
		return 0
	}
	// math.MaxInt32, not math.MaxInt: int is 32 bits on some platforms, so the
	// bound has to be the smallest one this can be compiled for. No real limit
	// comes near it, and anything that does is a broken reply either way.
	if v > math.MaxInt32 {
		return limit
	}
	if narrowed := int(v); narrowed <= limit {
		return narrowed
	}
	return limit
}

// run invokes a script by digest, registering it with the server the first time
// and recovering when the server has forgotten it.
//
// The digest comes from SCRIPT LOAD rather than being computed here. It is the
// server's identifier for the script, so asking for it removes any chance of
// the two disagreeing — and computing it locally would mean hashing with SHA-1,
// which is the protocol's choice and not something this package should be seen
// to be choosing.
//
// Recovery is not theoretical: a restart or a SCRIPT FLUSH empties the cache,
// and a limiter that started returning errors because somebody bounced Valkey
// would be worse than no limiter at all.
func (b *Backend) run(ctx context.Context, script, key string, argv []string) (any, error) {
	if digest, ok := b.digest(script); ok {
		reply, err := b.evalsha(ctx, digest, key, argv)
		if err == nil {
			return reply, nil
		}
		if !isNoScript(err) {
			return nil, err
		}
		b.forget(script)
	}

	digest, err := b.load(ctx, script)
	if err != nil {
		// A server that will not SCRIPT LOAD can still EVAL. Refusing to limit
		// because an optimisation is unavailable would be the wrong trade.
		return b.doer.Do(ctx, append([]string{"EVAL", script, "1", key}, argv...)...)
	}
	b.remember(script, digest)
	return b.evalsha(ctx, digest, key, argv)
}

func (b *Backend) evalsha(ctx context.Context, digest, key string, argv []string) (any, error) {
	return b.doer.Do(ctx, append([]string{"EVALSHA", digest, "1", key}, argv...)...)
}

// load registers a script and returns the digest the server will answer to.
func (b *Backend) load(ctx context.Context, script string) (string, error) {
	reply, err := b.doer.Do(ctx, "SCRIPT", "LOAD", script)
	if err != nil {
		return "", err
	}
	switch t := reply.(type) {
	case string:
		return t, nil
	case []byte:
		return string(t), nil
	default:
		return "", fmt.Errorf("%w: SCRIPT LOAD returned %T", ErrUnexpectedReply, reply)
	}
}

func (b *Backend) digest(script string) (string, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	d, ok := b.digests[script]
	return d, ok
}

func (b *Backend) remember(script, digest string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.digests[script] = digest
}

func (b *Backend) forget(script string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.digests, script)
}

// memberID returns a value unique to this process and call, so two sliding-log
// takes in the same millisecond — from one process or several — become two
// members rather than one.
func (b *Backend) memberID() string {
	b.seqMu.Lock()
	b.seq++
	seq := b.seq
	b.seqMu.Unlock()
	return b.nonce + "-" + strconv.FormatUint(seq, 36)
}

// isNoScript reports whether an error is Valkey's NOSCRIPT. Clients wrap errors
// differently and few expose a typed one, so this matches the message — which
// is part of the protocol and stable across implementations.
func isNoScript(err error) bool {
	return err != nil && strings.Contains(strings.ToUpper(err.Error()), "NOSCRIPT")
}

func boolArg(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
