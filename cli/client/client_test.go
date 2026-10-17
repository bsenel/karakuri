package client

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A server that is not there has to be named as such, with the flag that
// points the CLI elsewhere — not left as a bare "connection refused".
func TestUnreachableServerSaysSo(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close()

	_, _, err := New(base).GetPublic("/health")
	if err == nil {
		t.Fatal("no error from a closed server")
	}
	msg := err.Error()
	for _, want := range []string{"cannot reach the Karakuri API at " + base, "Is the server running?", "--api-url"} {
		if !strings.Contains(msg, want) {
			t.Errorf("%q does not contain %q", msg, want)
		}
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		t.Errorf("%q no longer wraps the transport error", msg)
	}
}

// An unknown ID used to print "null" and exit 0: the 404 body is plain text,
// not JSON. It has to be an error that says what to check.
func TestNotFoundSaysCheckTheID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "objective not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL)
	c.session = &Session{AccessToken: "t", ExpiresAt: time.Now().Add(time.Hour)}
	_, status, err := c.Get("/objectives/o_bogus")
	if err == nil {
		t.Fatal("no error from a 404")
	}
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
	msg := err.Error()
	for _, want := range []string{"objective not found\n", "Check the ID", "krk objective list"} {
		if !strings.Contains(msg, want) {
			t.Errorf("%q does not contain %q", msg, want)
		}
	}
}
