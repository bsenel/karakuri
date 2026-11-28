package command

import (
	"net/http"
	"testing"
)

// A stopped loop prints the server's record on stdout and, in the pretty
// format only, a line on stderr saying the loop is stopped for good.
func TestLoopStopSaysItStopped(t *testing.T) {
	for _, tc := range []struct {
		format, want string
	}{
		{"pretty", "loop loop_1 stopped; it stays stopped across a server restart\n"},
		{"json", ""},
		{"quiet", ""},
	} {
		_, apiURL := startLoopStopServer(t, http.StatusOK, `{"status":"stopped"}`)
		_, stderr, err := runKrkSplit("--api-url", apiURL, "--output", tc.format, "loop", "stop", "loop_1")
		if err != nil {
			t.Fatalf("%s: %v", tc.format, err)
		}
		if stderr != tc.want {
			t.Errorf("--output %s stderr = %q, want %q", tc.format, stderr, tc.want)
		}
	}
}
