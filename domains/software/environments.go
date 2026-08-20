package software

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/cliagent"
	"github.com/bsenel/karakuri/internal/platform/tools/messaging"
	"github.com/bsenel/karakuri/internal/platform/tools/projectmgmt"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// softwareEnvironmentFactories builds the six software environments. The Git,
// Ticket, and Communication envs dispatch to the tools.Registry adapters
// (GitHub / Linear / Slack when configured). The remaining envs are no-op.
// reg may be nil — in that case every env falls back to no-op behavior.
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
			Build: func(ctx environment.BuildContext) (environment.Environment, error) {
				var cli cliagent.CLIAgentAdapter = cliagent.NewNoOp()
				if reg != nil {
					if a, ok := reg.CLIAgents.Resolve(ctx.AdapterBindings["cli_agents"]); ok {
						cli = a
					}
				}
				return &cliEnv{id: "software.env.cli_agent", cli: cli}, nil
			},
		},
		noopFactory("software.env.ci", "CI pipeline: build status, test results, coverage"),
		noopFactory("software.env.observability", "Runtime: logs, metrics, alerts"),
		noopFactory("software.env.codebase", "Static analysis: file tree, symbols, dependency graph"),
		{
			EnvID:       "software.env.shell",
			Domain:      "software",
			Description: "Local shell executor (/bin/sh) — runs software.act.shell_exec actions with timeout, output capture, and safety guardrails. Defaults to the server's CWD; configurable per-deployment.",
			Build: func(_ environment.BuildContext) (environment.Environment, error) {
				return newShellEnv("software.env.shell", "", 60*time.Second), nil
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

func (e *gitEnv) ID() environment.EnvironmentID { return e.id }
func (e *gitEnv) Domain() string                { return "software" }

func (e *gitEnv) Observe(ctx context.Context, q environment.ObservationQuery) (environment.Observation, error) {
	adapter := e.vc
	if adapter == nil || !adapter.Active() {
		return noopObservation(e.id), nil
	}
	repo, _ := q.Filter["repo"].(string)
	state := map[string]any{"adapter": adapter.Name()}
	commits, err := adapter.GetCommits(ctx, repo, time.Time{})
	if err != nil {
		state["commits_error"] = err.Error()
	} else {
		state["commits"] = commits
	}
	prs, err := adapter.ListPRs(ctx, repo, time.Time{})
	if err != nil {
		state["prs_error"] = err.Error()
	} else {
		state["prs"] = prs
	}
	return environment.Observation{
		EnvID: e.id, State: state, Version: stateVersion(state), Timestamp: time.Now().UTC(),
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
	}

	adapter := e.vc
	if adapter == nil || !adapter.Active() {
		return noopAct(a), nil
	}
	switch string(a.CapabilityID) {
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
	if id, ok := q.Filter["ticket_id"].(string); ok && id != "" {
		ticket, err := adapter.GetTicket(ctx, id)
		if err != nil {
			state["error"] = err.Error()
		} else {
			state["ticket"] = ticket
		}
	}
	return environment.Observation{
		EnvID: e.id, State: state, Version: stateVersion(state), Timestamp: time.Now().UTC(),
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
	if channel != "" {
		messages, err := adapter.GetMessages(ctx, channel, time.Time{})
		if err != nil {
			state["error"] = err.Error()
		} else {
			state["messages"] = messages
		}
	}
	return environment.Observation{
		EnvID: e.id, State: state, Version: stateVersion(state), Timestamp: time.Now().UTC(),
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
}

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

	out, err := e.cli.Delegate(ctx, in)
	if err != nil {
		return environment.ActionResult{Success: false, Error: err.Error(),
			StateDelta: map[string]any{"adapter": e.cli.Name()}}, nil
	}
	return environment.ActionResult{
		Success: true,
		StateDelta: map[string]any{
			"adapter":    e.cli.Name(),
			"summary":    out.Summary,
			"tool_uses":  out.ToolUses,
			"exit_code":  out.ExitCode,
			"raw_output": out.RawOutput,
		},
		ArtifactSHAs: out.ArtifactSHAs,
	}, nil
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

func stateVersion(state map[string]any) string {
	var sb strings.Builder
	for k, v := range state {
		fmt.Fprintf(&sb, "%s=%v;", k, v)
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
