// Package valkey implements quota.Backend on top of Valkey (or Redis, or
// anything else speaking the same protocol).
//
// This is the backend to use behind a load balancer. The in-memory one keeps
// counters per replica, so a limit of 60/min across three replicas admits 180;
// this one keeps them in one place and every replica sees the same budget.
//
// # It brings no client
//
// The package defines a one-method [Doer] and ships the scripts and the key
// layout. You supply the connection, using whichever client you already have —
// go-redis, rueidis, valkey-go, or eighty lines of your own. This keeps the
// module dependency-free and means adopting it does not mean adopting somebody
// else's connection pool, the same way quota's action-free design means
// adopting it does not mean adopting somebody else's vocabulary.
//
// # Every take is one round trip
//
// Each algorithm is a Lua script, so the read-modify-write happens inside
// Valkey and is atomic by construction rather than by locking. The scripts are
// loaded on first use and thereafter invoked by digest, falling back to a full
// EVAL when the server has been restarted or its script cache flushed.
//
// The cost of that is the one real duplication in this module: the arithmetic
// exists twice, once in Go for the other backends and once in Lua here. It
// cannot be otherwise — the whole point is that it runs server-side — which is
// exactly why quotatest exists and why this package runs it in full.
package valkey

import (
	"context"
	"errors"
	"fmt"
	"strconv"
)

// Doer executes one command against Valkey and returns its reply.
//
// Implementations must map the reply to Go values the way every mainstream
// client already does:
//
//	simple string / bulk string  →  string
//	integer                      →  int64
//	array                        →  []any of the above
//	nil                          →  nil
//	error                        →  a non-nil error
//
// Anything numeric arriving as a string is tolerated — some clients hand back
// bulk strings for everything — so an adapter does not have to be clever.
//
// A four-line adapter over go-redis:
//
//	func (d goRedis) Do(ctx context.Context, args ...string) (any, error) {
//	    a := make([]any, len(args))
//	    for i, s := range args { a[i] = s }
//	    return d.client.Do(ctx, a...).Result()
//	}
type Doer interface {
	Do(ctx context.Context, args ...string) (any, error)
}

// DoerFunc adapts a function to [Doer].
type DoerFunc func(ctx context.Context, args ...string) (any, error)

func (f DoerFunc) Do(ctx context.Context, args ...string) (any, error) { return f(ctx, args...) }

// ErrUnexpectedReply is returned when the server's answer does not have the
// shape the script promises. It means a protocol or adapter problem, not an
// exhausted budget.
var ErrUnexpectedReply = errors.New("quota/valkey: unexpected reply")

// toInt64 coerces one element of a reply. Clients disagree about whether they
// hand back int64 or a bulk string for a number, and neither is wrong, so both
// are accepted rather than making the adapter's author guess which we wanted.
func toInt64(v any) (int64, error) {
	switch t := v.(type) {
	case int64:
		return t, nil
	case int:
		return int64(t), nil
	case string:
		n, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%w: %q is not an integer", ErrUnexpectedReply, t)
		}
		return n, nil
	case []byte:
		return toInt64(string(t))
	case nil:
		return 0, nil
	default:
		return 0, fmt.Errorf("%w: %T", ErrUnexpectedReply, v)
	}
}

// toInts coerces the four-element array every script returns.
func toInts(v any, want int) ([]int64, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: expected an array, got %T", ErrUnexpectedReply, v)
	}
	if len(arr) != want {
		return nil, fmt.Errorf("%w: expected %d elements, got %d", ErrUnexpectedReply, want, len(arr))
	}
	out := make([]int64, want)
	for i, e := range arr {
		n, err := toInt64(e)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}
