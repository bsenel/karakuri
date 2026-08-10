package quota

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// ErrInvalidPolicy is returned by Policy.Validate and by backends handed a
// policy they cannot enforce.
var ErrInvalidPolicy = errors.New("quota: invalid policy")

// Key identifies one budget. It is opaque to this module: whatever a caller's
// KeyExtractor produces is what gets counted.
//
// Keys are also storage identities in the persistent backends, so keep them
// stable and bounded — deriving one from an unvalidated request path mints a
// new counter for every URL an attacker invents.
type Key string

// KeyExtractor derives the budget a request draws from.
//
// Returning an empty Key means "not subject to this limit", and the middleware
// passes the request straight through. That is how per-route exemptions are
// expressed — one extractor, rather than a limiter wrapped in a conditional.
type KeyExtractor func(*http.Request) Key

// JoinKey builds a Key from parts, skipping empties.
//
// Use it to compose dimensions — JoinKey("twin", id, string(capability)) — so
// that a key's shape is readable in a log and two dimensions cannot collide by
// accident.
func JoinKey(parts ...string) Key {
	kept := parts[:0:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return Key(strings.Join(kept, "|"))
}

// Backend enforces a Policy for a Key.
//
// The interface is deliberately high-level: it hands out decisions rather than
// exposing counters. A low-level interface — increment, expire, add to a set —
// would push sorted-set semantics into SQL and mutex semantics into Valkey, and
// each implementation would end up reinterpreting the algorithms anyway. This
// way each is written in its own idiom: Go under a lock, one Lua script per
// round trip, one transaction.
//
// # Contract
//
// Implementations MUST satisfy all of the following.
//
//   - Take is atomic per key. Concurrent calls for the same key must behave as
//     if serialised; two callers must never both be granted the last unit. This
//     is the whole point of the interface, and the property the shared contract
//     suite tests hardest.
//   - A refusal consumes nothing. When Allowed is false the budget is unchanged,
//     so a client hammering a limiter cannot push its own reset further away.
//   - Time comes from the caller. Every method takes now, so the suite can drive
//     a clock forward without sleeping and two nodes can agree on a window.
//   - Errors are transport failures only. An exhausted budget is a Decision with
//     Allowed false, never an error. Reserve errors for "I could not find out".
type Backend interface {
	// Take consumes n units against key and reports the outcome. n is normally
	// 1; a request costing more (a batch, a token count) passes more.
	Take(ctx context.Context, key Key, p Policy, n int, now time.Time) (Decision, error)

	// Peek reports the budget without consuming any: Remaining is what is left,
	// and Allowed says whether one more unit would be admitted. It backs "show
	// me my usage" endpoints, which must not spend the budget they report on.
	Peek(ctx context.Context, key Key, p Policy, now time.Time) (Decision, error)

	// Reset discards a key's state, restoring it to a full budget. Resetting a
	// key that has none is not an error.
	Reset(ctx context.Context, key Key) error
}
