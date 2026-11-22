package command

import (
	"net/http"
	"testing"
)

// A granted binding prints the server's record on stdout and, in the pretty
// format only, a line on stderr saying who now holds which role over what.
func TestAuthBindingsAddSaysWhatWasGranted(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"scoped", []string{"--scope", "twin:abc"}, "alice now has role operator over twin:abc\n"},
		{"unscoped", nil, "alice now has role operator over everything\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, apiURL := startLoopStopServer(t, http.StatusOK, `{"id":"b1"}`)
			args := append([]string{"--api-url", apiURL, "auth", "bindings", "add", "--principal", "alice", "--role", "operator"}, tc.args...)
			_, stderr, err := runKrkSplit(args...)
			if err != nil {
				t.Fatal(err)
			}
			if stderr != tc.want {
				t.Errorf("stderr = %q, want %q", stderr, tc.want)
			}
			_, stderr, err = runKrkSplit(append([]string{"--output", "json"}, args...)...)
			if err != nil || stderr != "" {
				t.Errorf("--output json: err %v, stderr %q, want none", err, stderr)
			}
		})
	}
}
