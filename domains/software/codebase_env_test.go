package software

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// scratchRepo writes a small tree with the three things the scan looks for.
func scratchTree(t *testing.T, withRoadmap bool) string {
	t.Helper()
	dir := t.TempDir()

	write := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("internal/hot/a.go", "package hot\n\n// TODO: this one\nfunc A() {}\n// FIXME: and this\n")
	write("internal/hot/b.go", "package hot\n\n// TODO: a third\n")
	write("internal/hot/hot_test.go", "package hot\n")
	write("internal/cold/c.go", "package cold\n\nfunc C() {}\n") // no test file
	write("internal/cold/AGENTS.md", "rules live here\n")
	// Must be skipped, or a vendored dependency's TODOs drown the repository's.
	write("vendor/other/d.go", "package other\n\n// TODO: not ours\n")

	if withRoadmap {
		write("docs/roadmap.md", strings.Join([]string{
			"## Phase 9 — Something (Completed)",
			"**Still open (step 4).** The routing gap.",
			"## Phase 10 — Another (Partial)",
			"**Deferred to Phase 11.** The other thing.",
			"Some ordinary prose that is not deferred work.",
		}, "\n")+"\n")
	}
	return dir
}

// The three facts the scan is for, on a tree where the answers are known.
func TestTheRepositoryScanReportsWhatItClaims(t *testing.T) {
	env := newCodebaseEnv("software.env.codebase", scratchTree(t, true))

	res, err := env.Act(context.Background(), environment.Action{CapabilityID: CapAnalyseRepo})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if !res.Success {
		t.Fatalf("analyse_repo refused: %s", res.Error)
	}
	state := res.StateDelta

	if state["available"] != true {
		t.Fatalf("scan reported unavailable: %v", state["reason"])
	}

	// Deferred work, with the phase it belongs to — a line without its phase
	// is a sentence nobody can act on.
	deferred, _ := state["deferred_work"].([]string)
	if len(deferred) != 2 {
		t.Errorf("found %d deferred items, want 2: %v", len(deferred), deferred)
	}
	for _, want := range []string{"Phase 9", "Phase 10"} {
		var found bool
		for _, d := range deferred {
			if strings.Contains(d, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("deferred work does not name %s: %v", want, deferred)
		}
	}

	// TODO density, ranked, and not counting vendored code.
	density, _ := state["todo_density"].([]map[string]any)
	if len(density) == 0 {
		t.Fatal("no TODO density reported")
	}
	if got := density[0]["package"]; got != filepath.Join("internal", "hot") {
		t.Errorf("hottest package is %v, want internal/hot", got)
	}
	if got := density[0]["markers"]; got != 3 {
		t.Errorf("internal/hot has %v markers, want 3", got)
	}
	for _, d := range density {
		if strings.HasPrefix(d["package"].(string), "vendor") {
			t.Error("vendored code was counted; its TODOs are not this repository's")
		}
	}

	// Packages with source and no test file.
	untested, _ := state["untested_packages"].([]string)
	if len(untested) != 1 || untested[0] != filepath.Join("internal", "cold") {
		t.Errorf("untested packages = %v, want [internal/cold]", untested)
	}

	rules, _ := state["rules"].([]string)
	if len(rules) != 1 {
		t.Errorf("found %d AGENTS.md files, want 1: %v", len(rules), rules)
	}
}

// The point of the phase: a deployment with no history can still produce
// evidence, because the repository exists on day one.
func TestARepositoryIsAdequateEvidenceOnDayOne(t *testing.T) {
	withRoadmap := newCodebaseEnv("software.env.codebase", scratchTree(t, true))
	res, _ := withRoadmap.Act(context.Background(), environment.Action{CapabilityID: CapAnalyseRepo})
	if got := res.StateDelta["evidence"]; got != EvidenceAdequate {
		t.Errorf("a repository with deferred work graded %v, want adequate", got)
	}

	// And an empty tree is honest about having nothing, rather than reporting
	// adequate evidence for the absence of complaints.
	empty := newCodebaseEnv("software.env.codebase", t.TempDir())
	res, _ = empty.Act(context.Background(), environment.Action{CapabilityID: CapAnalyseRepo})
	if got := res.StateDelta["evidence"]; got != EvidenceNone {
		t.Errorf("an empty tree graded %v, want none", got)
	}
}

// An unreadable root is reported, not raised. The pack degrades the way the
// telemetry environment does with no reader wired: it says it is blind rather
// than recording its own core capability as failing, which after three tries
// would raise a bottleneck against itself.
func TestAnUnreadableRootIsReportedNotFailed(t *testing.T) {
	env := newCodebaseEnv("software.env.codebase", filepath.Join(t.TempDir(), "does-not-exist"))
	res, err := env.Act(context.Background(), environment.Action{CapabilityID: CapAnalyseRepo})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if !res.Success {
		t.Error("an unreadable root was recorded as the capability failing")
	}
	if res.StateDelta["available"] != false || res.StateDelta["evidence"] != EvidenceNone {
		t.Error("an unreadable root did not report itself blind")
	}
}

// It reads the repository; it does not act on it.
func TestTheCodebaseEnvironmentRefusesAnythingElse(t *testing.T) {
	env := newCodebaseEnv("software.env.codebase", t.TempDir())
	res, err := env.Act(context.Background(), environment.Action{CapabilityID: "software.act.write_code"})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if res.Success {
		t.Error("the codebase environment executed a write capability")
	}
}

// Against this repository, which is the only test of whether the evidence is
// worth anything. A scan that finds nothing in a codebase with a roadmap full
// of deferred work is a scan nobody should propose from.
func TestTheScanFindsRealWorkInThisRepository(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not running inside the repository: %v", err)
	}
	env := newCodebaseEnv("software.env.codebase", root)
	res, err := env.Act(context.Background(), environment.Action{CapabilityID: CapAnalyseRepo})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	state := res.StateDelta

	deferred, _ := state["deferred_work"].([]string)
	if len(deferred) == 0 {
		t.Error("no deferred work found in a roadmap that marks several phases still open")
	}
	if got := state["evidence"]; got != EvidenceAdequate {
		t.Errorf("this repository graded %v, want adequate", got)
	}
	t.Logf("deferred work found: %d items", len(deferred))
	for _, d := range deferred {
		t.Logf("  %s", d)
	}
}

// repoRoot walks up to the directory holding go.mod.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
