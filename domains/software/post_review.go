package software

import (
	"context"
	"fmt"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// postReview is not written yet.
func (e *gitEnv) postReview(_ context.Context, _ environment.Action) environment.ActionResult {
	return environment.ActionResult{Error: fmt.Sprintf("%s: not implemented", "software.act.post_review")}
}
