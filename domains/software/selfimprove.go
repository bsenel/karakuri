package software

// Self-improvement: reading a deployment's own behaviour, and drafting the
// work that follows from it.
//
// This lived in a separate `karakuri` domain pack until ADR 018. It is here
// now because the split did not earn its keep: drafting an ADR and proposing
// a roadmap phase are software practices rather than platform ones, the
// pack's repository environment duplicated software.env.git, and the pack
// boundary was never the thing enforcing "the agent that proposes cannot
// write" — the agent's own authority bounds are.
//
// What remains genuinely platform-specific is the telemetry environment: its
// subject is the deployment running the loop rather than the codebase the
// loop is working on. It is built only where a reader is wired, which is
// gating finer than the pack-level enable flag it replaces.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	coretelemetry "github.com/bsenel/karakuri/internal/core/telemetry"
	"github.com/bsenel/karakuri/internal/core/vfs"
)

// EnvGit is the software pack's existing repository environment. The
// drafting capabilities are served here rather than by a second environment
// wrapping the same VersionControlAdapter, which is what the karakuri pack's
// repo environment was.
const EnvGit = "software.env.git"

const (
	// EnvPlatformTelemetry is this deployment watching itself. Named for its
	// subject: every other software environment is about the codebase being
	// worked on, and this one is about the platform doing the working.
	EnvPlatformTelemetry = "software.env.platform_telemetry"

	// CapAnalyseUsage reads the deployment's telemetry and ranks what limits
	// it. Platform-specific, and the reason this file exists at all.
	CapAnalyseUsage = "software.reason.analyse_usage"
	// CapProposeRoadmap and CapDraftADR draft; they change nothing. Generic
	// software practices, which is why they are plain software capabilities
	// rather than platform ones.
	CapProposeRoadmap = "software.act.propose_roadmap_phase"
	CapDraftADR       = "software.act.draft_adr"
)

// telemetryWindow is how far back the telemetry environment looks.
//
// A week rather than a day: the questions this asks — what keeps failing,
// what is nobody deciding, where is the money going — are answered badly by a
// single day's noise and well by a week's shape.
const telemetryWindow = 7 * 24 * time.Hour

// servedBy answers which environment executes a capability, by reading the
// Serves declarations the loop itself routes on.
//
// It used to be a hand-written map beside them — a second copy of the routing,
// kept honest only by a test remembering to check it. Now the declaration and
// the route are the same fact: Phase 26 promoted Serves onto
// environment.Factory and stepAct resolves through it, so a capability listed
// here wrong is a capability that misroutes in production, not one that fails
// a mirror test. The executing test above still runs each capability where
// this says it lives, which is what catches a route pointing at an environment
// whose Act refuses it.
func servedBy(capID capability.CapabilityID) (environment.EnvironmentID, bool) {
	for _, f := range New().EnvironmentFactories() {
		for _, served := range f.Serves {
			if served == capID {
				return f.EnvID, true
			}
		}
	}
	return "", false
}

type telemetryEnv struct {
	reader coretelemetry.Reader
	twinID string
}

func (e *telemetryEnv) ID() environment.EnvironmentID { return EnvPlatformTelemetry }
func (e *telemetryEnv) Domain() string                { return "software" }

func (e *telemetryEnv) Observe(ctx context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	obs := environment.Observation{EnvID: EnvPlatformTelemetry, Timestamp: time.Now().UTC()}
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
		// The grade beside the boolean, because "thin" is the answer the
		// boolean cannot give and the one a proposal most needs to disclose.
		"evidence": evidenceLevel(snap),
	}
}

// Evidence grades.
//
// Three rather than two, because "I have none", "I have a little" and "I have
// enough" are three different claims and a proposal drawn from each deserves a
// different amount of belief. A boolean forced the middle one into whichever
// neighbour the author picked, and the author picked "enough".
const (
	EvidenceNone     = "none"
	EvidenceThin     = "thin"
	EvidenceAdequate = "adequate"
)

// minPattern is how many observations of one kind make a pattern rather than
// noise.
//
// Not chosen here: it is the threshold the telemetry reader already applies
// before it will call a capability failing (`if n < 3 { continue }` in
// readExecutes). Using a different number would mean the pack calls evidence
// adequate that the reader itself declines to draw a conclusion from, or the
// reverse. One justified number, used twice.
const minPattern = 3

// evidenceLevel grades what the window holds.
//
// The previous version was a boolean whose test was "the window contains
// anything at all", so one sense pass in a week counted the same as a
// thousand. That is the shape of judgement this pack exists to avoid making:
// a deployment three hours old would report sufficient evidence and propose
// roadmap phases from a single data point.
//
// Only window-scoped terms count. An earlier version ORed in
// Objectives.Total, which the reader computes with no time bound at all — so
// it was true in any deployment that had ever created an objective, including
// the self-improvement objective doing the asking. A flag that cannot be false
// in production is worse than no flag: it answers the question it was added to
// answer, wrongly, every time.
func evidenceLevel(snap coretelemetry.Snapshot) string {
	// A bottleneck is already a conclusion the reader was willing to draw, and
	// it only draws one from a repeated failure, a blocked objective or a
	// decision left waiting. Its presence is adequate evidence by
	// construction.
	if len(snap.Bottlenecks) > 0 {
		return EvidenceAdequate
	}

	counts := []int{
		snap.Work.Senses,
		snap.Work.Reconciles,
		snap.Work.Actions,
		snap.Work.Failures,
		snap.Escalation.Escalations,
	}
	total := 0
	for _, n := range counts {
		total += n
		if n >= minPattern {
			return EvidenceAdequate
		}
	}
	if total > 0 {
		return EvidenceThin
	}
	return EvidenceNone
}

// sufficient is the boolean the rest of the pack still asks for: adequate
// evidence, not merely some. Thin evidence is reported as thin and does not
// pass for sufficient — a proposal may still be drafted from it, and must say
// what it is standing on.
func sufficient(snap coretelemetry.Snapshot) bool {
	return evidenceLevel(snap) == EvidenceAdequate
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
					"evidence":   EvidenceNone,
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
		Error:   fmt.Sprintf("%s is read-only; %s cannot be executed here", EnvPlatformTelemetry, a.CapabilityID),
	}, nil
}

func (e *telemetryEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return nil, nil
}

func (e *telemetryEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	snap := environment.EnvironmentSnapshot{EnvID: EnvPlatformTelemetry, Timestamp: time.Now().UTC()}
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

// selfImproveCapabilities are the three this file adds to the software pack.
func selfImproveCapabilities() []capability.Capability {
	prop := func(t, d string) capability.SchemaProperty {
		return capability.SchemaProperty{Type: t, Description: d}
	}
	return []capability.Capability{
		{
			ID:          CapAnalyseUsage,
			Name:        "Analyse Usage",
			Domain:      "software",
			Description: "Read this deployment's telemetry and name what is limiting it, ranked",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"window":  prop("string", "How far back to look, e.g. 168h. Defaults to a week"),
					"twin_id": prop("string", "Narrow to one twin. Never widens beyond the environment's own twin"),
				},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"bottlenecks":   prop("array", "What is going wrong, ranked by how often"),
					"approval_rate": prop("number", "Share of resolved escalations approved; -1 when nothing was decided"),
					"sufficient":    prop("boolean", "Whether the window held enough to reason from: true only at evidence=adequate"),
					"evidence":      prop("string", "How much the window held: none, thin, or adequate. A proposal must say which it stood on."),
				},
			},
			Verifiable: true,
		},
		{
			ID:          CapProposeRoadmap,
			Name:        "Propose Roadmap Phase",
			Domain:      "software",
			Description: "Draft a roadmap phase in the repository's established style, from evidence rather than from taste",
			InputSchema: capability.Schema{
				Type:     "object",
				Required: []string{"problem"},
				Properties: map[string]capability.SchemaProperty{
					"problem":  prop("string", "The limitation this phase would remove, in one sentence"),
					"evidence": prop("string", "What says it is real — a phase proposed without this is a preference"),
					"scope":    prop("string", "What is in, and explicitly what is out"),
				},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"recorded": prop("boolean", "Whether a draft was accepted"),
					"draft":    prop("object", "The drafted fields, as supplied"),
					"note":     prop("string", "What happens to the draft next"),
				},
			},
			Verifiable: true,
		},
		{
			ID:          CapDraftADR,
			Name:        "Draft ADR",
			Domain:      "software",
			Description: "Draft an architecture decision record: the decision, its consequences, and what was rejected",
			InputSchema: capability.Schema{
				Type:     "object",
				Required: []string{"decision"},
				Properties: map[string]capability.SchemaProperty{
					"decision":     prop("string", "The decision, stated as a claim rather than a topic"),
					"context":      prop("string", "What made it necessary"),
					"alternatives": prop("string", "What else was considered, and why it lost"),
				},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"recorded": prop("boolean", "Whether a draft was accepted"),
					"draft":    prop("object", "The drafted fields, as supplied"),
					"note":     prop("string", "What happens to the draft next"),
				},
			},
			Verifiable: true,
		},
	}
}

// mutatingCapabilities are the software capabilities that change a repository
// or run commands against one.
//
// Named explicitly rather than matched on a substring like ".act.", because
// drafting is an act that changes nothing and the distinction that matters is
// what a capability *does*. This is the set the maintainer must hold none of,
// asserted by a test — the assertion that replaces the pack-boundary one.
func mutatingCapabilities() []capability.CapabilityID {
	return []capability.CapabilityID{
		"software.act.write_code",
		"software.act.write_test",
		"software.act.create_pr",
		"software.act.shell_exec",
		"software.act.delegate_to_cli",
	}
}

// selfImproveAgents are the two agents that reason about the platform.
func selfImproveAgents() []agent.Definition {
	return []agent.Definition{
		{
			ID:     "software.agent.maintainer",
			Name:   "Platform Maintainer",
			Domain: "software",
			Capabilities: []capability.CapabilityID{
				CapAnalyseUsage, CapProposeRoadmap, CapDraftADR,
			},
			// Reflexion, because this agent's output is a proposal somebody
			// will read as evidence-backed, and a critique-and-revise pass is
			// the cheapest available defence against a confident paragraph
			// with nothing behind it.
			ReasoningStrategy: agent.ReasoningReflexion,
			Authority: agent.AuthorityBounds{
				// Zero, and deliberately not a small number: every action it
				// plans escalates, whatever a standing objective's autonomy
				// level says. The objective's ceiling bounds how far it may
				// be promoted; this bounds what promotion can ever mean.
				//
				// This is the guarantee the karakuri pack boundary was
				// credited with and never actually provided — a pack is a
				// namespace, and stepAct resolves environments across every
				// domain an objective names.
				MaxAutonomousActions: 0,
				RequiresApprovalFor:  []capability.CapabilityID{CapProposeRoadmap, CapDraftADR},
				CanDelegate:          false,
				CanModifyObjective:   false,
				ConfidenceThreshold:  1.0,
			},
			Memory: agent.MemoryConfig{SemanticEnabled: true, ProceduralEnabled: true},
		},
		{
			ID:     "software.agent.analyst",
			Name:   "Platform Analyst",
			Domain: "software",
			// Reading only. Separate from the maintainer so an operator can
			// run "tell me what is limiting this deployment" at sense
			// autonomy without ever putting an agent that drafts changes into
			// the rotation.
			Capabilities:      []capability.CapabilityID{CapAnalyseUsage},
			ReasoningStrategy: agent.ReasoningChainOfThought,
			Authority: agent.AuthorityBounds{
				MaxAutonomousActions: 1,
				CanDelegate:          false,
				CanModifyObjective:   false,
				ConfidenceThreshold:  0.7,
			},
			Memory: agent.MemoryConfig{SemanticEnabled: true},
		},
	}
}

// selfImproveTemplates are the two objectives this file adds.
//
// Neither declares Criterion.Domain any more. The cross-domain reference was
// the whole reason self_improve's pull-request criterion could name a
// capability nothing exported and have nothing catch it: the conformance
// suite deliberately does not resolve foreign domains. Same-pack verifiers
// are checked.
func selfImproveTemplates() []objective.Template {
	// crit declares a criterion settled deterministically by a capability's
	// result. Use it only where the capability succeeding *is* the criterion
	// being met — create_pr succeeding means a pull request is open.
	crit := func(id, desc, verifier string, weight float64) objective.Criterion {
		return objective.Criterion{
			ID: id, Description: desc,
			Verifier: capability.CapabilityID(verifier), Weight: weight,
		}
	}

	// judged declares a criterion decided by reading what the actions
	// produced, because no capability's success answers it.
	//
	// The distinction was invisible until the verify step started reading
	// action results (Phase 25 step 6). Before that every criterion was
	// effectively judged by a model shown nothing, so naming a verifier that
	// could not decide the question cost nothing and four of them did it: the
	// two below and "the proposal names the telemetry" all named
	// analyse_usage, which produces the evidence rather than deciding the
	// question. Once a verifier settles a criterion deterministically, that
	// spelling means "this criterion is met whenever the analysis ran" — which
	// is every time, including on a deployment with no telemetry at all.
	//
	// The rule: a verifier answers the criterion, it does not supply the
	// material for answering it.
	judged := func(id, desc string, weight float64) objective.Criterion {
		return objective.Criterion{ID: id, Description: desc, Weight: weight}
	}
	hard := func(id, desc, expr string) objective.Constraint {
		return objective.Constraint{ID: id, Description: desc, Hard: true, Expression: expr}
	}
	return []objective.Template{
		{
			ID:     "software.objective.watch_platform_health",
			Title:  "Watch this deployment's health",
			Domain: "software",
			// Named, because selection otherwise takes the first agent the
			// pack declares — the strategist, which is not this.
			SuggestedAgents: []agent.Definition{{ID: "software.agent.analyst"}},
			Description: "Read the deployment's own telemetry and report what is limiting it. " +
				"Reads only — declare it standing at sense autonomy and it will never spend a model call on a quiet week.",
			SuccessCriteria: []objective.Criterion{
				judged("no-blocked", "The telemetry shows no standing objective blocked by the breaker or the stall detector", 0.5),
				judged("no-stale-decisions", "The telemetry shows no checkpoint waiting more than a day", 0.5),
			},
			Constraints: []objective.Constraint{
				hard("read-only", "This objective observes and reports; it must not act on the deployment", "no_write_capabilities"),
			},
		},
		{
			ID:     "software.objective.self_improve",
			Title:  "Improve this deployment from its own evidence",
			Domain: "software",
			// The maintainer, whose bounds are the ones this template's
			// safety story rests on. Before SuggestedAgents was read, this
			// ran under the strategist and the guarantee held by luck.
			SuggestedAgents: []agent.Definition{{ID: "software.agent.maintainer"}},
			Description: "Analyse telemetry, decide what is worth changing, and open a pull request that changes it. " +
				"The maintainer analyses and drafts; the writing capabilities belong to other agents in this pack, " +
				"so the change still arrives as a pull request somebody reviews.",
			SuccessCriteria: []objective.Criterion{
				judged("evidence", "The proposal names the specific telemetry that says the problem is real, and the analysis reported evidence adequate to support it", 0.3),
				judged("proposal", "A roadmap phase was drafted in the repository's established style", 0.3),
				// Same pack now, so the conformance suite resolves it. It
				// previously named software.act.open_pull_request, which
				// nothing exports, and being cross-domain is what stopped
				// anything noticing.
				crit("pull-request", "A pull request is open with the change and its tests", "software.act.create_pr", 0.4),
			},
			Constraints: []objective.Constraint{
				hard("evidence-first", "No proposal may be drafted before analyse_usage has run", "analysis_complete"),
				hard("human-approves", "Every change to the repository requires explicit approval", "change_approved"),
				hard("respect-repo-rules", "Changes must follow AGENTS.md: clean-architecture boundaries, tests for non-trivial logic, docs updated", "repo_rules_followed"),
			},
		},
	}
}

// platformTelemetryFactory builds the environment; the wired reader is what
// gates it.
//
// An earlier version refused to build without a reader, on the theory that a
// deployment which has not opted in should not get the environment at all.
// The conformance suite caught it: a declared factory must be constructible,
// and every other adapter-backed environment in this pack builds and degrades
// honestly rather than failing construction. Building and reporting
// `available: false` is both the convention here and enough for the property
// that mattered — an unwired deployment learns nothing about the platform,
// because there is nothing behind the port to learn it from.
func platformTelemetryFactory() environment.Factory {
	return environment.Factory{
		EnvID:       EnvPlatformTelemetry,
		Domain:      "software",
		Description: "This deployment's own behaviour: escalation rates, spend, failing objectives, unanswered decisions",
		Serves:      []capability.CapabilityID{CapAnalyseUsage},
		Build: func(bc environment.BuildContext) (environment.Environment, error) {
			// A nil reader is the ordinary case for a deployment that has not
			// wired telemetry. The environment says so rather than reporting
			// zeroes that read as a healthy system.
			return &telemetryEnv{reader: bc.Telemetry, twinID: bc.TwinID}, nil
		},
	}
}
