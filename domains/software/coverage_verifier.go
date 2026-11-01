package software

import (
	"context"
	"errors"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// capCoverage measures the statement coverage of params.packages by running
// go test -coverprofile in params.workdir (docs/roadmap.md, Phase 41 step 10).
//
// The file is coverage_verifier.go, not coverage.go, because .gitignore's
// "coverage.*" (meant for cover profiles) would leave coverage.go untracked.
const capCoverage = "software.verify.coverage"

// errCoverageNotImplemented is the scaffolding's answer until step 10's
// coverage code lands; the tests in coverage_test.go specify it.
var errCoverageNotImplemented = errors.New("software.verify.coverage is not implemented yet")

// coverPercent turns a Go cover profile into its statement-weighted
// percentage.
func coverPercent(_ string) (float64, error) {
	return 0, errCoverageNotImplemented
}

// packagePatterns splits params.packages into the Go package patterns go test
// is given, refusing anything that is not one.
func packagePatterns(_ string) ([]string, error) {
	return nil, errCoverageNotImplemented
}

// coverage answers software.verify.coverage.
func (e *verifyEnv) coverage(_ context.Context, a environment.Action) environment.ActionResult {
	return failureResult(e.id, a.CapabilityID, errCoverageNotImplemented.Error(), nil)
}
