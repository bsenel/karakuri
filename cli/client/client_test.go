package client

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
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
