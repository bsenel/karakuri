package command

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/cli/client"
)

// loopStopServer stands in for POST /api/v1/loops/{id}/stop, answering status
// with body and recording every request it saw.
type loopStopServer struct {
	status   int
	body     string
	requests int
	method   string
	path     string
	reqBody  []byte
}

func startLoopStopServer(t *testing.T, status int, body string) (*loopStopServer, string) {
	t.Helper()
	s := &loopStopServer{status: status, body: body}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests++
		s.method, s.path = r.Method, r.URL.Path
		s.reqBody, _ = io.ReadAll(r.Body)
		if s.status != http.StatusOK {
			http.Error(w, s.body, s.status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, s.body)
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
	return s, apiURL
}

// runKrkCapturingStdout runs krk and returns everything it printed: what went
// through cobra's writers and what client.PrintOutput wrote to os.Stdout.
func runKrkCapturingStdout(t *testing.T, args ...string) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan []byte)
	go func() {
		data, _ := io.ReadAll(r)
		done <- data
	}()

	root := NewRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	runErr := root.Execute()

	os.Stdout = saved
	_ = w.Close()
	printed := <-done
	_ = r.Close()
	return string(printed) + out.String(), runErr
}

const loopStopResponse = `{"loop_id":"loop_123","found":"running"}`

// The reason typed after --reason is the reason the server is asked to record,
// and what the server answers is what the operator sees.
func TestLoopStopPostsTheReasonAndPrintsTheResult(t *testing.T) {
	srv, apiURL := startLoopStopServer(t, http.StatusOK, loopStopResponse)

	printed, err := runKrkCapturingStdout(t, "--api-url", apiURL, "loop", "stop", "loop_123", "--reason", "wrong branch")
	if err != nil {
		t.Fatalf("krk loop stop: %v\n%s", err, printed)
	}
	if srv.requests != 1 {
		t.Fatalf("server saw %d requests, want exactly 1", srv.requests)
	}
	if srv.method != http.MethodPost || srv.path != "/api/v1/loops/loop_123/stop" {
		t.Fatalf("request = %s %s, want POST /api/v1/loops/loop_123/stop", srv.method, srv.path)
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(srv.reqBody, &body); err != nil {
		t.Fatalf("request body %q is not JSON: %v", srv.reqBody, err)
	}
	if body.Reason != "wrong branch" {
		t.Errorf("reason = %q, want %q", body.Reason, "wrong branch")
	}
	for _, want := range []string{"loop_123", "running"} {
		if !strings.Contains(printed, want) {
			t.Errorf("output %q does not show %q from the server's answer", printed, want)
		}
	}
}

// Without --reason the stop still goes out, and carries no reason of the
// CLI's making: the server fills one in from the principal.
func TestLoopStopWithoutReasonLeavesItToTheServer(t *testing.T) {
	srv, apiURL := startLoopStopServer(t, http.StatusOK, loopStopResponse)

	printed, err := runKrkCapturingStdout(t, "--api-url", apiURL, "loop", "stop", "loop_123")
	if err != nil {
		t.Fatalf("krk loop stop: %v\n%s", err, printed)
	}
	if srv.requests != 1 {
		t.Fatalf("server saw %d requests, want exactly 1", srv.requests)
	}
	if srv.method != http.MethodPost || srv.path != "/api/v1/loops/loop_123/stop" {
		t.Fatalf("request = %s %s, want POST /api/v1/loops/loop_123/stop", srv.method, srv.path)
	}
	if raw := bytes.TrimSpace(srv.reqBody); len(raw) > 0 && string(raw) != "null" {
		var body struct {
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("request body %q is not JSON: %v", raw, err)
		}
		if body.Reason != "" {
			t.Errorf("reason = %q, want none so the server names the principal", body.Reason)
		}
	}
}

// A loop the server does not know is an error that says so, not a printed
// body and exit 0.
func TestLoopStopUnknownLoopIsAnError(t *testing.T) {
	srv, apiURL := startLoopStopServer(t, http.StatusNotFound, `loop "loop_404" not found`)

	printed, err := runKrkCapturingStdout(t, "--api-url", apiURL, "loop", "stop", "loop_404")
	if err == nil {
		t.Fatalf("krk loop stop on a 404 succeeded, want an error\n%s", printed)
	}
	if srv.requests != 1 {
		t.Errorf("server saw %d requests, want the 404 to come from it", srv.requests)
	}
	for _, want := range []string{"loop_404", "not found"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// stop takes exactly one loop ID; anything else is refused before the server
// is asked, with the usage line that says what it takes.
func TestLoopStopNeedsExactlyOneLoopID(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"none", nil, "accepts 1 arg(s), received 0"},
		{"two", []string{"loop_1", "loop_2"}, "accepts 1 arg(s), received 2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, apiURL := startLoopStopServer(t, http.StatusOK, loopStopResponse)

			args := append([]string{"--api-url", apiURL, "loop", "stop"}, tc.args...)
			_, err := runKrkCapturingStdout(t, args...)
			if err == nil {
				t.Fatalf("krk %s succeeded, want an error", strings.Join(args, " "))
			}
			for _, want := range []string{tc.want, "Usage: krk loop stop"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if srv.requests != 0 {
				t.Errorf("server saw %d requests, want none", srv.requests)
			}
		})
	}
}

// stop hangs off krk loop, and its help shows how to run it with a reason.
func TestLoopStopHelp(t *testing.T) {
	var listed bool
	for _, sub := range loopCmd().Commands() {
		if sub.Name() == "stop" {
			listed = true
		}
	}
	if !listed {
		t.Error("krk loop does not list a stop subcommand")
	}

	cmd := loopStopCmd()
	requireExample(t, cmd)
	for _, want := range []string{"krk loop stop", "--reason"} {
		if !strings.Contains(cmd.Example, want) {
			t.Errorf("stop Example %q does not show %q", cmd.Example, want)
		}
	}
	if !strings.Contains(cmd.Long, "krk loop start") {
		t.Errorf("stop Long %q does not say where <loop-id> comes from", cmd.Long)
	}
	if cmd.Flags().Lookup("reason") == nil {
		t.Error("stop has no --reason flag")
	}
}
