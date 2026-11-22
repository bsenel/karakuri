package command

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/cli/client"
)

// oversightDigest is a digest over two streams: one with passes, a waiting
// pull request, a refusal and a quota block, one quiet.
const oversightDigest = `{
  "twin_id": "twin_1",
  "objectives": [
    {"id": "obj_delivery", "title": "Deliver phase 40", "status": "active",
     "senses": 6, "reconciles": 3, "converged": 1, "escalations": 1, "failures": 1,
     "actions": 4, "criteria_met": 0.5,
     "waiting_pull_requests": [
       {"number": 231, "title": "feat(report): oversight", "review": "changes requested"}
     ],
     "refusals": [
       {"at": "2026-11-21T10:00:00Z", "capability": "software.act.git_push", "reason": "force push is outside the bounds"}
     ],
     "blocks": [
       {"kind": "waiting_for_quota", "reason": "claude 5-hour limit", "at": "2026-11-21T11:00:00Z", "until": "2026-11-21T16:00:00Z"},
       {"kind": "verify_failed", "subject": "tests-green", "count": 2, "at": "2026-11-21T12:00:00Z"}
     ]},
    {"id": "obj_discovery", "title": "Discover backlog", "status": "active",
     "senses": 2, "reconciles": 0, "criteria_met": 1}
  ]
}`

// startOversightServer serves the digest and records every request's path and
// query, so a test can tell what the CLI asked for.
func startOversightServer(t *testing.T) (apiURL string, requests func() []string) {
	t.Helper()
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, oversightDigest)
	}))
	t.Cleanup(srv.Close)

	apiURL = srv.URL + "/api/v1"
	t.Setenv("KARAKURI_CREDENTIALS", filepath.Join(t.TempDir(), "credentials.json"))
	if err := client.SaveSession(apiURL, client.Session{
		AccessToken: "test-token", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	t.Cleanup(func() { api = nil })
	return apiURL, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), got...)
	}
}

// Phase 40 step 14a: a reconcile blocker is listed with its kind, subject and
// how many passes saw it.
func TestReportOversightPrintsBlockerKindSubjectAndCount(t *testing.T) {
	apiURL, _ := startOversightServer(t)
	stdout, stderr, err := runKrkSplit("--api-url", apiURL, "report", "oversight", "--twin", "twin_1")
	if err != nil {
		t.Fatalf("krk report oversight: %v\n%s", err, stderr)
	}
	var line string
	for _, l := range strings.Split(stdout, "\n") {
		if strings.Contains(l, "verify_failed") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("no line for the verify_failed blocker:\n%s", stdout)
	}
	if !strings.Contains(line, "tests-green") {
		t.Errorf("blocker line lacks its subject: %q", line)
	}
	if !strings.Contains(line, "2") {
		t.Errorf("blocker line lacks its count: %q", line)
	}
}

// One listing shows, per stream, its passes and outcomes, the pull requests
// waiting with the review's result, the refusals and the blocks.
func TestReportOversightListsEachStream(t *testing.T) {
	apiURL, _ := startOversightServer(t)

	stdout, stderr, err := runKrkSplit("--api-url", apiURL, "report", "oversight", "--twin", "twin_1")
	if err != nil {
		t.Fatalf("krk report oversight: %v\n%s", err, stderr)
	}
	for _, want := range []string{
		"Deliver phase 40", "Discover backlog",
		"3 reconciles", "6 senses", "1 converged", "1 escalation", "1 failure",
		"#231", "feat(report): oversight", "changes requested",
		"software.act.git_push", "force push is outside the bounds",
		"waiting_for_quota", "claude 5-hour limit", "2026-11-21T16:00:00Z",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("listing lacks %q:\n%s", want, stdout)
		}
	}
	delivery, discovery := strings.Index(stdout, "Deliver phase 40"), strings.Index(stdout, "Discover backlog")
	if delivery < 0 || discovery < 0 {
		t.Fatalf("both streams must be listed:\n%s", stdout)
	}
	if idx := strings.Index(stdout, "#231"); idx < delivery || (discovery > delivery && idx > discovery) {
		t.Errorf("the pull request is not listed under the delivery stream:\n%s", stdout)
	}
}

// A stream with no pull request, refusal or block lists none.
func TestReportOversightQuietStreamListsNoRefusalsOrBlocks(t *testing.T) {
	apiURL, _ := startOversightServer(t)

	stdout, _, err := runKrkSplit("--api-url", apiURL, "report", "oversight", "--twin", "twin_1")
	if err != nil {
		t.Fatalf("krk report oversight: %v", err)
	}
	at := strings.Index(stdout, "Discover backlog")
	if at < 0 {
		t.Fatalf("the quiet stream is not listed:\n%s", stdout)
	}
	quiet := stdout[at:]
	for _, unwanted := range []string{"#231", "waiting_for_quota", "git_push"} {
		if strings.Contains(quiet, unwanted) {
			t.Errorf("the quiet stream lists %q:\n%s", unwanted, quiet)
		}
	}
}

// --window is sent to the API and changes the request: an ignored flag would
// send the same window for every value.
func TestReportOversightWindowChangesTheRequest(t *testing.T) {
	apiURL, requests := startOversightServer(t)

	for _, args := range [][]string{
		{"report", "oversight", "--twin", "twin_1"},
		{"report", "oversight", "--twin", "twin_1", "--window", "168h"},
	} {
		if _, stderr, err := runKrkSplit(append([]string{"--api-url", apiURL}, args...)...); err != nil {
			t.Fatalf("krk %v: %v\n%s", args, err, stderr)
		}
	}
	got := requests()
	if len(got) != 2 {
		t.Fatalf("want 2 requests, got %v", got)
	}
	if !strings.Contains(got[0], "twin_id=twin_1") || !strings.Contains(got[0], "window=24h") {
		t.Errorf("default request = %q, want twin_id=twin_1 and window=24h", got[0])
	}
	if !strings.Contains(got[1], "window=168h") {
		t.Errorf("--window 168h request = %q, want window=168h", got[1])
	}
	if got[0] == got[1] {
		t.Errorf("--window did not change the request: %q", got[0])
	}
}

// The listing needs a twin to read; the CLI refuses before calling the API.
func TestReportOversightRequiresATwin(t *testing.T) {
	apiURL, requests := startOversightServer(t)

	if _, _, err := runKrkSplit("--api-url", apiURL, "report", "oversight"); err == nil {
		t.Fatal("krk report oversight without --twin succeeded")
	}
	if n := len(requests()); n != 0 {
		t.Errorf("%d API calls without --twin, want 0", n)
	}
}
