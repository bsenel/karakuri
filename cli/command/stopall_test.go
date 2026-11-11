package command

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Phase 37 step 4: krk stop-all --reason "..." calls POST /api/v1/stop-all.
// It is a top-level command, not a loop or objective subcommand, because one
// call reaches both kinds of run. The roadmap's working name already fits the
// CLI: `krk migrate` and `krk web` are top-level verbs too.

const stopAllResponse = `{
  "loops_stopped": [{"loop_id": "loop_123", "found": "running"}],
  "objectives_paused": [{"objective_id": "obj_456", "pass_cancelled": true}],
  "not_reached": [{"kind": "objective", "id": "obj_789", "reason": "not permitted"}]
}`

// The reason typed after --reason is the reason the server is asked to record,
// and what the server answers, including what it could not reach, is what the
// operator sees.
func TestStopAllPostsTheReasonAndPrintsTheResult(t *testing.T) {
	srv, apiURL := startLoopStopServer(t, http.StatusOK, stopAllResponse)

	printed, err := runKrkCapturingStdout(t, "--api-url", apiURL, "stop-all", "--reason", "pushing to the wrong remote")
	if err != nil {
		t.Fatalf("krk stop-all: %v\n%s", err, printed)
	}
	if srv.requests != 1 {
		t.Fatalf("server saw %d requests, want exactly 1", srv.requests)
	}
	if srv.method != http.MethodPost || srv.path != "/api/v1/stop-all" {
		t.Fatalf("request = %s %s, want POST /api/v1/stop-all", srv.method, srv.path)
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(srv.reqBody, &body); err != nil {
		t.Fatalf("request body %q is not JSON: %v", srv.reqBody, err)
	}
	if body.Reason != "pushing to the wrong remote" {
		t.Errorf("reason = %q, want %q", body.Reason, "pushing to the wrong remote")
	}
	for _, want := range []string{"loop_123", "obj_456", "obj_789", "not permitted"} {
		if !strings.Contains(printed, want) {
			t.Errorf("output %q does not show %q from the server's answer", printed, want)
		}
	}
}

// Without --reason the call still goes out, and carries no reason of the CLI's
// making: the server fills one in from the principal.
func TestStopAllWithoutReasonLeavesItToTheServer(t *testing.T) {
	srv, apiURL := startLoopStopServer(t, http.StatusOK, stopAllResponse)

	printed, err := runKrkCapturingStdout(t, "--api-url", apiURL, "stop-all")
	if err != nil {
		t.Fatalf("krk stop-all: %v\n%s", err, printed)
	}
	if srv.requests != 1 {
		t.Fatalf("server saw %d requests, want exactly 1", srv.requests)
	}
	if srv.method != http.MethodPost || srv.path != "/api/v1/stop-all" {
		t.Fatalf("request = %s %s, want POST /api/v1/stop-all", srv.method, srv.path)
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if raw := strings.TrimSpace(string(srv.reqBody)); raw != "" && raw != "null" {
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			t.Fatalf("request body %q is not JSON: %v", raw, err)
		}
	}
	if body.Reason != "" {
		t.Errorf("reason = %q, want none so the server names the principal", body.Reason)
	}
}

// A refusal from the server is an error that carries its body, not a printed
// body and exit 0.
func TestStopAllServerErrorIsAnError(t *testing.T) {
	srv, apiURL := startLoopStopServer(t, http.StatusInternalServerError, "list running loops: database is locked")

	printed, err := runKrkCapturingStdout(t, "--api-url", apiURL, "stop-all", "--reason", "x")
	if err == nil {
		t.Fatalf("krk stop-all on a 500 succeeded, want an error\n%s", printed)
	}
	if srv.requests != 1 {
		t.Errorf("server saw %d requests, want the 500 to come from it", srv.requests)
	}
	if !strings.Contains(err.Error(), "database is locked") {
		t.Errorf("error %q does not carry the server's body", err)
	}
}

// stop-all takes no argument: a stray one (a loop ID meant for krk loop stop)
// is refused before the server is asked, rather than stopping everything.
func TestStopAllTakesNoArgument(t *testing.T) {
	srv, apiURL := startLoopStopServer(t, http.StatusOK, stopAllResponse)

	_, err := runKrkCapturingStdout(t, "--api-url", apiURL, "stop-all", "loop_123")
	if err == nil {
		t.Fatal("krk stop-all loop_123 succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "Usage: krk stop-all") {
		t.Errorf("error %q does not show the usage line", err)
	}
	if srv.requests != 0 {
		t.Errorf("server saw %d requests, want none", srv.requests)
	}
}

// stop-all hangs off the root, and its help shows how to run it with a reason
// and says what it leaves for a person to do afterwards.
func TestStopAllHelp(t *testing.T) {
	var listed bool
	for _, sub := range NewRoot().Commands() {
		if sub.Name() == "stop-all" {
			listed = true
		}
	}
	if !listed {
		t.Error("krk does not list a stop-all command")
	}

	cmd := stopAllCmd()
	requireExample(t, cmd)
	for _, want := range []string{"krk stop-all", "--reason"} {
		if !strings.Contains(cmd.Example, want) {
			t.Errorf("stop-all Example %q does not show %q", cmd.Example, want)
		}
	}
	// Resuming is per objective, by a person: the help says how.
	if !strings.Contains(cmd.Long, "krk objective resume") {
		t.Errorf("stop-all Long %q does not say how to resume an objective it paused", cmd.Long)
	}
	if cmd.Flags().Lookup("reason") == nil {
		t.Error("stop-all has no --reason flag")
	}
}
