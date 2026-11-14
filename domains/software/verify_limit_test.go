package software

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 44 step 4: a verification that runs out of time, or whose tool is not
// installed, says nothing about the work. It settles inconclusive with its
// reason, never as a failed run. The go test output below is printed by the
// command itself, so these tests need no Go toolchain inside it and stay fast.

// runTestsWith runs software.verify.run_tests with cmd and the given extra
// params through the verify environment.
func runTestsWith(t *testing.T, cmd string, extra map[string]any) environment.ActionResult {
	t.Helper()
	params := map[string]any{"cmd": cmd}
	for k, v := range extra {
		params[k] = v
	}
	res, err := newTestVerifyEnv().Act(context.Background(), environment.Action{
		CapabilityID: "software.verify.run_tests",
		Params:       params,
	})
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	return res
}

// requireInconclusive asserts the run settled nothing, with reason, and said
// why. assertInconclusive in latency_error_rate_test.go is for measuring
// verifiers; this one is for a command.
func requireInconclusive(t *testing.T, res environment.ActionResult, reason objective.InconclusiveReason) {
	t.Helper()
	if res.Success {
		t.Errorf("a run that settled nothing succeeded (StateDelta %v)", res.StateDelta)
	}
	if got, _ := res.StateDelta["state"].(string); got != string(objective.CriterionInconclusive) {
		t.Errorf("state = %q, want %q (StateDelta %v)", got, objective.CriterionInconclusive, res.StateDelta)
	}
	if got, _ := res.StateDelta["reason"].(string); got != string(reason) {
		t.Errorf("reason = %q, want %q (StateDelta %v)", got, reason, res.StateDelta)
	}
	if res.Error == "" {
		t.Error("an inconclusive run says nothing about why")
	}
}

// finishedPackage is one entry of StateDelta "finished_packages".
type finishedPackage struct {
	Package string `json:"package"`
	Result  string `json:"result"`
}

// finishedPackages reads StateDelta "finished_packages" into package -> result,
// whichever slice type the environment used, by its JSON shape: the shape is
// what the audit log and the API carry.
func finishedPackages(t *testing.T, res environment.ActionResult) map[string]string {
	t.Helper()
	raw, ok := res.StateDelta["finished_packages"]
	if !ok {
		t.Fatalf("no finished_packages in StateDelta %v", res.StateDelta)
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("finished_packages does not encode: %v", err)
	}
	var list []finishedPackage
	if err := json.Unmarshal(b, &list); err != nil {
		t.Fatalf("finished_packages = %s, want a list of {package, result}: %v", b, err)
	}
	got := map[string]string{}
	for _, p := range list {
		if _, dup := got[p.Package]; dup {
			t.Errorf("package %q listed twice in %s", p.Package, b)
		}
		got[p.Package] = p.Result
	}
	return got
}

func assertPackages(t *testing.T, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("finished_packages = %v, want exactly %v", got, want)
	}
	for pkg, result := range want {
		if got[pkg] != result {
			t.Errorf("package %q: result = %q, want %q (all: %v)", pkg, got[pkg], result, got)
		}
	}
}

// (1) A command that outlives params.timeout_sec is inconclusive: timeout, not
// a failed run. Before this step it was Success=false with status "timeout"
// and no state, which the loop counted not_met.
func TestRunTestsKilledAtItsLimitIsInconclusiveTimeout(t *testing.T) {
	res := runTestsWith(t, "echo ok; exec sleep 5", map[string]any{"timeout_sec": 1})

	requireInconclusive(t, res, objective.ReasonTimeout)
	if !strings.Contains(res.Error, "timed out") {
		t.Errorf("error should say the command timed out, got %q", res.Error)
	}
	if res.StateDelta["verifier"] != "software.verify.run_tests" {
		t.Errorf("result should still name the verifier, got %v", res.StateDelta["verifier"])
	}
}

// (1, continued) The limit is the limit for the whole command, not for /bin/sh.
// go test runs each package's test binary as a child; killing only the shell
// left the child holding stdout, and the result arrived when the child ended.
func TestRunTestsReturnsAtItsLimitWhenAChildOutlivesTheShell(t *testing.T) {
	start := time.Now()
	res := runTestsWith(t, "echo ok; sleep 5", map[string]any{"timeout_sec": 1})
	elapsed := time.Since(start)

	requireInconclusive(t, res, objective.ReasonTimeout)
	if elapsed > 3*time.Second {
		t.Errorf("a 1s limit returned after %s: the child kept the run alive past its limit", elapsed.Round(time.Millisecond))
	}
}

// (2) test2json output: a line with Action pass or fail, a Package and no Test
// is a finished package. Test-level lines and a package still printing output
// when the limit came are not.
func TestRunTestsTimeoutListsFinishedPackagesFromTest2JSON(t *testing.T) {
	lines := []string{
		`{"Action":"start","Package":"example.com/a"}`,
		`{"Action":"run","Package":"example.com/a","Test":"TestOne"}`,
		`{"Action":"pass","Package":"example.com/a","Test":"TestOne","Elapsed":0.01}`,
		`{"Action":"pass","Package":"example.com/a","Elapsed":0.1}`,
		`{"Action":"run","Package":"example.com/b","Test":"TestTwo"}`,
		`{"Action":"fail","Package":"example.com/b","Test":"TestTwo","Elapsed":0.02}`,
		`{"Action":"fail","Package":"example.com/b","Elapsed":0.2}`,
		`{"Action":"run","Package":"example.com/slow","Test":"TestSleeps"}`,
		`{"Action":"output","Package":"example.com/slow","Test":"TestSleeps","Output":"still going\n"}`,
	}
	cmd := "printf '%s\\n' '" + strings.Join(lines, "' '") + "'; exec sleep 5"

	res := runTestsWith(t, cmd, map[string]any{"timeout_sec": 1})

	requireInconclusive(t, res, objective.ReasonTimeout)
	assertPackages(t, finishedPackages(t, res), map[string]string{
		"example.com/a": "pass",
		"example.com/b": "fail",
	})
}

// (2) Plain go test output: "ok  \tpkg\t0.1s" and "FAIL\tpkg\t0.2s" are
// finished packages; "--- FAIL: TestX" and the bare closing "FAIL" are not.
func TestRunTestsTimeoutListsFinishedPackagesFromPlainOutput(t *testing.T) {
	out := strings.Join([]string{
		"ok  \texample.com/a\t0.112s",
		"ok  \texample.com/cached\t(cached)",
		"--- FAIL: TestTwo (0.00s)",
		"    two_test.go:9: wrong",
		"FAIL",
		"FAIL\texample.com/b\t0.204s",
		"=== RUN   TestSleeps",
	}, "\n")
	cmd := "printf '" + strings.ReplaceAll(out, "\t", `\t`) + "\\n'; exec sleep 5"

	res := runTestsWith(t, cmd, map[string]any{"timeout_sec": 1})

	requireInconclusive(t, res, objective.ReasonTimeout)
	assertPackages(t, finishedPackages(t, res), map[string]string{
		"example.com/a":      "pass",
		"example.com/cached": "pass",
		"example.com/b":      "fail",
	})
}

// (2) A timeout before any package finished still says so: an empty list, not
// a missing one, so the reader can tell "nothing finished" from "not read".
func TestRunTestsTimeoutWithNothingFinishedListsNoPackages(t *testing.T) {
	res := runTestsWith(t, "echo '=== RUN   TestSleeps'; exec sleep 5", map[string]any{"timeout_sec": 1})

	requireInconclusive(t, res, objective.ReasonTimeout)
	assertPackages(t, finishedPackages(t, res), map[string]string{})
}

// (3) Exit 127 is a command the shell could not find. That is a tool this
// machine does not have (AGENTS.md rule 10), not a failed test run, and the
// error names the tool so a person knows what to install.
func TestRunTestsMissingToolIsInconclusiveUnsupported(t *testing.T) {
	const tool = "karakuri-no-such-tool-44"
	res := runTestsWith(t, tool+" test ./...", nil)

	requireInconclusive(t, res, objective.ReasonUnsupported)
	if !strings.Contains(res.Error, tool) {
		t.Errorf("error should name the missing tool %q, got %q", tool, res.Error)
	}
}

// (3) A bare exit 127 is the same signal, whatever printed it.
func TestRunTestsExit127IsInconclusiveUnsupported(t *testing.T) {
	requireInconclusive(t, runTestsWith(t, "exit 127", nil), objective.ReasonUnsupported)
}

// (3) Exit 126 is a command found but not executable: unsupported too, and the
// error names it.
func TestRunTestsNotExecutableIsInconclusiveUnsupported(t *testing.T) {
	dir := t.TempDir()
	const script = "notexec-44.sh"
	if err := os.WriteFile(filepath.Join(dir, script), []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	res := runTestsWith(t, "./"+script, map[string]any{"workdir": dir})

	if code, _ := res.StateDelta["exit_code"].(int); code != 126 {
		t.Fatalf("fixture: exit_code = %v, want 126 (StateDelta %v)", res.StateDelta["exit_code"], res.StateDelta)
	}
	requireInconclusive(t, res, objective.ReasonUnsupported)
	if !strings.Contains(res.Error, script) {
		t.Errorf("error should name the command that cannot run, %q, got %q", script, res.Error)
	}
}

// (4) A run that finishes inside its limit is settled as before: exit 0 is a
// pass, exit 1 a failure, and neither carries an inconclusive state.
func TestRunTestsInsideItsLimitSettlesAsBefore(t *testing.T) {
	pass := runTestsWith(t, "printf 'ok  \\texample.com/a\\t0.1s\\n'", map[string]any{"timeout_sec": 5})
	if !pass.Success {
		t.Errorf("a passing run did not succeed: %q (StateDelta %v)", pass.Error, pass.StateDelta)
	}
	if pass.StateDelta["state"] == string(objective.CriterionInconclusive) {
		t.Errorf("a passing run is inconclusive: %v", pass.StateDelta)
	}
	if pass.StateDelta["status"] != "ok" {
		t.Errorf("status = %v, want ok", pass.StateDelta["status"])
	}

	fail := runTestsWith(t, "printf 'FAIL\\texample.com/b\\t0.1s\\n'; exit 1", map[string]any{"timeout_sec": 5})
	if fail.Success {
		t.Error("a failing run succeeded")
	}
	if fail.StateDelta["state"] == string(objective.CriterionInconclusive) {
		t.Errorf("a failing run is inconclusive; a red suite is not_met: %v", fail.StateDelta)
	}
	if fail.StateDelta["status"] != "exit_nonzero" {
		t.Errorf("status = %v, want exit_nonzero", fail.StateDelta["status"])
	}
	if !strings.Contains(fail.Error, "code 1") {
		t.Errorf("error should give the exit code, got %q", fail.Error)
	}
}

// (4) lint goes through the same environment: a failure is still a failure.
func TestLintFailureIsStillAFailure(t *testing.T) {
	res, err := newTestVerifyEnv().Act(context.Background(), environment.Action{
		CapabilityID: "software.verify.lint",
		Params:       map[string]any{"cmd": "echo 'main.go:1: unused'; exit 1"},
	})
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	if res.Success || res.StateDelta["state"] == string(objective.CriterionInconclusive) {
		t.Errorf("a lint failure must stay a failure, got Success=%v StateDelta %v", res.Success, res.StateDelta)
	}
}
