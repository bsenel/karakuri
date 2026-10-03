package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/cli/client"
)

// calibrationJSON is a report as the API encodes eval.CalibrationReport: no
// JSON tags, so every field under its Go name. The numbers are chosen to be
// distinguishable in rendered output.
const calibrationJSON = `{
  "TwinID": "t1",
  "Since": "2026-09-25T00:00:00Z",
  "Until": "2026-09-28T00:00:00Z",
  "N": 20, "Agreed": 15, "Skipped": 6, "Agreement": 0.75,
  "Replayable": 42,
  "Confusion": {
    "JudgePassHumanApprove": 11, "JudgePassHumanReject": 3,
    "JudgeFailHumanApprove": 2, "JudgeFailHumanReject": 4
  },
  "ByDecision": {
    "approve": {"N": 13, "Agreed": 11, "JudgePass": 11},
    "reject":  {"N": 7,  "Agreed": 4,  "JudgePass": 3}
  },
  "Items": [
    {"CheckpointID": "cp-1", "ObjectiveID": "o-1", "Choice": "approve",
     "HumanApprove": true, "JudgePass": true, "Agreed": true,
     "Reply": "PASS", "Title": "Keep the build green",
     "Criterion": "CI passes", "Actions": "- run tests"},
    {"CheckpointID": "cp-2", "ObjectiveID": "o-2", "Choice": "reject",
     "JudgePass": true, "Reply": "PASS — it would work",
     "Title": "Ship the release", "Criterion": "tagged", "Actions": "- tag"},
    {"CheckpointID": "cp-3", "ObjectiveID": "o-3", "Choice": "modify",
     "Error": "judge unavailable", "Title": "Rotate keys"}
  ]
}`

// evalServer stands in for the API, answering POST /api/v1/eval/calibrate with
// calibrationJSON and recording what the CLI sent.
type evalServer struct {
	path, method string
	body         map[string]any
}

func startEvalServer(t *testing.T) (*evalServer, string) {
	t.Helper()
	s := &evalServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.path, s.method = r.URL.Path, r.Method
		raw, _ := io.ReadAll(r.Body)
		s.body = map[string]any{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &s.body); err != nil {
				t.Errorf("request body is not JSON: %v: %s", err, raw)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, calibrationJSON)
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

// runKrk runs the root command and returns what it wrote. Human-readable
// output goes through the command's writer, so a test can read it.
func runKrk(t *testing.T, args ...string) string {
	t.Helper()
	root := NewRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("krk %s: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

func TestEvalCalibratePostsTheWindow(t *testing.T) {
	srv, apiURL := startEvalServer(t)

	before := time.Now().UTC()
	runKrk(t, "--api-url", apiURL, "--output", "quiet",
		"eval", "calibrate", "--twin", "t1", "--since", "72h", "--limit", "5")

	if srv.method != http.MethodPost || srv.path != "/api/v1/eval/calibrate" {
		t.Fatalf("request = %s %s, want POST /api/v1/eval/calibrate", srv.method, srv.path)
	}
	if srv.body["twin"] != "t1" {
		t.Errorf("twin = %v, want t1", srv.body["twin"])
	}
	if srv.body["limit"] != float64(5) {
		t.Errorf("limit = %v, want 5", srv.body["limit"])
	}
	raw, _ := srv.body["since"].(string)
	since, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("since = %q, want RFC3339: %v", raw, err)
	}
	want := before.Add(-72 * time.Hour)
	if d := since.Sub(want); d < -time.Minute || d > time.Minute {
		t.Errorf("since = %v, want about %v", since, want)
	}
}

func TestEvalCalibrateMarkdown(t *testing.T) {
	_, apiURL := startEvalServer(t)

	out := runKrk(t, "--api-url", apiURL, "eval", "calibrate", "--since", "72h", "--markdown")

	if !strings.Contains(out, "Real-history judge calibration") {
		t.Errorf("no heading in:\n%s", out)
	}
	lower := strings.ToLower(out)
	for _, want := range []string{"agreement", "replayable", "confusion"} {
		if !strings.Contains(lower, want) {
			t.Errorf("no %q in:\n%s", want, out)
		}
	}
	// N, agreement (0.75 or 75%), the replayable count, and every cell of the
	// confusion matrix.
	for _, want := range []string{"20", "75", "42", "11", "3", "2", "4"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in:\n%s", want, out)
		}
	}
}

func TestEvalCalibrateExport(t *testing.T) {
	_, apiURL := startEvalServer(t)
	file := filepath.Join(t.TempDir(), "golden.json")

	runKrk(t, "--api-url", apiURL, "--output", "quiet",
		"eval", "calibrate", "--since", "72h", "--export", file)

	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	// The export holds plan text from real checkpoints: owner-only.
	info, err := os.Stat(file)
	if err != nil {
		t.Fatalf("stat export: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("export mode = %o, want 600", info.Mode().Perm())
	}
	var entries []goldenEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("export is not a JSON array of golden entries: %v\n%s", err, raw)
	}
	// cp-3 has no reply, so there is nothing for a parser to read: it is left
	// out, as eval.ExportGolden leaves it out.
	if len(entries) != 2 {
		t.Fatalf("exported %d entries, want 2: %+v", len(entries), entries)
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Provenance, "exported:") {
			t.Errorf("%s provenance = %q, want exported:…", e.ID, e.Provenance)
		}
		if e.ID == "" || e.Reply == "" || e.Label == "" {
			t.Errorf("incomplete entry %+v", e)
		}
	}
	if entries[1].Label != "reject" || entries[1].Reply != "PASS — it would work" || entries[1].Title != "Ship the release" {
		t.Errorf("entry = %+v, want cp-2's label, reply and title", entries[1])
	}
}

// Calibration spends a model call per checkpoint and runs for minutes, so the
// client it posts with must outlast the bound every other command keeps.
func TestEvalCalibrateClientOutlastsTheDefaultTimeout(t *testing.T) {
	def := client.New("http://example.invalid/api/v1")
	if def.HTTP.Timeout != 120*time.Second {
		t.Fatalf("default client timeout = %v, want 2m0s", def.HTTP.Timeout)
	}

	long := evalCalibrateClient(def)
	if long.HTTP.Timeout != evalCalibrateTimeout {
		t.Errorf("calibrate client timeout = %v, want %v", long.HTTP.Timeout, evalCalibrateTimeout)
	}
	if evalCalibrateTimeout < 10*time.Minute {
		t.Errorf("evalCalibrateTimeout = %v, want well past the 2m default", evalCalibrateTimeout)
	}
	if long.BaseURL != def.BaseURL {
		t.Errorf("calibrate client BaseURL = %q, want %q", long.BaseURL, def.BaseURL)
	}
	if def.HTTP.Timeout != 120*time.Second {
		t.Errorf("default client timeout became %v: the shared client must not be mutated", def.HTTP.Timeout)
	}
}

// Ctrl-C cancels the command's context; the request must go with it rather
// than wait out the long timeout.
func TestEvalCalibrateStopsWhenCancelled(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	apiURL := srv.URL + "/api/v1"
	t.Setenv("KARAKURI_CREDENTIALS", filepath.Join(t.TempDir(), "credentials.json"))
	if err := client.SaveSession(apiURL, client.Session{
		AccessToken: "test-token", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	t.Cleanup(func() { api = nil })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := NewRoot()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--api-url", apiURL, "--output", "quiet", "eval", "calibrate"})

	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()
	time.AfterFunc(100*time.Millisecond, cancel)

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("krk eval calibrate did not stop when its context was cancelled")
	}
}
