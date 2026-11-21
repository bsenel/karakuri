package software

// The empty-pipeline discovery trigger (Phase 40 step 13, discovery half): the
// same reading that says whether delivery has work says whether a market
// discovery cycle should start. Scaffolding only; the policy is the next action.

import (
	"context"
	"errors"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// DiscoveryMinGap is the minimum time between empty-pipeline cycles, declared
// on the discovery template (24 hours as a first guess).
const DiscoveryMinGap = 24 * time.Hour

// DiscoveryTrigger is the second outcome of the reading: whether an
// empty-pipeline discovery cycle starts now and, in words, why or why not.
type DiscoveryTrigger struct {
	Start   bool
	Reasons []string
}

// OpenDiscoveryProposals picks, from the open pull requests, those on the
// discovery stream's branch pattern (karakuri/discovery-*), without a model.
func OpenDiscoveryProposals(prs []versioncontrol.PRSummary) []versioncontrol.PRSummary {
	return nil
}

// ComputeDiscoveryTrigger decides from the delivery verdict, the open pull
// requests, the end of the last cycle of either trigger and the clock.
func ComputeDiscoveryTrigger(work DeliveryWork, prs []versioncontrol.PRSummary, lastCycleEnd, now time.Time, minGap time.Duration) DiscoveryTrigger {
	return DiscoveryTrigger{}
}

// DiscoveryTriggerSources is what the trigger's environment reads besides the forge.
type DiscoveryTriggerSources struct {
	ReadMain func(context.Context) (string, error)
	// LastCycleEnd is when the last discovery cycle of either trigger ended; zero if none.
	LastCycleEnd func(context.Context) (time.Time, error)
	// ClosingComment is the owner's comment on the most recently closed proposal, "" if none.
	ClosingComment func(context.Context) (string, error)
	MinGap         time.Duration
	Now            func() time.Time
}

// NewDiscoveryTriggerEnvironment is the environment the discovery stream's
// reconcile fingerprints: state "has_work" is true when a cycle should start,
// and "closing_comment" carries the text of a closed proposal's last comment
// with Trust set to environment.TrustThirdParty.
func NewDiscoveryTriggerEnvironment(id environment.EnvironmentID, vc versioncontrol.VersionControlAdapter, src DiscoveryTriggerSources) environment.Environment {
	return &discoveryTriggerEnv{id: id}
}

type discoveryTriggerEnv struct{ id environment.EnvironmentID }

func (e *discoveryTriggerEnv) ID() environment.EnvironmentID { return e.id }
func (e *discoveryTriggerEnv) Domain() string                { return "software" }
func (e *discoveryTriggerEnv) Observe(context.Context, environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{}, errors.New("not implemented")
}
func (e *discoveryTriggerEnv) Act(context.Context, environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{}, nil
}
func (e *discoveryTriggerEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return nil, nil
}
func (e *discoveryTriggerEnv) Snapshot(context.Context) (environment.EnvironmentSnapshot, error) {
	return environment.EnvironmentSnapshot{EnvID: e.id}, nil
}
