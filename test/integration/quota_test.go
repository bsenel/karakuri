package integration_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/bsenel/karakuri/config"
)

// A limit low enough to trip deliberately, with a burst equal to it so the
// arithmetic in these cases is exactly "n requests, then refused".
func lowLimit(n int) func(*config.Config) {
	return func(cfg *config.Config) {
		cfg.Quota.RequestsPerMinute = n
		cfg.Quota.RequestBurst = n
	}
}

func TestQuotaRefusesPastTheLimit(t *testing.T) {
	base, token, cleanup := startServerWith(t, lowLimit(5))
	defer cleanup()

	var lastResp *http.Response
	allowed := 0
	for range 6 {
		resp := doJSON(t, token, http.MethodGet, base+"/api/v1/twins", nil)
		if resp.StatusCode == http.StatusOK {
			allowed++
			resp.Body.Close()
			continue
		}
		lastResp = resp
	}
	if allowed != 5 {
		t.Fatalf("allowed %d of 6 against a limit of 5", allowed)
	}
	if lastResp == nil {
		t.Fatal("nothing was refused")
	}
	defer lastResp.Body.Close()

	if lastResp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", lastResp.StatusCode)
	}
	// A 429 without Retry-After leaves a client guessing, and the usual guess
	// is "immediately", which is how a limiter becomes a busy loop.
	retry := lastResp.Header.Get("Retry-After")
	if n, err := strconv.Atoi(retry); err != nil || n < 1 {
		t.Errorf("Retry-After = %q, want a positive number of seconds", retry)
	}
	if got := lastResp.Header.Get("X-RateLimit-Limit"); got != "5" {
		t.Errorf("X-RateLimit-Limit = %q, want 5", got)
	}
	if got := lastResp.Header.Get("X-RateLimit-Remaining"); got != "0" {
		t.Errorf("X-RateLimit-Remaining = %q, want 0", got)
	}
	if ct := lastResp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q — a JSON API should refuse in JSON too", ct)
	}
}

func TestQuotaExemptsHealth(t *testing.T) {
	// /health sits outside the authenticated group, so the limiter never sees
	// it. That matters: a load balancer polls it constantly and must never be
	// told to back off, or the instance gets pulled out of rotation for being
	// healthy.
	base, _, cleanup := startServerWith(t, lowLimit(2))
	defer cleanup()

	for i := range 20 {
		resp, err := http.Get(base + "/api/v1/health")
		if err != nil {
			t.Fatalf("health %d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("health %d: status %d", i, resp.StatusCode)
		}
	}
}

func TestQuotaIsPerPrincipal(t *testing.T) {
	// The property that makes the limit mean anything: one caller exhausting
	// its budget must not refuse anybody else, and a caller must not be able to
	// escape its own budget by varying what it asks for.
	base, adminToken, cleanup := startServerWith(t, lowLimit(4))
	defer cleanup()

	// Creating the second principal costs the admin two of its four.
	resp := doJSON(t, adminToken, http.MethodPost, base+"/api/v1/auth/users", map[string]any{
		"id": "vera", "roles": []string{"viewer"}, "password": "vera-password-1234",
	})
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = doJSON(t, "", http.MethodPost, base+"/api/v1/auth/token", map[string]any{
		"id": "vera", "password": "vera-password-1234",
	})
	assertStatus(t, resp, http.StatusOK)
	veraToken, _ := decodeJSON(t, resp)["access_token"].(string)
	if veraToken == "" {
		t.Fatal("no access token for vera")
	}

	// Spend the admin's remaining budget across *different* paths. Keying on
	// the twin rather than the principal would have let this slip the limit.
	for range 6 {
		r := doJSON(t, adminToken, http.MethodGet, base+"/api/v1/twins", nil)
		r.Body.Close()
	}
	r := doJSON(t, adminToken, http.MethodGet, base+"/api/v1/objectives", nil)
	defer r.Body.Close()
	if r.StatusCode != http.StatusTooManyRequests {
		t.Errorf("admin status = %d after exhausting its budget, want 429", r.StatusCode)
	}

	// Vera's budget is untouched.
	vr := doJSON(t, veraToken, http.MethodGet, base+"/api/v1/twins", nil)
	defer vr.Body.Close()
	if vr.StatusCode != http.StatusOK {
		t.Errorf("vera status = %d — she was charged for the admin's traffic", vr.StatusCode)
	}
}

func TestQuotaHeadersOnAllowedRequests(t *testing.T) {
	// A client should be able to slow itself down before being refused, which
	// means the headers have to be on the successful responses too.
	base, token, cleanup := startServerWith(t, lowLimit(10))
	defer cleanup()

	resp := doJSON(t, token, http.MethodGet, base+"/api/v1/twins", nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	if got := resp.Header.Get("X-RateLimit-Limit"); got != "10" {
		t.Errorf("X-RateLimit-Limit = %q, want 10", got)
	}
	remaining, err := strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining"))
	if err != nil || remaining != 9 {
		t.Errorf("X-RateLimit-Remaining = %q, want 9", resp.Header.Get("X-RateLimit-Remaining"))
	}
	if resp.Header.Get("X-RateLimit-Reset") == "" {
		t.Error("X-RateLimit-Reset is missing")
	}
	if got := resp.Header.Get("Retry-After"); got != "" {
		t.Errorf("Retry-After = %q on an allowed request", got)
	}
}
