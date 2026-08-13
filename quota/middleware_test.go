package quota

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func byPath(r *http.Request) Key { return Key(r.URL.Path) }

func frozen(at time.Time) Option { return WithClock(func() time.Time { return at }) }

// serve runs one request through the middleware and returns the recorder plus
// whether the wrapped handler was reached.
func serve(h func(http.Handler) http.Handler, req *http.Request) (*httptest.ResponseRecorder, bool) {
	reached := false
	rec := httptest.NewRecorder()
	h(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })).ServeHTTP(rec, req)
	return rec, reached
}

func TestLimitAllowsThenRefuses(t *testing.T) {
	mw := Limit(NewMemoryBackend(), Policy{Algorithm: FixedWindow, Limit: 2, Window: time.Minute},
		byPath, frozen(base))

	for i := range 2 {
		rec, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil))
		if !reached || rec.Code != http.StatusOK {
			t.Fatalf("request %d: reached=%t code=%d", i, reached, rec.Code)
		}
		if got := rec.Header().Get("X-RateLimit-Limit"); got != "2" {
			t.Errorf("X-RateLimit-Limit = %q", got)
		}
	}

	rec, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil))
	if reached {
		t.Error("the handler ran for a refused request")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("code = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got == "" || got == "0" {
		t.Errorf("Retry-After = %q, want a positive wait", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"error":"rate_limited"`) {
		t.Errorf("body = %q", body)
	}
}

func TestLimitEmptyKeyExempts(t *testing.T) {
	// One extractor that knows the exemption, rather than a limiter wrapped in
	// a conditional at every call site.
	exempt := func(r *http.Request) Key {
		if r.URL.Path == "/health" {
			return ""
		}
		return Key(r.URL.Path)
	}
	mw := Limit(NewMemoryBackend(), Policy{Algorithm: FixedWindow, Limit: 1, Window: time.Minute},
		exempt, frozen(base))

	for range 50 {
		rec, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/health", nil))
		if !reached || rec.Code != http.StatusOK {
			t.Fatalf("health check was limited: reached=%t code=%d", reached, rec.Code)
		}
	}
	// An exempt request must also not have spent anyone else's budget.
	if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil)); !reached {
		t.Error("a limited path was refused after exempt traffic")
	}
}

func TestLimitHooks(t *testing.T) {
	var (
		limited  []Key
		pressure []float64
	)
	mw := Limit(NewMemoryBackend(), Policy{Algorithm: FixedWindow, Limit: 5, Window: time.Minute}, byPath,
		frozen(base),
		OnLimited(func(_ *http.Request, k Key, _ Decision) { limited = append(limited, k) }),
		OnPressure(0.8, func(_ *http.Request, _ Key, d Decision) { pressure = append(pressure, d.Used()) }),
	)

	for range 6 {
		serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil))
	}

	// Pressure fires on allowed requests at or past the threshold: 4/5 and 5/5.
	if len(pressure) != 2 {
		t.Errorf("pressure fired %d times (%v), want twice — at 80%% and 100%%", len(pressure), pressure)
	}
	if len(limited) != 1 || limited[0] != "/twins" {
		t.Errorf("OnLimited saw %v, want one refusal of /twins", limited)
	}
}

func TestLimitPressureIgnoresNonsenseThresholds(t *testing.T) {
	fired := 0
	count := func(*http.Request, Key, Decision) { fired++ }
	for _, opt := range []Option{
		OnPressure(0, count),
		OnPressure(-1, count),
		OnPressure(1.5, count),
		OnPressure(0.5, nil),
	} {
		mw := Limit(NewMemoryBackend(), Policy{Algorithm: FixedWindow, Limit: 1, Window: time.Minute},
			byPath, frozen(base), opt)
		serve(mw, httptest.NewRequest(http.MethodGet, "/x", nil))
	}
	if fired != 0 {
		t.Errorf("a threshold outside (0,1] still fired %d times", fired)
	}
}

func TestLimitFailsOpenByDefault(t *testing.T) {
	boom := errors.New("store unreachable")
	var seen error
	mw := Limit(failingBackend{boom}, Policy{Algorithm: FixedWindow, Limit: 1, Window: time.Minute},
		byPath, OnError(func(_ *http.Request, _ Key, err error) { seen = err }))

	rec, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil))
	// A limiter that turns its own store's outage into a site outage has made
	// things worse than the traffic it was protecting against.
	if !reached || rec.Code != http.StatusOK {
		t.Errorf("failed closed by default: reached=%t code=%d", reached, rec.Code)
	}
	if !errors.Is(seen, boom) {
		t.Errorf("OnError saw %v, want the backend's error", seen)
	}
}

func TestLimitFailClosed(t *testing.T) {
	mw := Limit(failingBackend{errors.New("store unreachable")},
		Policy{Algorithm: FixedWindow, Limit: 1, Window: time.Minute},
		byPath, FailClosed())

	rec, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil))
	if reached {
		t.Error("the handler ran despite FailClosed")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("code = %d, want 429", rec.Code)
	}
}

func TestLimitWithCost(t *testing.T) {
	mw := Limit(NewMemoryBackend(), Policy{Algorithm: FixedWindow, Limit: 10, Window: time.Minute},
		byPath, frozen(base),
		WithCost(func(r *http.Request) int {
			if r.Method == http.MethodPost {
				return 4
			}
			return 1
		}))

	for i := range 2 {
		if _, reached := serve(mw, httptest.NewRequest(http.MethodPost, "/twins", nil)); !reached {
			t.Fatalf("expensive request %d refused inside the budget", i)
		}
	}
	// 8 of 10 spent; two cheap ones fit and a third does not.
	for range 2 {
		if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil)); !reached {
			t.Fatal("cheap request refused inside the budget")
		}
	}
	if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil)); reached {
		t.Error("allowed past the budget")
	}
}

func TestLimitNilOptionsAreIgnored(t *testing.T) {
	// Passing a nil hook should leave the default in place rather than panic on
	// the first request.
	mw := Limit(NewMemoryBackend(), Policy{Algorithm: FixedWindow, Limit: 1, Window: time.Minute},
		byPath, OnLimited(nil), OnError(nil), WithCost(nil), WithClock(nil))
	if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/x", nil)); !reached {
		t.Error("request refused")
	}
	if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/x", nil)); reached {
		t.Error("limit not enforced")
	}
}

func TestLimitPanicsOnMisconfiguration(t *testing.T) {
	// This runs at wire-up. A limiter that silently admits everything because
	// someone left Window at zero is the bug nobody finds until it matters.
	tests := []struct {
		name    string
		fn      func()
		wantMsg string
	}{
		{"invalid policy", func() {
			Limit(NewMemoryBackend(), Policy{Algorithm: FixedWindow, Limit: 1}, byPath)
		}, "window must be positive"},
		{"nil backend", func() {
			Limit(nil, Policy{Algorithm: FixedWindow, Limit: 1, Window: time.Minute}, byPath)
		}, "required"},
		{"nil extractor", func() {
			Limit(NewMemoryBackend(), Policy{Algorithm: FixedWindow, Limit: 1, Window: time.Minute}, nil)
		}, "required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("did not panic")
				}
				if msg, ok := r.(string); !ok || !strings.Contains(msg, tc.wantMsg) {
					t.Errorf("panic = %v, want it to mention %q", r, tc.wantMsg)
				}
			}()
			tc.fn()
		})
	}
}

type ctxKeyType struct{}

func TestKeyFromContext(t *testing.T) {
	extract := KeyFromContext(ctxKeyType{}, "principal")

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if got := extract(req); got != "" {
		t.Errorf("with no value in context = %q, want an exemption", got)
	}

	withValue := req.WithContext(context.WithValue(req.Context(), ctxKeyType{}, "alice"))
	if got := extract(withValue); got != "principal|alice" {
		t.Errorf("= %q, want principal|alice", got)
	}

	// A value of the wrong type exempts rather than panicking — the middleware
	// upstream may simply not have run.
	wrongType := req.WithContext(context.WithValue(req.Context(), ctxKeyType{}, 42))
	if got := extract(wrongType); got != "" {
		t.Errorf("with a non-string value = %q, want an exemption", got)
	}
}

func TestJoinKey(t *testing.T) {
	tests := []struct {
		name  string
		parts []string
		want  Key
	}{
		{"all present", []string{"twin", "t1", "read"}, "twin|t1|read"},
		{"empties dropped", []string{"twin", "", "read"}, "twin|read"},
		{"single", []string{"twin"}, "twin"},
		{"none", nil, ""},
		{"all empty", []string{"", ""}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := JoinKey(tc.parts...); got != tc.want {
				t.Errorf("JoinKey(%q) = %q, want %q", tc.parts, got, tc.want)
			}
		})
	}
}

// An override raises what one caller is allowed, without a redeploy and without
// touching anybody else — the point of the whole mechanism.
func TestLimitResolvesPerSubject(t *testing.T) {
	store := NewMemoryOverrideStore()
	if err := store.PutOverride(context.Background(), Override{
		Subject: "/twins", Name: "request", Cap: 3,
	}); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}

	mw := Limit(NewMemoryBackend(), Policy{Algorithm: FixedWindow, Limit: 1, Window: time.Minute},
		byPath, frozen(base), Resolve(NewResolver(store), "request"))

	// The key with an override gets three, not the configured one.
	for i := range 3 {
		rec, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil))
		if !reached {
			t.Fatalf("request %d was refused inside the raised limit", i)
		}
		if got := rec.Header().Get("X-RateLimit-Limit"); got != "3" {
			t.Errorf("request %d: X-RateLimit-Limit = %q, want the override's 3", i, got)
		}
	}
	if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil)); reached {
		t.Error("the raised limit did not apply at all — a fourth request got through")
	}

	// A key with no override keeps what was configured.
	if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/objectives", nil)); !reached {
		t.Fatal("an unrelated key was refused its first request")
	}
	if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/objectives", nil)); reached {
		t.Error("an unrelated key was given the override's ceiling")
	}
}

// Without the option nothing is consulted, so a deployment with no overrides
// pays nothing and behaves exactly as it did.
func TestLimitWithoutAResolverIsUnchanged(t *testing.T) {
	store := NewMemoryOverrideStore()
	if err := store.PutOverride(context.Background(), Override{
		Subject: "/twins", Name: "request", Cap: 100,
	}); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}
	mw := Limit(NewMemoryBackend(), Policy{Algorithm: FixedWindow, Limit: 1, Window: time.Minute},
		byPath, frozen(base))

	if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil)); !reached {
		t.Fatal("the first request was refused")
	}
	if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil)); reached {
		t.Error("an override applied without Resolve being set")
	}

	// Resolve ignores nonsense rather than panicking at wire-up: a nil resolver
	// or an unnamed limit means there is nothing to resolve against.
	mw = Limit(NewMemoryBackend(), Policy{Algorithm: FixedWindow, Limit: 1, Window: time.Minute},
		byPath, frozen(base), Resolve(nil, "request"), Resolve(NewResolver(store), ""))
	if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil)); !reached {
		t.Fatal("the first request was refused")
	}
	if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil)); reached {
		t.Error("an override applied through an unusable Resolve option")
	}
}

// The configured limit itself can move, for a caller that keeps its limits
// somewhere an operator edits rather than in a file read at boot.
func TestLimitResolvesItsBase(t *testing.T) {
	stored := Policy{Algorithm: FixedWindow, Limit: 3, Window: time.Minute}
	mw := Limit(NewMemoryBackend(), Policy{Algorithm: FixedWindow, Limit: 1, Window: time.Minute},
		byPath, frozen(base), Base(func(*http.Request) (Policy, error) { return stored, nil }))

	for i := range 3 {
		rec, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil))
		if !reached {
			t.Fatalf("request %d was refused inside the stored limit", i)
		}
		if got := rec.Header().Get("X-RateLimit-Limit"); got != "3" {
			t.Errorf("request %d: X-RateLimit-Limit = %q, want the stored 3", i, got)
		}
	}
	if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil)); reached {
		t.Error("the stored limit did not bound anything — a fourth request got through")
	}
}

// Base is the base and an override is the exception, in that order: "everybody
// gets this" composed with "except this one".
func TestLimitOverrideAppliesOnTopOfTheResolvedBase(t *testing.T) {
	store := NewMemoryOverrideStore()
	if err := store.PutOverride(context.Background(), Override{
		Subject: "/twins", Name: "request", Cap: 5,
	}); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}
	mw := Limit(NewMemoryBackend(), Policy{Algorithm: FixedWindow, Limit: 1, Window: time.Minute},
		byPath, frozen(base),
		Base(func(*http.Request) (Policy, error) {
			return Policy{Algorithm: FixedWindow, Limit: 2, Window: time.Minute}, nil
		}),
		Resolve(NewResolver(store), "request"))

	// The subject with an override gets the override's ceiling, not the base's.
	rec, _ := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil))
	if got := rec.Header().Get("X-RateLimit-Limit"); got != "5" {
		t.Errorf("X-RateLimit-Limit = %q, want the override's 5", got)
	}
	// A subject with no override gets the resolved base rather than the
	// constructed one — which is the composition under test.
	rec, _ = serve(mw, httptest.NewRequest(http.MethodGet, "/objectives", nil))
	if got := rec.Header().Get("X-RateLimit-Limit"); got != "2" {
		t.Errorf("X-RateLimit-Limit = %q, want the resolved base's 2", got)
	}
}

// A store that cannot be read, or that reads back nonsense, falls to the policy
// Limit was built with. It must never fall to no limit: a malformed row would
// then be a way to switch the limiter off.
func TestLimitFallsBackWhenTheBaseCannotBeResolved(t *testing.T) {
	for _, tc := range []struct {
		name string
		fn   func(*http.Request) (Policy, error)
	}{
		{"unreadable", func(*http.Request) (Policy, error) {
			return Policy{}, errors.New("database down")
		}},
		{"invalid", func(*http.Request) (Policy, error) {
			// Limit 0 with no window is exactly the shape that would admit
			// everything if it were used as-is.
			return Policy{Algorithm: FixedWindow}, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reported error
			mw := Limit(NewMemoryBackend(), Policy{Algorithm: FixedWindow, Limit: 1, Window: time.Minute},
				byPath, frozen(base), Base(tc.fn),
				OnError(func(_ *http.Request, _ Key, err error) { reported = err }))

			if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil)); !reached {
				t.Fatal("the first request was refused")
			}
			if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil)); reached {
				t.Error("a second request got through — the configured limit did not apply")
			}
			if reported == nil {
				t.Error("nothing was reported through OnError, so the fallback would be silent")
			}
		})
	}
}

// Without the option the configured policy is the whole story, and a nil
// function is ignored rather than panicking at wire-up.
func TestLimitWithoutABaseIsUnchanged(t *testing.T) {
	mw := Limit(NewMemoryBackend(), Policy{Algorithm: FixedWindow, Limit: 1, Window: time.Minute},
		byPath, frozen(base), Base(nil))

	if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil)); !reached {
		t.Fatal("the first request was refused")
	}
	if _, reached := serve(mw, httptest.NewRequest(http.MethodGet, "/twins", nil)); reached {
		t.Error("the configured limit did not apply")
	}
}
