package software

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// docs/roadmap.md, Phase 41 step 10, flaky half: "a fixture test that fails
// one run in three is counted as flaky over three runs".
// software.verify.flaky_tests runs go test -json over params.packages
// params.runs times and counts the tests that both passed and failed.

func TestTheFlakyTestVerifierDeclaresExactlyTheFlakyTestsMetric(t *testing.T) {
	c := registeredCapability(t, capFlakyTests)
	if !c.Verifiable {
		t.Error("software.verify.flaky_tests is not Verifiable, so no criterion may name it")
	}
	want := []capability.Metric{{
		Name: "flaky_tests", Unit: "count", Direction: capability.DirectionLowerIsBetter,
		Params: []capability.MetricParam{
			{Name: "packages", Type: capability.ParamString},
			{Name: "runs", Type: capability.ParamInteger},
		},
	}}
	if !reflect.DeepEqual(c.Metrics, want) {
		t.Errorf("metrics = %#v, want %#v", c.Metrics, want)
	}
}

// The flaky-test verifier runs go test, so it is served where run_tests,
// lint and coverage run, and nowhere else.
func TestTheFlakyTestVerifierIsServedByTheVerifyEnvironment(t *testing.T) {
	for _, f := range (&Pack{}).EnvironmentFactories() {
		for _, id := range f.Serves {
			if id == capFlakyTests && f.EnvID != "software.env.verify" {
				t.Errorf("software.verify.flaky_tests is served by %s, want software.env.verify", f.EnvID)
			}
			if id == capFlakyTests {
				return
			}
		}
	}
	t.Error("no environment serves software.verify.flaky_tests")
}

const (
	flakyPkgA = "example.com/m/a"
	flakyPkgB = "example.com/m/b"
)

// testJSONEvent is one line of go test -json (cmd/test2json's TestEvent). An
// empty test is a package-level event.
func testJSONEvent(action, pkg, test string) string {
	ev := map[string]any{"Time": "2026-11-01T12:00:00Z", "Action": action, "Package": pkg}
	if test != "" {
		ev["Test"] = test
	}
	b, err := json.Marshal(ev)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// testRan is what go test -json writes for one test that ended in action
// (pass, fail or skip): its run event, an output line and its outcome.
func testRan(action, pkg, test string) []string {
	out, err := json.Marshal(map[string]any{
		"Time": "2026-11-01T12:00:00Z", "Action": "output", "Package": pkg, "Test": test,
		"Output": "=== RUN   " + test + "\n",
	})
	if err != nil {
		panic(err)
	}
	return []string{testJSONEvent("run", pkg, test), string(out), testJSONEvent(action, pkg, test)}
}

// goTestRun is one run's stream: the lines given, newline-terminated, as go
// test -json writes them.
func goTestRun(lines ...[]string) string {
	var b strings.Builder
	for _, group := range lines {
		for _, l := range group {
			b.WriteString(l)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func raw(lines ...string) []string { return lines }

// A test is flaky when it passed in one run and failed in another; a test
// that failed in every run that ran it is broken, not flaky, and is not
// counted. The flaky tests come back in package, then name, order.
func TestFlakyTestsOfCountsATestThatBothPassedAndFailed(t *testing.T) {
	pass := func(pkg, test string) []string { return testRan("pass", pkg, test) }
	fail := func(pkg, test string) []string { return testRan("fail", pkg, test) }
	skip := func(pkg, test string) []string { return testRan("skip", pkg, test) }
	for name, tc := range map[string]struct {
		runs []string
		want []flakyTest
	}{
		"pass, fail, pass is flaky": {
			runs: []string{
				goTestRun(pass(flakyPkgA, "TestA")),
				goTestRun(fail(flakyPkgA, "TestA")),
				goTestRun(pass(flakyPkgA, "TestA")),
			},
			want: []flakyTest{{pkg: flakyPkgA, test: "TestA"}},
		},
		"fail, pass, pass is flaky: the order of the runs does not matter": {
			runs: []string{
				goTestRun(fail(flakyPkgA, "TestA")),
				goTestRun(pass(flakyPkgA, "TestA")),
				goTestRun(pass(flakyPkgA, "TestA")),
			},
			want: []flakyTest{{pkg: flakyPkgA, test: "TestA"}},
		},
		"fail, fail, fail is broken, not flaky": {
			runs: []string{
				goTestRun(fail(flakyPkgA, "TestA")),
				goTestRun(fail(flakyPkgA, "TestA")),
				goTestRun(fail(flakyPkgA, "TestA")),
			},
		},
		"pass, pass, pass is not flaky": {
			runs: []string{
				goTestRun(pass(flakyPkgA, "TestA")),
				goTestRun(pass(flakyPkgA, "TestA")),
				goTestRun(pass(flakyPkgA, "TestA")),
			},
		},
		// A skip is neither a pass nor a fail: it says nothing about the test.
		"a skip is neither a pass nor a fail": {
			runs: []string{
				goTestRun(pass(flakyPkgA, "TestA"), skip(flakyPkgA, "TestB")),
				goTestRun(skip(flakyPkgA, "TestA"), fail(flakyPkgA, "TestB")),
				goTestRun(pass(flakyPkgA, "TestA"), fail(flakyPkgA, "TestB")),
			},
		},
		// TestA passed in run 1, was not run in run 2 and failed in run 3:
		// flaky. TestC ran only in run 2 and failed there: broken where it
		// ran. TestD ran only in run 1 and passed.
		"a test seen in only some runs is judged on the runs that ran it": {
			runs: []string{
				goTestRun(pass(flakyPkgA, "TestA"), pass(flakyPkgA, "TestB"), pass(flakyPkgA, "TestD")),
				goTestRun(pass(flakyPkgA, "TestB"), fail(flakyPkgA, "TestC")),
				goTestRun(fail(flakyPkgA, "TestA"), pass(flakyPkgA, "TestB")),
			},
			want: []flakyTest{{pkg: flakyPkgA, test: "TestA"}},
		},
		// A subtest is its own test, under its full name. go test reports
		// the parent failed in the run its subtest failed, and the function
		// judges what go test reported, name by name, so the parent is
		// flaky too; the sibling that always passed is not.
		"subtests count by their full name": {
			runs: []string{
				goTestRun(pass(flakyPkgA, "TestT/a"), pass(flakyPkgA, "TestT/b"), pass(flakyPkgA, "TestT")),
				goTestRun(fail(flakyPkgA, "TestT/a"), pass(flakyPkgA, "TestT/b"), fail(flakyPkgA, "TestT")),
				goTestRun(pass(flakyPkgA, "TestT/a"), pass(flakyPkgA, "TestT/b"), pass(flakyPkgA, "TestT")),
			},
			want: []flakyTest{{pkg: flakyPkgA, test: "TestT"}, {pkg: flakyPkgA, test: "TestT/a"}},
		},
		// A test is its package plus its name: TestA of b failing in every
		// run does not make TestA of a, which passed twice, broken, and
		// TestX passing in a and failing in b is two tests, neither flaky.
		"the same name in two packages is two tests": {
			runs: []string{
				goTestRun(pass(flakyPkgA, "TestA"), fail(flakyPkgB, "TestA"), pass(flakyPkgA, "TestX")),
				goTestRun(fail(flakyPkgA, "TestA"), fail(flakyPkgB, "TestA"), fail(flakyPkgB, "TestX")),
				goTestRun(pass(flakyPkgA, "TestA"), fail(flakyPkgB, "TestA")),
			},
			want: []flakyTest{{pkg: flakyPkgA, test: "TestA"}},
		},
		"the flaky tests come back in package, then name, order": {
			runs: []string{
				goTestRun(pass(flakyPkgB, "TestA"), pass(flakyPkgA, "TestZ"), pass(flakyPkgA, "TestB")),
				goTestRun(fail(flakyPkgB, "TestA"), fail(flakyPkgA, "TestZ"), fail(flakyPkgA, "TestB")),
			},
			want: []flakyTest{
				{pkg: flakyPkgA, test: "TestB"}, {pkg: flakyPkgA, test: "TestZ"}, {pkg: flakyPkgB, test: "TestA"},
			},
		},
		// go test -json interleaves build output: text lines that are not
		// JSON at all, and JSON events that are not test events (a
		// package's own pass or fail, build-output, start). None is a test.
		"build output and package events between the test events are skipped": {
			runs: []string{
				goTestRun(
					raw("# example.com/m/b", "b/b_test.go:3:1: syntax error: non-declaration statement outside function body"),
					raw(`{"ImportPath":"example.com/m/b [example.com/m/b.test]","Action":"build-output","Output":"# example.com/m/b\n"}`),
					raw(testJSONEvent("start", flakyPkgA, "")),
					pass(flakyPkgA, "TestA"),
					raw(testJSONEvent("pass", flakyPkgA, ""), "FAIL\texample.com/m/b [build failed]", testJSONEvent("fail", flakyPkgB, "")),
				),
				goTestRun(raw(testJSONEvent("start", flakyPkgA, "")), fail(flakyPkgA, "TestA"), raw(testJSONEvent("fail", flakyPkgA, ""))),
				goTestRun(raw("ok  \texample.com/m/a\t0.012s"), pass(flakyPkgA, "TestA")),
			},
			want: []flakyTest{{pkg: flakyPkgA, test: "TestA"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := flakyTestsOf(tc.runs)
			if err != nil {
				t.Fatalf("flakyTestsOf: %v", err)
			}
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("flakyTestsOf = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// Runs that hold no test event saw nothing, and "no flaky test" read from
// them would settle a criterion met on a run that measured nothing: blind,
// not quiet (AGENTS.md rule 10). A line that begins as a JSON event and
// cannot be read is not skipped either: it may be the failure.
func TestFlakyTestsOfRefusesRunsThatSawNoTest(t *testing.T) {
	packageOnly := goTestRun(raw(
		testJSONEvent("start", flakyPkgA, ""),
		`{"Time":"2026-11-01T12:00:00Z","Action":"output","Package":"example.com/m/a","Output":"testing: warning: no tests to run\n"}`,
		testJSONEvent("pass", flakyPkgA, ""),
	))
	buildOnly := goTestRun(raw("# example.com/m/a", "a/a_test.go:3:1: syntax error", "FAIL\texample.com/m/a [build failed]"))
	for name, runs := range map[string][]string{
		"no runs":                           nil,
		"empty streams":                     {"", "", ""},
		"package events and no test":        {packageOnly, packageOnly, packageOnly},
		"build output and nothing else":     {buildOnly, buildOnly, buildOnly},
		"a test event that cannot be read":  {goTestRun(testRan("pass", flakyPkgA, "TestA")), `{"Action":"fail","Package":"example.com/m/a","Test":"TestA"` + "\n", goTestRun(testRan("pass", flakyPkgA, "TestA"))},
		"build output in one run, no tests": {buildOnly, "", packageOnly},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := flakyTestsOf(runs)
			if err == nil {
				t.Errorf("flakyTestsOf = %+v with no error; runs that saw no test must say so", got)
			}
		})
	}
}

const flakyCounterEnv = "KARAKURI_FLAKYFIXTURE_COUNTER"

func flakyFixtureDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", "flakyfixture"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// withFlakyCounter points the fixture's flaky test at a fresh counter file and
// returns its path. go test inherits the environment, so every run the
// verifier makes counts in it.
func withFlakyCounter(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "counter")
	t.Setenv(flakyCounterEnv, path)
	return path
}

// flakyRunsMade is how many times the fixture's flaky test ran: how many runs
// of go test the verifier made. 0 when nothing ran.
func flakyRunsMade(t *testing.T, counter string) string {
	t.Helper()
	b, err := os.ReadFile(counter) // #nosec G304 -- the counter file is in this test's own t.TempDir()
	if errors.Is(err, fs.ErrNotExist) {
		return "0"
	}
	if err != nil {
		t.Fatalf("read counter: %v", err)
	}
	return strings.TrimSpace(string(b))
}

func actFlaky(t *testing.T, env *verifyEnv, params map[string]any) environment.ActionResult {
	t.Helper()
	res, err := env.Act(t.Context(), environment.Action{CapabilityID: capFlakyTests, Params: params})
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	return res
}

// The acceptance itself. Run 2 of the three fails, so go test exits non-zero
// in it: that is a test failing, which is what is being counted, not a run
// that could not measure. The figure is the deployment's own: it ran go test
// itself, in its own workdir. runs is a JSON number, as a criterion's params
// arrive.
func TestTheFlakyTestVerifierCountsTheFixtureTestThatFailsOneRunInThree(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test on the fixture module")
	}
	counter := withFlakyCounter(t)
	res := actFlaky(t, newTestVerifyEnv(), map[string]any{
		"packages": "./...", "runs": float64(3), "workdir": flakyFixtureDir(t),
	})
	if !res.Success {
		t.Fatalf("flaky tests of the fixture failed: %s", res.Error)
	}
	if got := flakyRunsMade(t, counter); got != "3" {
		t.Errorf("go test ran %s times, want 3", got)
	}
	if len(res.Measurements) != 1 {
		t.Fatalf("measurements = %#v, want exactly one", res.Measurements)
	}
	m := res.Measurements[0]
	if m.Name != "flaky_tests" || m.Unit != "count" {
		t.Errorf("measurement is %q in %q, want flaky_tests in count", m.Name, m.Unit)
	}
	if m.Value != 1 {
		t.Errorf("flaky tests of the fixture = %v, want 1", m.Value)
	}
	if m.Trust != environment.TrustOperator || res.Trust.IsThirdParty() {
		t.Errorf("trust = %q (result %q), want first party: the deployment ran go test itself", m.Trust, res.Trust)
	}
	for _, part := range []string{"go test", "-json", "./...", "3"} {
		if !strings.Contains(m.Source, part) {
			t.Errorf("source %q does not name %q: it names the command, the packages and the runs", m.Source, part)
		}
	}
	if res.StateDelta["verifier"] != string(capFlakyTests) {
		t.Errorf("result names verifier %v, want %s", res.StateDelta["verifier"], capFlakyTests)
	}
	named, ok := res.StateDelta["flaky_tests"].([]string)
	if !ok {
		t.Fatalf("result output flaky_tests = %#v, want the flaky tests by name", res.StateDelta["flaky_tests"])
	}
	if len(named) != 1 || !strings.Contains(named[0], "TestFailsOneRunInThree") {
		t.Errorf("flaky tests named %q, want TestFailsOneRunInThree alone", named)
	}
	for _, n := range named {
		if strings.Contains(n, "TestAlwaysPasses") {
			t.Errorf("the always-passing test is named flaky: %q", named)
		}
	}
}

// Three runs is what the acceptance counts over, so it is the default.
func TestTheFlakyTestVerifierRunsThreeTimesByDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test on the fixture module")
	}
	counter := withFlakyCounter(t)
	res := actFlaky(t, newTestVerifyEnv(), map[string]any{"packages": "./...", "workdir": flakyFixtureDir(t)})
	if !res.Success {
		t.Fatalf("flaky tests of the fixture failed: %s", res.Error)
	}
	if got := flakyRunsMade(t, counter); got != "3" {
		t.Errorf("with no params.runs go test ran %s times, want 3", got)
	}
	if len(res.Measurements) != 1 || res.Measurements[0].Value != 1 {
		t.Errorf("measurements = %#v, want one of value 1", res.Measurements)
	}
}

// One run cannot tell flaky from broken, and runs are go test over whole
// packages, so the count is bounded: fewer than 2 or more than 10 is refused
// with an error naming params.runs, before anything runs. So is a runs that
// is not a whole number.
func TestTheFlakyTestVerifierRefusesRunsOutsideTwoToTen(t *testing.T) {
	for name, runs := range map[string]any{
		"0": 0, "1": 1, "1 as a JSON number": float64(1), "11": 11, "-3": -3,
		"2.5": 2.5, "a string": "three",
	} {
		t.Run(name, func(t *testing.T) {
			counter := withFlakyCounter(t)
			res := actFlaky(t, newTestVerifyEnv(), map[string]any{
				"packages": "./...", "runs": runs, "workdir": flakyFixtureDir(t),
			})
			if res.Success || len(res.Measurements) != 0 {
				t.Fatalf("runs %v was accepted: success %v, measurements %#v", runs, res.Success, res.Measurements)
			}
			if !strings.Contains(res.Error, "runs") {
				t.Errorf("error %q does not name params.runs as the reason", res.Error)
			}
			if got := flakyRunsMade(t, counter); got != "0" {
				t.Errorf("go test ran %s times for a refused runs %v, want none", got, runs)
			}
		})
	}
}

// packages goes to exec as arguments of go test, as coverage's does, so it is
// held to the same package-pattern rule before anything runs.
func TestTheFlakyTestVerifierRefusesPackagesThatAreNotPackagePatterns(t *testing.T) {
	for _, packages := range []string{"-exec=/bin/sh", "./...; rm -rf /", "$(id)"} {
		t.Run(packages, func(t *testing.T) {
			counter := withFlakyCounter(t)
			res := actFlaky(t, newTestVerifyEnv(), map[string]any{
				"packages": packages, "runs": 3, "workdir": flakyFixtureDir(t),
			})
			if res.Success || len(res.Measurements) != 0 {
				t.Fatalf("packages %q ran: success %v, measurements %#v", packages, res.Success, res.Measurements)
			}
			if !strings.Contains(res.Error, "packages") {
				t.Errorf("error %q does not name params.packages as the reason", res.Error)
			}
			if got := flakyRunsMade(t, counter); got != "0" {
				t.Errorf("go test ran %s times for refused packages %q, want none", got, packages)
			}
		})
	}
}

// A run that cannot measure fails with an error and carries no Measurement:
// a 0 would read as "nothing is flaky" and settle a criterion met on runs
// that saw nothing.
func TestTheFlakyTestVerifierNeverReportsAZeroItDidNotMeasure(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test on the fixture module")
	}
	fixture := flakyFixtureDir(t)
	for name, params := range map[string]map[string]any{
		"go test fails on a package that is not there": {"packages": "./nosuchpackage", "workdir": fixture},
		"a workdir with no module":                     {"packages": "./...", "workdir": t.TempDir()},
		"no packages":                                  {"workdir": fixture},
	} {
		t.Run(name, func(t *testing.T) {
			withFlakyCounter(t)
			res := actFlaky(t, newTestVerifyEnv(), params)
			if res.Success {
				t.Error("a flaky-test run that measured nothing succeeded")
			}
			if res.Error == "" {
				t.Error("a failed flaky-test run carries no error")
			}
			if len(res.Measurements) != 0 {
				t.Errorf("a failed flaky-test run carries measurements %#v, want none", res.Measurements)
			}
		})
	}
}

// writeFixtureFile writes a file of a module built in a test's temp dir.
func writeFixtureFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A package whose tests do not build ran no test, so the run cannot say
// whether they are flaky. The other package's passing tests are real test
// events, and counting over them alone would report 0: the run is a failed
// result naming the build failure, never a Measurement.
func TestTheFlakyTestVerifierFailsARunWhoseTestsDoNotBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test on a module it writes")
	}
	root := t.TempDir()
	writeFixtureFile(t, root, "go.mod", "module example.com/brokenfixture\n\ngo 1.22\n")
	writeFixtureFile(t, root, "ok/ok_test.go", "package ok\n\nimport \"testing\"\n\nfunc TestPasses(t *testing.T) {}\n")
	writeFixtureFile(t, root, "broken/broken_test.go", "package broken\n\nimport \"testing\"\n\nfunc TestNeverBuilds(t *testing.T) { undefinedHelper(t) }\n")

	res := actFlaky(t, newTestVerifyEnv(), map[string]any{"packages": "./...", "runs": 2, "workdir": root})
	if res.Success {
		t.Error("a flaky-test run whose tests did not build succeeded")
	}
	if len(res.Measurements) != 0 {
		t.Errorf("a run whose tests did not build carries measurements %#v, want none", res.Measurements)
	}
	if !strings.Contains(res.Error, "build") {
		t.Errorf("error %q does not say the tests did not build", res.Error)
	}
}

// Step 9's mechanism, as for run_tests and coverage: which tests are flaky is
// defined by the test files of the measured packages, so the verifier declares
// them as its measure paths, and a pass that edits one is inconclusive:
// measure_changed.
func TestTheFlakyTestVerifierDeclaresTheTestFilesAsItsMeasurePaths(t *testing.T) {
	c := registeredCapability(t, capFlakyTests)
	if !capability.TouchesAnyGlob([]string{"internal/core/objective/objective_test.go"}, c.MeasurePaths) {
		t.Errorf("flaky_tests measure paths %q miss a Go test file", c.MeasurePaths)
	}
	if capability.TouchesAnyGlob([]string{"internal/core/objective/objective.go", "docs/roadmap.md"}, c.MeasurePaths) {
		t.Errorf("flaky_tests measure paths %q catch files that are not tests", c.MeasurePaths)
	}
}

func TestTheFlakyTestVerifierCarriesTheChangedFilesOfTheBranchPR(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test on the fixture module")
	}
	withFlakyCounter(t)
	forge := &fakeForge{prs: []versioncontrol.PRSummary{streamPR(passStart, passStart)}, files: []string{"flaky_test.go"}}
	env := newVerifyEnv("software.env.verify", newShellEnv("software.env.verify", "", 60*time.Second), forge)
	res := actFlaky(t, env, map[string]any{
		"packages": "./...", "runs": 2, "workdir": flakyFixtureDir(t), "branch": streamBranch,
	})
	if !res.Success {
		t.Fatalf("flaky tests of the fixture failed: %s", res.Error)
	}
	files, _ := res.StateDelta["changed_files"].([]string)
	if !reflect.DeepEqual(files, []string{"flaky_test.go"}) {
		t.Fatalf("changed_files = %#v, want [flaky_test.go]", res.StateDelta["changed_files"])
	}
	if !capability.TouchesAnyGlob(files, registeredCapability(t, capFlakyTests).MeasurePaths) {
		t.Error("a pass that edited flaky_test.go does not touch the flaky-test verifier's measure paths")
	}
}
