package command

import (
	"net/http"
	"strings"
	"testing"
)

// A --until that is not RFC3339 is refused before anything is sent, with the
// command to type instead, not Go's time layout.
func TestQuotaRequestBadUntilShowsTheCommand(t *testing.T) {
	srv, apiURL := startLoopStopServer(t, http.StatusOK, `{}`)

	_, err := runKrkCapturingStdout(t, "--api-url", apiURL, "quota", "request",
		"--tier", "llm-tokens", "--twin", "t_7f2a", "--cap", "5000000", "--reason", "launch week", "--until", "tomorrow")
	if err == nil {
		t.Fatal("krk quota request --until tomorrow succeeded, want an error")
	}
	for _, want := range []string{
		`--until "tomorrow" is not an RFC3339 timestamp`,
		`krk quota request --tier llm-tokens --twin t_7f2a --cap 5000000 --reason "launch week" --until 2026-09-01T00:00:00Z`,
		"drop --until",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "2006-01-02") {
		t.Errorf("error %q leaks Go's time layout", err)
	}
	if srv.requests != 0 {
		t.Errorf("server saw %d requests, want none", srv.requests)
	}
}
