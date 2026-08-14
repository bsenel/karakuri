# Karakuri Architecture

Karakuri is a continuous autonomous reasoning platform structured as a clean three-layer Go monolith with a thin `krk` CLI client.

## Layers

```
cmd/              → binary entry points (server, krk)
internal/core/    → domain types and interfaces; zero vendor imports
internal/feature/ → business logic services; depends only on core
internal/platform/→ all vendor bindings (LangChain Go, GORM, go-git, OTel)
internal/api/     → HTTP delivery; delegates entirely to feature services
domains/          → pluggable domain packs (software v1, agriculture v1, stubs)
cli/              → krk commands; thin HTTP client
```

**Import rules (enforced by golangci-lint depguard):**
- LangChain Go imports only in `internal/platform/`
- Domain pack imports only in `cmd/` and `internal/app/`

## Autonomous Reasoning Loop

```
OBSERVE → REASON → DECIDE → ACT → VERIFY → LEARN
   ↑                                          │
   └──────────────────────────────────────────┘
        re-enters if criteria not met and iterations remain
```

Each step is a separate file in `internal/feature/loop/`:

| Step    | File        | What it does |
|---------|-------------|--------------|
| Observe | observe.go  | Fan-out env.Observe() across all environments; recall episodic+semantic memory |
| Reason  | reason.go   | Call agent.Run() with world state + memory; parse JSON plan |
| Decide  | decide.go   | Check AuthorityBounds; bias confidence from procedural memory; emit checkpoint if escalating |
| Act     | act.go      | Execute each planned action; create git worktrees for code capabilities |
| Verify  | verify.go   | Evaluate success criteria via agent or env results; compute weighted score |
| Learn   | learn.go    | Write episodic + procedural memory; trigger consolidation to semantic tier |

## The Outer Loop — Standing Objectives

The loop above converges once. `internal/feature/reconcile` is what keeps
calling it for an objective declared **standing** (Phase 20, [ADR 015](adr/015-standing-objectives-and-reconciliation.md)):

```
        due-wheel tick (one goroutine, indexed next_due_at)
                        │
                   claim lease
                        │
        SENSE ── Snapshot() ×N → sorted composite hash
                        │            (adapter calls, no model call)
       ┌────────────────┴────────────────┐
  unchanged                     drift, schedule, or resync
       │                                 │
  record, sleep              RECONCILE ── loop.Service.Run(req)
                                          with req.Agent.Authority
                                          rewritten by earned autonomy
```

It is a **caller** of the loop, not a change to it: the six steps are
untouched, and autonomy becomes enforcement by writing
`agent.AuthorityBounds` into the request — the struct the decide step has
enforced since Phase 1, so there is no second gate to disagree with the first.

| Concern | Where |
|---------|-------|
| Declaration (mode, cadence, autonomy) | `internal/core/objective/standing.go` |
| Runtime state, drift, outcomes | `internal/core/reconcile/` |
| Supervisor, sense tier, authority mapping | `internal/feature/reconcile/` |
| Cadence maths (cron, timezone, quiet windows) | `internal/platform/schedule/` |

Watch mode used to live in `loop/watch.go`; it is now a standing objective at
`sense` autonomy, which behaves the same and survives a restart.

## Four-Tier Memory

| Tier        | Storage               | Purpose |
|-------------|-----------------------|---------|
| Working     | sync.Map              | In-flight state within a single loop run |
| Episodic    | SQLite                | Iteration traces: actions, scores, reasoning |
| Semantic    | SQLite (vec fallback) | Consolidated facts; promoted from episodic |
| Procedural  | SQLite                | Per-capability success/failure rates |

Consolidation: after each learn step, high-confidence (≥0.8) episodic entries are promoted to semantic tier. The procedural tier biases plan confidence at the decide step (+0.05 for >80% success rate, -0.10 for <30%).

## Domain Pack System

Domain packs implement the `domain.Pack` interface and register capabilities, environments, agent definitions, objective templates, and planner hints at startup. The core engine imports no domain knowledge.

```
domains/software/    → 20 capabilities, 6 envs, 7 agents, 7 templates
domains/agriculture/ → 8 capabilities, 2 envs, 2 agents, 2 templates
domains/*/           → stubs for healthcare, legal, mechanical, consulting
```

All packs are validated at startup and on demand via `krk domain test <id>` (7 conformance checks).

## Tool Adapters (Multi-Instance, Twin-Bound)

External integrations live in `internal/platform/tools/` behind per-category **slot** interfaces (`versioncontrol`, `projectmgmt`, `messaging`, `design`, `testing`, `calendar`, `email`). Each slot can hold many named adapter instances simultaneously, and each `DigitalTwin` selects which instance answers for it via `AdapterBindings`.

```
config.ToolsConfig
  └── per slot: SlotConfig{ Default, Instances }
       └── per instance: InstanceConfig{ Type, Options }   // type = "github", "gmail", "slack", …

tools.Registry
  └── per slot: SlotInstances[T]   // generic, typed instance map
       └── Resolve(name) → adapter        (empty name → DefaultName())

DigitalTwin.AdapterBindings map[string]string   // slot → instance name (persisted)
```

At loop start the runner fetches the assigned twin and passes `environment.BuildContext{TwinID, AdapterBindings}` to every env factory. Software envs (`gitEnv`, `ticketEnv`, `commsEnv`) capture the resolved adapter once at construction — `Act()` is a direct call with no per-action lookup. Twins without a binding fall back to the slot's `default` instance; missing default → no-op.

Credentials are referenced from environment variables via `*_env` sibling keys (`token_env: ACME_GITHUB_TOKEN`); `resolveEnvRefs` substitutes the values at config load. Inline plaintext stays supported for local dev.

`/api/v1/health` returns one row per `(slot, instance, type, active, is_default)` so operators see the full topology. See ADR 006 for the rationale.

Operator commands:
```bash
krk twin bindings <id>                                                    # show
krk twin bindings <id> --set versioncontrol=acme_github --set email=...   # replace
```

## Performance Baseline

Measured on Apple M1 with no-op environments:

| Scenario | Wall time |
|----------|-----------|
| Single loop, no-op envs, no criteria | ~50ms excluding LLM |
| LLM call (claude-sonnet-4-6, single step) | 1–5s |
| Memory recall (SQLite, top-5) | <1ms |
| Worktree create (go-git) | ~200ms |

LLM latency dominates; all other operations are sub-millisecond.

## Key Design Decisions

**Primitive-first, not role-first.** The engine knows only Capabilities, Environments, Objectives, and Agents. Teams, workflows, and roles are expressed through these four types.

**Domain isolation.** `internal/core/` and `internal/feature/` import no domain packages. Adding a new domain requires zero changes to the engine.

**LangChain Go confinement.** All LangChain Go imports live in `internal/platform/agent/` and `internal/platform/llm/`. The rest of the system depends on the `AgentFactory` interface.

**Async loop execution.** `Run()` returns a loop ID immediately; the loop runs in a background goroutine. `Resume()` unblocks via a buffered channel; `Status()` reads from a protected in-memory state map.

**Interface-first, no-op by default.** Every tool slot ships with a no-op fallback. The loop runs to completion when no instances are configured; configured instances activate real-world side effects (PRs, tickets, messages, emails). LLM providers follow the same pattern (Gemini/Cursor/Copilot currently return `ErrNotImplemented`).

**Multi-tenant by construction.** Tool adapters are multi-instance per slot and twin-bound at dispatch time. One server can host Acme's GitHub + Slack + Outlook alongside a personal GitHub + SMTP — each twin's loop resolves the right instance from its `AdapterBindings`. See the Tool Adapters section above and ADR 006.

See ADRs in `docs/adr/` for design decisions.
