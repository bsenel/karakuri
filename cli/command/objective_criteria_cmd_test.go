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

// criteriaObjective is GET /objectives/{id} as the API writes it: met derived
// from state, required absent meaning required, and no state on a criterion
// no verify step has settled yet. The API returns no measured value, only
// the metric op target unit a measured criterion is compared against.
const criteriaObjective = `{
  "id": "obj_42", "title": "Speed up search", "domain": "software", "status": "active",
  "success_criteria": [
    {"id": "tests-pass", "description": "The test suite passes", "kind": "verified",
     "verifier": "software.verify.run_tests", "weight": 0.4, "required": true, "state": "met", "met": true},
    {"id": "ci-green", "description": "CI is green on the pull request", "kind": "verified",
     "verifier": "software.verify.ci_green", "weight": 0.2, "state": "inconclusive", "reason": "verifier_error", "met": false},
    {"id": "p95", "description": "Search p95 latency is at most 250 ms", "kind": "measured",
     "verifier": "ci.latency", "metric": "p95_latency", "op": "<=", "target": 250, "unit": "ms",
     "weight": 0.2, "required": true, "state": "not_met", "met": false},
    {"id": "changelog", "description": "The changelog explains the change", "kind": "judged",
     "evidence": "The CHANGELOG.md diff", "weight": 0.2, "met": false},
    {"id": "coverage", "description": "Statement coverage is at least 80 percent", "kind": "measured",
     "verifier": "ci.coverage", "metric": "statement_coverage", "op": ">=", "target": 80.5, "unit": "percent",
     "weight": 0.1, "required": false, "state": "not_met", "met": false},
    {"id": "bundle", "description": "The bundle is at most 300 kB", "kind": "measured",
     "verifier": "ci.bundle", "metric": "bundle_size", "op": "<=", "target": 300, "unit": "kB",
     "weight": 0.1, "required": false, "state": "met", "met": true}
  ],
  "created_at": "2026-10-25T09:00:00Z", "updated_at": "2026-10-25T09:30:00Z"
}`

// criteriaServer stands in for the API, recording the request and answering
// with the given status and body.
type criteriaServer struct {
	calls        int
	method, path string
}

func startCriteriaServer(t *testing.T, status int, reply string) (*criteriaServer, string) {
	t.Helper()
	s := &criteriaServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls++
		s.method, s.path = r.Method, r.URL.Path
		if status == http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
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

// showCriteria runs krk objective criteria obj_42 against the stand-in and
// returns its stdout split into lines.
func showCriteria(t *testing.T) []string {
	t.Helper()
	return showCriteriaOf(t, criteriaObjective)
}

// showCriteriaOf is showCriteria against an objective the test writes.
func showCriteriaOf(t *testing.T, objective string) []string {
	t.Helper()
	srv, apiURL := startCriteriaServer(t, http.StatusOK, objective)

	stdout, stderr, err := runKrkSplit("--api-url", apiURL, "objective", "criteria", "obj_42")

	if err != nil {
		t.Fatalf("krk objective criteria obj_42: %v\n%s", err, stderr)
	}
	if srv.method != http.MethodGet || srv.path != "/api/v1/objectives/obj_42" {
		t.Fatalf("request = %s %s, want GET /api/v1/objectives/obj_42", srv.method, srv.path)
	}
	return strings.Split(stdout, "\n")
}

// criterionLine is the index and text of the one line naming the criterion.
func criterionLine(t *testing.T, lines []string, id string) (int, string) {
	t.Helper()
	at := -1
	for i, line := range lines {
		if strings.Contains(line, id) {
			if at >= 0 {
				t.Fatalf("criterion %s is on more than one line:\n%s", id, strings.Join(lines, "\n"))
			}
			at = i
		}
	}
	if at < 0 {
		t.Fatalf("no line names criterion %s:\n%s", id, strings.Join(lines, "\n"))
	}
	return at, lines[at]
}

func requireContains(t *testing.T, line string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(line, want) {
			t.Errorf("line %q does not show %q", line, want)
		}
	}
}

// Each required criterion is one line with its id, description, kind and
// state; an inconclusive one says why, and one no verify step has settled
// says not_run rather than looking unmet.
func TestObjectiveCriteriaShowsOneLinePerRequiredCriterion(t *testing.T) {
	lines := showCriteria(t)

	for _, tc := range []struct {
		id    string
		wants []string
	}{
		{"tests-pass", []string{"The test suite passes", "verified", "met"}},
		{"ci-green", []string{"CI is green on the pull request", "verified", "inconclusive: verifier_error"}},
		{"p95", []string{"Search p95 latency is at most 250 ms", "measured", "not_met"}},
		{"changelog", []string{"The changelog explains the change", "judged", "inconclusive: not_run"}},
	} {
		_, line := criterionLine(t, lines, tc.id)
		requireContains(t, line, tc.wants...)
	}
	if _, line := criterionLine(t, lines, "tests-pass"); strings.Contains(line, "not_met") {
		t.Errorf("met criterion reads as not_met: %q", line)
	}
}

// A criterion with required: false is progress, not the gate: it is printed
// in its own section after the required ones and never as a failure.
func TestObjectiveCriteriaPrintsOptionalCriteriaAsProgress(t *testing.T) {
	lines := showCriteria(t)

	progress := -1
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), "progress") {
			progress = i
			break
		}
	}
	if progress < 0 {
		t.Fatalf("no progress section:\n%s", strings.Join(lines, "\n"))
	}
	for _, id := range []string{"tests-pass", "ci-green", "p95", "changelog"} {
		if at, _ := criterionLine(t, lines, id); at > progress {
			t.Errorf("required criterion %s is under the progress section", id)
		}
	}
	for _, id := range []string{"coverage", "bundle"} {
		at, line := criterionLine(t, lines, id)
		if at < progress {
			t.Errorf("optional criterion %s is printed with the gate, before the progress section", id)
		}
		lower := strings.ToLower(line)
		for _, failure := range []string{"not_met", "unmet", "fail"} {
			if strings.Contains(lower, failure) {
				t.Errorf("optional criterion %s reads as a failure (%q): %q", id, failure, line)
			}
		}
	}
	if _, line := criterionLine(t, lines, "bundle"); !strings.Contains(line, "met") {
		t.Errorf("optional criterion bundle does not say it is met: %q", line)
	}
}

// A measured criterion shows what it is compared against, metric op target
// unit, in the gate and in progress alike. The API returns no measured value,
// so none is asserted here.
func TestObjectiveCriteriaShowsTheMeasuredComparison(t *testing.T) {
	lines := showCriteria(t)

	for id, want := range map[string]string{
		"p95":      "p95_latency <= 250 ms",
		"coverage": "statement_coverage >= 80.5 percent",
		"bundle":   "bundle_size <= 300 kB",
	} {
		_, line := criterionLine(t, lines, id)
		requireContains(t, line, want)
	}
}

// baselineObjective is GET /objectives/{id} with relative targets (Phase 41
// step 7): an at_start baseline the loop measured and froze, a baseline
// fixed by hand, an at_start baseline not measured yet, and a measured
// criterion with no baseline at all. baseline_value is the frozen number; the
// API leaves it out for at_start until it has been measured.
const baselineObjective = `{
  "id": "obj_43", "title": "Keep search fast", "domain": "software", "status": "active",
  "success_criteria": [
    {"id": "p95-regress", "description": "Search p95 latency is not more than 10% above the start", "kind": "measured",
     "verifier": "ci.latency", "metric": "p95_latency", "op": "<=", "target": 10, "unit": "ms",
     "baseline": "at_start", "baseline_value": 212.5, "change": "percent",
     "weight": 0.4, "state": "met", "met": true},
    {"id": "bundle-growth", "description": "The bundle grows by at most 20 kB", "kind": "measured",
     "verifier": "ci.bundle", "metric": "bundle_size", "op": "<=", "target": 20, "unit": "kB",
     "baseline": 300, "baseline_value": 300, "change": "absolute",
     "weight": 0.3, "state": "not_met", "met": false},
    {"id": "p99-regress", "description": "Search p99 latency is not more than 15% above the start", "kind": "measured",
     "verifier": "ci.latency", "metric": "p99_latency", "op": "<=", "target": 15, "unit": "ms",
     "baseline": "at_start", "change": "percent",
     "weight": 0.2, "state": "inconclusive", "reason": "no_baseline", "met": false},
    {"id": "p50", "description": "Search p50 latency is at most 80 ms", "kind": "measured",
     "verifier": "ci.latency", "metric": "p50_latency", "op": "<=", "target": 80, "unit": "ms",
     "weight": 0.1, "state": "met", "met": true}
  ],
  "created_at": "2026-10-25T09:00:00Z", "updated_at": "2026-10-25T09:30:00Z"
}`

// requireBesideTarget checks that want is on the criterion's line after its
// metric op target unit and before its description: beside the target, not
// trailing after the prose. Only that stretch is searched, so the state's
// "no_baseline" does not count as showing a baseline.
func requireBesideTarget(t *testing.T, line, target, want, description string) {
	t.Helper()
	tgt, desc := strings.Index(line, target), strings.Index(line, description)
	if tgt < 0 || desc < tgt {
		t.Errorf("line %q does not show the target %q before the description %q", line, target, description)
		return
	}
	if beside := line[tgt:desc]; !strings.Contains(beside, want) {
		t.Errorf("line %q does not show %q beside the target %q", line, want, target)
	}
}

// The person sees the baseline value beside the target, in the criterion's
// unit and with where it came from, because a baseline taken from a start that
// was already bad is met by construction (roadmap Phase 41, section 3).
func TestObjectiveCriteriaShowsTheFrozenBaselineBesideTheTarget(t *testing.T) {
	lines := showCriteriaOf(t, baselineObjective)

	for _, tc := range []struct {
		id, target, baseline, source, otherSource, description string
	}{
		// at_start: measured once when the objective started, then frozen.
		{"p95-regress", "p95_latency <= 10", "baseline 212.5 ms", "at_start", "fixed",
			"Search p95 latency is not more than 10% above the start"},
		// A number in the criterion: fixed by hand, not measured.
		{"bundle-growth", "bundle_size <= 20", "baseline 300 kB", "fixed", "at_start",
			"The bundle grows by at most 20 kB"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			_, line := criterionLine(t, lines, tc.id)
			requireBesideTarget(t, line, tc.target, tc.baseline, tc.description)
			requireBesideTarget(t, line, tc.target, tc.source, tc.description)
			if strings.Contains(line, tc.otherSource) {
				t.Errorf("line %q says the baseline is %s; it is %s", line, tc.otherSource, tc.source)
			}
		})
	}
}

// An at_start baseline the loop has not measured yet has no value to show:
// the line says so, beside the target, rather than printing a zero or
// leaving the person to guess why the criterion is no_baseline.
func TestObjectiveCriteriaSaysAnAtStartBaselineIsNotMeasuredYet(t *testing.T) {
	lines := showCriteriaOf(t, baselineObjective)

	_, line := criterionLine(t, lines, "p99-regress")
	const description = "Search p99 latency is not more than 15% above the start"
	requireContains(t, line, "inconclusive: no_baseline")
	requireBesideTarget(t, line, "p99_latency <= 15", "baseline", description)
	requireBesideTarget(t, line, "p99_latency <= 15", "at_start", description)
	requireBesideTarget(t, line, "p99_latency <= 15", "not measured", description)
	for _, wrong := range []string{"baseline 0", "baseline  ms", "fixed"} {
		if strings.Contains(line, wrong) {
			t.Errorf("an unmeasured at_start baseline reads %q: %q", wrong, line)
		}
	}
}

// A measured criterion with no baseline shows its comparison and nothing
// about a baseline, beside criteria that do have one.
func TestObjectiveCriteriaShowsNoBaselineWhereThereIsNone(t *testing.T) {
	lines := showCriteriaOf(t, baselineObjective)

	_, line := criterionLine(t, lines, "p50")
	requireContains(t, line, "p50_latency <= 80 ms", "Search p50 latency is at most 80 ms")
	if strings.Contains(line, "baseline") {
		t.Errorf("a criterion with no baseline mentions one: %q", line)
	}
}

// With no baseline on any criterion the output is exactly what it was before
// baselines were shown, padding included.
func TestObjectiveCriteriaWithoutBaselinesIsUnchanged(t *testing.T) {
	got := strings.Join(showCriteria(t), "\n")

	if got != criteriaGolden {
		t.Errorf("krk objective criteria obj_42 changed:\n%s\nwant:\n%s", got, criteriaGolden)
	}
}

const criteriaGolden = `obj_42  Speed up search

Required:
  tests-pass  verified  met                                                  The test suite passes
  ci-green    verified  inconclusive: verifier_error                         CI is green on the pull request
  p95         measured  not_met                       p95_latency <= 250 ms  Search p95 latency is at most 250 ms
  changelog   judged    inconclusive: not_run                                The changelog explains the change

Progress (required: false, not part of the gate):
  coverage  measured  not yet met  statement_coverage >= 80.5 percent  Statement coverage is at least 80 percent
  bundle    measured  met          bundle_size <= 300 kB               The bundle is at most 300 kB
`

// A missing objective exits non-zero naming the id the user typed; the API's
// 404 body does not carry it.
func TestObjectiveCriteriaMissingObjectiveNamesTheID(t *testing.T) {
	srv, apiURL := startCriteriaServer(t, http.StatusNotFound, "objective not found\n")

	stdout, stderr, err := runKrkSplit("--api-url", apiURL, "objective", "criteria", "obj_missing")

	if srv.calls != 1 || srv.path != "/api/v1/objectives/obj_missing" {
		t.Fatalf("server got %d requests, last %s %s; want one GET /api/v1/objectives/obj_missing",
			srv.calls, srv.method, srv.path)
	}
	if err == nil {
		t.Fatalf("a 404 exited zero; stdout %q", stdout)
	}
	if seen := err.Error() + stderr; !strings.Contains(seen, "obj_missing") {
		t.Errorf("the user saw %q, which does not name obj_missing", seen)
	}
}

func TestObjectiveCriteriaHelp(t *testing.T) {
	root := NewRoot()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"objective", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("krk objective --help: %v", err)
	}
	if !strings.Contains(out.String(), "criteria") {
		t.Errorf("krk objective --help does not list criteria:\n%s", out.String())
	}

	cmd := objectiveCriteriaCmd()
	requireExample(t, cmd)
	if !strings.Contains(cmd.Use, "<id>") {
		t.Errorf("criteria Use %q does not take an <id>", cmd.Use)
	}
	if !strings.Contains(cmd.Long, "krk objective list") {
		t.Errorf("criteria Long %q does not say where <id> comes from", cmd.Long)
	}
	if cmd.Args == nil || cmd.Args(cmd, nil) == nil {
		t.Error("criteria accepts no <id>")
	}
}
