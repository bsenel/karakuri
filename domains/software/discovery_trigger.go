package software

// The empty-pipeline discovery trigger (Phase 40 step 13, discovery half): the
// same reading that says whether delivery has work says whether a market
// discovery cycle should start. One reading, two outcomes: no second mechanism.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// DiscoveryMinGap is the minimum time between empty-pipeline cycles, declared
// on the discovery template (24 hours as a first guess).
const DiscoveryMinGap = 24 * time.Hour

// discoveryBranchPrefix is the discovery stream's branch pattern (step 2).
const discoveryBranchPrefix = "karakuri/discovery-"

// DiscoveryTrigger is the second outcome of the reading: whether an
// empty-pipeline discovery cycle starts now and, in words, why or why not.
type DiscoveryTrigger struct {
	Start   bool
	Reasons []string
}

// OpenDiscoveryProposals picks, from the open pull requests, those on the
// discovery stream's branch pattern (karakuri/discovery-*), without a model.
func OpenDiscoveryProposals(prs []versioncontrol.PRSummary) []versioncontrol.PRSummary {
	var out []versioncontrol.PRSummary
	for _, pr := range prs {
		if strings.HasPrefix(pr.HeadBranch, discoveryBranchPrefix) {
			out = append(out, pr)
		}
	}
	return out
}

// ComputeDiscoveryTrigger decides from the delivery verdict, the open pull
// requests, the end of the last cycle of either trigger and the clock. Reasons
// carry nothing that changes with the clock, so the fingerprint stays still
// while the verdict does.
func ComputeDiscoveryTrigger(work DeliveryWork, prs []versioncontrol.PRSummary, lastCycleEnd, now time.Time, minGap time.Duration) DiscoveryTrigger {
	if work.HasWork {
		return DiscoveryTrigger{Reasons: append([]string{"the pipeline is not empty"}, work.Reasons...)}
	}
	if open := OpenDiscoveryProposals(prs); len(open) > 0 {
		reasons := make([]string, 0, len(open))
		for _, pr := range open {
			reasons = append(reasons, fmt.Sprintf("discovery proposal %d is open, waiting for the owner", pr.Number))
		}
		return DiscoveryTrigger{Reasons: reasons}
	}
	if !lastCycleEnd.IsZero() && now.Sub(lastCycleEnd) < minGap {
		return DiscoveryTrigger{Reasons: []string{"the last discovery cycle ended inside the minimum time"}}
	}
	return DiscoveryTrigger{Start: true, Reasons: []string{"the pipeline is empty, no discovery proposal is open and the minimum time has passed"}}
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
	return &discoveryTriggerEnv{id: id, vc: vc, src: src}
}

type discoveryTriggerEnv struct {
	id  environment.EnvironmentID
	vc  versioncontrol.VersionControlAdapter
	src DiscoveryTriggerSources
}

func (e *discoveryTriggerEnv) ID() environment.EnvironmentID { return e.id }
func (e *discoveryTriggerEnv) Domain() string                { return "software" }

// Observe is blind (an error) when main, the pull requests or the record of the
// last cycle cannot be read: that must never look like an empty pipeline.
func (e *discoveryTriggerEnv) Observe(ctx context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	if e.vc == nil || e.src.ReadMain == nil || e.src.LastCycleEnd == nil {
		return environment.Observation{}, errors.New("no forge or no source for main's roadmap or the last cycle is bound")
	}
	work, prs, err := readDeliveryWork(ctx, e.vc, e.src.ReadMain)
	if err != nil {
		return environment.Observation{}, err
	}
	last, err := e.src.LastCycleEnd(ctx)
	if err != nil {
		return environment.Observation{}, fmt.Errorf("the end of the last discovery cycle could not be read: %w", err)
	}
	now := time.Now().UTC()
	if e.src.Now != nil {
		now = e.src.Now().UTC()
	}
	verdict := ComputeDiscoveryTrigger(work, prs, last, now, e.src.MinGap)
	state := map[string]any{"has_work": verdict.Start, "reasons": verdict.Reasons}
	trust := environment.TrustOperator
	if e.src.ClosingComment != nil {
		comment, err := e.src.ClosingComment(ctx)
		if err != nil {
			return environment.Observation{}, fmt.Errorf("the closing comment could not be read: %w", err)
		}
		if comment != "" {
			state["closing_comment"] = comment
			trust = environment.TrustThirdParty
		}
	}
	// The comment is left out of the hash: the pass reads it, it does not decide whether one starts.
	sum := sha256.Sum256([]byte(fmt.Sprintf("%v|%s", verdict.Start, strings.Join(verdict.Reasons, "\n"))))
	return environment.Observation{
		EnvID: e.id, State: state, Version: hex.EncodeToString(sum[:])[:16], Timestamp: time.Now().UTC(), Trust: trust,
	}, nil
}

func (e *discoveryTriggerEnv) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{Success: false, Error: fmt.Sprintf("%s: the discovery-trigger environment only observes", a.CapabilityID)}, nil
}

func (e *discoveryTriggerEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return nil, nil
}

// Snapshot has an empty SHA when Observe could not read (AGENTS.md rule 10).
func (e *discoveryTriggerEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	obs, err := e.Observe(ctx, environment.ObservationQuery{})
	if err != nil {
		return environment.EnvironmentSnapshot{EnvID: e.id}, nil
	}
	return environment.EnvironmentSnapshot{SHA: obs.Version, EnvID: e.id, State: obs.State, Timestamp: obs.Timestamp}, nil
}
