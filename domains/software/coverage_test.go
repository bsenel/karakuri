package software

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// docs/roadmap.md, Phase 41 step 10, coverage half: "coverage of a fixture
// package against at_start". software.verify.coverage is the first production
// verifier that declares a metric and emits a Measurement.

// registeredCapability reads a capability through the pack's own
// registration, not a copy of it.
func registeredCapability(t *testing.T, id capability.CapabilityID) capability.Capability {
	t.Helper()
	for _, c := range (&Pack{}).Capabilities() {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("the software pack registers no %s", id)
	return capability.Capability{}
}

func TestTheCoverageVerifierDeclaresExactlyTheCoverageMetric(t *testing.T) {
	c := registeredCapability(t, capCoverage)
	if !c.Verifiable {
		t.Error("software.verify.coverage is not Verifiable, so no criterion may name it")
	}
	want := []capability.Metric{{
		Name: "coverage", Unit: "percent", Direction: capability.DirectionHigherIsBetter,
		Params: []capability.MetricParam{{Name: "packages", Type: capability.ParamString}},
	}}
	if !reflect.DeepEqual(c.Metrics, want) {
		t.Errorf("metrics = %#v, want %#v", c.Metrics, want)
	}
}

// The coverage verifier is served by the verify environment, where run_tests
// and lint already run, and nowhere else.
func TestTheCoverageVerifierIsServedByTheVerifyEnvironment(t *testing.T) {
	for _, f := range (&Pack{}).EnvironmentFactories() {
		for _, id := range f.Serves {
			if id == capCoverage && f.EnvID != "software.env.verify" {
				t.Errorf("software.verify.coverage is served by %s, want software.env.verify", f.EnvID)
			}
			if id == capCoverage {
				return
			}
		}
	}
	t.Error("no environment serves software.verify.coverage")
}

// A cover profile is weighted by statements, not by blocks: two blocks of 3
// and 1 statements with only the 3 covered is 75, where counting blocks would
// say 50. An empty profile is blind, not quiet (AGENTS.md rule 10): it is an
// error, never 0.
func TestCoverPercentWeighsAProfileByStatements(t *testing.T) {
	for name, tc := range map[string]struct {
		profile string
		want    float64
		wantErr bool
	}{
		"three of four statements": {
			profile: "mode: set\n" +
				"example.com/m/a.go:3.20,6.2 3 1\n" +
				"example.com/m/a.go:6.2,8.3 1 0\n",
			want: 75,
		},
		"a count above one is covered once": {
			profile: "mode: count\n" +
				"example.com/m/a.go:3.20,6.2 3 7\n" +
				"example.com/m/a.go:6.2,8.3 1 0\n",
			want: 75,
		},
		// go test -coverpkg writes one line per block per test binary, so the
		// same block may appear twice; it counts once, covered if either is.
		"a block listed twice counts once": {
			profile: "mode: set\n" +
				"example.com/m/a.go:3.20,6.2 3 0\n" +
				"example.com/m/a.go:3.20,6.2 3 1\n" +
				"example.com/m/a.go:6.2,8.3 1 0\n",
			want: 75,
		},
		"nothing covered": {
			profile: "mode: set\nexample.com/m/a.go:3.20,6.2 3 0\n",
			want:    0,
		},
		"an empty profile":            {profile: "", wantErr: true},
		"a mode line and no blocks":   {profile: "mode: set\n", wantErr: true},
		"no mode line":                {profile: "example.com/m/a.go:3.20,6.2 3 1\n", wantErr: true},
		"a malformed block":           {profile: "mode: set\nexample.com/m/a.go:3.20,6.2 three 1\n", wantErr: true},
		"blocks with no statements":   {profile: "mode: set\nexample.com/m/a.go:3.20,6.2 0 1\n", wantErr: true},
		"a block line with no counts": {profile: "mode: set\nexample.com/m/a.go:3.20,6.2\n", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := coverPercent(tc.profile)
			if tc.wantErr {
				if err == nil {
					t.Errorf("coverPercent = %v with no error; a profile that measures nothing must say so", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("coverPercent: %v", err)
			}
			if got != tc.want {
				t.Errorf("coverPercent = %v, want %v", got, tc.want)
			}
		})
	}
}

func coverFixtureDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", "coverfixture"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func actCoverage(t *testing.T, env *verifyEnv, params map[string]any) environment.ActionResult {
	t.Helper()
	res, err := env.Act(t.Context(), environment.Action{CapabilityID: capCoverage, Params: params})
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	return res
}

// The fixture's Sign has four statements and its test reaches two, so the
// verifier measures exactly 50 percent, as go test -cover says of it. The
// figure is the deployment's own: it ran go test itself, in its own workdir.
func TestTheCoverageVerifierMeasuresTheFixturePackage(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test on the fixture module")
	}
	res := actCoverage(t, newTestVerifyEnv(), map[string]any{"packages": "./...", "workdir": coverFixtureDir(t)})
	if !res.Success {
		t.Fatalf("coverage of the fixture failed: %s", res.Error)
	}
	if len(res.Measurements) != 1 {
		t.Fatalf("measurements = %#v, want exactly one", res.Measurements)
	}
	m := res.Measurements[0]
	if m.Name != "coverage" || m.Unit != "percent" {
		t.Errorf("measurement is %q in %q, want coverage in percent", m.Name, m.Unit)
	}
	if m.Value != 50 {
		t.Errorf("coverage of the fixture = %v, want 50", m.Value)
	}
	if m.Trust != environment.TrustOperator || res.Trust.IsThirdParty() {
		t.Errorf("trust = %q (result %q), want first party: the deployment ran go test itself", m.Trust, res.Trust)
	}
	if !strings.Contains(m.Source, "go test") || !strings.Contains(m.Source, "./...") {
		t.Errorf("source %q does not name the command and the packages", m.Source)
	}
	if res.StateDelta["verifier"] != string(capCoverage) {
		t.Errorf("result names verifier %v, want %s", res.StateDelta["verifier"], capCoverage)
	}
}

// A coverage run that cannot measure fails with an error and carries no
// Measurement at all: a 0 would read as "nothing is covered" and settle a
// criterion not_met on a run that saw nothing.
func TestTheCoverageVerifierNeverReportsAZeroItDidNotMeasure(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test on the fixture module")
	}
	fixture := coverFixtureDir(t)
	for name, params := range map[string]map[string]any{
		"go test fails on a package that is not there": {"packages": "./nosuchpackage", "workdir": fixture},
		"a workdir with no module":                     {"packages": "./...", "workdir": t.TempDir()},
		"no packages":                                  {"workdir": fixture},
	} {
		t.Run(name, func(t *testing.T) {
			res := actCoverage(t, newTestVerifyEnv(), params)
			if res.Success {
				t.Error("a coverage run that measured nothing succeeded")
			}
			if res.Error == "" {
				t.Error("a failed coverage run carries no error")
			}
			if len(res.Measurements) != 0 {
				t.Errorf("a failed coverage run carries measurements %#v, want none", res.Measurements)
			}
		})
	}
}

// packages goes to exec as arguments of go test, so anything that is not a Go
// package pattern is refused before anything runs: a leading '-' would be a
// flag (-exec runs any program), and a pattern is never a shell word.
func TestTheCoverageVerifierRefusesPackagesThatAreNotPackagePatterns(t *testing.T) {
	fixture := coverFixtureDir(t)
	for _, packages := range []string{
		"-exec=/bin/sh",
		"./... -exec=/bin/sh",
		"--coverprofile=/etc/passwd",
		"./...; rm -rf /",
		"$(id)",
		"./a|b",
	} {
		t.Run(packages, func(t *testing.T) {
			res := actCoverage(t, newTestVerifyEnv(), map[string]any{"packages": packages, "workdir": fixture})
			if res.Success || len(res.Measurements) != 0 {
				t.Fatalf("packages %q ran: success %v, measurements %#v", packages, res.Success, res.Measurements)
			}
			if !strings.Contains(res.Error, "packages") {
				t.Errorf("error %q does not name params.packages as the reason", res.Error)
			}
		})
	}
}

func TestPackagePatternsAcceptsWhatGoTestTakes(t *testing.T) {
	for _, packages := range []string{
		"./...", "./internal/core/...", "github.com/bsenel/karakuri/domains/software",
		"./a ./b", "std",
	} {
		if _, err := packagePatterns(packages); err != nil {
			t.Errorf("packagePatterns(%q): %v", packages, err)
		}
	}
	got, err := packagePatterns("./a  ./b")
	if err != nil {
		t.Fatalf("packagePatterns: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"./a", "./b"}) {
		t.Errorf("packagePatterns(\"./a  ./b\") = %q, want [./a ./b]", got)
	}
	if _, err := packagePatterns(""); err == nil {
		t.Error("an empty packages param was accepted")
	}
}

// Step 9's mechanism is per-verifier and declared: verify reads the named
// verifier's MeasurePaths from the registry and the changed files from its
// result's StateDelta "changed_files" (internal/feature/loop/verify.go), and a
// pass that touches one is inconclusive: measure_changed. The coverage figure
// is defined by the test files of the measured packages, so the verifier
// declares them, the way run_tests does, and carries the changed files of the
// branch's pull request the way run_tests does.
func TestTheCoverageVerifierDeclaresTheTestFilesAsItsMeasurePaths(t *testing.T) {
	c := registeredCapability(t, capCoverage)
	if !capability.TouchesAnyGlob([]string{"internal/core/objective/objective_test.go"}, c.MeasurePaths) {
		t.Errorf("coverage measure paths %q miss a Go test file", c.MeasurePaths)
	}
	if capability.TouchesAnyGlob([]string{"internal/core/objective/objective.go", "docs/roadmap.md"}, c.MeasurePaths) {
		t.Errorf("coverage measure paths %q catch files that are not tests", c.MeasurePaths)
	}
}

func TestTheCoverageVerifierCarriesTheChangedFilesOfTheBranchPR(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test on the fixture module")
	}
	forge := &fakeForge{prs: []versioncontrol.PRSummary{streamPR(passStart, passStart)}, files: []string{"sign.go", "sign_test.go"}}
	env := newVerifyEnv("software.env.verify", newShellEnv("software.env.verify", "", 60*time.Second), forge)
	res := actCoverage(t, env, map[string]any{"packages": "./...", "workdir": coverFixtureDir(t), "branch": streamBranch})
	if !res.Success {
		t.Fatalf("coverage of the fixture failed: %s", res.Error)
	}
	files, _ := res.StateDelta["changed_files"].([]string)
	if !reflect.DeepEqual(files, []string{"sign.go", "sign_test.go"}) {
		t.Fatalf("changed_files = %#v, want [sign.go sign_test.go]", res.StateDelta["changed_files"])
	}
	// With the declared paths, that list is what makes verify settle a
	// criterion naming coverage inconclusive: measure_changed.
	if !capability.TouchesAnyGlob(files, registeredCapability(t, capCoverage).MeasurePaths) {
		t.Error("a pass that edited sign_test.go does not touch the coverage verifier's measure paths")
	}
}
