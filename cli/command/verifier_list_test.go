package command

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/cli/client"
)

// verifierCapabilities is GET /domains/capabilities as the API writes it. A
// verifier is a capability marked verifiable, the marker a discovered MCP tool
// never carries because it may not settle a criterion; its metrics are the
// declarations beside output_schema. ci_green is a verifier that declares no
// metric; write_code and the discovered tool are not verifiers.
const verifierCapabilities = `[
  {"id": "software.verify.ci_green", "name": "CI Green", "domain": "software", "verifiable": true,
   "input_schema": {"type": "object"}, "output_schema": {"type": "object"}},
  {"id": "software.verify.coverage", "name": "Coverage", "domain": "software", "verifiable": true,
   "input_schema": {"type": "object"}, "output_schema": {"type": "object"},
   "metrics": [
     {"name": "statement_coverage", "unit": "percent", "direction": "higher_is_better",
      "params": [{"name": "packages", "type": "string"}]},
     {"name": "flaky_tests", "unit": "count", "direction": "lower_is_better"}
   ]},
  {"id": "software.act.write_code", "name": "Write Code", "domain": "software", "needs_workspace": true,
   "input_schema": {"type": "object"}, "output_schema": {"type": "object"}},
  {"id": "web.verify.latency", "name": "Latency", "domain": "web", "verifiable": true,
   "input_schema": {"type": "object"}, "output_schema": {"type": "object"},
   "metrics": [
     {"name": "p95_latency", "unit": "ms", "direction": "lower_is_better",
      "params": [{"name": "route", "type": "string"}, {"name": "window_min", "type": "integer"}]}
   ]},
  {"id": "mcp__search__query", "name": "query", "domain": "mcp",
   "input_schema": {"type": "object"}, "output_schema": {"type": "object"}}
]`

// verifierServer stands in for the API. It records the request and, like the
// real handler, answers ?domain= with that domain's capabilities only.
type verifierServer struct {
	calls        int
	method, path string
	domain       string
}

func startVerifierServer(t *testing.T) (*verifierServer, string) {
	t.Helper()
	s := &verifierServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls++
		s.method, s.path = r.Method, r.URL.Path
		s.domain = r.URL.Query().Get("domain")
		reply := verifierCapabilities
		if s.domain == "software" {
			reply = `[` + strings.Join([]string{
				`{"id": "software.verify.ci_green", "name": "CI Green", "domain": "software", "verifiable": true}`,
				`{"id": "software.verify.coverage", "name": "Coverage", "domain": "software", "verifiable": true,
				  "metrics": [{"name": "statement_coverage", "unit": "percent", "direction": "higher_is_better",
				               "params": [{"name": "packages", "type": "string"}]},
				              {"name": "flaky_tests", "unit": "count", "direction": "lower_is_better"}]}`,
				`{"id": "software.act.write_code", "name": "Write Code", "domain": "software", "needs_workspace": true}`,
			}, ",") + `]`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply)
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

// listVerifiers runs krk verifier list with the given extra arguments against
// the stand-in, checks it asked GET /domains/capabilities once and returns its
// stdout split into lines.
func listVerifiers(t *testing.T, args ...string) (*verifierServer, []string) {
	t.Helper()
	srv, apiURL := startVerifierServer(t)

	stdout, stderr, err := runKrkSplit(append([]string{"--api-url", apiURL, "verifier", "list"}, args...)...)

	if err != nil {
		t.Fatalf("krk verifier list %s: %v\n%s", strings.Join(args, " "), err, stderr)
	}
	if srv.calls != 1 || srv.method != http.MethodGet || srv.path != "/api/v1/domains/capabilities" {
		t.Fatalf("server got %d requests, last %s %s; want one GET /api/v1/domains/capabilities",
			srv.calls, srv.method, srv.path)
	}
	return srv, strings.Split(stdout, "\n")
}

// verifierBlock is the lines that belong to one verifier: its own line and the
// lines after it up to the next verifier's line.
func verifierBlock(t *testing.T, lines []string, id string, verifiers []string) []string {
	t.Helper()
	at, _ := criterionLine(t, lines, id)
	end := len(lines)
	for _, other := range verifiers {
		if other == id {
			continue
		}
		for i := at + 1; i < end; i++ {
			if strings.Contains(lines[i], other) {
				end = i
				break
			}
		}
	}
	return lines[at:end]
}

// metricLine is the one line of a verifier's block naming the metric.
func metricLine(t *testing.T, block []string, metric string) string {
	t.Helper()
	found := ""
	for _, line := range block {
		if strings.Contains(line, metric) {
			if found != "" {
				t.Fatalf("metric %s is on more than one line:\n%s", metric, strings.Join(block, "\n"))
			}
			found = line
		}
	}
	if found == "" {
		t.Fatalf("no line under the verifier names metric %s:\n%s", metric, strings.Join(block, "\n"))
	}
	return found
}

// Each verifier is listed by its id, and under it each metric it declares
// with the metric's name, unit and direction on one line, and the typed
// parameters the metric takes.
func TestVerifierListShowsEachVerifierWithItsMetrics(t *testing.T) {
	_, lines := listVerifiers(t)
	verifiers := []string{"software.verify.ci_green", "software.verify.coverage", "web.verify.latency"}

	coverage := verifierBlock(t, lines, "software.verify.coverage", verifiers)
	requireContains(t, metricLine(t, coverage, "statement_coverage"), "percent", "higher_is_better", "packages", "string")
	requireContains(t, metricLine(t, coverage, "flaky_tests"), "count", "lower_is_better")

	latency := verifierBlock(t, lines, "web.verify.latency", verifiers)
	requireContains(t, metricLine(t, latency, "p95_latency"), "ms", "lower_is_better", "route", "window_min", "integer")
	for _, other := range []string{"statement_coverage", "flaky_tests"} {
		if strings.Contains(strings.Join(latency, "\n"), other) {
			t.Errorf("web.verify.latency shows %s, a metric of another verifier:\n%s", other, strings.Join(latency, "\n"))
		}
	}
}

// A verifier that declares no metric is still a verifier: it settles verified
// criteria by pass or fail, so it is listed, with no metric under it.
func TestVerifierListShowsAVerifierWithoutMetrics(t *testing.T) {
	_, lines := listVerifiers(t)
	verifiers := []string{"software.verify.ci_green", "software.verify.coverage", "web.verify.latency"}

	ciGreen := verifierBlock(t, lines, "software.verify.ci_green", verifiers)
	text := strings.Join(ciGreen, "\n")
	for _, metric := range []string{"statement_coverage", "flaky_tests", "p95_latency"} {
		if strings.Contains(text, metric) {
			t.Errorf("software.verify.ci_green shows %s, which it does not declare:\n%s", metric, text)
		}
	}
}

// A capability that is not marked verifiable is not a verifier, whether a pack
// declared it or it was discovered over MCP.
func TestVerifierListLeavesOutCapabilitiesThatAreNotVerifiers(t *testing.T) {
	_, lines := listVerifiers(t)

	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "software.verify.ci_green") {
		t.Fatalf("krk verifier list does not show software.verify.ci_green, so leaving the others out proves nothing:\n%s", out)
	}
	for _, id := range []string{"software.act.write_code", "mcp__search__query"} {
		if strings.Contains(out, id) {
			t.Errorf("krk verifier list shows %s, which is not a verifier:\n%s", id, out)
		}
	}
}

// --domain is sent to the server as ?domain=, and only that domain's verifiers
// are shown. A CLI that dropped the flag would get every domain's capabilities
// back and show web.verify.latency.
func TestVerifierListDomainFilterIsSentToTheServer(t *testing.T) {
	srv, lines := listVerifiers(t, "--domain", "software")

	if srv.domain != "software" {
		t.Errorf("request carried domain=%q, want software", srv.domain)
	}
	out := strings.Join(lines, "\n")
	if strings.Contains(out, "web.verify.latency") {
		t.Errorf("krk verifier list --domain software shows the web domain's verifier:\n%s", out)
	}
	for _, id := range []string{"software.verify.ci_green", "software.verify.coverage"} {
		if !strings.Contains(out, id) {
			t.Errorf("krk verifier list --domain software does not show %s:\n%s", id, out)
		}
	}
}

func TestVerifierListHelp(t *testing.T) {
	root := NewRoot()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("krk --help: %v", err)
	}
	if !strings.Contains(out.String(), "verifier") {
		t.Errorf("krk --help does not list verifier:\n%s", out.String())
	}

	out.Reset()
	root = NewRoot()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"verifier", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("krk verifier --help: %v", err)
	}
	if !strings.Contains(out.String(), "list") {
		t.Errorf("krk verifier --help does not list list:\n%s", out.String())
	}

	cmd := verifierListCmd()
	requireExample(t, cmd)
	flag := cmd.Flags().Lookup("domain")
	if flag == nil {
		t.Fatal("verifier list has no --domain flag")
	}
	if !strings.Contains(flag.Usage, "krk domain list") {
		t.Errorf("--domain usage %q does not say where a domain ID comes from", flag.Usage)
	}
}
