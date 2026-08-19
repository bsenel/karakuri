package karakuri

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	coretelemetry "github.com/bsenel/karakuri/internal/core/telemetry"
	"github.com/bsenel/karakuri/internal/core/vfs"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

const (
	// EnvTelemetry is this deployment watching itself.
	EnvTelemetry = "karakuri.env.telemetry"
	// EnvRepo is the repository Karakuri is built from.
	EnvRepo = "karakuri.env.repo"
)

// telemetryWindow is how far back the telemetry environment looks.
//
// A week rather than a day: the questions this pack asks — what keeps failing,
// what is nobody deciding, where is the money going — are answered badly by a
// single day's noise and well by a week's shape.
const telemetryWindow = 7 * 24 * time.Hour

func karakuriEnvironmentFactories(reg *tools.Registry) []environment.Factory {
	return []environment.Factory{
		{
			EnvID:       EnvTelemetry,
			Domain:      "karakuri",
			Description: "This deployment's own behaviour: escalation rates, spend, failing objectives, unanswered decisions",
			Build: func(bc environment.BuildContext) (environment.Environment, error) {
				// A nil reader is the ordinary case for a deployment that has
				// not wired telemetry, and the environment says so rather than
				// reporting zeroes that read as a healthy system.
				return &telemetryEnv{reader: bc.Telemetry, twinID: bc.TwinID}, nil
			},
		},
		{
			EnvID:       EnvRepo,
			Domain:      "karakuri",
			Description: "The repository Karakuri is built from: open pull requests and their checks",
			Build: func(bc environment.BuildContext) (environment.Environment, error) {
				var adapter versioncontrol.VersionControlAdapter
				if reg != nil {
					if a, ok := reg.VC.Resolve(bc.AdapterBindings["versioncontrol"]); ok {
						adapter = a
					}
				}
				return &repoEnv{adapter: adapter}, nil
			},
		},
	}
}

// ── telemetry ─────────────────────────────────────────────────────────────

type telemetryEnv struct {
	reader coretelemetry.Reader
	twinID string
}

func (e *telemetryEnv) ID() environment.EnvironmentID { return EnvTelemetry }
func (e *telemetryEnv) Domain() string                { return "karakuri" }

func (e *telemetryEnv) Observe(ctx context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	obs := environment.Observation{EnvID: EnvTelemetry, Timestamp: time.Now().UTC()}
	if e.reader == nil {
		obs.State = map[string]any{
			"available": false,
			"reason":    "no telemetry reader is wired into this deployment",
		}
		return obs, nil
	}
	snap, err := e.reader.Snapshot(ctx, coretelemetry.Query{
		Since:  time.Now().UTC().Add(-telemetryWindow),
		TwinID: e.twinID,
	})
	if err != nil {
		return obs, err
	}
	obs.State = map[string]any{
		"available":     true,
		"since":         snap.Since,
		"objectives":    snap.Objectives,
		"work":          snap.Work,
		"escalation":    snap.Escalation,
		"approval_rate": snap.Escalation.ApprovalRate(),
		"spend":         snap.Spend,
		"bottlenecks":   snap.Bottlenecks,
	}
	obs.Version = coarseFingerprint(snap)
	return obs, nil
}

func (e *telemetryEnv) Act(ctx context.Context, a environment.Action) (environment.ActionResult, error) {
	// "Read-only" means this environment cannot change the deployment. It does
	// not mean it can do nothing: the loop runs every capability through Act,
	// including the ones that only look, so refusing unconditionally left the
	// pack unable to execute the analysis it exists to perform.
	if a.CapabilityID == CapAnalyseUsage {
		if e.reader == nil {
			return environment.ActionResult{
				Success: false,
				Error:   "no telemetry reader is wired into this deployment",
			}, nil
		}
		snap, err := e.reader.Snapshot(ctx, coretelemetry.Query{
			Since:  time.Now().UTC().Add(-telemetryWindow),
			TwinID: e.twinID,
		})
		if err != nil {
			return environment.ActionResult{Success: false, Error: err.Error()}, nil
		}
		// Reported alongside the numbers rather than left to be inferred from
		// them: a window with nothing in it and a deployment with nothing
		// wrong produce the same zeroes, and a pack reasoning about what to
		// improve must be able to tell them apart.
		return environment.ActionResult{
			Success: true,
			StateDelta: map[string]any{
				"capability":    string(CapAnalyseUsage),
				"since":         snap.Since,
				"objectives":    snap.Objectives,
				"work":          snap.Work,
				"escalation":    snap.Escalation,
				"approval_rate": snap.Escalation.ApprovalRate(),
				"spend":         snap.Spend,
				"bottlenecks":   snap.Bottlenecks,
				"sufficient":    snap.Objectives.Total > 0 || snap.Work.Senses > 0 || snap.Work.Reconciles > 0,
			},
		}, nil
	}

	// Anything else is refused out loud rather than succeeding quietly. The
	// whole value of letting Karakuri watch itself is that the watching cannot
	// be edited by the thing being watched.
	return environment.ActionResult{
		Success: false,
		Error:   fmt.Sprintf("%s is read-only; %s cannot be executed here", EnvTelemetry, a.CapabilityID),
	}, nil
}

func (e *telemetryEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return nil, nil
}

func (e *telemetryEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	snap := environment.EnvironmentSnapshot{EnvID: EnvTelemetry, Timestamp: time.Now().UTC()}
	if e.reader == nil {
		return snap, nil // blind, and the supervisor reads it as blind
	}
	s, err := e.reader.Snapshot(ctx, coretelemetry.Query{
		Since:  time.Now().UTC().Add(-telemetryWindow),
		TwinID: e.twinID,
	})
	if err != nil {
		return snap, err
	}
	snap.SHA = coarseFingerprint(s)
	snap.State = map[string]any{"bottlenecks": len(s.Bottlenecks)}
	return snap, nil
}

// coarseFingerprint hashes the *shape* of the deployment rather than its
// counters.
//
// A hash over raw numbers would move every time anything happened, which for
// a busy deployment is constantly — and a self-improvement objective that
// reconciled on every counter tick would spend all day noticing that work
// occurred. What is worth waking up for is a change in kind: a new bottleneck,
// a bottleneck getting materially worse, decisions piling up, the approval
// rate crossing a line. So counts are bucketed by order of magnitude and only
// the bottleneck set is hashed exactly.
func coarseFingerprint(s coretelemetry.Snapshot) string {
	parts := []string{
		fmt.Sprintf("blocked=%d", s.Objectives.Blocked),
		fmt.Sprintf("pending=%s", bucket(s.Escalation.Pending)),
		fmt.Sprintf("failures=%s", bucket(s.Work.Failures)),
		fmt.Sprintf("approval=%s", rateBand(s.Escalation.ApprovalRate())),
	}
	kinds := make([]string, 0, len(s.Bottlenecks))
	for _, b := range s.Bottlenecks {
		kinds = append(kinds, b.Kind+":"+b.Detail+":"+bucket(b.Count))
	}
	sort.Strings(kinds)
	parts = append(parts, kinds...)
	return vfs.SHA([]byte(strings.Join(parts, "|")))
}

// bucket collapses a count to an order of magnitude, so going from 41 failures
// to 42 is not news and going from 9 to 30 is.
func bucket(n int) string {
	switch {
	case n == 0:
		return "0"
	case n < 3:
		return "1-2"
	case n < 10:
		return "3-9"
	case n < 30:
		return "10-29"
	case n < 100:
		return "30-99"
	default:
		return "100+"
	}
}

// rateBand bands an approval rate. -1 means nothing was decided, which is a
// different state from "everything was rejected" and bands separately.
func rateBand(r float64) string {
	switch {
	case r < 0:
		return "undecided"
	case r < 0.5:
		return "mostly-rejected"
	case r < 0.9:
		return "mixed"
	default:
		return "mostly-approved"
	}
}

// ── repository ────────────────────────────────────────────────────────────

type repoEnv struct {
	adapter versioncontrol.VersionControlAdapter
}

func (e *repoEnv) ID() environment.EnvironmentID { return EnvRepo }
func (e *repoEnv) Domain() string                { return "karakuri" }

func (e *repoEnv) Observe(ctx context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	obs := environment.Observation{EnvID: EnvRepo, Timestamp: time.Now().UTC()}
	if e.adapter == nil || !e.adapter.Active() {
		obs.State = map[string]any{
			"available": false,
			"reason":    "no version-control adapter is bound to this twin",
		}
		return obs, nil
	}
	// A month rather than everything: an open pull request nobody has touched
	// in a month is not a signal about what to do next, it is a signal about
	// somebody's backlog, and this environment is watching the first.
	prs, err := e.adapter.ListPRs(ctx, "", time.Now().UTC().AddDate(0, -1, 0))
	if err != nil {
		return obs, err
	}
	titles := make([]string, 0, len(prs))
	for _, pr := range prs {
		titles = append(titles, pr.ID+" "+pr.Title)
	}
	// Sorted before hashing, for the same reason the reconcile fingerprint
	// sorts environment IDs: an adapter free to return rows in any order would
	// otherwise report drift on nothing having changed.
	sort.Strings(titles)
	obs.State = map[string]any{
		"available":          true,
		"open_pull_requests": len(prs),
		"titles":             titles,
	}
	obs.Version = vfs.SHA([]byte(strings.Join(titles, "|")))
	return obs, nil
}

func (e *repoEnv) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	// Drafting is not writing. propose_roadmap_phase and draft_adr produce
	// text for a human to read, and the checkpoint that carries it is the
	// review — nothing here touches the repository. Changing it is still the
	// software pack's job, in a worktree, through capabilities an operator
	// already reviews.
	switch a.CapabilityID {
	case CapProposeRoadmap, CapDraftADR:
		draft := map[string]any{
			"capability": string(a.CapabilityID),
			"drafted":    true,
			// The agent's own params are the draft. Echoed back so the verify
			// step scores what was actually produced, and so the checkpoint
			// shows a reviewer the text rather than the fact that text exists.
			"content": a.Params,
			"note":    "a draft only; applying it to the repository is the software pack's job",
		}
		return environment.ActionResult{Success: true, StateDelta: draft}, nil
	}

	return environment.ActionResult{
		Success: false,
		Error:   fmt.Sprintf("%s is read-only; use the software pack to change the repository (%s)", EnvRepo, a.CapabilityID),
	}, nil
}

func (e *repoEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return nil, nil
}

func (e *repoEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	snap := environment.EnvironmentSnapshot{EnvID: EnvRepo, Timestamp: time.Now().UTC()}
	obs, err := e.Observe(ctx, environment.ObservationQuery{})
	if err != nil {
		return snap, err
	}
	snap.SHA = obs.Version
	snap.State = obs.State
	return snap, nil
}
