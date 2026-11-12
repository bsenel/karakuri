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

// The server records a choice it does not recognise as an approval, so a
// typo in --decision must stop in the CLI, before any request is sent, and
// the error must name the three values that work.
func TestInvalidDecisionIsRefusedBeforeSending(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
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

	for _, args := range [][]string{
		{"checkpoint", "resolve", "cp_123", "--decision", "deny"},
		{"loop", "resume", "loop_123", "--decision", "approved"},
	} {
		_, _, err := runKrkSplit(append([]string{"--api-url", apiURL}, args...)...)
		if err == nil {
			t.Fatalf("krk %s = nil error, want one", strings.Join(args, " "))
		}
		for _, want := range []string{"invalid --decision", "approve, reject or modify", args[2]} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("krk %s: error %q does not contain %q", strings.Join(args, " "), err, want)
			}
		}
	}
	if requests != 0 {
		t.Errorf("server got %d requests, want 0: an invalid decision must not be sent", requests)
	}

	if _, _, err := runKrkSplit("--api-url", apiURL, "checkpoint", "resolve", "cp_123", "--decision", "approve"); err != nil {
		t.Fatalf("krk checkpoint resolve --decision approve: %v", err)
	}
	if requests != 1 {
		t.Errorf("server got %d requests after a valid decision, want 1", requests)
	}
}
