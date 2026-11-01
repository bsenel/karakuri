package software

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strings"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// capFlakyTests counts the tests of params.packages that both passed and
// failed over params.runs runs of go test -json in params.workdir
// (docs/roadmap.md, Phase 41 step 10, flaky half).
const capFlakyTests = "software.verify.flaky_tests"

// Fewer than two runs cannot tell flaky from broken; each run is go test over
// whole packages, so the count is bounded above too.
const (
	defaultFlakyRuns = 3
	minFlakyRuns     = 2
	maxFlakyRuns     = 10
)

// flakyTest names one test go test ran: its package and its full name, a
// subtest's included ("TestT/case").
type flakyTest struct {
	pkg  string
	test string
}

// testEvent is the part of one go test -json line (cmd/test2json's TestEvent)
// the flaky count and the build check read.
type testEvent struct {
	Action      string
	Package     string
	Test        string
	FailedBuild string
}

// flakyTestsOf takes the go test -json event streams of N runs and returns the
// tests that passed in at least one run and failed in at least one other, in
// package, then name, order. A test that failed in every run that ran it is
// broken, not flaky; a skip says nothing. Lines that are not JSON (build
// output) are skipped; a line that starts as JSON and cannot be read is an
// error, as is a run that holds no test event (AGENTS.md rule 10).
func flakyTestsOf(runs []string) ([]flakyTest, error) {
	if len(runs) == 0 {
		return nil, errors.New("no go test run to count flaky tests over")
	}
	type outcome struct{ passed, failed bool }
	seen := map[flakyTest]outcome{}
	for i, run := range runs {
		testEvents := 0
		for _, line := range strings.Split(run, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "{") {
				continue
			}
			var ev testEvent
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				return nil, fmt.Errorf("run %d: go test -json line cannot be read: %v: %q", i+1, err, truncate(line, 200))
			}
			if ev.Test == "" {
				continue
			}
			testEvents++
			key := flakyTest{pkg: ev.Package, test: ev.Test}
			o := seen[key]
			switch ev.Action {
			case "pass":
				o.passed = true
			case "fail":
				o.failed = true
			}
			seen[key] = o
		}
		if testEvents == 0 {
			return nil, fmt.Errorf("run %d of go test -json holds no test event: it ran no test, so it cannot say which are flaky", i+1)
		}
	}
	var flaky []flakyTest
	for t, o := range seen {
		if o.passed && o.failed {
			flaky = append(flaky, t)
		}
	}
	sort.Slice(flaky, func(i, j int) bool {
		if flaky[i].pkg != flaky[j].pkg {
			return flaky[i].pkg < flaky[j].pkg
		}
		return flaky[i].test < flaky[j].test
	})
	return flaky, nil
}

// buildFailureOf returns what in one go test -json stream says a package's
// tests did not build or could not be set up, or "" when nothing does. Such a
// run ran none of that package's tests, so it cannot say whether they are
// flaky.
func buildFailureOf(stream string) string {
	for _, line := range strings.Split(stream, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			if strings.HasSuffix(line, "[build failed]") || strings.HasSuffix(line, "[setup failed]") {
				return line
			}
			continue
		}
		var ev testEvent
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if ev.FailedBuild != "" {
			return "the tests of " + ev.FailedBuild + " did not build"
		}
		if ev.Action == "build-fail" {
			return "the tests of " + ev.Package + " did not build"
		}
	}
	return ""
}

// flakyRuns reads params.runs: absent is the default, otherwise a whole
// number from minFlakyRuns to maxFlakyRuns. A criterion's params arrive as
// JSON, so a whole float64 is accepted.
func flakyRuns(params map[string]any) (int, error) {
	v, ok := params["runs"]
	if !ok || v == nil {
		return defaultFlakyRuns, nil
	}
	var n int
	switch r := v.(type) {
	case int:
		n = r
	case int64:
		n = int(r)
	case float64:
		if r != math.Trunc(r) || math.IsInf(r, 0) {
			return 0, fmt.Errorf("params.runs is %v, not a whole number of runs", r)
		}
		n = int(r)
	default:
		return 0, fmt.Errorf("params.runs is %v, not a whole number of runs", v)
	}
	if n < minFlakyRuns || n > maxFlakyRuns {
		return 0, fmt.Errorf("params.runs is %d: it must be from %d to %d, since one run cannot tell flaky from broken", n, minFlakyRuns, maxFlakyRuns)
	}
	return n, nil
}

// flakyTests answers software.verify.flaky_tests: it runs go test -json over
// the validated patterns params.runs times, without a shell, and returns the
// count of tests that both passed and failed as one first-party Measurement,
// with their names. A failing test makes go test exit non-zero, which is what
// is being counted; a run that cannot measure (no test event, a build that
// failed, go test not starting) is a failed result and no Measurement.
func (e *verifyEnv) flakyTests(ctx context.Context, a environment.Action) environment.ActionResult {
	patterns, err := packagePatterns(asString(a.Params, "packages"))
	if err != nil {
		return failureResult(e.id, a.CapabilityID, err.Error(), nil)
	}
	runs, err := flakyRuns(a.Params)
	if err != nil {
		return failureResult(e.id, a.CapabilityID, err.Error(), nil)
	}
	workdir, err := e.shell.resolveWorkdir(asString(a.Params, "workdir"))
	if err != nil {
		return failureResult(e.id, a.CapabilityID, err.Error(), nil)
	}

	source := fmt.Sprintf("go test -count=1 -json %s, %d runs", strings.Join(patterns, " "), runs)
	extra := map[string]any{"cmd": source, "workdir": workdir, "verifier": string(a.CapabilityID)}
	args := append([]string{"test", "-count=1", "-json"}, patterns...)
	streams := make([]string, 0, runs)
	for i := 1; i <= runs; i++ {
		stream, stderr, err := e.goTestJSON(ctx, workdir, args)
		var exitErr *exec.ExitError
		if err != nil && !errors.As(err, &exitErr) {
			extra["stderr"] = truncate(stderr, 4000)
			return failureResult(e.id, a.CapabilityID, fmt.Sprintf("run %d of %s did not complete: %v: %s", i, source, err, truncate(stderr, 1000)), extra)
		}
		if failure := buildFailureOf(stream); failure != "" {
			extra["stderr"] = truncate(stderr, 4000)
			return failureResult(e.id, a.CapabilityID, fmt.Sprintf("run %d of %s: %s", i, source, failure), extra)
		}
		streams = append(streams, stream)
	}
	flaky, err := flakyTestsOf(streams)
	if err != nil {
		return failureResult(e.id, a.CapabilityID, err.Error(), extra)
	}
	names := make([]string, 0, len(flaky))
	for _, t := range flaky {
		names = append(names, t.pkg+" "+t.test)
	}

	res := environment.ActionResult{
		Success: true,
		Trust:   environment.TrustOperator,
		StateDelta: map[string]any{
			"action":      string(a.CapabilityID),
			"status":      "ok",
			"env_id":      string(e.id),
			"verifier":    string(a.CapabilityID),
			"cmd":         source,
			"workdir":     workdir,
			"runs":        runs,
			"flaky_tests": names,
		},
		Measurements: []environment.Measurement{{
			Name: "flaky_tests", Value: float64(len(flaky)), Unit: "count",
			Source: source, Trust: environment.TrustOperator,
		}},
	}
	if files, trust, ok := e.changedFiles(ctx, a.Params); ok {
		res.StateDelta["changed_files"] = files
		if trust.IsThirdParty() {
			res.Trust = trust
		}
	}
	return res
}

// goTestJSON runs go test once with args in workdir and returns its stdout
// (the -json event stream) and stderr.
func (e *verifyEnv) goTestJSON(ctx context.Context, workdir string, args []string) (stdout, stderr string, err error) {
	cmdCtx, cancel := context.WithTimeout(ctx, e.shell.timeout)
	defer cancel()
	c := exec.CommandContext(cmdCtx, "go", args...) // #nosec G204 -- every package pattern passed packagePatterns, the other arguments are constants, and no shell is involved
	c.Dir = workdir
	var out, errOut bytes.Buffer
	c.Stdout = &out
	c.Stderr = &errOut
	err = c.Run()
	if cmdCtx.Err() != nil {
		err = fmt.Errorf("%w (%v)", cmdCtx.Err(), err)
	}
	return out.String(), errOut.String(), err
}
