package middleware

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// IPRateLimiter throttles unauthenticated requests by client IP. It exists for
// the public /auth/* routes, which the principal-keyed quota limiter cannot
// cover (no principal yet) — leaving login and refresh open to credential
// brute-force and spraying. See SECURITY_AUDIT.md F-03.
//
// It is in-memory and per-process: a single instance with a modest attempt rate
// is the intended deployment, and unlike the request-rate limiter there is no
// backend to fail open. Behind a reverse proxy the operator must propagate the
// real client address (RemoteAddr), or every client shares one bucket.
type IPRateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*ipBucket
	limit    rate.Limit
	burst    int
	lastReap time.Time
}

type ipBucket struct {
	lim  *rate.Limiter
	seen time.Time
}

// NewIPRateLimiter builds a limiter admitting perMinute requests per IP with the
// given burst. Sensible login defaults are 10/min, burst 20.
func NewIPRateLimiter(perMinute, burst int) *IPRateLimiter {
	return &IPRateLimiter{
		buckets:  make(map[string]*ipBucket),
		limit:    rate.Every(time.Minute / time.Duration(max(perMinute, 1))),
		burst:    burst,
		lastReap: time.Now(),
	}
}

func (l *IPRateLimiter) allow(ip string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.reap(now)
	b, ok := l.buckets[ip]
	if !ok {
		b = &ipBucket{lim: rate.NewLimiter(l.limit, l.burst)}
		l.buckets[ip] = b
	}
	b.seen = now
	r := b.lim.ReserveN(now, 1)
	if !r.OK() {
		return false, time.Minute
	}
	if d := r.DelayFrom(now); d > 0 {
		r.CancelAt(now)
		return false, d
	}
	return true, 0
}

// reap drops buckets untouched for 10 minutes so the map cannot grow without
// bound under a rotating-IP flood. Called under the lock, at most once a minute.
func (l *IPRateLimiter) reap(now time.Time) {
	if now.Sub(l.lastReap) < time.Minute {
		return
	}
	l.lastReap = now
	for ip, b := range l.buckets {
		if now.Sub(b.seen) > 10*time.Minute {
			delete(l.buckets, ip)
		}
	}
}

// Middleware returns the http middleware. On refusal it sets Retry-After and
// returns 429.
func (l *IPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, retry := l.allow(clientIP(r), time.Now())
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP returns the direct peer address. It deliberately does NOT trust
// X-Forwarded-For: an attacker can rotate that header to mint a fresh bucket per
// request and defeat the limit. Operators terminating at a proxy should ensure
// the proxy sets the connection source, or run the limiter at the proxy.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
