package command

import "testing"

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
