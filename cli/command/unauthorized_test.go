package command

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/cli/client"
)

// A 401 means the cached token no longer works, usually because it was
// revoked or the server's signing key changed. Printing the body and exiting 0
// hides that; the error says so and names the command that gets a new one.
func TestUnauthorizedPointsAtLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"invalid token"}`)
	}))
	t.Cleanup(srv.Close)
	apiURL := srv.URL + "/api/v1"
	t.Setenv("KARAKURI_CREDENTIALS", filepath.Join(t.TempDir(), "credentials.json"))
	if err := client.SaveSession(apiURL, client.Session{
		AccessToken: "stale-token", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	t.Cleanup(func() { api = nil })

	_, _, err := runKrkSplit("--api-url", apiURL, "objective", "list")
	if err == nil {
		t.Fatal("krk objective list on a 401 = nil error, want one")
	}
	for _, want := range []string{"invalid token", "krk auth login"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}
