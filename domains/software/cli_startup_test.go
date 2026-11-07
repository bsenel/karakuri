package software

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// docs/roadmap.md, Phase 41 step 11, startup half: software.verify.cli_startup_ms
// runs params.binary once, without a shell, and reports the wall time from its
// start to its exit. One action is one sample; a criterion over several takes
// the median of several runs.

func cliStartupCapability(t *testing.T) capability.Capability {
	t.Helper()
	for _, c := range (&Pack{}).Capabilities() {
		if c.ID == capCLIStartup {
			return c
		}
	}
	t.Fatalf("the software pack registers no %s", capCLIStartup)
	return capability.Capability{}
}

// capability.MetricParam has no list type, so args is declared a string; the
// verifier itself takes a list of strings (the shape a plan's JSON gives).
func TestTheCLIStartupVerifierDeclaresExactlyTheStartupMetric(t *testing.T) {
	c := cliStartupCapability(t)
	if !c.Verifiable {
		t.Error("software.verify.cli_startup_ms is not Verifiable, so no criterion may name it")
	}
	want := []capability.Metric{{
		Name: "cli_startup_ms", Unit: "ms", Direction: capability.DirectionLowerIsBetter,
		Params: []capability.MetricParam{
			{Name: "binary", Type: capability.ParamString},
			{Name: "args", Type: capability.ParamString},
		},
	}}
	if !reflect.DeepEqual(c.Metrics, want) {
		t.Errorf("metrics = %#v, want %#v", c.Metrics, want)
	}
}

func TestTheCLIStartupVerifierIsServedByTheVerifyEnvironment(t *testing.T) {
	for _, f := range (&Pack{}).EnvironmentFactories() {
		for _, id := range f.Serves {
			if id != capCLIStartup {
				continue
			}
			if f.EnvID != "software.env.verify" {
				t.Errorf("software.verify.cli_startup_ms is served by %s, want software.env.verify", f.EnvID)
			}
			return
		}
	}
	t.Error("no environment serves software.verify.cli_startup_ms")
}

// buildStartupFixture builds testdata/startupfixture into a fresh workdir as
// bin/startupfixture and returns the workdir. The fixture has its own go.mod,
// so the main module's ./... does not include it.
func buildStartupFixture(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("testdata", "startupfixture"))
	if err != nil {
		t.Fatal(err)
	}
	workdir := t.TempDir()
	out := filepath.Join(workdir, "bin", "startupfixture")
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", out, ".") // #nosec G204 -- fixed arguments and a path this test chose; no shell
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build the startup fixture: %v\n%s", err, b)
	}
	return workdir
}

func actCLIStartup(t *testing.T, params map[string]any) environment.ActionResult {
	t.Helper()
	res, err := newTestVerifyEnv().Act(t.Context(), environment.Action{CapabilityID: capCLIStartup, Params: params})
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	return res
}

func wantOneStartupMeasurement(t *testing.T, res environment.ActionResult) environment.Measurement {
	t.Helper()
	if !res.Success {
		t.Fatalf("measuring the startup failed: %s", res.Error)
	}
	if len(res.Measurements) != 1 {
		t.Fatalf("measurements = %#v, want exactly one: one run is one sample", res.Measurements)
	}
	m := res.Measurements[0]
	if m.Name != "cli_startup_ms" || m.Unit != "ms" {
		t.Errorf("measurement is %q in %q, want cli_startup_ms in ms", m.Name, m.Unit)
	}
	return m
}

func wantFailedStartup(t *testing.T, res environment.ActionResult, why string) {
	t.Helper()
	if res.Success {
		t.Errorf("%s succeeded with %#v; it must fail", why, res.Measurements)
	}
	if res.Error == "" {
		t.Errorf("%s failed with no reason", why)
	}
	if len(res.Measurements) != 0 {
		t.Errorf("%s carries measurements %#v, want none: a run that did not exit cleanly has no startup time", why, res.Measurements)
	}
}

// With no params.args the fixture runs with --help, which exits 0, and the
// verifier reports one first-party time.
func TestTheCLIStartupVerifierTimesTheBinaryWithHelpByDefault(t *testing.T) {
	workdir := buildStartupFixture(t)
	res := actCLIStartup(t, map[string]any{"binary": "bin/startupfixture", "workdir": workdir})
	m := wantOneStartupMeasurement(t, res)
	if m.Value <= 0 || m.Value >= 10_000 {
		t.Errorf("cli_startup_ms of a binary that exits at once = %v, want above 0 and below 10 s", m.Value)
	}
	if m.Trust != environment.TrustOperator || res.Trust.IsThirdParty() {
		t.Errorf("trust = %q (result %q), want first party: the deployment timed the run itself", m.Trust, res.Trust)
	}
	if !strings.Contains(m.Source, "startupfixture") {
		t.Errorf("source %q does not name the binary it timed", m.Source)
	}
	if res.StateDelta["verifier"] != capCLIStartup {
		t.Errorf("result names verifier %v, want %s", res.StateDelta["verifier"], capCLIStartup)
	}
}

// The time is the run's wall time: a fixture told to sleep 50 ms measures at
// least 50 ms, and less than a generous bound above it.
func TestTheCLIStartupVerifierMeasuresTheWallTimeOfTheRun(t *testing.T) {
	workdir := buildStartupFixture(t)
	start := time.Now()
	res := actCLIStartup(t, map[string]any{
		"binary": "bin/startupfixture", "workdir": workdir,
		"args": []any{"-sleep", "50ms"},
	})
	elapsed := time.Since(start)
	m := wantOneStartupMeasurement(t, res)
	if m.Value < 50 {
		t.Errorf("cli_startup_ms of a run that sleeps 50 ms = %v, want at least 50", m.Value)
	}
	if limit := float64(elapsed.Milliseconds()) + 1; m.Value > limit || m.Value >= 5_000 {
		t.Errorf("cli_startup_ms = %v, more than the %v ms the whole action took, or above 5 s", m.Value, limit)
	}
}

// params.args is applied: args that make the fixture exit non-zero give a
// failed result, so a verifier that ignored them and ran --help would pass.
func TestTheCLIStartupVerifierAppliesParamsArgs(t *testing.T) {
	workdir := buildStartupFixture(t)
	res := actCLIStartup(t, map[string]any{
		"binary": "bin/startupfixture", "workdir": workdir,
		"args": []any{"-exit", "3"},
	})
	wantFailedStartup(t, res, "a run that exits 3")
	if !strings.Contains(res.Error, "3") {
		t.Errorf("error %q does not give the exit status", res.Error)
	}
}

// A run that does not exit within the verifier's bound is a failed result,
// not a time. The fixture sleeps far longer than any bound the verifier keeps
// (30 s or so); the test allows a minute for the verifier to give up.
func TestTheCLIStartupVerifierGivesUpOnARunThatDoesNotExit(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the verifier's timeout")
	}
	workdir := buildStartupFixture(t)
	start := time.Now()
	res := actCLIStartup(t, map[string]any{
		"binary": "bin/startupfixture", "workdir": workdir,
		"args": []any{"-sleep", "10m"},
	})
	if took := time.Since(start); took > time.Minute {
		t.Errorf("the verifier waited %v for a run that does not exit, want it bounded (about 30 s)", took)
	}
	wantFailedStartup(t, res, "a run that does not exit")
}

// Blind, not quiet (AGENTS.md rule 10): a binary the verifier cannot run, or
// is not allowed to, is a failed result with a reason naming params.binary and
// no time. A 0 would read as the fastest start there is.
func TestTheCLIStartupVerifierNeverReportsATimeItDidNotMeasure(t *testing.T) {
	workdir := buildStartupFixture(t)
	root := t.TempDir()
	outsideBin := filepath.Join(root, "outside", "startupfixture")
	if err := os.MkdirAll(filepath.Dir(outsideBin), 0o750); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(workdir, "bin", "startupfixture")) // #nosec G304 -- the binary this test just built
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsideBin, data, 0o700); err != nil { // #nosec G306 -- an executable fixture must be executable
		t.Fatal(err)
	}
	if err := os.Symlink(outsideBin, filepath.Join(workdir, "bin", "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outsideBin), filepath.Join(workdir, "out")); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(workdir, outsideBin)
	if err != nil {
		t.Fatal(err)
	}
	for name, params := range map[string]map[string]any{
		"no params.binary":                       {"workdir": workdir},
		"a binary that is not there":             {"binary": "bin/nosuch", "workdir": workdir},
		"an absolute binary":                     {"binary": filepath.Join(workdir, "bin", "startupfixture"), "workdir": workdir},
		"a binary that escapes with ..":          {"binary": rel, "workdir": workdir},
		"a binary that escapes after a subdir":   {"binary": filepath.Join("bin", "..", rel), "workdir": workdir},
		"a symlink to a binary outside":          {"binary": "bin/linked", "workdir": workdir},
		"a path through a symlink to outside":    {"binary": "out/startupfixture", "workdir": workdir},
		"a directory, not a binary":              {"binary": "bin", "workdir": workdir},
		"a shell line rather than a binary path": {"binary": "bin/startupfixture; true", "workdir": workdir},
	} {
		t.Run(name, func(t *testing.T) {
			res := actCLIStartup(t, params)
			wantFailedStartup(t, res, "timing "+name)
			if !strings.Contains(res.Error, "binary") {
				t.Errorf("error %q does not name params.binary as the reason", res.Error)
			}
		})
	}
}
