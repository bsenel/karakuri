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

// A 403 is a role gap, not a bad login; the error says how to see who you are
// and which command an administrator uses to grant the role.
func TestForbiddenPointsAtWhoamiAndBindings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":"not allowed"}`)
	}))
	t.Cleanup(srv.Close)
	apiURL := srv.URL + "/api/v1"
	t.Setenv("KARAKURI_CREDENTIALS", filepath.Join(t.TempDir(), "credentials.json"))
	if err := client.SaveSession(apiURL, client.Session{
		AccessToken: "token", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	t.Cleanup(func() { api = nil })

	_, _, err := runKrkSplit("--api-url", apiURL, "objective", "list")
	if err == nil {
		t.Fatal("krk objective list on a 403 = nil error, want one")
	}
	for _, want := range []string{"not allowed", "krk auth whoami", "krk auth bindings add"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}
