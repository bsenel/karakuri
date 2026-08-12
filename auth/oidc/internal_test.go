package oidc

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The flow cookie's failure modes are reachable only from inside the package:
// a well-behaved browser returns the cookie it was given, and the interesting
// cases are the ones where it does not.
func newFlowProvider(t *testing.T) *Provider {
	t.Helper()
	return &Provider{
		cfg:  Config{StateKey: []byte("test-key"), CookieName: "flow", StateTTL: time.Minute},
		now:  time.Now,
		rand: randomString,
	}
}

func TestOpenFlowRoundTrip(t *testing.T) {
	t.Parallel()
	p := newFlowProvider(t)

	sealed := p.sealFlow(flowState{State: "s", Nonce: "n", Verifier: "v", Expires: time.Now().Add(time.Minute).Unix()})
	got, err := p.openFlow(sealed)
	if err != nil {
		t.Fatalf("openFlow: %v", err)
	}
	if got.State != "s" || got.Nonce != "n" || got.Verifier != "v" {
		t.Fatalf("round trip lost data: %+v", got)
	}
}

func TestOpenFlowRejects(t *testing.T) {
	t.Parallel()
	p := newFlowProvider(t)

	payload, _, _ := strings.Cut(p.sealFlow(flowState{State: "s", Expires: time.Now().Add(time.Minute).Unix()}), ".")
	expired := p.sealFlow(flowState{State: "s", Expires: time.Now().Add(-time.Minute).Unix()})

	// A payload that is valid base64 but not valid JSON, signed correctly — so
	// only the unmarshal can catch it.
	notJSON := base64.RawURLEncoding.EncodeToString([]byte("not json"))
	signedNotJSON := notJSON + "." + base64.RawURLEncoding.EncodeToString(p.sign([]byte(notJSON)))

	// Likewise a signature over a payload that is not valid base64.
	badPayload := "!!!not-base64!!!"
	signedBadPayload := badPayload + "." + base64.RawURLEncoding.EncodeToString(p.sign([]byte(badPayload)))

	cases := []struct {
		name  string
		value string
	}{
		{name: "no separator", value: "no-dot-here"},
		{name: "signature is not base64", value: payload + ".!!!"},
		{name: "signature is wrong", value: payload + "." + base64.RawURLEncoding.EncodeToString([]byte("wrong"))},
		{name: "payload is not base64", value: signedBadPayload},
		{name: "payload is not JSON", value: signedNotJSON},
		{name: "expired", value: expired},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := p.openFlow(tc.value); err == nil {
				t.Fatalf("openFlow(%q) returned nil", tc.name)
			}
		})
	}
}

// A cookie sealed with one key must not open with another — otherwise a
// browser could mint its own state and defeat the CSRF protection entirely.
func TestOpenFlowRejectsForeignKey(t *testing.T) {
	t.Parallel()
	mine := newFlowProvider(t)
	theirs := &Provider{cfg: Config{StateKey: []byte("another-key"), CookieName: "flow", StateTTL: time.Minute}, now: time.Now}

	sealed := theirs.sealFlow(flowState{State: "s", Expires: time.Now().Add(time.Minute).Unix()})
	if _, err := mine.openFlow(sealed); err == nil {
		t.Fatal("a cookie signed with a different key was accepted")
	}
}

// Randomness failing is not a reason to proceed with a predictable state.
func TestLoginHandlerFailsWithoutRandomness(t *testing.T) {
	t.Parallel()
	boom := errors.New("no entropy")

	for _, failOn := range []int{1, 2} {
		t.Run([]string{"", "state", "nonce"}[failOn], func(t *testing.T) {
			t.Parallel()
			calls := 0
			p := newFlowProvider(t)
			p.rand = func(n int) (string, error) {
				calls++
				if calls == failOn {
					return "", boom
				}
				return randomString(n)
			}

			rec := httptest.NewRecorder()
			p.LoginHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", rec.Code)
			}
			if len(rec.Result().Cookies()) != 0 {
				t.Error("a failed login still set a flow cookie")
			}
		})
	}
}
