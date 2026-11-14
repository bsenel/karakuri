package software

import (
	"context"
	"fmt"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// CapMarkReady takes the open draft pull request on params.branch out of
// draft once every item of its body's checklist is ticked and every check at
// its head is green (Phase 44 step 1).
const CapMarkReady = "software.act.mark_ready"

// markReady is a stub until Phase 44 step 1 is implemented.
func (e *gitEnv) markReady(_ context.Context, _ environment.Action) environment.ActionResult {
	return environment.ActionResult{Success: false, Error: fmt.Sprintf("%s: not implemented", CapMarkReady)}
}
