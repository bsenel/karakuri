package software

// Whether delivery has work (Phase 40 step 13), computed without a model.
//
// SCAFFOLD ONLY: the stubs below return zero values so the step's tests
// compile and fail for the right reason. The implementation is the next slice.

import (
	"context"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// DeliveryWork is the verdict: whether a delivery pass has anything to do and,
// in words, why or why not.
type DeliveryWork struct {
	HasWork bool
	Reasons []string
}

// ComputeDeliveryWork reads main's roadmap text, the open pull requests and the
// git observation's state (branches_without_pr, unanswered_review_comments).
// A roadmap that could not be read is an error, never "no work" (AGENTS.md rule 10).
func ComputeDeliveryWork(roadmap string, roadmapErr error, prs []versioncontrol.PRSummary, state map[string]any) (DeliveryWork, error) {
	return DeliveryWork{}, nil
}

// NewDeliveryWorkEnvironment is the environment the delivery stream's
// reconcile fingerprints: its snapshot is a hash of the computed verdict and
// what it was computed from, so it moves when the work changes and not otherwise.
func NewDeliveryWorkEnvironment(id environment.EnvironmentID, vc versioncontrol.VersionControlAdapter, readMain func(context.Context) (string, error)) environment.Environment {
	return &deliveryWorkEnv{id: id}
}

type deliveryWorkEnv struct{ id environment.EnvironmentID }

func (e *deliveryWorkEnv) ID() environment.EnvironmentID { return e.id }
func (e *deliveryWorkEnv) Domain() string                { return "software" }
func (e *deliveryWorkEnv) Observe(context.Context, environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{EnvID: e.id}, nil
}
func (e *deliveryWorkEnv) Act(context.Context, environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{}, nil
}
func (e *deliveryWorkEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return nil, nil
}
func (e *deliveryWorkEnv) Snapshot(context.Context) (environment.EnvironmentSnapshot, error) {
	return environment.EnvironmentSnapshot{EnvID: e.id}, nil
}
