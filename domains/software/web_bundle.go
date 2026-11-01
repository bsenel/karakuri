package software

import (
	"context"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// capWebBundle measures the size of a built web bundle: the sum of the
// apparent sizes of the regular files under params.dir, a directory relative
// to params.workdir (docs/roadmap.md, Phase 41 step 11).
const capWebBundle = "software.verify.web_bundle_bytes"

// webBundleBytes answers software.verify.web_bundle_bytes. It does not build
// anything: building the bundle is an earlier step of the plan, and this
// verifier only measures what that step left in params.dir.
//
// Scaffolding only (Phase 41 step 11, slice 1): the measurement is not written
// yet, so every call is a failed result, never a value.
func (e *verifyEnv) webBundleBytes(_ context.Context, a environment.Action) environment.ActionResult {
	return failureResult(e.id, a.CapabilityID, capWebBundle+" is not implemented yet", nil)
}
