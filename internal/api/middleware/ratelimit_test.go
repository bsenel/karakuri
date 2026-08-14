package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bsenel/karakuri/internal/api/middleware"
)

func TestIPRateLimiterThrottlesBurst(t *testing.T) {
	// 1/min, burst 3: the fourth immediate request from an IP is refused.
	lim := middleware.NewIPRateLimiter(1, 3)
	h := lim.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	call := func(ip string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", nil)
		req.RemoteAddr = ip + ":12345"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < 3; i++ {
		if got := call("10.0.0.1"); got != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i, got)
		}
	}
	if got := call("10.0.0.1"); got != http.StatusTooManyRequests {
		t.Errorf("4th request: got %d, want 429", got)
	}
	// A different IP has its own bucket and is not affected.
	if got := call("10.0.0.2"); got != http.StatusOK {
		t.Errorf("other IP: got %d, want 200", got)
	}
}
