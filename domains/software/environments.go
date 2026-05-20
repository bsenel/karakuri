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
			EnvID:       "software.env.git",
			Domain:      "software",
			Description: "Git repository: commits, branches, PRs, worktrees, diffs",
			Build: func(ctx environment.BuildContext) (environment.Environment, error) {
				var vc versioncontrol.VersionControlAdapter = versioncontrol.NewNoOp()
				if reg != nil {
					if a, ok := reg.VC.Resolve(ctx.AdapterBindings["versioncontrol"]); ok {
						vc = a
					}
				}
				return &gitEnv{id: "software.env.git", vc: vc}, nil
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
		noopFactory("software.env.ci", "CI pipeline: build status, test results, coverage"),
		noopFactory("software.env.observability", "Runtime: logs, metrics, alerts"),
		noopFactory("software.env.codebase", "Static analysis: file tree, symbols, dependency graph"),
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

func (e *noopEnv) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{
		Success:    true,
		StateDelta: map[string]any{"action": string(a.CapabilityID), "status": "noop"},
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

func noopAct(a environment.Action) environment.ActionResult {
	return environment.ActionResult{
		Success:    true,
		StateDelta: map[string]any{"action": string(a.CapabilityID), "status": "noop"},
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
