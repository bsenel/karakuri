package software

import (
	"context"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// capCLIStartup measures how long a CLI takes to start: the wall time from
// starting params.binary, a path relative to params.workdir, with params.args
// (default --help) to its exit, in ms (docs/roadmap.md, Phase 41 step 11).
const capCLIStartup = "software.verify.cli_startup_ms"

// cliStartupMs answers software.verify.cli_startup_ms. Not implemented yet:
// it fails, so it never reports a time it did not measure.
func (e *verifyEnv) cliStartupMs(_ context.Context, a environment.Action) environment.ActionResult {
	return failureResult(e.id, a.CapabilityID, "software.verify.cli_startup_ms is not implemented yet", nil)
}
