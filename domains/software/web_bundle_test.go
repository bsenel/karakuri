package software

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// docs/roadmap.md, Phase 41 step 11, bundle half: "the bundle bytes match du
// on the built fixture". software.verify.web_bundle_bytes measures a bundle a
// plan's earlier step has already built; it builds nothing itself.

func webBundleCapability(t *testing.T) capability.Capability {
	t.Helper()
	for _, c := range (&Pack{}).Capabilities() {
		if c.ID == capWebBundle {
			return c
		}
	}
	t.Fatalf("the software pack registers no %s", capWebBundle)
	return capability.Capability{}
}

func TestTheWebBundleVerifierDeclaresExactlyTheBundleBytesMetric(t *testing.T) {
	c := webBundleCapability(t)
	if !c.Verifiable {
		t.Error("software.verify.web_bundle_bytes is not Verifiable, so no criterion may name it")
	}
	want := []capability.Metric{{
		Name: "web_bundle_bytes", Unit: "bytes", Direction: capability.DirectionLowerIsBetter,
		Params: []capability.MetricParam{{Name: "dir", Type: capability.ParamString}},
	}}
	if !reflect.DeepEqual(c.Metrics, want) {
		t.Errorf("metrics = %#v, want %#v", c.Metrics, want)
	}
}

// Served by the verify environment, where run_tests and lint already run.
func TestTheWebBundleVerifierIsServedByTheVerifyEnvironment(t *testing.T) {
	for _, f := range (&Pack{}).EnvironmentFactories() {
		for _, id := range f.Serves {
			if id != capWebBundle {
				continue
			}
			if f.EnvID != "software.env.verify" {
				t.Errorf("software.verify.web_bundle_bytes is served by %s, want software.env.verify", f.EnvID)
			}
			return
		}
	}
	t.Error("no environment serves software.verify.web_bundle_bytes")
}

func bundleFixtureDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", "bundlefixture"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func actWebBundle(t *testing.T, params map[string]any) environment.ActionResult {
	t.Helper()
	res, err := newTestVerifyEnv().Act(t.Context(), environment.Action{CapabilityID: capWebBundle, Params: params})
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	return res
}

// duTotal is the independent figure: the regular files under dir, listed with
// filepath.WalkDir (which does not follow symlinks), handed to GNU du, and the
// number on its "total" line. CI runs on Ubuntu, which has GNU du; a du that is
// missing or does not take these flags fails the test, it does not skip it.
func duTotal(t *testing.T, dir string) float64 {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	if len(files) == 0 {
		t.Fatalf("%s holds no regular file to hand to du", dir)
	}
	if _, err := exec.LookPath("du"); err != nil {
		t.Fatalf("du is not on PATH (%v); this test needs GNU du to check the bundle bytes", err)
	}
	args := append([]string{"-cb", "--apparent-size"}, files...)
	cmd := exec.Command("du", args...) // #nosec G204 -- fixed flags and paths this test listed itself; no shell
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("du -cb --apparent-size failed (%v): %s; this test needs GNU du, which takes those flags", err, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) != 2 || fields[1] != "total" {
		t.Fatalf("du printed no total line: %q", stdout.String())
	}
	total, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		t.Fatalf("du total %q is not a number: %v", fields[0], err)
	}
	return total
}

func wantOneBundleMeasurement(t *testing.T, res environment.ActionResult) environment.Measurement {
	t.Helper()
	if !res.Success {
		t.Fatalf("measuring the bundle failed: %s", res.Error)
	}
	if len(res.Measurements) != 1 {
		t.Fatalf("measurements = %#v, want exactly one", res.Measurements)
	}
	m := res.Measurements[0]
	if m.Name != "web_bundle_bytes" || m.Unit != "bytes" {
		t.Errorf("measurement is %q in %q, want web_bundle_bytes in bytes", m.Name, m.Unit)
	}
	return m
}

// The bytes the verifier reports on the built fixture are what du says of it.
func TestTheWebBundleVerifierMatchesDuOnTheBuiltFixture(t *testing.T) {
	fixture := bundleFixtureDir(t)
	want := duTotal(t, filepath.Join(fixture, "dist"))
	res := actWebBundle(t, map[string]any{"dir": "dist", "workdir": fixture})
	m := wantOneBundleMeasurement(t, res)
	if m.Value != want {
		t.Errorf("web_bundle_bytes of the fixture = %v, du says %v", m.Value, want)
	}
	if m.Trust != environment.TrustOperator || res.Trust.IsThirdParty() {
		t.Errorf("trust = %q (result %q), want first party: the deployment read the files itself", m.Trust, res.Trust)
	}
	if !strings.Contains(m.Source, "dist") {
		t.Errorf("source %q does not name the directory it measured", m.Source)
	}
	if res.StateDelta["verifier"] != string(capWebBundle) {
		t.Errorf("result names verifier %v, want %s", res.StateDelta["verifier"], capWebBundle)
	}
}

// params.dir is applied: a subdirectory of the bundle measures only itself.
func TestTheWebBundleVerifierMeasuresTheDirItIsGiven(t *testing.T) {
	fixture := bundleFixtureDir(t)
	whole := duTotal(t, filepath.Join(fixture, "dist"))
	want := duTotal(t, filepath.Join(fixture, "dist", "assets"))
	if want == whole {
		t.Fatalf("the fixture's dist/assets is as large as dist (%v); the fixture cannot tell the dirs apart", whole)
	}
	m := wantOneBundleMeasurement(t, actWebBundle(t, map[string]any{"dir": "dist/assets", "workdir": fixture}))
	if m.Value != want {
		t.Errorf("web_bundle_bytes of dist/assets = %v, du says %v (dist is %v)", m.Value, want, whole)
	}
}

func writeBundleFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), size), 0o600); err != nil {
		t.Fatal(err)
	}
}

// params.workdir is applied: the same dir under another workdir is another
// bundle.
func TestTheWebBundleVerifierMeasuresUnderTheWorkdirItIsGiven(t *testing.T) {
	workdir := t.TempDir()
	writeBundleFile(t, filepath.Join(workdir, "dist", "index.html"), 1000)
	writeBundleFile(t, filepath.Join(workdir, "dist", "assets", "app.js"), 2345)
	m := wantOneBundleMeasurement(t, actWebBundle(t, map[string]any{"dir": "dist", "workdir": workdir}))
	if m.Value != 3345 {
		t.Errorf("web_bundle_bytes under %s = %v, want 3345", workdir, m.Value)
	}
}

// A symlink is not followed: neither one to a file of the bundle, which would
// count it twice, nor one to a directory outside it.
func TestTheWebBundleVerifierDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	workdir := filepath.Join(root, "work")
	outside := filepath.Join(root, "outside")
	writeBundleFile(t, filepath.Join(workdir, "dist", "app.js"), 100)
	writeBundleFile(t, filepath.Join(outside, "huge.js"), 5000)
	if err := os.Symlink("app.js", filepath.Join(workdir, "dist", "again.js")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workdir, "dist", "vendor")); err != nil {
		t.Fatal(err)
	}
	want := duTotal(t, filepath.Join(workdir, "dist"))
	if want != 100 {
		t.Fatalf("du of the regular files = %v, want 100", want)
	}
	m := wantOneBundleMeasurement(t, actWebBundle(t, map[string]any{"dir": "dist", "workdir": workdir}))
	if m.Value != want {
		t.Errorf("web_bundle_bytes = %v, want %v: a symlink was followed", m.Value, want)
	}
}

// Blind, not quiet (AGENTS.md rule 10): a bundle the verifier cannot see, or
// is not allowed to look at, is a failed result with a reason and no
// Measurement. A 0 would read as the smallest bundle there is and settle a
// lower_is_better criterion met on a run that measured nothing.
func TestTheWebBundleVerifierNeverReportsAZeroItDidNotMeasure(t *testing.T) {
	root := t.TempDir()
	workdir := filepath.Join(root, "work")
	outside := filepath.Join(root, "outside")
	writeBundleFile(t, filepath.Join(workdir, "dist", "index.html"), 10)
	writeBundleFile(t, filepath.Join(outside, "dist", "app.js"), 20)
	if err := os.MkdirAll(filepath.Join(workdir, "empty", "assets"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(workdir, "dist", "index.html"), filepath.Join(workdir, "empty", "index.html")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workdir, "out")); err != nil {
		t.Fatal(err)
	}
	for name, params := range map[string]map[string]any{
		"no params.dir":                     {"workdir": workdir},
		"a dir that is not there":           {"dir": "nosuch", "workdir": workdir},
		"a dir that holds no regular file":  {"dir": "empty", "workdir": workdir},
		"an absolute dir":                   {"dir": filepath.Join(workdir, "dist"), "workdir": workdir},
		"a dir that escapes with ..":        {"dir": "../outside/dist", "workdir": workdir},
		"a dir that escapes after a subdir": {"dir": "dist/../../outside/dist", "workdir": workdir},
		"a symlink that points outside":     {"dir": "out", "workdir": workdir},
		"a path through a symlink outside":  {"dir": "out/dist", "workdir": workdir},
	} {
		t.Run(name, func(t *testing.T) {
			res := actWebBundle(t, params)
			if res.Success {
				t.Errorf("measuring %v succeeded with %#v; it must fail", params, res.Measurements)
			}
			if res.Error == "" {
				t.Error("a failed bundle measurement carries no reason")
			}
			if len(res.Measurements) != 0 {
				t.Errorf("a failed bundle measurement carries measurements %#v, want none", res.Measurements)
			}
			if !strings.Contains(res.Error, "dir") {
				t.Errorf("error %q does not name params.dir as the reason", res.Error)
			}
		})
	}
}
