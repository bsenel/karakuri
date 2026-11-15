package command

import (
	"net/http"
	"testing"
)

// A --tier there is no tier for is refused before any request, with the four
// tiers there are and a command that uses one.
func TestMemoryRecallListsTheTiersOnAnUnknownTier(t *testing.T) {
	srv, apiURL := startLoopStopServer(t, http.StatusOK, "[]")
	_, _, err := runKrkSplit("--api-url", apiURL, "memory", "recall", "--agent", "agent_42", "--query", "deploy", "--tier", "long")
	if err == nil {
		t.Fatal("krk memory recall --tier long succeeded, want an error")
	}
	want := "invalid --tier \"long\": use working, episodic, semantic or procedural, e.g. `krk memory recall --agent agent_42 --query \"deploy\" --tier semantic`"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if srv.requests != 0 {
		t.Errorf("server saw %d requests, want none", srv.requests)
	}
}

// A recall that matches nothing says so, in the words the web UI's memory
// page uses, and points at what to change. Stdout is the server's bytes in
// every format, and --output json says nothing on stderr.
func TestMemoryRecallSaysWhenNothingMatched(t *testing.T) {
	args := []string{"memory", "recall", "--agent", "agent_42", "--query", "deploy"}
	want := "No memories matched. Try a broader --query or another --tier.\n"
	for _, body := range []string{"null", "[]\n"} {
		apiURL := startListServer(t, body)
		_, stderr, err := runKrkSplit(append([]string{"--api-url", apiURL}, args...)...)
		if err != nil {
			t.Fatalf("krk %v: %v", args, err)
		}
		if stderr != want {
			t.Errorf("krk %v on %q wrote %q to stderr, want %q", args, body, stderr, want)
		}
	}
	for _, tc := range []struct{ format, body string }{
		{"json", "null"},
		{"pretty", `[{"id":"m1"}]`},
	} {
		apiURL := startListServer(t, tc.body)
		_, stderr, err := runKrkSplit(append([]string{"--api-url", apiURL, "--output", tc.format}, args...)...)
		if err != nil {
			t.Fatalf("krk --output %s %v: %v", tc.format, args, err)
		}
		if stderr != "" {
			t.Errorf("krk --output %s %v on %q wrote %q to stderr, want nothing", tc.format, args, tc.body, stderr)
		}
	}
}
