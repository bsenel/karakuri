package command

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/cli/client"
)

// A checkpoint resolved twice is refused in plain text. The refusal is the
// error, with a pointer at the checkpoints still open, not "null" and exit 0.
func TestCheckpointResolveSaysWhyTheServerRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `checkpoint "cp_1" is already resolved`, http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	apiURL := srv.URL + "/api/v1"
	t.Setenv("KARAKURI_CREDENTIALS", filepath.Join(t.TempDir(), "credentials.json"))
	if err := client.SaveSession(apiURL, client.Session{
		AccessToken: "test-token", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	t.Cleanup(func() { api = nil })

	_, _, err := runKrkSplit("--api-url", apiURL, "checkpoint", "resolve", "cp_1", "--decision", "approve")
	if err == nil {
		t.Fatal("resolve succeeded on a refused checkpoint")
	}
	for _, want := range []string{
		`resolve checkpoint cp_1: HTTP 500: checkpoint "cp_1" is already resolved`,
		"`krk checkpoint list` shows the checkpoints still waiting for a decision",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
}
