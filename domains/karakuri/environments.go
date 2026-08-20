package karakuri

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
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

// servedBy names the environment that executes each capability this pack
// owns.
//
// One table, pinned by a test against the pack's declared capabilities. The
// alternative — a switch inside each environment's Act and nothing tying them
// together — is how this pack shipped inert: a capability nobody routed is
// refused at runtime with every test still passing. Phase 25 plans
// karakuri.analyse_repo, and adding it without an entry here should fail the
// suite rather than fail in production.
var servedBy = map[capability.CapabilityID]environment.EnvironmentID{
	CapAnalyseUsage:   EnvTelemetry,
	CapProposeRoadmap: EnvRepo,
	CapDraftADR:       EnvRepo,
}

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
	obs.State = snapshotState(snap)
	obs.Version = coarseFingerprint(snap)
	return obs, nil
}

// snapshotState renders a telemetry snapshot for both Observe and Act.
//
// One builder rather than two copies, because the two had already diverged on
// the fields that matter: the insufficiency marker existed only on the Act
// path, and Act is not the path that feeds planning. stepObserve's result is
// what stepReason reasons from, so a distinction absent from Observe is
// absent from the decision it was added to inform.
func snapshotState(snap coretelemetry.Snapshot) map[string]any {
	return map[string]any{
		"available":     true,
		"since":         snap.Since,
		"taken_at":      snap.TakenAt,
		"objectives":    snap.Objectives,
		"work":          snap.Work,
		"escalation":    snap.Escalation,
		"approval_rate": snap.Escalation.ApprovalRate(),
		"spend":         snap.Spend,
		"bottlenecks":   snap.Bottlenecks,
		"sufficient":    sufficient(snap),
	}
}

// sufficient reports whether the window holds enough to reason from.
//
// Only window-scoped terms count. An earlier version ORed in
// Objectives.Total, which the reader computes with no time bound at all — so
// it was true in any deployment that had ever created an objective, including
// the self-improvement objective doing the asking. A flag that cannot be
// false in production is worse than no flag: it answers the question it was
// added to answer, wrongly, every time.
func sufficient(snap coretelemetry.Snapshot) bool {
	return snap.Work.Senses > 0 ||
		snap.Work.Reconciles > 0 ||
		snap.Work.Actions > 0 ||
		snap.Escalation.Escalations > 0 ||
		len(snap.Bottlenecks) > 0
}

func (e *telemetryEnv) Act(ctx context.Context, a environment.Action) (environment.ActionResult, error) {
	// "Read-only" means this environment cannot change the deployment. It does
	// not mean it can do nothing: the loop runs every capability through Act,
	// including the ones that only look, so refusing unconditionally left the
	// pack unable to execute the analysis it exists to perform.
	if a.CapabilityID == CapAnalyseUsage {
		if e.reader == nil {
			// Unwired is not broken. Observe and Snapshot both report a nil
			// reader as blind; a failed action here would be recorded as the
			// pack's own core capability failing, and after three such the
			// reader would raise a failing_capability bottleneck against it —
			// the pack diagnosing its own missing configuration as a defect.
			return environment.ActionResult{
				Success: true,
				StateDelta: map[string]any{
					"capability": string(CapAnalyseUsage),
					"available":  false,
					"sufficient": false,
					"reason":     "no telemetry reader is wired into this deployment",
				},
			}, nil
		}

		window := telemetryWindow
		if s, ok := a.Params["window"].(string); ok && s != "" {
			if d, err := time.ParseDuration(s); err == nil && d > 0 {
				window = d
			}
		}

		// The environment's twin is a ceiling, not a default. A plan asking
		// for another tenant's numbers — or for the whole deployment from
		// inside one tenant — is asking for the cross-tenant read that the
		// telemetry reader was fixed to prevent, and it gets the same answer
		// here: narrowing is allowed, widening is not.
		twinID := e.twinID
		if s, ok := a.Params["twin_id"].(string); ok && s != "" && e.twinID == "" {
			twinID = s
		}

		snap, err := e.reader.Snapshot(ctx, coretelemetry.Query{
			Since:  time.Now().UTC().Add(-window),
			TwinID: twinID,
		})
		if err != nil {
			return environment.ActionResult{Success: false, Error: err.Error()}, nil
		}
		state := snapshotState(snap)
		state["capability"] = string(CapAnalyseUsage)
		state["window"] = window.String()
		return environment.ActionResult{Success: true, StateDelta: state}, nil
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
	case CapProposeRoadmap:
		return recordDraft(a, "problem")
	case CapDraftADR:
		return recordDraft(a, "decision")
	}

	return environment.ActionResult{
		Success: false,
		Error:   fmt.Sprintf("%s is read-only; use the software pack to change the repository (%s)", EnvRepo, a.CapabilityID),
	}, nil
}

// recordDraft accepts a drafted proposal and carries it, refusing one that is
// missing the input its capability declares as required.
//
// The agent is the drafter — the text arrives in the action's params, and
// this environment neither writes it to the repository nor generates it. What
// it must not do is report a draft that is not there: a capability that
// returns success for empty input feeds a 100% success rate into procedural
// memory, which biases the next plan's confidence *upward* for having
// produced nothing. That is the silently-succeeding no-op this codebase has
// already had to fix in the act and verify steps.
//
// Note that the declared OutputSchema (title, goal, steps / title, body,
// consequences) is not produced here, because turning a problem statement
// into a phase is model work and nothing calls a model inside Act. Recording
// what actually arrived is the honest half; closing that gap is Phase 25's.
func recordDraft(a environment.Action, required string) (environment.ActionResult, error) {
	text, _ := a.Params[required].(string)
	if strings.TrimSpace(text) == "" {
		return environment.ActionResult{
			Success: false,
			Error: fmt.Sprintf("%s requires a non-empty %q; a proposal without one is not a proposal",
				a.CapabilityID, required),
		}, nil
	}

	// Copied rather than aliased: stepAct writes into the params map for some
	// capabilities and persists it beside this result, so holding a reference
	// would let a later write retroactively edit the recorded draft.
	recorded := make(map[string]any, len(a.Params))
	for k, v := range a.Params {
		recorded[k] = v
	}
	return environment.ActionResult{
		Success: true,
		StateDelta: map[string]any{
			"capability": string(a.CapabilityID),
			"recorded":   true,
			"draft":      recorded,
			"note":       "carried for review; writing it into the repository is the software pack's job",
		},
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
