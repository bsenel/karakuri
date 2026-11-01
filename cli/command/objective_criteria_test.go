package command

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/cli/client"
)

// exampleCriteriaFile is the example the create help points at.
const exampleCriteriaFile = "docs/examples/criteria.yaml"

// createServer stands in for the API, recording the POST /objectives body and
// answering with the given status and body.
type createServer struct {
	calls        int
	method, path string
	body         map[string]json.RawMessage
}

func startCreateServer(t *testing.T, status int, reply string) (*createServer, string) {
	t.Helper()
	s := &createServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls++
		s.method, s.path = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		s.body = nil
		if err := json.Unmarshal(raw, &s.body); err != nil {
			t.Errorf("request body %q is not a JSON object: %v", raw, err)
		}
		w.Header().Set("Content-Type", "application/json")
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

func createArgs(apiURL string, extra ...string) []string {
	return append([]string{"--api-url", apiURL, "--output", "quiet",
		"objective", "create", "--title", "Ship the export endpoint", "--twin", "t_7f2a"}, extra...)
}

func writeCriteriaFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "criteria.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// sentCriteria is the success_criteria the server received, decoded the way
// the server decodes JSON, so a number is a float64 and a boolean a bool.
func (s *createServer) sentCriteria(t *testing.T) []any {
	t.Helper()
	if s.method != http.MethodPost || s.path != "/api/v1/objectives" {
		t.Fatalf("request = %s %s, want POST /api/v1/objectives", s.method, s.path)
	}
	raw, ok := s.body["success_criteria"]
	if !ok {
		t.Fatalf("request body has no success_criteria; keys: %v", keys(s.body))
	}
	var got []any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("success_criteria %s is not a JSON array: %v", raw, err)
	}
	return got
}

func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func decodeJSON(t *testing.T, s string) []any {
	t.Helper()
	var v []any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("decode %s: %v", s, err)
	}
	return v
}

func requireCriteria(t *testing.T, got []any, want string) {
	t.Helper()
	if w := decodeJSON(t, want); !reflect.DeepEqual(got, w) {
		gotJSON, _ := json.Marshal(got)
		t.Errorf("success_criteria = %s\nwant %s", gotJSON, want)
	}
}

const criteriaYAML = `- id: tests-pass
  description: The test suite passes on the delivery branch
  kind: verified
  verifier: software.verify.run_tests
  required: true
  weight: 0.5
- id: coverage
  description: Statement coverage is at least 80 percent
  kind: measured
  verifier: ci.coverage
  metric: statement_coverage
  op: ">="
  target: 80
  unit: percent
  required: false
  weight: 0.25
- id: changelog-explains
  description: The changelog entry tells a user what changed
  kind: judged
  evidence: The CHANGELOG.md diff
  required: true
  weight: 0.25
`

const criteriaJSON = `[
  {"id": "tests-pass", "description": "The test suite passes on the delivery branch",
   "kind": "verified", "verifier": "software.verify.run_tests", "required": true, "weight": 0.5},
  {"id": "coverage", "description": "Statement coverage is at least 80 percent",
   "kind": "measured", "verifier": "ci.coverage", "metric": "statement_coverage",
   "op": ">=", "target": 80, "unit": "percent", "required": false, "weight": 0.25},
  {"id": "changelog-explains", "description": "The changelog entry tells a user what changed",
   "kind": "judged", "evidence": "The CHANGELOG.md diff", "required": true, "weight": 0.25}
]`

// The file has the API's shape, so its criteria go out as they are written:
// target and weight as JSON numbers, required as a JSON boolean, in order.
func TestCriteriaFileSendsTheCriteria(t *testing.T) {
	srv, apiURL := startCreateServer(t, http.StatusCreated, `{"id":"obj_1"}`)
	path := writeCriteriaFile(t, criteriaYAML)

	runKrk(t, createArgs(apiURL, "--criteria-file", path)...)

	requireCriteria(t, srv.sentCriteria(t), criteriaJSON)
}

// Each --criterion is one criterion, sent in the order the flags were given,
// with target a JSON number.
func TestCriterionFlagSendsOnePerFlagInOrder(t *testing.T) {
	srv, apiURL := startCreateServer(t, http.StatusCreated, `{"id":"obj_1"}`)

	runKrk(t, createArgs(apiURL,
		"--criterion", "id=p95;kind=measured;verifier=ci.latency;metric=p95_latency;op=<=;target=250;unit=ms",
		"--criterion", "id=coverage;kind=measured;verifier=ci.coverage;metric=statement_coverage;op=>=;target=80.5;unit=percent",
	)...)

	requireCriteria(t, srv.sentCriteria(t), `[
	  {"id": "p95", "kind": "measured", "verifier": "ci.latency", "metric": "p95_latency",
	   "op": "<=", "target": 250, "unit": "ms"},
	  {"id": "coverage", "kind": "measured", "verifier": "ci.coverage", "metric": "statement_coverage",
	   "op": ">=", "target": 80.5, "unit": "percent"}
	]`)
}

// The file is the primary form: its criteria come first, then the one-offs.
func TestCriteriaFileComesBeforeCriterionFlags(t *testing.T) {
	srv, apiURL := startCreateServer(t, http.StatusCreated, `{"id":"obj_1"}`)
	path := writeCriteriaFile(t, `- id: tests-pass
  description: The test suite passes
  kind: verified
  verifier: software.verify.run_tests
`)

	runKrk(t, createArgs(apiURL,
		"--criterion", "id=p95;kind=measured;verifier=ci.latency;metric=p95_latency;op=<=;target=250;unit=ms",
		"--criteria-file", path,
	)...)

	requireCriteria(t, srv.sentCriteria(t), `[
	  {"id": "tests-pass", "description": "The test suite passes", "kind": "verified",
	   "verifier": "software.verify.run_tests"},
	  {"id": "p95", "kind": "measured", "verifier": "ci.latency", "metric": "p95_latency",
	   "op": "<=", "target": 250, "unit": "ms"}
	]`)
}

// Validation is the server's: an unknown verifier and a judged criterion with
// no evidence go out as written, and the server's 400 reaches the user with
// the field it names and a non-zero exit.
func TestCriteriaAreNotValidatedByTheCLI(t *testing.T) {
	const field = "success_criteria[1].evidence"
	srv, apiURL := startCreateServer(t, http.StatusBadRequest,
		`{"error":"a judged criterion needs evidence","field":"`+field+`"}`)
	path := writeCriteriaFile(t, `- id: speed
  description: The page loads fast
  kind: verified
  verifier: no.such.verifier
- id: tone
  description: The release note reads well
  kind: judged
`)

	stdout, stderr, err := runKrkSplit(createArgs(apiURL, "--criteria-file", path)...)

	if srv.calls != 1 {
		t.Fatalf("server got %d requests, want 1: the CLI must send the criteria, not check them", srv.calls)
	}
	requireCriteria(t, srv.sentCriteria(t), `[
	  {"id": "speed", "description": "The page loads fast", "kind": "verified", "verifier": "no.such.verifier"},
	  {"id": "tone", "description": "The release note reads well", "kind": "judged"}
	]`)
	if err == nil {
		t.Fatalf("a 400 exited zero; stdout %q", stdout)
	}
	if seen := err.Error() + stderr; !strings.Contains(seen, field) {
		t.Errorf("the user saw %q, which does not name the field %s", seen, field)
	}
}

// The CLI's own errors are syntax only, each naming what it could not read,
// and nothing is sent.
func TestCriteriaSyntaxErrorsNameTheInput(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.yaml")
	unparsable := writeCriteriaFile(t, "- id: tests-pass\n  description: [unclosed\n")
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"unreadable file":   {[]string{"--criteria-file", missing}, missing},
		"unparsable file":   {[]string{"--criteria-file", unparsable}, unparsable},
		"token without '='": {[]string{"--criterion", "id=p95;kindmeasured;target=250"}, "kindmeasured"},
	} {
		t.Run(name, func(t *testing.T) {
			srv, apiURL := startCreateServer(t, http.StatusCreated, `{"id":"obj_1"}`)

			_, stderr, err := runKrkSplit(createArgs(apiURL, tc.args...)...)

			if err == nil {
				t.Fatal("expected an error")
			}
			if seen := err.Error() + stderr; !strings.Contains(seen, tc.want) {
				t.Errorf("error %q does not name %q", seen, tc.want)
			}
			if srv.calls != 0 {
				t.Errorf("server got %d requests, want none", srv.calls)
			}
		})
	}
}

// With neither flag the key is absent, so the template's criteria apply; an
// empty or null list would be a request for none.
func TestNoCriteriaFlagsSendsNoCriteriaKey(t *testing.T) {
	srv, apiURL := startCreateServer(t, http.StatusCreated, `{"id":"obj_1"}`)

	runKrk(t, createArgs(apiURL, "--template", "software.objective.delivery")...)

	if srv.method != http.MethodPost || srv.path != "/api/v1/objectives" {
		t.Fatalf("request = %s %s, want POST /api/v1/objectives", srv.method, srv.path)
	}
	if raw, ok := srv.body["success_criteria"]; ok {
		t.Errorf("request body has success_criteria %s, want no key", raw)
	}
}

func TestObjectiveCreateHelpShowsCriteria(t *testing.T) {
	cmd := objectiveCreateCmd()
	for _, want := range []string{"--criteria-file", exampleCriteriaFile, "--criterion"} {
		if !strings.Contains(cmd.Example, want) {
			t.Errorf("create Example %q does not show %q", cmd.Example, want)
		}
	}
	for _, name := range []string{"criteria-file", "criterion"} {
		flag := cmd.Flags().Lookup(name)
		if flag == nil {
			t.Fatalf("create has no --%s flag", name)
		}
		if strings.TrimSpace(flag.Usage) == "" {
			t.Errorf("--%s has no usage", name)
		}
	}
	if usage := cmd.Flags().Lookup("criterion").Usage; !strings.Contains(usage, "id=") {
		t.Errorf("--criterion usage %q does not show the id=..;kind=.. form", usage)
	}
}

// The example the help points at goes through --criteria-file like any other
// file, so it cannot rot: every criterion is an outcome with an id, a
// description and a kind, and a judged one names its evidence.
func TestExampleCriteriaFileParses(t *testing.T) {
	srv, apiURL := startCreateServer(t, http.StatusCreated, `{"id":"obj_1"}`)

	runKrk(t, createArgs(apiURL, "--criteria-file", filepath.Join("..", "..", exampleCriteriaFile))...)

	got := srv.sentCriteria(t)
	if len(got) < 2 {
		t.Fatalf("example has %d criteria, want several", len(got))
	}
	judged, required := 0, 0
	for i, c := range got {
		crit, ok := c.(map[string]any)
		if !ok {
			t.Fatalf("criterion %d is %T, want an object", i, c)
		}
		for _, field := range []string{"id", "description", "kind"} {
			if s, _ := crit[field].(string); s == "" {
				t.Errorf("criterion %d has no %s", i, field)
			}
		}
		if crit["kind"] == "judged" {
			judged++
			if s, _ := crit["evidence"].(string); s == "" {
				t.Errorf("judged criterion %v has no evidence", crit["id"])
			}
		}
		if crit["required"] == true {
			required++
		}
	}
	if judged == 0 {
		t.Error("example has no judged criterion to show evidence on")
	}
	if required == 0 {
		t.Error("example has no required criterion; the server refuses such an objective")
	}
}
