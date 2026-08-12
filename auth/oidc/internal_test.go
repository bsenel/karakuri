package oidc

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bsenel/karakuri/auth"
)

// Randomness failing is not a reason to proceed with a predictable state, and
// the only way to stage it is from inside the package.
func TestLoginHandlerFailsWithoutRandomness(t *testing.T) {
	t.Parallel()
	boom := errors.New("no entropy")

	for _, failOn := range []int{1, 2} {
		t.Run([]string{"", "state", "nonce"}[failOn], func(t *testing.T) {
			t.Parallel()
			calls := 0
			p := &Provider{
				cfg:    Config{CookieName: "flow", StateTTL: time.Minute},
				sealer: auth.Sealer{Key: []byte("test-key")},
				rand: func(n int) (string, error) {
					calls++
					if calls == failOn {
						return "", boom
					}
					return randomString(n)
				},
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

// A sealer with no key cannot start a login, and saying so beats redirecting
// the user to an identity provider they will never get back from.
func TestLoginHandlerFailsWithoutSealKey(t *testing.T) {
	t.Parallel()
	p := &Provider{
		cfg:    Config{CookieName: "flow", StateTTL: time.Minute},
		sealer: auth.Sealer{},
		rand:   randomString,
	}

	rec := httptest.NewRecorder()
	p.LoginHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
