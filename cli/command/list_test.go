package command

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/bsenel/karakuri/cli/client"
)

// startListServer answers every request with the given body.
func startListServer(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
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
	return apiURL
}

// An empty list says so on stderr, in the same words for every list command.
func TestEmptyListsSaySo(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"objective", "list"}, "No objectives.\n"},
		{[]string{"twin", "list"}, "No twins.\n"},
		{[]string{"checkpoint", "list"}, "No pending checkpoints.\n"},
		{[]string{"artifact", "list"}, "No artifacts.\n"},
		{[]string{"objective", "templates"}, "No objective templates.\n"},
		{[]string{"quota", "tiers"}, "No stored tier limits.\n"},
	} {
		for _, body := range []string{"null", "[]\n"} {
			apiURL := startListServer(t, body)
			_, stderr, err := runKrkSplit(append([]string{"--api-url", apiURL}, tc.args...)...)
			if err != nil {
				t.Fatalf("krk %v: %v", tc.args, err)
			}
			if stderr != tc.want {
				t.Errorf("krk %v on %q wrote %q to stderr, want %q", tc.args, body, stderr, tc.want)
			}
		}
	}
}

// The note is for a person reading the pretty format: json and quiet stay as
// they were, and so does a list that has rows.
func TestEmptyListNoteStaysOutOfTheWay(t *testing.T) {
	for _, tc := range []struct{ format, body string }{
		{"json", "null"},
		{"quiet", "null"},
		{"pretty", `[{"id":"o1"}]`},
	} {
		apiURL := startListServer(t, tc.body)
		_, stderr, err := runKrkSplit("--api-url", apiURL, "--output", tc.format, "objective", "list")
		if err != nil {
			t.Fatalf("krk --output %s objective list: %v", tc.format, err)
		}
		if stderr != "" {
			t.Errorf("--output %s on %q wrote %q to stderr, want nothing", tc.format, tc.body, stderr)
		}
	}
}
