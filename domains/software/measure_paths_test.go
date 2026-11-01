package software

import (
	"slices"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
)

// docs/roadmap.md, Phase 41 step 9: run_tests declares the paths its
// definition is read from, the test files, so a pass that edits one leaves a
// criterion naming it inconclusive: measure_changed. The web globs are the ones
// web/ actually runs: vitest's include (web/vitest.config.ts) and Playwright's
// testDir (web/playwright.config.ts).
func TestRunTestsDeclaresTheTestFilesAsItsMeasurePaths(t *testing.T) {
	var runTests capability.Capability
	for _, c := range (&Pack{}).Capabilities() {
		if c.ID == "software.verify.run_tests" {
			runTests = c
		}
	}
	if runTests.ID == "" {
		t.Fatal("the pack registers no software.verify.run_tests")
	}

	for _, want := range []string{"**/*_test.go", "web/src/**/*.test.ts", "web/src/**/*.test.tsx", "web/e2e/**/*.spec.ts"} {
		if !slices.Contains(runTests.MeasurePaths, want) {
			t.Errorf("run_tests measure paths %q lack %q", runTests.MeasurePaths, want)
		}
	}

	// Each declared glob has to catch the files it is there for.
	for _, file := range []string{
		"internal/feature/loop/verify_test.go",
		"web/src/components/StandingPanel.test.tsx",
		"web/src/api/client.test.ts",
		"web/e2e/admin.spec.ts",
	} {
		if !capability.TouchesAnyGlob([]string{file}, runTests.MeasurePaths) {
			t.Errorf("run_tests measure paths %q miss %s", runTests.MeasurePaths, file)
		}
	}
	// And not the code under test.
	if capability.TouchesAnyGlob([]string{"internal/feature/loop/verify.go", "web/src/App.tsx"}, runTests.MeasurePaths) {
		t.Errorf("run_tests measure paths %q catch code that is not a test", runTests.MeasurePaths)
	}
}
