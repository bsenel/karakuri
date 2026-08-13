package quota

import (
	"fmt"
	"net/http"
	"time"
)

// Limit returns middleware enforcing p for the key extract derives from each
// request. It is chi- and net/http-compatible.
//
// A refused request gets 429 with X-RateLimit-* and Retry-After set, and a JSON
// body — `{"error":"rate_limited","message":"…"}` — matching what an API that
// returns JSON everywhere else should return here too.
//
// An extractor returning an empty key exempts the request. That is how you skip
// health checks: one extractor that knows the exemption, rather than a limiter
// wrapped in an if.
//
// Panics if p is invalid. This runs at wire-up time, and a limiter silently
// admitting everything because someone left Window at zero is exactly the bug
// nobody finds until it matters.
func Limit(b Backend, p Policy, extract KeyExtractor, opts ...Option) func(http.Handler) http.Handler {
	if err := p.Validate(); err != nil {
		panic(fmt.Sprintf("quota.Limit: %v", err))
	}
	if b == nil || extract == nil {
		panic("quota.Limit: backend and key extractor are required")
	}
	cfg := newOptions(opts)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := extract(r)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			now := cfg.now()
			// Two things can move a limit since this middleware was built: the
			// configured policy itself, if the caller keeps it somewhere
			// editable, and an override raising one subject's ceiling.
			effective := cfg.resolver.Policy(r.Context(), key, cfg.limitName, cfg.base(r, p), now)

			d, err := b.Take(r.Context(), key, effective, cfg.cost(r), now)
			if err != nil {
				cfg.onError(r, key, err)
				if cfg.failClosed {
					writeLimited(w, d, "limiter unavailable")
					return
				}
				// Fail open. A limiter that turns its own store's outage into
				// a site outage has made things worse than the traffic it was
				// protecting against — and unlike an authorization check,
				// letting one request through is recoverable.
				next.ServeHTTP(w, r)
				return
			}

			d.SetHeaders(w.Header())
			if !d.Allowed {
				// A refusal reports itself through OnLimited. Firing OnPressure
				// as well would count one event twice, and "you are at 100%" is
				// not news to whoever just got a 429.
				cfg.onLimited(r, key, d)
				writeLimited(w, d, fmt.Sprintf("rate limit exceeded; retry in %s", d.RetryAfter.Round(time.Second)))
				return
			}
			if cfg.onPressure != nil && d.Used() >= cfg.pressureAt {
				cfg.onPressure(r, key, d)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeLimited(w http.ResponseWriter, d Decision, msg string) {
	w.Header().Set("Content-Type", "application/json")
	if d.Limit > 0 {
		d.SetHeaders(w.Header())
	}
	w.WriteHeader(http.StatusTooManyRequests)
	fmt.Fprintf(w, `{"error":"rate_limited","message":%q}`+"\n", msg)
}

// Option configures Limit.
type Option func(*options)

type options struct {
	onLimited  func(*http.Request, Key, Decision)
	onError    func(*http.Request, Key, error)
	onPressure func(*http.Request, Key, Decision)
	pressureAt float64
	failClosed bool
	cost       func(*http.Request) int
	now        func() time.Time
	resolver   *Resolver
	limitName  string
	baseFn     func(*http.Request) (Policy, error)
}

// base returns the configured policy in force for this request.
//
// Without the Base option this is the policy handed to Limit, which is the
// whole story for a caller whose limits live in a config file. With it, the
// configured limit is itself something that can move — and a limit that cannot
// be read, or that reads back invalid, falls to the one Limit was built with
// rather than to no limit at all. That fallback is the point: a bad row in a
// table must not be a way to switch the limiter off.
func (o options) base(r *http.Request, configured Policy) Policy {
	if o.baseFn == nil {
		return configured
	}
	p, err := o.baseFn(r)
	if err == nil {
		err = p.Validate()
	}
	if err != nil {
		o.onError(r, "", err)
		return configured
	}
	return p
}

func newOptions(opts []Option) options {
	cfg := options{
		onLimited: func(*http.Request, Key, Decision) {},
		onError:   func(*http.Request, Key, error) {},
		cost:      func(*http.Request) int { return 1 },
		now:       time.Now,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// OnLimited is called for every refused request, before the response is
// written. Use it to audit or to emit a metric; it must not write to the
// response.
func OnLimited(fn func(*http.Request, Key, Decision)) Option {
	return func(o *options) {
		if fn != nil {
			o.onLimited = fn
		}
	}
}

// OnError is called when the backend itself fails. See Limit for what happens
// next, which is fail-open unless FailClosed is set.
func OnError(fn func(*http.Request, Key, error)) Option {
	return func(o *options) {
		if fn != nil {
			o.onError = fn
		}
	}
}

// OnPressure is called on every allowed request once usage reaches threshold, a
// fraction in (0,1] — 0.8 for "warn at 80%".
//
// It fires on each request past the threshold, not once per crossing. Whoever
// consumes it decides how to debounce, because only they know whether the sink
// is a log line, a metric, or an event stream a browser is watching.
func OnPressure(threshold float64, fn func(*http.Request, Key, Decision)) Option {
	return func(o *options) {
		if fn == nil || threshold <= 0 || threshold > 1 {
			return
		}
		o.pressureAt, o.onPressure = threshold, fn
	}
}

// Resolve makes the limit per-subject: the policy passed to Limit becomes the
// base, and an override for name replaces its ceiling.
//
// It is an option rather than a change to Limit's signature so that every
// existing caller keeps working and a deployment with no overrides pays
// nothing — without it the resolver is never consulted at all.
//
// The lookup is a cache read on the overwhelming majority of requests; see
// Resolver.
func Resolve(r *Resolver, name string) Option {
	return func(o *options) {
		if r == nil || name == "" {
			return
		}
		o.resolver, o.limitName = r, name
	}
}

// Base makes the *configured* limit resolvable per request, for a deployment
// that keeps its limits somewhere an operator can edit without a redeploy.
//
// Limit captures its Policy by value, which is right when the policy comes from
// a config file read at boot: it cannot change, so reading it once is honest.
// A caller storing limits in a database has the opposite problem — the value
// captured at wire-up is stale the moment somebody edits it.
//
// This is the base, not the answer. Resolve still applies a subject's override
// on top, in that order, because the two mean different things: the base is
// "what everybody gets" and an override is "except this one".
//
// The function is called on every request that is subject to the limit, so it
// wants a cache behind it — see Resolver for the shape and the reasoning about
// how stale is stale enough.
//
// Returning an error, or a policy that does not validate, falls back to the
// policy Limit was built with and reports through OnError. A limiter that
// switched itself off because a row was malformed would be the worst outcome
// available.
func Base(fn func(*http.Request) (Policy, error)) Option {
	return func(o *options) {
		if fn != nil {
			o.baseFn = fn
		}
	}
}

// FailClosed refuses requests the backend could not decide on, instead of
// letting them through.
//
// The default is the other way round. Choose this only where exceeding the
// limit is worse than being unavailable — a hard spend cap, say, where the
// budget is the point and a paused caller is the intended outcome.
func FailClosed() Option {
	return func(o *options) { o.failClosed = true }
}

// WithCost makes a request consume more than one unit — a batch size, an
// estimated token count. Returning less than zero is treated as zero.
func WithCost(fn func(*http.Request) int) Option {
	return func(o *options) {
		if fn != nil {
			o.cost = fn
		}
	}
}

// WithClock replaces time.Now. For tests.
func WithClock(fn func() time.Time) Option {
	return func(o *options) {
		if fn != nil {
			o.now = fn
		}
	}
}

// KeyFromContext builds a KeyExtractor that reads a value the surrounding
// middleware has already put on the request context — a principal ID from an
// authenticator, say. Returns an empty key, and so exempts the request, when
// the value is absent or is not a string.
func KeyFromContext(ctxKey any, prefix string) KeyExtractor {
	return func(r *http.Request) Key {
		v, _ := r.Context().Value(ctxKey).(string)
		if v == "" {
			return ""
		}
		return JoinKey(prefix, v)
	}
}
