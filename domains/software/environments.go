package software

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/cliagent"
	"github.com/bsenel/karakuri/internal/platform/tools/messaging"
	"github.com/bsenel/karakuri/internal/platform/tools/observability"
	"github.com/bsenel/karakuri/internal/platform/tools/projectmgmt"
	"github.com/bsenel/karakuri/internal/platform/tools/research"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// softwareEnvironmentFactories builds the six software environments. The Git,
// Ticket, and Communication envs dispatch to the tools.Registry adapters
// (GitHub / Linear / Slack when configured). The observability env reads the
// twin's bound observability instance and reports itself blind without one;
// only the CI env is still a no-op.
// reg may be nil — in that case every adapter-backed env falls back to no-op
// behavior, except observability, which has no no-op to fall back on.
func softwareEnvironmentFactories(reg *tools.Registry) []environment.Factory {
	noopFactory := func(id, desc string) environment.Factory {
		return environment.Factory{
			EnvID:       environment.EnvironmentID(id),
			Domain:      "software",
			Description: desc,
			Build: func(_ environment.BuildContext) (environment.Environment, error) {
				return &noopEnv{id: environment.EnvironmentID(id)}, nil
			},
		}
	}
	return []environment.Factory{
		{
			EnvID:       EnvGit,
			Domain:      "software",
			Description: "Git repository: commits, branches, PRs, worktrees, diffs",
			Serves: []capability.CapabilityID{
				"software.act.create_pr",
				// Drafting lives here because a roadmap phase and an ADR are
				// files in this repository, and because gitEnv answers both
				// before its adapter check — a deployment with no version
				// control wired can still draft.
				CapProposeRoadmap,
				CapDraftADR,
				"software.act.write_design_doc",
				// The reads gitEnv.Observe already makes, asked for by name:
				// the adapter that holds the commits and PRs is bound here.
				"software.observe.fetch_commits",
				"software.observe.fetch_prs",
			},
			Build: func(ctx environment.BuildContext) (environment.Environment, error) {
				var vc versioncontrol.VersionControlAdapter = versioncontrol.NewNoOp()
				if reg != nil {
					if a, ok := reg.VC.Resolve(ctx.AdapterBindings["versioncontrol"]); ok {
						vc = a
					}
				}
				return &gitEnv{id: EnvGit, vc: vc}, nil
			},
		},
		{
			EnvID:       "software.env.ticket",
			Domain:      "software",
			Description: "Project management: issues, epics, sprints",
			Serves:      []capability.CapabilityID{"software.act.create_ticket"},
			Build: func(ctx environment.BuildContext) (environment.Environment, error) {
				var pm projectmgmt.ProjectManagementAdapter = projectmgmt.NewNoOp()
				if reg != nil {
					if a, ok := reg.ProjectMgmt.Resolve(ctx.AdapterBindings["projectmgmt"]); ok {
						pm = a
					}
				}
				return &ticketEnv{id: "software.env.ticket", pm: pm}, nil
			},
		},
		{
			EnvID:       "software.env.communication",
			Domain:      "software",
			Description: "Team signals: messages, threads, mentions",
			Serves:      []capability.CapabilityID{"software.act.send_message"},
			Build: func(ctx environment.BuildContext) (environment.Environment, error) {
				var msg messaging.MessagingAdapter = messaging.NewNoOp()
				if reg != nil {
					if a, ok := reg.Messaging.Resolve(ctx.AdapterBindings["messaging"]); ok {
						msg = a
					}
				}
				return &commsEnv{id: "software.env.communication", msg: msg}, nil
			},
		},
		{
			EnvID:       "software.env.cli_agent",
			Domain:      "software",
			Description: "Coding-agent CLI delegate (Claude Code, Cursor, Gemini, Copilot)",
			// The three capabilities that write source. This is the routing
			// the planner hint used to describe and could not enforce.
			Serves: []capability.CapabilityID{
				"software.act.delegate_to_cli",
				"software.act.write_code",
				"software.act.write_test",
			},
			Build: func(ctx environment.BuildContext) (environment.Environment, error) {
				var cli cliagent.CLIAgentAdapter = cliagent.NewNoOp()
				if reg != nil {
					if a, ok := reg.CLIAgents.Resolve(ctx.AdapterBindings["cli_agents"]); ok {
						cli = a
					}
				}
				env := &cliEnv{id: "software.env.cli_agent", cli: cli, twinID: ctx.TwinID}
				if reg != nil {
					env.attachMCP, env.mcpURL = cliMCPOptions(reg.CLIAgents.Options(ctx.AdapterBindings["cli_agents"]))
				}
				return env, nil
			},
		},
		noopFactory("software.env.ci", "CI pipeline: build status, test results, coverage"),
		{
			EnvID:       "software.env.observability",
			Domain:      "software",
			Description: "Runtime: logs, metrics, alerts",
			Serves: []capability.CapabilityID{
				CapFetchLogs,
				CapFetchMetrics,
				CapAlertsResolved,
			},
			Build: func(ctx environment.BuildContext) (environment.Environment, error) {
				// Left a true nil when nothing resolves: there is no no-op
				// adapter, and the env says it is blind rather than quiet.
				var adapter observability.ObservabilityAdapter
				if reg != nil {
					if a, ok := reg.Observability.Resolve(ctx.AdapterBindings["observability"]); ok {
						adapter = a
					}
				}
				return newObservabilityEnv("software.env.observability", adapter), nil
			},
		},
		{
			EnvID:       "software.env.research",
			Domain:      "software",
			Description: "External sources: what the field has published on a topic, ranked by confidence",
			Serves:      []capability.CapabilityID{CapResearch},
			Build: func(_ environment.BuildContext) (environment.Environment, error) {
				// Single-instance slot, so no adapter binding to resolve. The
				// capability has been declared since Phase 2 and the adapter
				// built since Phase 6; nothing had introduced them.
				var rs research.ResearchAdapter
				if reg != nil {
					rs = reg.Research
				}
				return newResearchEnv("software.env.research", rs), nil
			},
		},
		{
			EnvID:       "software.env.codebase",
			Domain:      "software",
			Description: "The repository as evidence: the roadmap's own deferred work, TODO density by package, packages with no tests, and where AGENTS.md rules live",
			Serves:      []capability.CapabilityID{CapAnalyseRepo, "software.observe.read_codebase"},
			Build: func(_ environment.BuildContext) (environment.Environment, error) {
				// Root defaults to the server's working directory, like
				// shellEnv. Declared since Phase 2 and a noop until Phase 25.
				return newCodebaseEnv("software.env.codebase", ""), nil
			},
		},
		{
			EnvID:  "software.env.verify",
			Domain: "software",
			Description: "Verification commands: runs a test suite or a linter and reports the verdict by exit code. " +
				"Takes params.cmd (required) and optional params.workdir, params.timeout_sec (max 600).",
			// Declared since Phase 2 on four criteria of the delivery
			// template and served by nothing until now. A criterion naming a
			// verifier that never runs does not fail — verify.go falls back to
			// asking a model, so the criterion silently cost a judgement call
			// on every iteration while claiming to be settled by a test run.
			Serves: []capability.CapabilityID{
				"software.verify.run_tests",
				"software.verify.lint",
			},
			Build: func(_ environment.BuildContext) (environment.Environment, error) {
				return newVerifyEnv("software.env.verify", newShellEnv("software.env.verify", "", 600*time.Second)), nil
			},
		},
		{
			EnvID:       "software.env.shell",
			Domain:      "software",
			Description: "Local shell executor (/bin/sh) — runs software.act.shell_exec actions with timeout, output capture, and safety guardrails. Defaults to the server's CWD; configurable per-deployment.",
			Serves:      []capability.CapabilityID{"software.act.shell_exec"},
			Build: func(_ environment.BuildContext) (environment.Environment, error) {
				return newShellEnv("software.env.shell", "", 60*time.Second), nil
			},
		},
		{
			EnvID:  "software.env.remediation",
			Domain: "software",
			Description: "Remediation commands: runs a command that changes a running system, for one observed alert and a stated reason. " +
				"Takes params.alert_id, params.rationale and params.cmd (all required) and optional params.workdir, params.timeout_sec (max 600).",
			Serves: []capability.CapabilityID{CapRunRemediation},
			Build: func(_ environment.BuildContext) (environment.Environment, error) {
				return newRemediationEnv("software.env.remediation", newShellEnv("software.env.remediation", "", 60*time.Second)), nil
			},
		},
	}
}

// ── noopEnv ──────────────────────────────────────────────────────────────────

type noopEnv struct {
	id environment.EnvironmentID
}

func (e *noopEnv) ID() environment.EnvironmentID { return e.id }
func (e *noopEnv) Domain() string                { return "software" }

func (e *noopEnv) Observe(_ context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{
		EnvID: e.id, State: map[string]any{"status": "noop"},
		Version: "noop-0", Timestamp: time.Now().UTC(),
	}, nil
}

// Act returns a failure for any capability invocation — the noop env
// has no implementation. Before this change the noop env returned
// Success=true, which silently masked the absence of real tool
// adapters and let downstream verify steps trivially pass with a
// score of 1.0. Honest failure: operators see the gap and either
// register an adapter or accept that the action was a no-op.
func (e *noopEnv) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{
		Success: false,
		Error:   fmt.Sprintf("capability %q has no implementation: software noop environment cannot execute", a.CapabilityID),
		StateDelta: map[string]any{
			"action": string(a.CapabilityID),
			"status": "unimplemented",
			"env_id": string(e.id),
		},
	}, nil
}

func (e *noopEnv) Subscribe(_ context.Context, _ environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	ch := make(chan environment.EnvironmentEvent)
	return ch, nil
}

func (e *noopEnv) Snapshot(_ context.Context) (environment.EnvironmentSnapshot, error) {
	return environment.EnvironmentSnapshot{
		SHA: "noop-snapshot", EnvID: e.id,
		State: map[string]any{"status": "noop"}, Timestamp: time.Now().UTC(),
	}, nil
}

// ── gitEnv (versioncontrol adapter) ──────────────────────────────────────────

type gitEnv struct {
	id environment.EnvironmentID
	vc versioncontrol.VersionControlAdapter
}

// gitObservationWindow is how far back the repository observation reaches.
//
// A week, matching telemetryWindow: the questions an observation answers —
// what has been landing, what is open, what is red — are answered badly by a
// single day and well by a week, and anything older is history rather than
// state.
const gitObservationWindow = 7 * 24 * time.Hour

// Bounds on params.since_days for the fetch_commits and fetch_prs reads. The
// default is the observation window; the cap keeps one read from pulling a
// quarter of history into a single result.
const (
	gitReadDefaultSinceDays = 7
	gitReadMaxSinceDays     = 90
)

// gitReadParams turns params.repo and params.since_days into what the adapter
// is asked for. An empty repo is the adapter's default, as in Observe.
func gitReadParams(params map[string]any) (repo string, since time.Time) {
	days := positiveIntParam(params, "since_days", gitReadDefaultSinceDays)
	if days > gitReadMaxSinceDays {
		days = gitReadMaxSinceDays
	}
	return asString(params, "repo"), time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
}

// prTrust is the provenance of a list of pull requests: third party when any
// of them was opened by a login the operator has not listed as the
// deployment's own (EB-001). With no list configured nothing is the
// deployment's own, so any pull request makes the payload third party — the
// behaviour before the list existed. The deployment's own pull requests were
// escalating almost every plan, which made the reason say nothing.
func prTrust(prs []versioncontrol.PRSummary) environment.Trust {
	for _, pr := range prs {
		if !pr.OwnAuthor {
			return environment.TrustThirdParty
		}
	}
	return environment.TrustOperator
}

func (e *gitEnv) ID() environment.EnvironmentID { return e.id }
func (e *gitEnv) Domain() string                { return "software" }

func (e *gitEnv) Observe(ctx context.Context, q environment.ObservationQuery) (environment.Observation, error) {
	adapter := e.vc
	if adapter == nil || !adapter.Active() {
		return noopObservation(e.id), nil
	}
	repo, _ := q.Filter["repo"].(string)
	// Bounded, because every byte of this lands in the reason prompt of every
	// iteration of every loop watching this repository. An unfiltered read
	// returned the adapter's whole first page — fifty commits of JSON — and
	// re-serialised it each time, which is a per-iteration token cost that
	// grows with nothing the operator can see.
	since := time.Now().UTC().Add(-gitObservationWindow)
	state := map[string]any{"adapter": adapter.Name(), "window": gitObservationWindow.String()}
	// Third party only when a pull request by somebody other than the
	// deployment's own logins is carried (prTrust). Commits and an adapter name
	// are the operator's own infrastructure; a PR title is typed by whoever
	// opened it, which on a public repository is anybody. Computed
	// from what the payload ends up holding rather than fixed on the
	// environment, because a repository with no open PRs should not escalate
	// every plan in the deployment for the rest of the run.
	trust := environment.TrustOperator
	commits, err := adapter.GetCommits(ctx, repo, since)
	if err != nil {
		state["commits_error"] = err.Error()
	} else {
		state["commits"] = commits
	}
	prs, err := adapter.ListPRs(ctx, repo, since)
	if err != nil {
		state["prs_error"] = err.Error()
	} else {
		state["prs"] = prs
		trust = prTrust(prs)
		// What is currently broken, pulled out of the list rather than left
		// for a reader to derive. Deferred from Phase 22, where "an operator
		// relays it" was the plan; a pack proposing work from evidence should
		// be able to see a red build without being told.
		var broken []map[string]any
		for _, pr := range prs {
			if pr.CheckState != "failure" {
				continue
			}
			broken = append(broken, map[string]any{
				"pr": pr.ID, "title": pr.Title, "url": pr.URL, "failing": pr.FailingChecks,
			})
		}
		state["failing_prs"] = broken
	}
	return environment.Observation{
		EnvID: e.id, State: state, Version: stateVersion(state),
		Timestamp: time.Now().UTC(), Trust: trust,
	}, nil
}

func (e *gitEnv) Act(ctx context.Context, a environment.Action) (environment.ActionResult, error) {
	// Drafting needs no adapter and touches no repository, so it is answered
	// before the adapter check: a deployment with no version control wired can
	// still draft a roadmap phase for a human to read. Checking the adapter
	// first would route these to noopAct and report a draft that never
	// happened.
	switch a.CapabilityID {
	case CapProposeRoadmap:
		return recordDraft(a, "problem")
	case CapDraftADR:
		return recordDraft(a, "decision")
	case "software.act.write_design_doc":
		// Declared in Phase 2, on two agents' capability lists, and required
		// by a priority-9 hint before any write_code action — and served by
		// nothing until Phase 26, so every plan that obeyed the hint got "no
		// active adapter" for the step the hint made mandatory. It is a draft
		// like the two above and is recorded the same way.
		return recordDraft(a, "design")
	}

	adapter := e.vc
	if adapter == nil || !adapter.Active() {
		switch string(a.CapabilityID) {
		case "software.observe.fetch_commits", "software.observe.fetch_prs":
			// A read with nothing to read from is a failure, not an empty
			// list: "no commits this week" and "no repository" are different
			// facts, and noopAct would report the first.
			return environment.ActionResult{Success: false,
				Error: fmt.Sprintf("%s: no version control instance is bound", a.CapabilityID)}, nil
		}
		return noopAct(a), nil
	}
	switch string(a.CapabilityID) {
	case "software.observe.fetch_commits":
		repo, since := gitReadParams(a.Params)
		commits, err := adapter.GetCommits(ctx, repo, since)
		if err != nil {
			return environment.ActionResult{Success: false, Error: err.Error(),
				StateDelta: map[string]any{"adapter": adapter.Name()}}, nil
		}
		// Commits are the operator's own infrastructure, as in Observe.
		return environment.ActionResult{Success: true, Trust: environment.TrustOperator,
			StateDelta: map[string]any{"commits": commits, "count": len(commits)}}, nil
	case "software.observe.fetch_prs":
		repo, since := gitReadParams(a.Params)
		prs, err := adapter.ListPRs(ctx, repo, since)
		if err != nil {
			return environment.ActionResult{Success: false, Error: err.Error(),
				StateDelta: map[string]any{"adapter": adapter.Name()}}, nil
		}
		// Same rule as Observe: a PR title is typed by whoever opened it.
		return environment.ActionResult{Success: true, Trust: prTrust(prs),
			StateDelta: map[string]any{"prs": prs, "count": len(prs)}}, nil
	case "software.act.create_pr":
		pr := versioncontrol.PullRequest{
			Title:        asString(a.Params, "title"),
			Body:         asString(a.Params, "body"),
			HeadBranch:   asString(a.Params, "branch"),
			BaseBranch:   asString(a.Params, "base_branch"),
			WorktreePath: asString(a.Params, "worktree_path"),
		}
		if pr.BaseBranch == "" {
			pr.BaseBranch = "main"
		}
		url, err := adapter.CreatePR(ctx, pr)
		if err != nil {
			return environment.ActionResult{Success: false, Error: err.Error(),
				StateDelta: map[string]any{"adapter": adapter.Name()}}, nil
		}
		return environment.ActionResult{Success: true,
			StateDelta: map[string]any{"adapter": adapter.Name(), "pr_url": url}}, nil
	default:
		// Capability not handled by this env — return noop success so the loop continues.
		return noopAct(a), nil
	}
}

func (e *gitEnv) Subscribe(_ context.Context, _ environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	ch := make(chan environment.EnvironmentEvent)
	return ch, nil
}

func (e *gitEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	obs, _ := e.Observe(ctx, environment.ObservationQuery{})
	return environment.EnvironmentSnapshot{SHA: obs.Version, EnvID: e.id, State: obs.State, Timestamp: obs.Timestamp}, nil
}

// ── ticketEnv (projectmgmt adapter) ──────────────────────────────────────────

type ticketEnv struct {
	id environment.EnvironmentID
	pm projectmgmt.ProjectManagementAdapter
}

func (e *ticketEnv) ID() environment.EnvironmentID { return e.id }
func (e *ticketEnv) Domain() string                { return "software" }

func (e *ticketEnv) Observe(ctx context.Context, q environment.ObservationQuery) (environment.Observation, error) {
	adapter := e.pm
	if adapter == nil || !adapter.Active() {
		return noopObservation(e.id), nil
	}
	state := map[string]any{"adapter": adapter.Name()}
	// A ticket's title and body are written by whoever filed it. The adapter
	// name on its own is not.
	trust := environment.TrustOperator
	if id, ok := q.Filter["ticket_id"].(string); ok && id != "" {
		ticket, err := adapter.GetTicket(ctx, id)
		if err != nil {
			state["error"] = err.Error()
		} else {
			state["ticket"] = ticket
			trust = environment.TrustThirdParty
		}
	}
	return environment.Observation{
		EnvID: e.id, State: state, Version: stateVersion(state),
		Timestamp: time.Now().UTC(), Trust: trust,
	}, nil
}

func (e *ticketEnv) Act(ctx context.Context, a environment.Action) (environment.ActionResult, error) {
	adapter := e.pm
	if adapter == nil || !adapter.Active() {
		return noopAct(a), nil
	}
	switch string(a.CapabilityID) {
	case "software.act.create_ticket":
		ticket := projectmgmt.Ticket{
			Title: asString(a.Params, "title"),
			Body:  asString(a.Params, "body"),
		}
		id, err := adapter.CreateTicket(ctx, ticket)
		if err != nil {
			return environment.ActionResult{Success: false, Error: err.Error(),
				StateDelta: map[string]any{"adapter": adapter.Name()}}, nil
		}
		return environment.ActionResult{Success: true,
			StateDelta: map[string]any{"adapter": adapter.Name(), "ticket_id": id}}, nil
	default:
		return noopAct(a), nil
	}
}

func (e *ticketEnv) Subscribe(_ context.Context, _ environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	ch := make(chan environment.EnvironmentEvent)
	return ch, nil
}

func (e *ticketEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	obs, _ := e.Observe(ctx, environment.ObservationQuery{})
	return environment.EnvironmentSnapshot{SHA: obs.Version, EnvID: e.id, State: obs.State, Timestamp: obs.Timestamp}, nil
}

// ── commsEnv (messaging adapter) ─────────────────────────────────────────────

type commsEnv struct {
	id  environment.EnvironmentID
	msg messaging.MessagingAdapter
}

func (e *commsEnv) ID() environment.EnvironmentID { return e.id }
func (e *commsEnv) Domain() string                { return "software" }

func (e *commsEnv) Observe(ctx context.Context, q environment.ObservationQuery) (environment.Observation, error) {
	adapter := e.msg
	if adapter == nil || !adapter.Active() {
		return noopObservation(e.id), nil
	}
	channel, _ := q.Filter["channel"].(string)
	state := map[string]any{"adapter": adapter.Name()}
	// Message bodies are the widest untrusted surface this environment has, and
	// today stepObserve never asks for them: it calls Observe with no filter, so
	// channel is always empty and GetMessages is never reached. The label is on
	// the branch that would carry them rather than on the environment, so that
	// wiring a channel filter later is a one-line change that is already
	// governed rather than a hole that opens quietly.
	trust := environment.TrustOperator
	if channel != "" {
		messages, err := adapter.GetMessages(ctx, channel, time.Time{})
		if err != nil {
			state["error"] = err.Error()
		} else {
			state["messages"] = messages
			trust = environment.TrustThirdParty
		}
	}
	return environment.Observation{
		EnvID: e.id, State: state, Version: stateVersion(state),
		Timestamp: time.Now().UTC(), Trust: trust,
	}, nil
}

func (e *commsEnv) Act(ctx context.Context, a environment.Action) (environment.ActionResult, error) {
	adapter := e.msg
	if adapter == nil || !adapter.Active() {
		return noopAct(a), nil
	}
	switch string(a.CapabilityID) {
	case "software.act.send_message":
		channel := asString(a.Params, "channel")
		text := asString(a.Params, "text")
		if text == "" {
			text = asString(a.Params, "message")
		}
		if err := adapter.PostMessage(ctx, channel, text); err != nil {
			return environment.ActionResult{Success: false, Error: err.Error(),
				StateDelta: map[string]any{"adapter": adapter.Name()}}, nil
		}
		return environment.ActionResult{Success: true,
			StateDelta: map[string]any{"adapter": adapter.Name(), "channel": channel}}, nil
	default:
		return noopAct(a), nil
	}
}

func (e *commsEnv) Subscribe(_ context.Context, _ environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	ch := make(chan environment.EnvironmentEvent)
	return ch, nil
}

func (e *commsEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	obs, _ := e.Observe(ctx, environment.ObservationQuery{})
	return environment.EnvironmentSnapshot{SHA: obs.Version, EnvID: e.id, State: obs.State, Timestamp: obs.Timestamp}, nil
}

// ── cliEnv (CLI coding-agent adapter) ────────────────────────────────────────

type cliEnv struct {
	id  environment.EnvironmentID
	cli cliagent.CLIAgentAdapter

	// twinID is the twin this environment was built for (BuildContext.TwinID).
	twinID string
	// attachMCP and mcpURL are the instance's attach_karakuri_mcp and
	// karakuri_mcp_url options; issuer mints the credential for one delegation.
	attachMCP bool
	mcpURL    string
	issuer    delegationIssuer
}

// DelegationCredential is what one delegation carries to Karakuri's own MCP
// endpoint. It mirrors the issuer's credential so this package does not import
// the feature layer.
type DelegationCredential struct {
	Token       string
	PrincipalID string
	TwinID      string
	ExpiresAt   time.Time
}

// delegationIssuer is the part of the delegation issuer cliEnv needs.
type delegationIssuer interface {
	Issue(ctx context.Context, twinID string, timeout time.Duration) (DelegationCredential, error)
	Revoke(ctx context.Context, c DelegationCredential) error
}

// cliMCPOptions reads attach_karakuri_mcp and karakuri_mcp_url off a
// cli_agents instance's options.
func cliMCPOptions(options map[string]any) (attach bool, url string) {
	attach, _ = options["attach_karakuri_mcp"].(bool)
	return attach, config.InstanceConfig{Options: options}.OptString("karakuri_mcp_url")
}

// revokeTimeout bounds the revoke that follows a delegation, which runs even
// when the action's own context is already cancelled.
const revokeTimeout = 10 * time.Second

func (e *cliEnv) ID() environment.EnvironmentID { return e.id }
func (e *cliEnv) Domain() string                { return "software" }

func (e *cliEnv) Observe(_ context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	// CLI agents have no observe surface — observation reports the configured
	// instance name so the agent's reason step can pick a target.
	state := map[string]any{"adapter": e.cli.Name(), "active": e.cli.Active()}
	return environment.Observation{
		EnvID: e.id, State: state, Version: stateVersion(state), Timestamp: time.Now().UTC(),
	}, nil
}

// karakuriMCPServer is the name Act attaches Karakuri's MCP server under, and
// karakuriToolPrefix is how a coding-agent CLI then names that server's tools.
const (
	karakuriMCPServer  = "karakuri"
	karakuriToolPrefix = "mcp__" + karakuriMCPServer + "__"
)

func (e *cliEnv) Act(ctx context.Context, a environment.Action) (environment.ActionResult, error) {
	if e.cli == nil || !e.cli.Active() {
		return noopAct(a), nil
	}
	prompt, ok := cliPrompt(a)
	if !ok {
		// Not this environment's concern.
		return noopAct(a), nil
	}
	if strings.TrimSpace(prompt) == "" {
		return environment.ActionResult{
			Success: false,
			Error:   fmt.Sprintf("%s needs a task to delegate: pass params.prompt (or task/instruction)", a.CapabilityID),
		}, nil
	}
	// A capability that declares NeedsWorkspace is given one by stepAct before
	// it runs. Arriving without it means the provisioning failed, and writing
	// into the checked-out tree instead is the one outcome a planner hint
	// explicitly forbids — so it refuses rather than guessing a path.
	worktree := asString(a.Params, "worktree_path")
	if worktree == "" {
		return environment.ActionResult{
			Success: false,
			Error:   fmt.Sprintf("%s has no worktree to write in; refusing to write to the checked-out tree", a.CapabilityID),
		}, nil
	}

	in := cliagent.DelegateInput{
		Prompt:         prompt,
		WorktreePath:   worktree,
		TimeoutSeconds: 600,
	}
	if files, ok := a.Params["files"].([]any); ok {
		for _, f := range files {
			if s, ok := f.(string); ok {
				in.Files = append(in.Files, s)
			}
		}
	}
	if tools, ok := a.Params["allowed_tools"].([]any); ok {
		for _, t := range tools {
			if s, ok := t.(string); ok {
				in.AllowedTools = append(in.AllowedTools, s)
			}
		}
	}

	// karakuri_tools: the Karakuri MCP tools the agent may call during this
	// one delegation, e.g. ["audit_list"]. Needs the instance option
	// attach_karakuri_mcp; the agent holds a read-only credential scoped to
	// the twin, revoked when the action ends.
	var karakuriTools []string
	if list, ok := a.Params["karakuri_tools"].([]any); ok {
		for _, t := range list {
			if s, ok := t.(string); ok && s != "" {
				karakuriTools = append(karakuriTools, s)
			}
		}
	}
	// scrub keeps the delegation token out of anything the result carries.
	scrub := func(s string) string { return s }
	var credential map[string]any
	if len(karakuriTools) > 0 {
		refuse := func(why string) (environment.ActionResult, error) {
			return environment.ActionResult{
				Success: false,
				Error:   fmt.Sprintf("%s asked for karakuri_tools %v but %s", a.CapabilityID, karakuriTools, why),
			}, nil
		}
		switch {
		case !e.attachMCP:
			return refuse("the cli_agents instance does not set attach_karakuri_mcp")
		case e.mcpURL == "":
			return refuse("the cli_agents instance sets no karakuri_mcp_url")
		case e.twinID == "":
			return refuse("the action has no twin to scope a delegation credential to")
		case e.issuer == nil:
			return refuse("no delegation credential issuer is wired in this deployment")
		}
		cred, err := e.issuer.Issue(ctx, e.twinID, time.Duration(in.TimeoutSeconds)*time.Second)
		if err != nil {
			return refuse("a delegation credential could not be issued: " + err.Error())
		}
		// The map is shared with the result, so the deferred revoke can
		// still record its outcome after the return value is built.
		credential = map[string]any{
			"twin_id":    cred.TwinID,
			"read_only":  true,
			"tools":      karakuriTools,
			"expires_at": cred.ExpiresAt.Format(time.RFC3339),
			"revoked":    true,
		}
		defer func() {
			// The action's context may be the reason it ended.
			rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), revokeTimeout)
			defer cancel()
			if err := e.issuer.Revoke(rctx, cred); err != nil {
				credential["revoked"] = false
				credential["revoke_error"] = err.Error()
			}
		}()
		if cred.Token != "" {
			scrub = func(s string) string { return strings.ReplaceAll(s, cred.Token, "[redacted]") }
		}
		in.MCP = &cliagent.MCPAttachment{
			ServerName: karakuriMCPServer,
			URL:        e.mcpURL,
			Token:      cred.Token,
			Tools:      karakuriTools,
		}
	}

	out, err := e.cli.Delegate(ctx, in)
	// What a Karakuri tool returned can be a stranger's prose (an audit row
	// holds pull-request titles), and the agent wrote everything after it
	// having read that. Decided from the calls the run made, not from what was
	// attached (ADR 021); the error of a run that failed is the run's text too.
	trust := environment.TrustOperator
	for _, u := range out.ToolUses {
		if strings.HasPrefix(u.Name, karakuriToolPrefix) {
			trust = environment.TrustThirdParty
			break
		}
	}
	if err != nil {
		delta := map[string]any{"adapter": e.cli.Name()}
		if credential != nil {
			delta["delegation_credential"] = credential
		}
		return environment.ActionResult{Success: false, Trust: trust, Error: scrub(err.Error()), StateDelta: delta}, nil
	}
	delta := map[string]any{
		"adapter":    e.cli.Name(),
		"summary":    scrub(out.Summary),
		"tool_uses":  out.ToolUses,
		"exit_code":  out.ExitCode,
		"raw_output": scrub(out.RawOutput),
	}
	if credential != nil {
		delta["delegation_credential"] = credential
	}
	return environment.ActionResult{Success: true, Trust: trust, StateDelta: delta, ArtifactSHAs: out.ArtifactSHAs}, nil
}

func (e *cliEnv) Subscribe(_ context.Context, _ environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	ch := make(chan environment.EnvironmentEvent)
	return ch, nil
}

func (e *cliEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	obs, _ := e.Observe(ctx, environment.ObservationQuery{})
	return environment.EnvironmentSnapshot{SHA: obs.Version, EnvID: e.id, State: obs.State, Timestamp: obs.Timestamp}, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func asString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func noopObservation(id environment.EnvironmentID) environment.Observation {
	return environment.Observation{
		EnvID: id, State: map[string]any{"status": "noop"},
		Version: "noop-0", Timestamp: time.Now().UTC(),
	}
}

// noopAct used to return Success=true — masking inactive adapters and
// capabilities routed to the wrong env. Same lie PR #28 fixed for the
// standalone noopEnv, missed here. Now reports failure honestly so the
// audit log surfaces both (a) "adapter X is not active" and (b) "agent
// targeted env Y with a capability Y doesn't handle". Callers (gitEnv,
// ticketEnv, commsEnv, cliEnv) reach this when their adapter is nil/
// inactive or when the agent routes an unknown capability.
func noopAct(a environment.Action) environment.ActionResult {
	return environment.ActionResult{
		Success: false,
		Error:   fmt.Sprintf("capability %q has no active adapter on this env", a.CapabilityID),
		StateDelta: map[string]any{
			"action": string(a.CapabilityID),
			"status": "no_active_adapter",
		},
	}
}

// verifyEnv answers the two verification capabilities a command can settle.
//
// It runs the command the plan supplies and reports success by exit code,
// which is what "the tests pass" means. It deliberately ships no default
// command: a pack that guessed `go test ./...` would report a green suite on
// a repository that is not Go, and a verifier that can be wrong in that
// direction is worse than one that asks to be told.
//
// review and tech_lead_review are not here. They are judgements, not commands,
// and the delivery template now says so rather than naming a verifier that
// cannot answer them.
type verifyEnv struct {
	id    environment.EnvironmentID
	shell *shellEnv
}

func newVerifyEnv(id environment.EnvironmentID, sh *shellEnv) *verifyEnv {
	return &verifyEnv{id: id, shell: sh}
}

func (e *verifyEnv) ID() environment.EnvironmentID { return e.id }
func (e *verifyEnv) Domain() string                { return "software" }

func (e *verifyEnv) Observe(_ context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	state := map[string]any{
		"adapter":      "verify.exec",
		"capabilities": []string{"software.verify.run_tests", "software.verify.lint"},
	}
	return environment.Observation{
		EnvID: e.id, State: state, Version: stateVersion(state), Timestamp: time.Now().UTC(),
	}, nil
}

func (e *verifyEnv) Act(ctx context.Context, a environment.Action) (environment.ActionResult, error) {
	switch string(a.CapabilityID) {
	case "software.verify.run_tests", "software.verify.lint":
	default:
		return failureResult(e.id, a.CapabilityID,
			fmt.Sprintf("verifyEnv does not handle capability %q", a.CapabilityID), nil), nil
	}
	if asString(a.Params, "cmd") == "" {
		return failureResult(e.id, a.CapabilityID,
			fmt.Sprintf("%s needs params.cmd: the command that decides it, e.g. 'go test ./... -count=1'", a.CapabilityID),
			nil), nil
	}
	// Reuse the shell executor wholesale: the denylist, the workdir
	// confinement and the timeout are the same guarantees a verification
	// command needs, and a second copy of them would be a second set of bugs.
	res, err := e.shell.Act(ctx, environment.Action{
		CapabilityID: "software.act.shell_exec",
		Params:       a.Params,
	})
	if err != nil {
		return res, err
	}
	if res.StateDelta == nil {
		res.StateDelta = map[string]any{}
	}
	res.StateDelta["verifier"] = string(a.CapabilityID)
	return res, nil
}

func (e *verifyEnv) Subscribe(_ context.Context, _ environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	ch := make(chan environment.EnvironmentEvent)
	return ch, nil
}

func (e *verifyEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	obs, _ := e.Observe(ctx, environment.ObservationQuery{})
	return environment.EnvironmentSnapshot{SHA: obs.Version, EnvID: e.id, State: obs.State, Timestamp: obs.Timestamp}, nil
}

// stateVersion hashes an observation's state into the SHA the loop records and
// the reconcile supervisor compares.
//
// Keys are sorted before hashing. Ranging a Go map is randomised per iteration,
// so the same state hashed twice in a row produced two different SHAs — and
// every consumer of this value asks "is this the same as last time".
//
// The cost landed on the outer loop. reconcile.fingerprint feeds each
// environment's Snapshot().SHA into a composite, compares it against the SHA at
// the last convergence, and runs the expensive tier when they differ. With an
// unstable SHA they always differ, so every standing objective over gitEnv,
// ticketEnv, commsEnv, cliEnv, shellEnv or codebaseEnv ran the full loop on
// every sense tick — which is the entire economic argument of Phase 20
// inverted: the cheap tier exists so an objective can be checked every fifteen
// minutes all year and only spend money on the days something moved.
//
// sense.go sorts environment IDs before building the composite and says why in
// a comment — "an unsorted hash would report drift every time a pack's
// registration order changed". The per-environment SHA underneath it was
// unsorted the whole time.
func stateVersion(state map[string]any) string {
	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&sb, "%s=%v;", k, state[k])
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])[:16]
}

// cliPrompt returns the task to hand the coding-agent CLI, and whether this
// capability is one the CLI environment serves.
//
// write_code and write_test are delegated rather than reimplemented. They were
// declared for three phases with no implementation at all: stepAct provisioned
// a worktree for them by name and then routed them to noopEnv, which returned
// "unimplemented" — so the capability with a workspace could not write, and
// delegate_to_cli, which can, was never given one. They are the same act with
// different emphasis, and saying so beats a second code path.
func cliPrompt(a environment.Action) (string, bool) {
	// Callers spell the task differently depending on the capability; accept
	// any of them rather than failing on vocabulary.
	task := asString(a.Params, "prompt")
	if task == "" {
		task = asString(a.Params, "task")
	}
	if task == "" {
		task = asString(a.Params, "instruction")
	}

	switch string(a.CapabilityID) {
	case "software.act.delegate_to_cli":
		return task, true
	case "software.act.write_code":
		return task, true
	case "software.act.write_test":
		if task == "" {
			return "", true
		}
		// Stated in the prompt rather than left to the capability's name: the
		// CLI receives text, and "write a test" is not implied by an ID it
		// never sees.
		return "Write tests only, no implementation changes. " + task, true
	}
	return "", false
}
