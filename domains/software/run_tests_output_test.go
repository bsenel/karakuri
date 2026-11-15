package software

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 44 follow-up to step 4 (#245): run_tests runs go test with -json, and
// the result keeps only the first 4000 bytes of stdout. In -json mode every
// passing test writes run, output and pass events, so a failing test's message
// fell outside that head. The stdout a run_tests result carries is now an
// excerpt built from the whole output: the failures, then one line per
// finished package.

// failFixtureMessage is the message testdata/failfixture's failing test gives.
const failFixtureMessage = "karakuri-failfixture: the widget count is 3, want 4"

// shellRun runs cmd through a plain shell environment, as verifyEnv did before
// this change, and returns its result.
func shellRun(t *testing.T, cmd, workdir string) environment.ActionResult {
	t.Helper()
	params := map[string]any{"cmd": cmd}
	if workdir != "" {
		params["workdir"] = workdir
	}
	res, err := newShellEnv("software.env.verify", "", newTestVerifyEnv().shell.timeout).Act(context.Background(),
		environment.Action{CapabilityID: "software.act.shell_exec", Params: params})
	if err != nil {
		t.Fatalf("shell Act: %v", err)
	}
	return res
}

func TestRunTestsKeepsAFailedTestsMessageInItsOutput(t *testing.T) {
	dir, err := filepath.Abs(filepath.Join("testdata", "failfixture"))
	if err != nil {
		t.Fatal(err)
	}
	const cmd = "GOWORK=off go test -count=1 ./..."

	res := runTestsWith(t, cmd, map[string]any{"workdir": dir})
	out, _ := res.StateDelta["stdout"].(string)

	// What the head held before this change, with -json and without it.
	before := shellRun(t, withGoTestJSON(cmd), dir)
	beforeOut, _ := before.StateDelta["stdout"].(string)
	plain, _ := shellRun(t, cmd, dir).StateDelta["stdout"].(string)
	t.Logf("failure message in the 4000-byte head: %d time(s) with -json before, %d time(s) without -json, %d time(s) now",
		strings.Count(beforeOut, failFixtureMessage), strings.Count(plain, failFixtureMessage), strings.Count(out, failFixtureMessage))

	if !strings.Contains(out, failFixtureMessage) {
		t.Errorf("stdout does not carry the failing test's message %q:\n%s", failFixtureMessage, out)
	}
	if !strings.Contains(out, "FAIL example.com/failfixture\n") && !strings.HasSuffix(out, "FAIL example.com/failfixture") {
		t.Errorf("stdout has no 'FAIL example.com/failfixture' line:\n%s", out)
	}
	if len(out) > 4000 {
		t.Errorf("stdout is %d bytes, want at most 4000", len(out))
	}

	// The verdict is the shell's, as before.
	if res.Success {
		t.Error("a run with a failing test succeeded")
	}
	if res.StateDelta["state"] == string(objective.CriterionInconclusive) {
		t.Errorf("a red suite is inconclusive: %v", res.StateDelta)
	}
	if res.Error != before.Error {
		t.Errorf("Error = %q, want %q as before", res.Error, before.Error)
	}
	for _, key := range []string{"status", "exit_code", "stderr", "timed_out"} {
		if res.StateDelta[key] != before.StateDelta[key] {
			t.Errorf("StateDelta[%q] = %v, want %v as before", key, res.StateDelta[key], before.StateDelta[key])
		}
	}
	if code, _ := res.StateDelta["exit_code"].(int); code != 1 {
		t.Errorf("exit_code = %v, want 1", res.StateDelta["exit_code"])
	}
}

// When no line of stdout is a test2json event, stdout is the raw head exactly
// as the shell returns it: a go test the rewrite reached that printed no JSON,
// and a command that is not go test at all.
func TestRunTestsOutputWithoutJSONEventsIsTheRawHead(t *testing.T) {
	long := `i=0; while [ $i -lt 300 ]; do echo "line $i of output that is not json"; i=$((i+1)); done`
	for name, cmd := range map[string]string{
		"go test printing no json": `go() { ` + long + `; return 1; }; go test ./...`,
		"not go test":              long + `; exit 1`,
	} {
		t.Run(name, func(t *testing.T) {
			res := runTestsWith(t, cmd, nil)
			want := shellRun(t, withGoTestJSON(cmd), "")
			if res.StateDelta["stdout"] != want.StateDelta["stdout"] {
				t.Errorf("stdout = %q\nwant the raw head %q", res.StateDelta["stdout"], want.StateDelta["stdout"])
			}
			if res.Success || res.Error != want.Error {
				t.Errorf("verdict changed: Success=%v Error=%q, want false and %q", res.Success, res.Error, want.Error)
			}
		})
	}
}

// The excerpt's order: the output of each failed test and failed package as
// they appear, then one line per finished package. A passing test's output
// and a test still running are not in it.
func TestGoTestJSONExcerpt(t *testing.T) {
	stdout := strings.Join([]string{
		`{"Action":"start","Package":"example.com/a"}`,
		`{"Action":"run","Package":"example.com/a","Test":"TestOK"}`,
		`{"Action":"output","Package":"example.com/a","Test":"TestOK","Output":"=== RUN   TestOK\n"}`,
		`{"Action":"output","Package":"example.com/a","Test":"TestOK","Output":"    a_test.go:5: all good\n"}`,
		`{"Action":"pass","Package":"example.com/a","Test":"TestOK"}`,
		`{"Action":"output","Package":"example.com/a","Output":"ok  \texample.com/a\t0.1s\n"}`,
		`{"Action":"pass","Package":"example.com/a"}`,
		`{"Action":"run","Package":"example.com/b","Test":"TestBad"}`,
		`{"Action":"output","Package":"example.com/b","Test":"TestBad","Output":"=== RUN   TestBad\n"}`,
		`{"Action":"output","Package":"example.com/b","Test":"TestBad","Output":"    b_test.go:9: wrong answer\n"}`,
		`{"Action":"output","Package":"example.com/b","Test":"TestBad","Output":"--- FAIL: TestBad (0.00s)\n"}`,
		`{"Action":"fail","Package":"example.com/b","Test":"TestBad"}`,
		`{"Action":"output","Package":"example.com/b","Output":"FAIL\n"}`,
		`{"Action":"fail","Package":"example.com/b"}`,
		`{"Action":"output","Package":"example.com/c","Output":"?   \texample.com/c\t[no test files]\n"}`,
		`{"Action":"skip","Package":"example.com/c"}`,
		`{"Action":"run","Package":"example.com/slow","Test":"TestSleeps"}`,
		`{"Action":"output","Package":"example.com/slow","Test":"TestSleeps","Output":"still going\n"}`,
		`not json`,
	}, "\n")

	got, ok := goTestJSONExcerpt(stdout)
	if !ok {
		t.Fatal("no excerpt from test2json output")
	}
	want := "=== RUN   TestBad\n    b_test.go:9: wrong answer\n--- FAIL: TestBad (0.00s)\nFAIL\n" +
		"ok example.com/a\nFAIL example.com/b\n? example.com/c\n"
	if got != want {
		t.Errorf("excerpt =\n%s\nwant\n%s", got, want)
	}

	if _, ok := goTestJSONExcerpt("ok  \texample.com/a\t0.1s\n"); ok {
		t.Error("an excerpt from output with no test2json event")
	}
}

// A test file that does not compile is reported as build-output events keyed
// by ImportPath, not Package, then a build-fail (go 1.24 and later print them
// to stdout under -json). The compiler's message is the failure a person needs,
// and it is the usual one for a pushed red test.
func TestGoTestJSONExcerptKeepsACompileError(t *testing.T) {
	stdout := strings.Join([]string{
		`{"ImportPath":"example.com/d [example.com/d.test]","Action":"build-output","Output":"# example.com/d [example.com/d.test]\n"}`,
		`{"ImportPath":"example.com/d [example.com/d.test]","Action":"build-output","Output":"./d_test.go:3:27: undefined: notYetWritten\n"}`,
		`{"ImportPath":"example.com/d [example.com/d.test]","Action":"build-fail"}`,
		`{"Action":"start","Package":"example.com/d"}`,
		`{"Action":"output","Package":"example.com/d","Output":"FAIL\texample.com/d [build failed]\n"}`,
		`{"Action":"fail","Package":"example.com/d","FailedBuild":"example.com/d [example.com/d.test]"}`,
	}, "\n")

	got, ok := goTestJSONExcerpt(stdout)
	if !ok {
		t.Fatal("no excerpt from test2json output")
	}
	want := "# example.com/d [example.com/d.test]\n./d_test.go:3:27: undefined: notYetWritten\n" +
		"FAIL\texample.com/d [build failed]\nFAIL example.com/d\n"
	if got != want {
		t.Errorf("excerpt =\n%s\nwant\n%s", got, want)
	}
}
