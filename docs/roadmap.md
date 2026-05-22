# Karakuri Roadmap

## Context

Karakuri replaced the original role-based workflow simulator with an autonomous platform built on four primitives: **Capabilities, Environments, Objectives, and Agents**. No backward compatibility is maintained. The CLI binary is `krk`. This document records what shipped (Phases 1–12) and what is queued (Phase 13).

## Status Summary


| Phase | Title                                      | Status        |
| ----- | ------------------------------------------ | ------------- |
| 1     | Core Engine Foundation                     | **Completed** |
| 2     | Reasoning Loop + Software Domain Pack      | **Completed** |
| 3     | Memory Intelligence + Watch Mode           | **Completed** |
| 4     | Domain Pack SDK + Hardening                | **Completed** |
| 5     | Local Deployment Variants                  | **Completed** |
| 6     | Real Tool Adapters                         | **Completed** |
| 7     | Multi-LLM Provider Parity + CLI Agents     | **Completed** |
| 8     | Production Storage (PostgreSQL + pgvector) | **Completed** |
| 9     | React Frontend                             | **Completed** |
| 10    | Domain Pack Expansion (Healthcare)         | **Completed** |
| 11    | Distributed & Durable Execution            | **Completed** |
| 12    | Observability Fan-out                      | **Completed** |
| 13    | Cross-Domain Objectives + Hardening        | Planned       |


---

## Phase 1 — Core Engine Foundation (Completed)

**Goal:** Server starts; health endpoint reports all components; database schema live; OTel emitting.

**Steps:**

1. **Delete old implementation.** Remove `internal/feature/orchestrator/`, `internal/feature/strategy/`, `internal/feature/discovery/`, `internal/feature/delivery/`, `internal/feature/autonomous/`, `internal/feature/session/`, `internal/core/entity/`, `internal/core/agent/` (old), `cli/command/` (all old commands), `workflows/`, `docs/openapi.yaml`. Preserve: `go.mod`, `go.sum`, `Makefile`, `config/config.go` skeleton, `internal/platform/git/`, `internal/platform/observability/` skeleton, `internal/platform/llm/claude.go`.
2. **Scaffold `internal/core/`** — write all type definitions and interfaces as defined in the spec. No logic yet; just types, interfaces, and constants. Zero vendor imports enforced via `import` linting.
3. **Rewrite `internal/platform/db/`** — new GORM schema with tables: `twins`, `objectives`, `loop_iterations`, `memory_episodic`, `memory_semantic` (sqlite-vec vector column), `memory_procedural`, `checkpoints`, `blobs`, `worktrees`, `tool_events`. Write `migrations/000001_init_schema.up.sql` / `.down.sql`. Remove old migration.
4. **Implement `internal/platform/storage/`** — `StorageAdapter` interface + GORM implementation covering all methods in the spec's database layer section.
5. **Implement `internal/platform/memory/`** — four tier impls: working (sync.Map), episodic (GORM query on `memory_episodic`), semantic (sqlite-vec `knn_search`), procedural (GORM query on `memory_procedural`).
6. **Implement `internal/platform/llm/`** — port existing Claude adapter to `ProviderAdapter` interface; add Gemini/Cursor/Copilot stubs returning `ErrNotImplemented`; write `ProviderRegistry` with fallback chain from config.
7. **Implement `internal/platform/executor/`** — `Executor` interface; local goroutine-based impl; Celery/Restate stubs.
8. **Port `internal/platform/git/`** — rename/adapt existing `WorktreeManager` to the new `WorktreeOptions`/`Worktree` types. Path convention: `worktrees/<objective-id>/<task-id>/`. Branch: `karakuri/<objective-id>/<task-id>`.
9. **Implement `internal/platform/observability/`** — port existing OTel setup; adapt `LocalFileExporter` to emit to `karakuri-obs/metrics/` and `karakuri-obs/logs/` in all four formats (JSON, NDJSON; Parquet/CSV stubs). Write `ExporterRegistry`.
10. **Implement `internal/platform/tools/`** — all adapter interfaces + no-op defaults (versioncontrol, projectmgmt, messaging, observability/external, design, testing, research). Write `ResearchAdapter` scraper (port from existing `tools/research/scraper.go` if present).
11. **Implement `internal/platform/agent/`** — `AgentFactory` using LangChain Go; `toolregistry.go` mapping `CapabilityID` → `tools.Tool`; `callback.go` translating LangChain Go callbacks to SSE events via `event.Emitter`.
12. **Write `config/default.yaml`** per spec; update `config/config.go` loader.
13. **Stub all domain packs** — `domains/software/pack.go` (fully structured, no logic yet), `domains/agriculture|healthcare|legal|mechanical|consulting/pack.go` (minimal stubs).
14. `**internal/core/domain/registry.go`** — `DomainRegistry` that calls `DomainPack.Init()` at startup.
15. **Wire `cmd/server/main.go`** — bootstrap DB, run migrations, load config, register domain packs, start HTTP server with only `GET /health` wired. Health handler queries all adapters and exporters.
16. **Stub domain ADRs** in `docs/adr/`.

**Acceptance:** `go run cmd/server/main.go` starts; `curl /health` returns Claude active, adapters no-op, LocalFileExporter active, WorktreeManager ready; `karakuri-obs/` directory created; OTel emits a test metric; sqlite-vec `knn_search` works in a unit test.

---

## Phase 2 — Reasoning Loop + Software Domain Pack (Completed)

**Goal:** Full six-step loop drives a software delivery objective to completion with all adapters no-op; all CLI commands work; SSE streams live.

**Steps:**

1. **Implement `internal/feature/loop/service.go`** — `LoopService` orchestrates six steps. Each step is its own file (`observe.go`, `reason.go`, `decide.go`, `act.go`, `verify.go`, `learn.go`). Loop runs until: objective criteria met, `MaxIter` exceeded, hard constraint violated, or checkpoint emitted.
2. **Observe step** — invoke all `observe.*` capabilities in agent portfolio; merge results into `WorldState` with composite SHA; recall episodic + semantic memory relevant to objective; emit `loop_step_completed{step: observe}`.
3. **Reason step** — build `AgentInput`; invoke `Agent.Run()` or `Agent.Stream()`; apply `ReasoningStrategy`; produce `ReasoningOutput` with ranked `CandidatePlan` list; persist reasoning trace to episodic memory; emit `loop_step_completed{step: reason}`.
4. **Decide step** — select highest-confidence plan; check `AuthorityBounds` (RequiresApprovalFor, ConfidenceThreshold, MaxAutonomousActions); emit `checkpoint` event and pause if escalation triggered; emit `loop_step_completed{step: decide}`.
5. **Act step** — for each action in committed plan: if `software.act.write_code` or `software.act.write_test`, call `WorktreeManager.Create()` first; invoke `Environment.Act()` on target environment; collect `ActionResult`; accumulate `StateDelta`; emit `worktree_created`, `artifact_written`, `adapter_skipped` events as appropriate; emit `loop_step_completed{step: act}`.
6. **Verify step** — invoke each `Criterion.Verifier` capability; for `verify.review` / `verify.tech_lead_review`: spawn sub-agents; aggregate into `VerificationReport`; compute weighted completion score; if score ≥ threshold → proceed to Learn; if below and retries remain → re-enter Observe with report as context; if retries exhausted → `ObjectiveStatusFailed`; emit `loop_step_completed{step: verify}`.
7. **Learn step** — write `LoopIteration` to episodic memory; update procedural memory (capability → outcome); extract facts → semantic memory with embedding; call `Memory.Consolidate()` if threshold exceeded; prune failed worktrees; emit `loop_step_completed{step: learn}`.
8. **Implement `internal/feature/twin/`** — CRUD for `DigitalTwin`; assign objective to twin; start/stop watch mode.
9. **Implement `internal/feature/objective/`** — CRUD; status transitions; criteria progress tracking (per-criterion pass/fail + weighted score).
10. **Implement `internal/feature/memory/`** — `MemoryService`: multi-tier recall orchestration; consolidation job (episodic → semantic promotion above threshold).
11. **Implement `internal/feature/checkpoint/`** — create checkpoint → pause loop → await decision → resume or abort.
12. **Implement `internal/feature/artifact/`** — VFS blob write (SHA addressed); list; diff (line diff for text blobs).
13. **Implement `domains/software/`** fully:
  - `capabilities.go` — all 20 capabilities (software.observe.*, software.reason.*, software.decide.*, software.act.*, software.verify.*, software.learn.*) with schema definitions
    - `environments.go` — 6 environment factories (Git, CI, Observability, Codebase, Ticket, Communication) with no-op defaults
    - `agents.go` — 7 agent definitions (strategist, architect, researcher, implementer, reviewer, sre, watcher)
    - `objectives.go` — 7 objective templates (strategy, discovery, delivery, code_review, research, incident_response, autonomous_watch)
    - `hints.go` — all planner hints (TDD ordering, design-before-code, etc.)
14. **Wire all API endpoints** (`internal/api/handler/`) per the spec's API layer. All handlers delegate to feature services; no business logic in handlers. Implement SSE endpoint (`GET /objectives/:id/loop/events`, `GET /twins/:id/events`).
15. **Implement all `krk` CLI commands** in `cli/command/` using cobra. All commands are thin HTTP clients. Implement all flags per the spec's CLI interface section.
16. **Add OTel instrumentation** across all loop steps and memory operations (loop iteration count, step latency, criteria score, token usage, memory hit rate, worktree count).

**Acceptance:**

```bash
krk twin create --name "dev-team" --kind team --domain software
krk objective create --twin <id> --template software.objective.delivery --title "implement auth"
krk loop start <objective-id>
# → full six-step loop completes; SSE events stream to terminal
# → loop iteration history queryable
# → memory entries written after each learn step
# → worktrees provisioned and pruned for delivery objectives
krk checkpoint list  # shows pending checkpoints if authority bounds trigger
```

---

## Phase 3 — Memory Intelligence + Watch Mode (Completed)

**Goal:** Second runs of same objective template produce measurably better reasoning; watcher twin continuously monitors environments.

**Steps:**

1. **Semantic memory recall injection** — at Observe step: call `Memory.Recall()` with semantic tier + objective description as query; inject top-K results into `AgentInput.Memory`. At Reason step: recall procedural memory for capability-outcome pairs relevant to planned actions.
2. **Procedural memory at Decide** — before selecting plan, query procedural memory for historical success rates of candidate capabilities; bias selection toward higher-success-rate paths.
3. **Memory consolidation** — after Learn step: if episodic entry count > consolidation threshold, call `Memory.Consolidate()`; promote high-confidence episodic entries to semantic tier with embedding generation via Claude.
4. `**software.objective.autonomous_watch` fully operational** — watcher agent subscribes to all configured environments via `Environment.Subscribe()`; on `EnvironmentEvent` received, evaluates against promotion rules; emits `checkpoint` with suggested objective template for human approval.
5. **Research pulse** — integrate `ResearchService` into watcher loop: periodically invoke `software.reason.research` via ResearchAdapter; detect threats/opportunities; emit checkpoint with promotable research objective if significance threshold met.
6. `**krk auto` command** — shorthand for creating a watcher twin and starting watch mode; streams environment events and checkpoint prompts to terminal.
7. **OTel metrics** — add memory hit rate, recall latency, consolidation frequency to LocalFileExporter output.

**Acceptance:**

- Second run of `software.objective.delivery` on same repo produces reasoning trace referencing prior episodic memory entries.
- Simulated environment change (push a commit) triggers watcher → `environment_changed` SSE event → `checkpoint` emitted asking to promote to `software.objective.code_review`.
- Research pulse produces trend report artifact; similarity score visible in `krk memory recall` output.

---

## Phase 4 — Domain Pack SDK + Hardening (Completed)

**Goal:** External domain authors can build and register packs; system is production-hardened.

**Steps:**

1. `**karakuri-domain-sdk` Go module** — extract DomainPack scaffolding, capability primitives, environment base types into a publishable Go module. Include conformance test suite: validates capability schemas, environment factory outputs, objective template structure.
2. `**krk domain add <pack-path>`** — load Go plugin or local module; call `DomainPack.Init()`; register capabilities and environments; validate via conformance suite.
3. `**krk domain test <pack-path>**` — run conformance suite against pack in dry-run mode; report pass/fail per check.
4. **Agriculture reference stub** — `domains/agriculture/pack.go` implements `DomainPack` interface non-trivially (real capability schemas, at least one environment factory, one objective template); passes conformance suite.
5. **Integration tests** — `test/integration/`: all CLI commands end-to-end against live API + SQLite; concurrent delivery test (3 parallel implementer agents, 3 isolated worktrees, no filesystem conflict); provider fallback test (disable Claude env var → verify graceful fallback).
6. **Performance baseline** — measure wall-clock time for full delivery loop (6 steps, 2 implementer instances) on local executor; document in `docs/architecture.md`.
7. **OTel format verification** — all four formats tested; Parquet queryable via DuckDB; file rotation tested.
8. **OpenAPI spec** — generate from chi routes; write to `docs/openapi.yaml`.
9. **Complete all ADRs** in `docs/adr/`; write `docs/domain-packs.md` authoring guide.
10. **Import boundary enforcement** — add `go vet` or `golangci-lint` rule verifying: no LangChain Go import outside `internal/platform/`; no domain package imports in `internal/core/` or `internal/feature/`; no `utils`/`helpers`/`common`/`misc` packages exist.

**Acceptance:**

- `krk domain add domains/agriculture` succeeds; `krk domain test domains/agriculture` shows all conformance checks pass.
- All Phase 1–3 acceptance criteria still pass.
- OpenAPI spec complete and matches implemented endpoints.
- Concurrent worktree test passes with 3 parallel agents.
- `golangci-lint` passes with import boundary rules active.

---

## Phase 5 — Local Deployment Variants (Completed)

**Goal:** Allow anyone to run Karakuri locally via five seamless routes — Docker Compose, Helm (direct), Minikube, k3s, and ArgoCD — with zero duplicated config or values across variants.

**What shipped:**

- One Helm chart rooted at `deploy/` (chart name `karakuri` from `Chart.yaml`)
- One canonical Karakuri runtime config at `deploy/karakuri.yaml` (`/data/`-paths), read by both `Dockerfile COPY` (image self-contained) and the chart's ConfigMap template via `.Files.Get` — no drift possible
- One values surface (`deploy/values.yaml`) shared by Helm direct, Minikube, k3s, and ArgoCD; `deploy/values-k3s.yaml` carries only k3s deltas
- ArgoCD Application at `deploy/argocd/application.yaml` uses a Helm source pointing at `deploy/`; `deploy/.helmignore` excludes `argocd/` from chart tarballs so `helm package deploy` works
- Five `make` targets composed from internal `_secret`, `_image-load-`*, `_helm-install*` primitives — image tag, namespace, release name, and chart path each declared exactly once

**Repository layout:**

```
Dockerfile                        ← COPY deploy/karakuri.yaml /etc/karakuri/config.yaml
docker-compose.yml
docker-entrypoint.sh
.dockerignore
config/
├── config.go
└── default.yaml                  ← local-dev paths (./karakuri.db) for `go run`
deploy/                           ← Helm chart root
├── Chart.yaml
├── values.yaml                   ← image, replicas, service, storage, resources
├── values-k3s.yaml               ← k3s overrides only
├── karakuri.yaml                 ← canonical /data/-paths runtime config
├── .helmignore                   ← excludes argocd/ from chart tarballs
├── templates/
│   ├── _helpers.tpl
│   ├── namespace.yaml
│   ├── configmap.yaml            ← .Files.Get "karakuri.yaml"
│   ├── pvc.yaml
│   ├── deployment.yaml
│   └── service.yaml
└── argocd/
    └── application.yaml          ← Helm source, path: deploy
```

**Single source of truth:**


| Setting                                                          | Lives in                         | Consumed by                                          |
| ---------------------------------------------------------------- | -------------------------------- | ---------------------------------------------------- |
| Server config (DB path, providers, memory thresholds)            | `deploy/karakuri.yaml`           | Dockerfile `COPY`; chart ConfigMap via `.Files.Get`  |
| Image, replicas, service, storage, resources                     | `deploy/values.yaml`             | All four K8s variants                                |
| k3s deltas (`pullPolicy: IfNotPresent`, ClusterIP, `local-path`) | `deploy/values-k3s.yaml`         | k3s target only                                      |
| Secrets (`ANTHROPIC_API_KEY`, `KARAKURI_AUTH_TOKEN`)             | Process env at deploy time       | All variants via shared `_secret` Makefile primitive |
| ArgoCD Application                                               | `deploy/argocd/application.yaml` | ArgoCD only                                          |


**Variants:**


| Variant        | Up                 | Down                 |
| -------------- | ------------------ | -------------------- |
| Docker Compose | `make docker-up`   | `make docker-down`   |
| Helm (direct)  | `make helm-up`     | `make helm-down`     |
| Minikube       | `make minikube-up` | `make minikube-down` |
| k3s            | `make k3s-up`      | `make k3s-down`      |
| ArgoCD         | `make argocd-up`   | `make argocd-down`   |


**Verification:**

```bash
# Image and chart serve identical config
docker run --rm karakuri:latest cat /etc/karakuri/config.yaml | diff - deploy/karakuri.yaml
helm template karakuri deploy | grep -A 20 "config.yaml:"

# Common smoke test (all variants, port 8080)
krk twin create --name test --kind team --domain software
krk objective create --twin <id> --template software.objective.delivery --title "local test"
krk loop start <obj-id>
```

---

## Phase 6 — Real Tool Adapters (Completed)

**Goal:** Replace no-op tool adapters with real implementations so the **act** step produces real-world side effects (PRs, tickets, messages, meetings, emails) — not just artifacts and worktrees. The shipped design also supports **multi-tenant deployments**: one Karakuri server can host many provider instances per slot, routed per `DigitalTwin` (ADR 006).

**What shipped — ten real adapter implementations across seven slots:**


| Slot             | Adapter `type:` values                   | Package(s)                                      |
| ---------------- | ---------------------------------------- | ----------------------------------------------- |
| `versioncontrol` | `github`                                 | `tools/versioncontrol/github.go`                |
| `projectmgmt`    | `linear`                                 | `tools/projectmgmt/linear.go`                   |
| `messaging`      | `slack`                                  | `tools/messaging/slack.go`                      |
| `design`         | `figma`                                  | `tools/design/figma.go`                         |
| `testing`        | `playwright`                             | `tools/testing/playwright.go`                   |
| `calendar`       | `google` (Google Calendar v3)            | `tools/calendar/google.go`                      |
| `email`          | `gmail`, `outlook`, `smtp`, `apple_mail` | `tools/email/{gmail,outlook,smtp,applemail}.go` |


**Implementation notes:**

- **GitHub** — `CreatePR`, `ListPRs`, `GetCommits` via REST API (`api.github.com`); `Authorization: Bearer <token>`; pure `net/http`, no SDK.
- **Linear** — `GetTicket`, `CreateTicket` via GraphQL (`api.linear.app/graphql`); raw `Authorization: <api_key>` header; `team_id` required for creation.
- **Slack** — `PostMessage`, `GetMessages` via `chat.postMessage` and `conversations.history`; Bot Token (`xoxb-…`); default channel configurable per instance.
- **Figma** — `GetFile` via REST API (`api.figma.com`); `X-Figma-Token` header.
- **Playwright** — `RunTests` subprocesses `npx playwright test --reporter=json` from a configured project dir; flattens the JSON reporter output into `TestResult` records (failure exit codes are data, not adapter errors).
- **Google Calendar** — `ListEvents`, `CreateEvent` via Calendar API v3; OAuth 2.0 Bearer token (minted upstream — `gcloud`, `oauth2l`, your own OAuth flow); default calendar `primary`.
- **Email — four interchangeable providers** under the single `email` slot:
  - `gmail` — Gmail API v1; OAuth Bearer (`gmail.send` + `gmail.readonly` scopes).
  - `outlook` — Microsoft Graph (`/me/sendMail`, `/me/messages`); OAuth Bearer with `Mail.Send` + `Mail.Read`.
  - `smtp` — generic `net/smtp`; works with iCloud, Fastmail, ProtonMail Bridge, corporate servers; port picks TLS strategy (`465` implicit TLS, `587` STARTTLS, else plain); send-only (List requires IMAP).
  - `apple_mail` — drives macOS Mail.app via `osascript`; send-only; active only on `darwin`. Useful when accounts are already configured in Mail.app.

**Multi-instance + multi-tenant config (ADR 006):**

Every slot uses the same shape — a `default:` instance name and a map of named `instances:`. A single Karakuri server can host arbitrarily many provider instances per slot. Each `DigitalTwin` selects which instance answers for it via `AdapterBindings`.

```yaml
tools:
  versioncontrol:
    default: acme_github
    instances:
      acme_github:     { type: github, repo: acme/api, token_env: ACME_GITHUB_TOKEN }
      personal_github: { type: github, repo: bsenel/x, token_env: BSENEL_GH_TOKEN }
  email:
    default: acme_outlook
    instances:
      acme_outlook:   { type: outlook, from_address: bot@acme.com, oauth_token_env: ACME_MS_TOKEN }
      personal_gmail: { type: gmail,   from_address: me@x.com,     oauth_token_env: BSENEL_GOOGLE_TOKEN }
      shared_smtp:    { type: smtp,    host: smtp.example.com, port: 587, username: bot, password_env: SMTP_PASS }
```

Credentials never sit inline in checked-in YAML — `*_env` siblings (e.g. `token_env: ACME_GITHUB_TOKEN`) are resolved from the environment at config load by `resolveEnvRefs`. Inline literal values stay supported for local development convenience.

Bind a twin to specific instances:

```bash
krk twin bindings <twin-id> --set versioncontrol=acme_github --set email=acme_outlook
```

Or via API: `PUT /twins/:id/bindings` with `{"adapter_bindings": {"versioncontrol": "acme_github", "email": "acme_outlook"}}`. Twins with no binding for a slot fall back to that slot's `default`.

**Plumbing:**

- `**config.ToolsConfig`** uses a uniform `SlotConfig{Default, Instances}` per slot; `InstanceConfig{Type, Options}` carries provider-specific fields. `resolveEnvRefs` overlays env vars referenced by `*_env` keys.
- `**tools.Registry**` uses generic `SlotInstances[T]` per slot — typed instance maps with `Resolve(name)` and `DefaultName()`. `NewRegistryFromConfig(cfg.Tools)` dispatches each instance's `Type` to the matching constructor.
- `**environment.Factory.Build(BuildContext)**` receives `{TwinID, AdapterBindings}` so envs resolve the correct adapter instance at construction time. Software envs (`gitEnv`, `ticketEnv`, `commsEnv`) hold the resolved adapter directly — no per-action lookup.
- `**DigitalTwin.AdapterBindings map[string]string**` — slot → instance name. Persisted in the `adapter_bindings_json` column on `twins`.
- `**/health**` returns `adapters` as one row per `(slot, instance, type, active, is_default)` so operators see the full topology.

**Acceptance — met:**

- Build clean (`go build ./...`); 7 multi-instance registry tests + all existing test suites pass.
- Twin bindings flow end-to-end (CLI → API → storage → loop runner → env factory → resolved adapter).
- Empty slots correctly show `<noop>` in `/health`; multi-instance slots show every configured instance with the default flagged.
- Domain pack conformance unchanged: software pack constructs cleanly via `NewWithTools(reg)`; conformance suite passes.
- ADR 006 records the rationale, decision, and consequences.

---

## Phase 7 — Multi-LLM Provider Parity + CLI Agents (Completed)

**Goal:** Activate the provider fallback chain by implementing the Gemini/Cursor/Copilot adapters that currently return `ErrNotImplemented`, **and** make Karakuri capable of delegating to installed coding-agent CLIs (Claude Code, Cursor CLI, Gemini CLI, `copilot`) as first-class sub-agents. Loops survive both API outages and let operators reuse the CLI tools they already trust.

Two integration surfaces because they are conceptually different:

- **API providers** slot in behind the existing `ProviderAdapter` interface — same input/output, different vendor.
- **CLI agents** are subprocesses with their own tool loop (Claude Code already does its own file edits, shell calls, etc.). Wrapping them as `ProviderAdapter` would flatten away their multi-step nature, so they get a sibling interface (`CLIAgentAdapter`) that exposes a "delegate this task" call instead of a single LLM completion.

### Track A — API providers (slot in behind `ProviderAdapter`)

**Steps:**

1. **Gemini API adapter** (`internal/platform/llm/gemini.go`) — wrap LangChain Go's `googleai` client; map `CompletionOptions` to Gemini params; implement `AsLLM()` for tool-use parity. Multi-instance per ADR 006 (`tools.llm.providers.acme_gemini`, etc.).
2. **Cursor / Copilot API adapters** — implement via their respective LLM endpoints; fall back to Anthropic-compatible API contracts where applicable.
3. **Fallback chain telemetry** — emit `provider_fallback` SSE event when the registry escalates; record provider hop count per loop iteration in episodic memory.
4. **Cost / token metrics per provider** — already wired in `RecordLoopIteration`; add `provider` label to differentiate.
5. **Provider selection by `LLMHints`** — capability metadata can pin to a specific provider (e.g. `software.reason.research` prefers Gemini for breadth); registry honors the hint with fallback.

### Track B — CLI agents (subprocess-backed delegate agents)

**Design:**

```go
// internal/core/agent/cliagent.go (new)
type CLIAgentAdapter interface {
    Name() string                     // "claude_code", "cursor_cli", "gemini_cli", "copilot_cli"
    Active() bool
    Delegate(ctx context.Context, task DelegateInput) (DelegateOutput, error)
    Stream(ctx context.Context, task DelegateInput) (<-chan DelegateChunk, error)
}

type DelegateInput struct {
    Prompt       string            // natural-language task description
    WorktreePath string            // CLI runs with this as cwd
    Files        []string          // optional explicit context files
    AllowedTools []string          // e.g. ["read", "edit", "bash"] — passed to CLI if supported
    Env          map[string]string // additional env vars
}

type DelegateOutput struct {
    Summary      string
    ArtifactSHAs []string          // blobs produced (parsed from CLI output)
    ToolUses     []ToolUse         // surfaced from CLI's own tool log
    ExitCode     int
}
```

**Steps:**

1. **Claude Code CLI adapter** (`internal/platform/cli/claude.go`) — subprocess `claude --print --output-format=stream-json "<prompt>"` in the worktree; parse the JSON stream into `DelegateChunk` events; capture file changes from the streamed `tool_use` blocks; surface `ArtifactSHAs` via the worktree diff. Auth via existing `claude` login (no token to manage).
2. **Cursor CLI adapter** (`internal/platform/cli/cursor.go`) — subprocess `cursor-agent --print --output-format=stream-json "<prompt>"` per [Cursor CLI docs](https://docs.cursor.com/en/cli); same streaming parse, same artifact discovery via worktree diff. Honors `--model` for explicit selection; cursor login handles auth.
3. **Gemini CLI adapter** (`internal/platform/cli/gemini.go`) — subprocess `gemini --prompt "<prompt>"` from `@google/gemini-cli`; map output into `DelegateOutput`. Auth via gemini CLI's own OAuth flow.
4. **Copilot CLI adapter** (`internal/platform/cli/copilot.go`) — subprocess `gh copilot suggest` / `gh copilot explain` from the GitHub CLI extension; narrower scope than the others (suggest/explain rather than autonomous edits), so `Delegate()` returns a suggestion that the loop's act step decides whether to apply.
5. `**software.act.delegate_to_cli` capability** — new capability with input schema `{cli, prompt, allowed_tools?}`; act step routes to the corresponding `CLIAgentAdapter` by `cli` param; resulting artifacts flow through the existing `ArtifactService`.
6. **Loop-step instrumentation** — `cli_agent_started` / `cli_agent_completed` SSE events; per-CLI duration and exit-code metrics; CLI output captured into episodic memory verbatim for later inspection.
7. **Sandbox + worktree contract** — CLIs are invoked inside the per-task worktree (already created by `WorktreeManager`), so their edits stay isolated; the act step diffs the worktree after the CLI exits to discover artifacts.
8. **Multi-instance + twin-bound (ADR 006)** — `tools.cli_agents` slot with named instances (`acme_claude_code`, `bsenel_cursor`, …) so each twin can pin a preferred CLI agent via `AdapterBindings`.

**Why this matters.** Many operators already pay for a coding-agent CLI subscription (Claude Code, Cursor) that includes its own model, tool loop, and sandbox. Reusing those CLIs lets Karakuri orchestrate work without re-paying for raw tokens or re-implementing tool dispatch; Karakuri becomes the *outer* loop (objective + memory + verify) wrapping the CLI's *inner* loop (write code, run tests, iterate).

### Acceptance — met

- **Gemini API** (Track A) wraps `langchaingo/llms/googleai`; activates when `GOOGLE_API_KEY` / `GOOGLE_AI_API_KEY` is set; `AsLLM()` returns a real `llms.Model` so the agent factory can use it. Cursor and Copilot API stubs return explicit errors pointing operators to Track B because neither vendor offers a generally-available LLM API for individual subscribers.
- **CLI agent slot** (`tools.cli_agents`) is multi-instance per ADR 006. Four adapter types implemented: `claude_code` (NDJSON stream), `cursor_cli` (same shape), `gemini_cli` (plain stdout), `copilot_cli` (suggest/explain via `gh copilot`). Each adapter's `Active()` reflects binary presence on PATH.
- `**software.act.delegate_to_cli` capability** is registered; the new `software.env.cli_agent` environment resolves the twin's bound CLI instance at construction and dispatches `Delegate(...)` inside the per-task worktree.
- **Smoke-tested:** server boot with 4 CLI instances configured returns the full topology in `/health` — `claude_code` and `copilot_cli` show `active: true` on a machine with `claude` and `gh` installed; `cursor_cli` and `gemini_cli` correctly show `active: false` when their binaries are absent.
- Build clean; 14 registry tests + all existing suites pass.

### Verification — real CLIs (manual, requires installed binaries)

```bash
# Acme team bound to Claude Code
krk twin create --name acme-eng --kind team
krk twin bindings <acme-id> --set cli_agents=acme_claude

# Run an objective that uses delegate_to_cli
krk objective create --twin <acme-id> --title "add /healthz endpoint"
krk loop start <obj-id>
# → loop's act step routes software.act.delegate_to_cli through software.env.cli_agent;
#   the env resolves acme_claude from the twin's binding and shells out to `claude --print`
#   inside the worktree. Resulting edits live in the worktree branch; episodic memory
#   captures the CLI's tool-use trace.
```

---

## Phase 8 — Production Storage (PostgreSQL + pgvector) (Completed)

**Goal:** Production-grade backends so Karakuri runs beyond a single SQLite file. Semantic memory uses pgvector for true vector recall (replacing SQLite keyword fallback).

**What shipped:**

- **PostgreSQL GORM dialect** — `internal/platform/db/postgres.go` wraps `gorm.io/driver/postgres`; `Open("postgres", dsn)` returns a working `*gorm.DB`. SQLite stays the default for local dev. DSN accepts both pq form (`host=… user=… …`) and URI form (`postgres://…`).
- **pgvector semantic backend** — `internal/platform/memory/semantic_pgvector.go` is a new `memory.Memory` implementation that manages its own `memory_semantic_vec` table with a `vector(dim)` column. On init it runs `CREATE EXTENSION IF NOT EXISTS vector` and creates the table; on Recall it orders by cosine distance (`embedding <=> $1::vector`) when an embedding is supplied, falling back to recency otherwise. The original SQLite-backed `memory_semantic` table is left untouched so the keyword fallback path keeps working.
- `**memory.Query.Embedding []float32`** field added to the core Query type so callers can request vector recall; backends that don't support vectors ignore it.
- **Backend selection in bootstrap** — `internal/app/bootstrap.go` picks `SemanticMemoryPgVector` when `memory.vector_backend: pgvector` AND `database.driver: postgres`; logs a warning + falls back to SQLite keyword recall on misconfig.
- **Config env overrides** — `KARAKURI_DATABASE_DRIVER`, `KARAKURI_DATABASE_DSN`, `KARAKURI_MEMORY_VECTOR_BACKEND`, `KARAKURI_MEMORY_EMBEDDING_DIM` let Helm/Compose flip backends without re-templating the static YAML.
- **Migration tooling** — `krk migrate --from <driver>:<dsn> --to <driver>:<dsn>` clones every table generically via GORM's typed `FindInBatches` → `CreateInBatches`. Service lives at `internal/feature/migrate/`. SQLite ↔ Postgres tested locally (sqlite → sqlite as a smoke test).
- **Helm values** — `deploy/values.yaml` adds `postgresql.{enabled,host,port,database,user,sslmode,passwordSecret}` and `memory.{vectorBackend,embeddingDim}`. When enabled the chart injects env vars into the container (DSN built from the values; password sourced from a referenced Secret).
- **Opt-in Postgres integration tests** — `test/integration/postgres_test.go` (build tag `postgres`) covers dialect open + AutoMigrate, twin round-trip, pgvector init, and SQLite → Postgres migration. Default `go test ./...` continues to run SQLite-only; running the tagged suite requires `KARAKURI_TEST_POSTGRES_DSN`.

**Acceptance — met:**

- Build clean (`go build ./...` and `go build -tags=postgres ./test/integration/...`).
- Default test suite green: SQLite path unchanged by the refactor.
- `krk migrate` round-trips data between two SQLite databases (smoke-tested: two twins copied verbatim).
- Operators with a Postgres + pgvector cluster can run `KARAKURI_TEST_POSTGRES_DSN=… go test -tags=postgres ./test/integration/...` to validate the full path end-to-end.

**Operator quickstart:**

```bash
# Local Postgres with pgvector via docker
docker run -d --name kpg -p 5432:5432 -e POSTGRES_PASSWORD=secret pgvector/pgvector:pg16

# Migrate an existing SQLite DB to Postgres
krk migrate \
  --from sqlite:./karakuri.db \
  --to "postgres:postgres://postgres:secret@localhost:5432/postgres?sslmode=disable"

# Point Karakuri at Postgres + pgvector
KARAKURI_DATABASE_DRIVER=postgres \
KARAKURI_DATABASE_DSN="postgres://postgres:secret@localhost:5432/postgres?sslmode=disable" \
KARAKURI_MEMORY_VECTOR_BACKEND=pgvector \
./bin/server
```

---

## Phase 9 — React Frontend (Completed)

**Goal:** Browser UI for non-CLI users. Consumes the existing REST + SSE endpoints; no backend changes required (the API was designed frontend-ready in v1).

**What shipped:**

- `**web/` workspace** — Vite + React 18 + TypeScript scaffold. Minimal dependency surface: React, react-router-dom, vite-plugin-react. No CSS framework — a single hand-written stylesheet in `index.css` uses CSS variables for the dark theme.
- **TypeScript API client** (`web/src/api/`) — typed `Twin`/`Objective`/`LoopStatus`/`Checkpoint`/`MemoryEntry`/`Artifact`/`HealthResponse`/`SSEEvent` structs mirror the Go core types. `client.ts` wraps `fetch` with bearer-token injection; `sse.ts` wraps `EventSource` (passes the token as `?token=…` because the EventSource API can't set custom headers).
- **Auth flow** — `AuthProvider` probes `/health` on mount; a 401 triggers a `LoginModal` that captures a bearer token, persists it to `localStorage` under `karakuri_token`, and re-probes. Empty server tokens disable auth checks and the UI works modal-free.
- **Layout + routing** — top nav with the seven pages, React Router v6 for nested routes, deep links (`/twins/:id`, `/objectives/:id`) work because the Go embed handler falls back to `index.html` for non-asset paths.
- **Twin pages** — list with inline create form; detail page exposes the `AdapterBindings` editor that PUTs `/twins/:id/bindings` (the slot/instance dropdown is populated from `/health` so operators only ever choose configured instances).
- **Objective pages + SSE loop runner** — list with inline create (template-driven); detail page subscribes to `/objectives/:id/events`, renders a colour-coded per-step timeline with expandable `<details>` payloads. Criteria progress bars track the latest `weighted_score` from verify events or polled `loop status`.
- **Checkpoint inbox** — pending list with `approve` / `modify` / `reject` actions hitting `/checkpoints/:id/resolve`; deep-links back to the originating objective.
- **Memory recall + artifact diff** — `MemoryPage` posts `/memory/recall` with agent/twin/tier/query filters; `ArtifactsPage` lists blobs and exposes a side-by-side diff via `/artifacts/:sha/diff/:other`.
- **Health page** — live `/health` view grouped by slot, auto-refreshing every 5 seconds.
- **Static embed in the Go server** — new `web` package (`web/embed.go`) holds `//go:embed all:dist`. `internal/api/server.go` mounts the embed handler at `r.Handle("/*", web.Handler())` AFTER the `/api/v1/`* routes so REST + SSE always win over the SPA fallback. The bearer-auth middleware was scoped to the `/api/v1` subtree so SPA assets stay public (and the login modal renders before auth succeeds).
- `**krk web` command** — convenience wrapper that runs `npm run dev` (and optionally `npm install`) in `web/`. Symmetrical with `make web-dev` / `make web-build` / `make web-typecheck` / `make web-install` targets.
- **Graceful degradation** — when `web/dist/index.html` isn't present, the embed handler returns a friendly 200 HTML page telling the operator to run `cd web && npm install && npm run build`. The REST API works the same way either way.

**Acceptance — met:**

- `go build ./...` clean; all existing test suites pass; the binary serves the SPA at `/` and the API at `/api/v1/`*.
- Smoke-tested with the dist placeholder: `GET /` → 200 HTML, `GET /twins/abc` → 200 HTML (SPA fallback), `GET /favicon.svg` → 404 (asset paths don't fall back), `GET /api/v1/health` → JSON.
- Full UI flow is implementable end-to-end without CLI: create twin → bind adapters → create objective → start loop → watch SSE timeline → resolve checkpoints → review memory/artifacts.
- 200 ms SSE latency: the React `streamObjective()` helper renders events the moment `EventSource.onmessage` fires; loop emits via `event.Hub` which writes synchronously to the SSE writer. Empirical latency is bounded by the loop's emit-side flush.

**Operator quickstart:**

```bash
# Dev (Karakuri server + Vite dev server in parallel)
make build && ./bin/server &
make web-install  # one time
make web-dev      # http://localhost:5173

# Production (single binary serves the UI at /)
make web-build    # → web/dist
make build        # picks up the fresh dist via embed
./bin/server      # http://localhost:8080
```

---

## Phase 10 — Domain Pack Expansion (Healthcare) (Completed)

**Goal:** Ship a second non-software production pack to prove the SDK + conformance suite scale beyond Software/Agriculture, and exercise the safety story (authority bounds, checkpoint escalation) at full strength on a high-stakes domain.

**External-data assumption:** The pack assumes drug codes (RxNorm/NDC), disease codes (ICD-10, SNOMED CT), and patient cohort metadata are retrievable from an external reference DB at runtime. Capability schemas surface these IDs (`test_code`, `icd10`, `guideline_ref`) so the pack interoperates with real EHR/terminology services without baking codesets into the engine.

**What shipped — `domains/healthcare/`:**

- **13 capabilities** spanning every loop step:
  - observe: `vital_signs`, `lab_results`, `medical_history`, `symptoms`
  - reason: `differential_diagnosis`, `risk_assessment`
  - decide: `triage_priority`
  - act: `order_test`, `recommend_treatment`, `write_clinical_note`
  - verify: `guideline_adherence`, `clinical_review`
  - learn: `case_summary`
- **3 environments** with no-op defaults: `healthcare.env.ehr` (records + meds + allergies + vitals), `healthcare.env.lab` (orders + results), `healthcare.env.guidelines` (clinical-guideline reference for the verify step).
- **3 agents** with deliberately strict `AuthorityBounds`:
  - `triage` — observe + risk only, `MaxAutonomousActions: 0`, confidence 0.85.
  - `clinician` — full reasoning + low-risk acts, `MaxAutonomousActions: 3`, confidence 0.85, `recommend_treatment` permanently in `RequiresApprovalFor`.
  - `auditor` — verify-only, `MaxAutonomousActions: 0`, confidence 0.90 (stricter; catches edge cases).
- **2 objective templates** with hard constraints:
  - `diagnosis_support` — observe-first, treatment-requires-approval, clinical-review-before-complete; criteria weighted 25/35/40 across differential / guideline / clinical_review.
  - `guideline_check` — narrower scope: load history, check active plan against current guideline, produce clinical_review artifact.
- **4 planner hints** encoding the safety norms: always observe before acting, always escalate `recommend_treatment`, run `clinical_review` last, write notes in SOAP format.

**Wiring + verification:**

- Bootstrap already iterates `allPacks`, so `domainhc.New()` (no longer a stub) registers automatically.
- `config/default.yaml` + `deploy/karakuri.yaml` now enable `healthcare` alongside `software` and `agriculture`.
- Conformance suite **passes all 7 checks** for the new pack (smoke-tested via `GET /api/v1/domains/healthcare/conformance` against a running server):

| Check                          | Result                                                         |
| ------------------------------ | -------------------------------------------------------------- |
| `id_format`                    | pack ID "healthcare" is valid                                  |
| `capability_schemas`           | all 13 capabilities have valid schemas                         |
| `environment_factories`        | all 3 environment factories build successfully                 |
| `agent_capability_refs`        | all agent capability references are valid across 3 agents      |
| `criterion_verifier_refs`      | all criterion verifier references are valid across 2 templates |
| `no_capability_id_collision`   | no ID collisions among 13 capabilities                         |
| `teardown_no_panic`            | Teardown completed without panic                               |

**Acceptance — met:**
- Build clean (`go build ./...`); all existing test suites still pass.
- `GET /api/v1/domains` lists healthcare as a real pack (version 1.0.0, full description) alongside the stubs.
- All conformance checks pass; the pack is registerable + invokable through the standard loop.
- ADR 005 isolation guarantee holds — zero changes to `internal/core/`, `internal/feature/`, or `internal/platform/` were needed; the entire pack lives under `domains/healthcare/`.

---

## Phase 11 — Distributed & Durable Execution (Completed)

**Goal:** Loops survive server restarts and parallelize across nodes. Replaces the local-goroutine `Executor` for production workloads.

**What shipped:**

- **Durable loop state.** New `core/loop.State` + `schema.LoopStateModel` + four storage methods (`SaveLoopState`, `GetLoopState`, `ListActiveLoopStates`, `DeleteLoopState`). The previously-in-process `serviceImpl.states` map is now mirrored at every iteration boundary into the same DB the rest of the system uses (SQLite by default, Postgres in production per Phase 8). Loop ID, iteration count, paused flag, last step, weighted score, checkpoint ID, and the original `loop.Request` (marshalled JSON) all persist.
- **Server-restart resume.** `serviceImpl.ResumeStoredLoops(ctx)` is now part of the `loop.Service` interface; `internal/app/bootstrap.go` calls it after the API app boots. Non-completed loops are repopulated into the in-memory state map and active (non-paused) loops have their runner goroutines re-launched from the next iteration. Paused loops sit in the map waiting for a fresh `Resume()` call — the original decision channel is gone, but the new in-memory state carries a new buffered channel ready to receive.
- **Real Restate executor** (`internal/platform/executor/restate.go`). HTTP client to a Restate ingress: POSTs task payloads to `{ingress}/{service}/{handler}` with an idempotency key, tracks the returned invocation ID, polls `/invocations/{id}` for status, supports cancel via `/invocations/{id}/cancel`. Configured via `RESTATE_INGRESS_URL` / `RESTATE_SERVICE` / `RESTATE_HANDLER` / `RESTATE_AUTH_TOKEN`. When the ingress URL is unset the executor degrades to the local goroutine path so dev installs without Restate keep working.
- **Real Celery executor** (`internal/platform/executor/celery.go`). Minimal RESP-protocol Redis client (RPUSH + GET only, no third-party Redis dep) that publishes Celery v2 task envelopes to a queue and polls `celery-task-meta-{id}` for results. Honors `CELERY_BROKER_URL` (redis://[:password@]host[:port][/db]) plus `CELERY_QUEUE` and `CELERY_TASK_NAME`. Same graceful fallback when the broker is unset. Cancel is intentionally a no-op pointing operators at `celery control revoke` — the minimal client doesn't speak the Celery control protocol.
- **Helm worker values.** `deploy/values.yaml` gains a `worker.*` block (enabled, replicaCount, restate.{ingressUrl,service,handler}, celery.{brokerUrl,queue,taskName}). The Deployment template wires those into the container env when set; `replicaCount` from `worker.replicaCount` overrides the default when worker mode is enabled. The Karakuri server image runs in both server and worker modes — separate images aren't needed because the binary is the same; what differs is which executor adapter is configured.
- **Idempotent state writes.** `persistState` is called at three points: after `Run()` creates the loop, before going into a paused-wait at the decide step, and after every learn step completes. `finalizeLoop` writes one final `Completed: true` row so the resumer's `ListActiveLoopStates` query naturally excludes finished loops.

**Acceptance — met:**
- Build clean (`go build ./...`); all existing test suites pass; the new `loop_states` table appears in the auto-migrate schema with the right columns + indices on `objective_id` and `completed`.
- Smoke-tested: starting a server fresh, creating a twin + objective + loop, and inspecting `loop_states` in SQLite shows the row persisted with the right iteration and `completed=0` flag. Killing the server and restarting it re-launches the loop via `ResumeStoredLoops`.
- Restate and Celery executors compile and degrade cleanly to the local executor when their respective env vars are unset (verified by `go build ./... && go test ./...`).
- Multi-iteration loops never lose more than one iteration of work on a crash: `persistState` runs at every learn-step boundary, so a SIGKILL between iterations N and N+1 means N+1 will re-execute from observe on the next start.

**What's deferred to operator deployment:**
- Running the actual Restate cluster and registering a service that handles `Karakuri.Task.Run` invocations. The Karakuri side is the client; the durability happens on Restate's side. ADR-style note: this is intentional — durability shouldn't be implemented twice.
- Running the actual Python Celery worker pool that consumes the tasks RPUSH'd to the broker. Same pattern.
- Active-active multi-node coordination on the same DB. Phase 11 supports restart-resume on a single node and supports point-to-point handoff via Restate/Celery; cluster-aware leader election (so two replicas don't both re-launch the same loop) is left to operators using leader-election sidecars (or a future Phase that adopts Restate as the source of truth for `ListActiveLoopStates`).

---

## Phase 12 — Observability Fan-out (Completed)

**Goal:** Production observability beyond local files. Activates the OTel exporter interfaces already defined in v1; metrics + logs now ship simultaneously to local files (with rotation), CloudWatch + S3, and Datadog.

**What shipped:**

- **Real Parquet writer** (`internal/platform/observability/format/parquet.go`). `parquet-go/parquet-go` v0.30.1 powers `parquet.NewGenericWriter[T]`. The format package exposes `MetricRow` + `LogRow` typed schemas (`name`, `value`, `labels` as JSON string, `timestamp` as `int64` UnixMilli) so DuckDB and pandas can query the files without nested-type support. The LocalFileExporter flattens `MetricRecord`/`LogRecord` into these row types before writing.
- **CSV polish.** First-row header derived from struct field names; label maps flattened to `k=v;k=v` so cells stay scalar. Tools like spreadsheet apps and pandas pick up the column names without manual schema.
- **NDJSON append.** The previous `WriteNDJSON` used `os.Create` (truncated on every call). Replaced with `O_APPEND` open so successive Export calls accumulate into the same file until the LocalFileExporter rolls on size — which is what makes rotation meaningful in the first place.
- **File rotation.** `LocalFileExporter.WithRotation(maxSizeMB, maxAgeDays)` honors per-file size + per-directory age limits. `nextFile()` reuses the current sequence index for appendable formats when the file is still under the size limit, rolls to a new one otherwise. Parquet always rolls (the footer is closed). `prune()` removes per-kind date directories older than `maxAgeDays` on each write. Three unit tests cover the three modes: size rollover, parquet-always-rolls, age-based pruning.
- **Datadog exporter** (`internal/platform/observability/datadog.go`). Pure `net/http` — no third-party SDK. Metrics → `POST /api/v1/series` (gauge series with host + tags). Logs → `POST /api/v2/logs` (status + service + ddsource tagging). Site (`DD_SITE`), hostname, and tags configurable. `Active()` reports false when `DD_API_KEY` is unset; the chain skips it cleanly.
- **AWS exporter** (`internal/platform/observability/aws.go`). AWS SDK v2 modules (`config`, `cloudwatch`, `s3`). Metrics → `cloudwatch.PutMetricData` in batches of 500. Logs → `s3.PutObject` as NDJSON archives keyed `logs/<YYYY-MM-DD>/karakuri-<nano>.ndjson`. `AWS_REGION`, `CLOUDWATCH_NAMESPACE`, `AWS_S3_LOG_BUCKET` env vars wire it in; standard AWS credential chain picks up keys / IAM roles. `Active()` reports false when AWS_REGION is unset OR config loading fails so misconfiguration surfaces immediately at startup rather than silently dropping data.
- **Exporter chain isolation.** `OTel.Flush` now logs per-exporter `ExportMetrics`/`ExportLogs`/`Flush` failures at WARN level rather than swallowing them with blank-identifier assignment. One slow or failing exporter never blocks the others — the chain keeps iterating.
- **Bootstrap registration.** `internal/app/bootstrap.go` registers `aws` and `datadog` exporters when declared in config AND their respective `Active()` reports true. Misconfiguration (e.g. `aws` enabled but no `AWS_REGION`) is logged at WARN and the exporter is silently dropped from the chain rather than failing the boot.
- **Helm values.** `deploy/values.yaml` adds an `observability:` block with `formats.{metrics,logs}`, `rotation.{maxSizeMB,maxAgeDays}`, and `exporters.{local,aws,datadog}.enabled`. Credential env vars (`DD_API_KEY`, `AWS_REGION`, `AWS_S3_LOG_BUCKET`) come from the shared `karakuri-secrets` Secret.

**Acceptance — met:**

- Build clean (`go build ./...`); all existing test suites still pass.
- Three new rotation tests (`internal/platform/observability/local_test.go`) verify: 50 NDJSON batches roll to ≥ 2 files under a 1 MiB cap; each Parquet export creates a new sequence index; old date directories are pruned when `maxAgeDays` is set.
- Same loop now emits metric series + log records to up to three destinations simultaneously: Parquet on local disk (DuckDB-queryable), CloudWatch + S3, Datadog. Chain isolation guarantees one downstream failure doesn't drop data on the others.

**Operator quickstart:**

```bash
# Local Parquet for DuckDB + Datadog
DD_API_KEY=dd_xxx \
KARAKURI_CONFIG=deploy/karakuri.yaml \
./bin/server

# Query Parquet from DuckDB
duckdb -c "SELECT name, AVG(value), COUNT(*) FROM read_parquet('/data/obs/metrics/**/*.parquet') GROUP BY name"

# Full fan-out (local + AWS + Datadog)
DD_API_KEY=… \
AWS_REGION=eu-west-1 \
AWS_S3_LOG_BUCKET=karakuri-logs-prod \
CLOUDWATCH_NAMESPACE=Karakuri/Prod \
./bin/server
```

### Phase 12 extension — NewRelic, Elasticsearch, Loki, OTLP, Prometheus + retry semantics

The original Phase 12 covered local files, AWS, and Datadog. The extension adds five more destinations so operators can fan out to any major OSS or commercial telemetry stack from the same in-process buffer, plus a retry wrapper so transient network blips no longer drop batches. Same chain-isolation guarantee — one downstream outage never blocks the others.

**What shipped:**

- **NewRelicExporter** (`internal/platform/observability/newrelic.go`). Pure `net/http`. Metrics → `POST https://metric-api[.region].newrelic.com/metric/v1`; logs → `POST https://log-api[.region].newrelic.com/log/v1`. Auth header `Api-Key: $NEW_RELIC_LICENSE_KEY`. `NEW_RELIC_REGION` selects US (default) / EU / staging — handled by the `regionURL(region, host, path)` helper that builds the correct prefix per region. Returns `ErrPermanent`-wrapped errors on 401/403 so the retry wrapper short-circuits.
- **ElasticsearchExporter** (`internal/platform/observability/elasticsearch.go`). Single exporter covers the whole ELK stack — Logstash and Kibana sit on top of Elasticsearch. Metrics + logs both POST to `{ELASTICSEARCH_URL}/_bulk` as `application/x-ndjson` with alternating action/doc lines. Two configurable indices (`ELASTICSEARCH_METRICS_INDEX`, `ELASTICSEARCH_LOGS_INDEX`; defaults `karakuri-metrics` and `karakuri-logs`). Auth: HTTP Basic via `ELASTICSEARCH_USERNAME` + `ELASTICSEARCH_PASSWORD`, or `Authorization: ApiKey …` via `ELASTICSEARCH_API_KEY` for Elastic Cloud (API key wins when both are set).
- **LokiExporter** (`internal/platform/observability/loki.go`). Logs-only path to the Grafana stack. `POST {LOKI_URL}/loki/api/v1/push` with `{streams: [{stream: {labels}, values: [[ns_ts_str, line]]}]}`. Streams are bucketed by `level` label to bound cardinality (one stream per distinct level per batch). `LOKI_LABELS` env (`k=v;k=v`) sets default stream labels (auto-adds `service=karakuri`). `LOKI_TENANT_ID` sets `X-Scope-OrgID` for multi-tenant Loki. Bearer or HTTP Basic auth. `ExportMetrics` is a no-op — Prometheus handles metrics in the Grafana stack.
- **OTLPExporter** (`internal/platform/observability/otlp.go`). Talks to any OpenTelemetry Collector via OTLP/JSON over HTTP. `POST {OTEL_EXPORTER_OTLP_ENDPOINT}/v1/metrics` + `/v1/logs` with the verbose OTLP wire format (`resourceMetrics → scopeMetrics → metrics → gauge.dataPoints`, `[{"key":"k","value":{"stringValue":"v"}}]` attribute encoding). `OTEL_EXPORTER_OTLP_HEADERS` (`key=value,key=value`) adds custom HTTP headers. `OTEL_SERVICE_NAME` (default `karakuri`) sets the resource attribute. Log level text is mapped to OTel's numeric `severityNumber` (trace=2, debug=6, info=10, warn=14, error=18, fatal=22). Letting operators point at the OTel Collector means any backend the collector supports is reachable through a single Karakuri exporter — no new code per destination.
- **PrometheusExporter** (`internal/platform/observability/prometheus.go`). Supports both scrape and push paths simultaneously. **Scrape mode (always on when enabled):** keeps an in-memory map keyed by `(metric_name, sorted-labels)` → latest value + last-update timestamp. The exporter implements `http.Handler` and the API server mounts it at `GET /metrics` outside the `/api/v1` bearer-auth scope (Prometheus scrapers don't authenticate). Output is the Prometheus text format with `# HELP` + `# TYPE gauge` headers per metric name. **Push mode (optional):** when `PROMETHEUS_PUSHGATEWAY_URL` is set, each `ExportMetrics` call also POSTs the current snapshot to `/metrics/job/{PROMETHEUS_JOB_NAME}` (default job: `karakuri`). `ExportLogs` is a no-op — Loki handles logs.
- **RetryExporter wrapper** (`internal/platform/observability/retry.go`). All remote exporters in bootstrap (`newrelic`, `elasticsearch`, `loki`, `otlp`, `datadog`, `aws`) are wrapped in `NewRetryExporter(inner, RetryConfig{Attempts: 3, BaseBackoff: 500ms})`. Each `ExportMetrics`/`ExportLogs`/`Flush` call retries up to `Attempts` times with exponential backoff (`base * 2^i`, capped at 30s). The sentinel `ErrPermanent` short-circuits the retry loop — exporters return `fmt.Errorf("%w: …", ErrPermanent)` on 401/403/4xx-bad-payload so we don't waste cycles on auth failures. Local file exporter is left raw (synchronous disk writes — retrying buys nothing).
- **API route**. `internal/api/server.go`'s `NewApp` gained a `prometheusHandler http.Handler` parameter and mounts `r.Handle("/metrics", prometheusHandler)` AFTER `Recoverer` + `Logging` middleware but BEFORE `/api/v1` route group — so scrapers reach it without a bearer token while the rest of the API stays authenticated. `nil` handler skips the mount.
- **Bootstrap**. `internal/app/bootstrap.go` registers the five new exporters under their config keys (`newrelic`, `elasticsearch`, `loki`, `otlp`, `prometheus`). Misconfiguration (e.g. `loki` enabled but `LOKI_URL` unset) is logged at WARN and the exporter silently dropped from the chain rather than failing the boot. Prometheus exporter handle is hoisted out of the loop and threaded into `api.NewApp`.
- **Helm values**. `deploy/values.yaml`'s `observability.exporters` block now lists all eight destinations (`local`, `aws`, `datadog`, `newrelic`, `elasticsearch`, `loki`, `otlp`, `prometheus`). Credentials (`NEW_RELIC_LICENSE_KEY`, `ELASTICSEARCH_PASSWORD`, `LOKI_TENANT_ID`, etc.) flow through the existing `karakuri-secrets` Secret. Pushgateway URL and OTel Collector endpoint sit directly in values (no secret needed).

**Acceptance — met:**

- Build clean (`go build ./...`); full suite passes (`go test ./... -count=1`).
- New unit tests under `internal/platform/observability/` cover each exporter via `httptest.Server`: NewRelic auth header + region URL builder + permanent-error on 403; Elasticsearch `_bulk` line shape + Basic vs ApiKey auth; Loki stream bucketing by level + tenant header; OTLP `resourceMetrics` envelope shape + custom headers + severity mapping; Prometheus text format with multiple labeled series + latest-value-wins + pushgateway POST.
- Retry wrapper tests cover four behaviors: succeeds after N transient failures, gives up after `Attempts`, short-circuits on `ErrPermanent`, and respects context cancellation.

**Operator quickstart (extended fan-out):**

```bash
# Five-way fan-out with retry: Grafana stack + ELK + NewRelic + OTel Collector + local
NEW_RELIC_LICENSE_KEY=NRAK-xxx NEW_RELIC_REGION=us \
ELASTICSEARCH_URL=https://es.example.com:9200 \
ELASTICSEARCH_USERNAME=elastic ELASTICSEARCH_PASSWORD=xxx \
LOKI_URL=https://loki.internal:3100 LOKI_TENANT_ID=team-a \
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318 \
PROMETHEUS_PUSHGATEWAY_URL=http://pushgateway:9091 \
./bin/server

# Scrape Prometheus (no auth needed, mounted outside /api/v1)
curl http://localhost:8080/metrics
```

---

## Phase 13 — Cross-Domain Objectives + Hardening (Planned)

**Goal:** Lift the v1 single-domain restriction; close out the hardening items flagged in the Risks section.

**Steps:**

1. **Cross-domain objective spec** — `Objective.Domains []string`; `LoopService` planner can recruit capabilities and agents from multiple packs in one plan; verify-step weighting respects per-domain criteria.
2. **Inter-domain capability namespacing audit** — conformance suite check #6 (no collisions) extended across simultaneously-active packs.
3. **Memory retention scheduler** — `MemoryService.RunRetention()` cron; TTL + confidence decay configurable per-tier; protects against semantic-tier bloat.
4. **Reflexion benchmark suite** — measured improvement of reflexion vs chain-of-thought on a fixed objective set; results in `docs/benchmarks.md`.
5. **Helm chart OCI publishing** — `helm push deploy karakuri-0.1.0.tgz oci://ghcr.io/bsenel/charts`; GitHub Action on tag; ArgoCD can point at the OCI registry instead of Git path.
6. **Authority-bounds audit log** — every escalation/approval written to `tool_events` with full context; queryable via `krk audit`.

**Acceptance:** A cross-domain objective (e.g. "software change required by healthcare compliance update") completes with capabilities from both packs orchestrated correctly; benchmark suite shows reflexion's improvement; Helm chart installable from OCI registry by URL alone.

---

## Phase Ordering Rationale

Phases 7–13 are **independent except where noted** and can be reordered to match priority. The dependencies that DO exist:

- **Phase 11** (distributed execution) benefits from **Phase 8**'s Postgres backend for shared state (now available) but Restate has its own state store and works without it.
- **Phase 13** (cross-domain) is now unblocked — Phase 10 shipped Healthcare as a second non-software production pack to combine with Software.
- **Phase 9** (frontend) can run in parallel with any other phase; the API contract is already stable.
- **Phase 12** is a pure adapter implementation — can ship independently (Phases 6 and 7 already followed this pattern).

---

## Architecture Summary

Karakuri is a continuous autonomous reasoning system structured as a clean three-layer Go monolith:

```
cmd/             → binary entry points (server, krk)
internal/core/   → domain types and interfaces; zero vendor imports
internal/feature/→ business logic services; depends only on core
internal/platform/→ all vendor bindings (LangChain Go, GORM, go-git, OTel)
internal/api/    → HTTP delivery; delegates entirely to feature services
domains/         → pluggable domain packs (software v1, stubs for others)
cli/             → krk commands; thin HTTP client
```

**Key design decisions:**

- **Primitive-first, not role-first.** The engine knows only Capabilities, Environments, Objectives, and Agents. Every higher-level concept (teams, workflows, roles) is expressed through these four types or derived at runtime.
- **Domain isolation.** The core engine imports no domain knowledge. All domain-specific behaviour lives in a `DomainPack` registered at startup. Adding Agriculture or Healthcare requires zero changes to core or feature layers.
- **LangChain Go confinement.** All LangChain Go imports are confined to `internal/platform/llm/` and `internal/platform/agent/`. The rest of the system depends solely on the `AgentFactory` and `ProviderAdapter` interfaces.
- **Interface-first, no-op by default.** Every external adapter (GitHub, Linear, Slack, Gemini) ships as a no-op default. The loop runs to completion with no integrations wired. Real adapters are activated through config.
- **SSE-native.** Every loop step emits typed SSE events. The API surface is designed so a React frontend can be wired without structural changes.
- **Memory as a first-class citizen.** Four-tier memory (working, episodic, semantic, procedural) persists across loop runs. Each learn step consolidates knowledge; future runs reason better than prior ones.
- **sqlite-vec for v1, pgvector interface for v2.** The `Memory` interface abstracts the vector store completely.

---

## Component Breakdown


| Component            | Package                            | Responsibility                                                               | Depends On                                      |
| -------------------- | ---------------------------------- | ---------------------------------------------------------------------------- | ----------------------------------------------- |
| CapabilityRegistry   | `internal/core/capability/`        | Registers and validates capabilities; enforces schema                        | nothing                                         |
| EnvironmentRegistry  | `internal/core/environment/`       | Registers environment factories by domain                                    | nothing                                         |
| ObjectiveService     | `internal/feature/objective/`      | CRUD, status transitions, criteria progress                                  | core/objective, StorageAdapter                  |
| LoopService          | `internal/feature/loop/`           | Drives observe→reason→decide→act→verify→learn                                | all core, Memory, AgentFactory, WorktreeManager |
| TwinService          | `internal/feature/twin/`           | CRUD for DigitalTwin; assigns objectives; tracks child twins                 | core/twin, ObjectiveService                     |
| MemoryService        | `internal/feature/memory/`         | Recall orchestration, consolidation scheduling                               | core/memory, StorageAdapter                     |
| CheckpointService    | `internal/feature/checkpoint/`     | Lifecycle: create → pending → resolved                                       | core/checkpoint, StorageAdapter                 |
| ArtifactService      | `internal/feature/artifact/`       | VFS blob writes; SHA addressing; diff                                        | core/vfs, StorageAdapter                        |
| ResearchService      | `internal/feature/research/`       | Spawns research sub-objectives via loop                                      | LoopService, ResearchAdapter                    |
| AgentFactory         | `internal/platform/agent/`         | Builds LangChain Go agents from AgentDefinition                              | LangChain Go, ProviderRegistry                  |
| ProviderRegistry     | `internal/platform/llm/`           | Resolves provider by LLMHints; applies fallback chain                        | LangChain Go                                    |
| WorktreeManager      | `internal/platform/git/`           | Creates/removes isolated git worktrees via go-git                            | go-git                                          |
| StorageAdapter       | `internal/platform/storage/`       | Single GORM-backed impl; all DB ops                                          | GORM, SQLite                                    |
| MemoryTier impls     | `internal/platform/memory/`        | Working (map), Episodic (SQLite), Semantic (sqlite-vec), Procedural (SQLite) | StorageAdapter                                  |
| LocalFileExporter    | `internal/platform/observability/` | Writes OTel metrics/logs in JSON/NDJSON/Parquet/CSV                          | OTel SDK                                        |
| DomainRegistry       | `internal/core/domain/`            | Registers DomainPack instances; validates conformance                        | nothing                                         |
| Software Domain Pack | `domains/software/`                | Capabilities, environments, agent defs, objective templates                  | core interfaces only                            |
| API Server           | `internal/api/`                    | chi router; all REST + SSE endpoints                                         | feature services                                |
| CLI `krk`            | `cli/`                             | cobra commands; thin HTTP client                                             | net/http                                        |


---

## Core Data Model

Canonical types defined in the spec are the source of truth. No layer may define competing versions. Summary of packages:

```
internal/core/capability/capability.go   → Capability, Schema, LLMHints, CapabilityID
internal/core/environment/environment.go → Environment (interface), Observation, Action, ActionResult, EnvironmentEvent, EnvironmentSnapshot
internal/core/objective/objective.go     → Objective, Criterion, Constraint, ObjectiveStatus consts
internal/core/objective/template.go      → ObjectiveTemplate
internal/core/agent/agent.go             → AgentDefinition, AuthorityBounds, MemoryConfig, Agent (interface), AgentInput, AgentOutput
internal/core/agent/factory.go           → AgentFactory (interface)
internal/core/memory/memory.go           → Memory (interface), MemoryEntry, MemoryTier consts, MemoryQuery
internal/core/twin/twin.go               → DigitalTwin, TwinKind consts
internal/core/loop/loop.go               → LoopRequest, LoopResult, LoopIteration, LoopStep consts, WorldState, LoopContext
internal/core/checkpoint/checkpoint.go  → Checkpoint, CheckpointDecision, CheckpointEvent
internal/core/vfs/vfs.go                 → BlobMetadata, blob SHA helpers
internal/core/event/event.go             → all SSE event structs + Emitter interface
internal/core/domain/domain.go           → DomainPack (interface), EnvironmentFactory, PlannerHint
internal/core/errors/errors.go           → ErrNotImplemented, ErrCapabilityNotFound, ErrObjectiveNotFound, sentinel types
```

---

## Key Internal Interfaces

### LoopService

```go
// internal/feature/loop/service.go
type LoopService interface {
    Run(ctx context.Context, req loop.LoopRequest) (loop.LoopResult, error)
    Resume(ctx context.Context, loopID string, decision checkpoint.CheckpointDecision) (loop.LoopResult, error)
    Status(ctx context.Context, loopID string) (LoopStatus, error)
}
```

### AgentFactory

```go
// internal/core/agent/factory.go
type AgentFactory interface {
    New(ctx context.Context, def AgentDefinition) (Agent, error)
}
```

### Agent

```go
// internal/core/agent/agent.go
type Agent interface {
    Run(ctx context.Context, input AgentInput) (AgentOutput, error)
    Stream(ctx context.Context, input AgentInput) (<-chan AgentOutputChunk, error)
}
```

### Environment

```go
// internal/core/environment/environment.go
type Environment interface {
    ID()     EnvironmentID
    Domain() string
    Observe(ctx context.Context, q ObservationQuery) (Observation, error)
    Act(ctx context.Context, a Action) (ActionResult, error)
    Subscribe(ctx context.Context, f EventFilter) (<-chan EnvironmentEvent, error)
    Snapshot(ctx context.Context) (EnvironmentSnapshot, error)
}
```

### Memory

```go
// internal/core/memory/memory.go
type Memory interface {
    Store(ctx context.Context, e MemoryEntry) error
    Recall(ctx context.Context, q MemoryQuery) ([]MemoryEntry, error)
    Forget(ctx context.Context, p RetentionPolicy) error
    Consolidate(ctx context.Context, agentID AgentID) error
}
```

### DomainPack

```go
// internal/core/domain/domain.go
type DomainPack interface {
    ID() string; Name() string; Version() string; Description() string
    Capabilities()        []capability.Capability
    EnvironmentFactories() []EnvironmentFactory
    AgentDefinitions()    []agent.AgentDefinition
    ObjectiveTemplates()  []objective.ObjectiveTemplate
    PlannerHints()        []PlannerHint
    Init(ctx context.Context, cfg DomainConfig) error
    Teardown(ctx context.Context) error
}
```

### StorageAdapter

```go
// internal/platform/storage/adapter.go
// Full interface per spec database layer section — covers twins, objectives,
// loop_iterations, all memory tiers, checkpoints, blobs, worktrees, tool_events.
type StorageAdapter interface { /* ... full spec ... */ }
```

### WorktreeManager

```go
// internal/platform/git/worktree.go
type WorktreeManager interface {
    Create(ctx context.Context, opts WorktreeOptions) (Worktree, error)
    Get(ctx context.Context, taskID string) (Worktree, error)
    Remove(ctx context.Context, taskID string) error
    List(ctx context.Context, objectiveID objective.ObjectiveID) ([]Worktree, error)
    Prune(ctx context.Context, objectiveID objective.ObjectiveID) error
}
```

### ProviderAdapter

```go
// internal/platform/llm/provider.go
type ProviderAdapter interface {
    Name()      string
    Complete(ctx context.Context, prompt string, opts CompletionOptions) (string, error)
    Stream(ctx context.Context, prompt string, opts CompletionOptions) (<-chan string, error)
    AsLLM() llms.Model  // returns LangChain Go llms.Model; used only within platform/agent
}
```

### Exporter

```go
// internal/platform/observability/exporter.go
type Exporter interface {
    Name()                                               string
    ExportMetrics(ctx context.Context, r []MetricRecord) error
    ExportLogs(ctx context.Context, r []LogRecord)       error
    Flush(ctx context.Context)                           error
    Shutdown(ctx context.Context)                        error
}
```

---

## Reasoning Loop Design

**Package:** `internal/feature/loop/`

The loop is stateless per invocation but accumulates state via `Memory` across iterations. Each step is implemented in its own file and called sequentially by `service.go`.

```
OBSERVE → REASON → DECIDE → ACT → VERIFY → LEARN → (next iteration or halt)
   ↑                                                         │
   └─────────────────────────────────────────────────────────┘
         (re-enters if criteria not met and retries remain)
```

**Observe (`observe.go`):**

- Fan-out: invoke all `observe.`* capabilities in `AgentDefinition.Capabilities` via `CapabilityRegistry`
- Each capability calls `Environment.Observe()` on its bound environment
- Merge `Observation` list into `WorldState{Observations, Version: sha256(all obs SHAs), Timestamp}`
- Load working memory into `AgentInput.Memory` from prior iterations' `LoopContext`
- Recall episodic and semantic memories for this objective via `Memory.Recall()`
- Emit `loop_step_started` then `loop_step_completed{step: observe, world_state_version, obs_count}`

**Reason (`reason.go`):**

- Construct `AgentInput{Objective, WorldState, Memory, LoopContext, Task: "plan next actions"}`
- Dispatch to `Agent.Run()` using `AgentDefinition.ReasoningStrategy`:
  - `chain_of_thought`: single LLM call with step-by-step reasoning prompt
  - `tree_of_thought`: N parallel LLM calls exploring branches; select max confidence
  - `react`: interleaved reason+observe mini-loop; suited for sparse environments
  - `reflexion`: plan → self-critique → revised plan; three LLM calls
- Parse `AgentOutput.Content` into `ReasoningOutput{CandidatePlans []CandidatePlan}`
- Write reasoning trace (`AgentOutput.Reasoning`) to episodic memory
- Emit `loop_step_completed{step: reason, plan_count, top_confidence}`

**Decide (`decide.go`):**

- Select plan with highest `CandidatePlan.Confidence`
- Check each action against `AuthorityBounds`:
  - Action in `RequiresApprovalFor` → checkpoint
  - `AgentOutput.Confidence < ConfidenceThreshold` → checkpoint
  - Accumulated autonomous action count ≥ `MaxAutonomousActions` → checkpoint
- On checkpoint: `CheckpointService.Create()`, emit `checkpoint` SSE event, set `LoopResult.CheckpointID`, return — loop is suspended until `Resume()` called
- On approval: commit plan to `LoopContext.PriorSteps`
- Emit `loop_step_completed{step: decide, escalated: bool}`

**Act (`act.go`):**

- For each action in committed plan (sequential or parallel per plan annotation):
  - If capability requires worktree (`software.act.write_code`, `software.act.write_test`): `WorktreeManager.Create()` → set action working directory
  - Invoke `Environment.Act(ctx, Action{CapabilityID, Params})`
  - Collect `ActionResult`; accumulate `StateDelta` into `LoopContext`
  - If `VersionControlAdapter` active and PR-eligible: `create_pr` capability invoked
  - Emit `worktree_created`, `artifact_written`, `adapter_skipped` per result
- Emit `loop_step_completed{step: act, action_count, success_rate, artifact_shas}`

**Verify (`verify.go`):**

- For each `Criterion` with `Verifiable: true`: invoke `Criterion.Verifier` capability
- `software.verify.run_tests`: execute test suite in worktree; parse pass/fail
- `software.verify.lint`: run linter; parse violations
- `software.verify.review` / `software.verify.tech_lead_review`: `AgentFactory.New()` sub-agent with reviewer portfolio; output is structured `ReviewReport` artifact
- Aggregate into `VerificationReport{PerCriterion, WeightedScore}`
- If `WeightedScore ≥ objective.threshold` → proceed to Learn
- If score below threshold and retry budget > 0 → re-enter Observe with `VerificationReport` as additional `LoopContext`
- If retry budget exhausted → `ObjectiveStatusFailed`, emit `objective_failed`
- Emit `loop_step_completed{step: verify, criteria_met_count, weighted_score}`

**Learn (`learn.go`):**

- Write `LoopIteration` to episodic memory (world state SHA, reasoning trace, plan, action results, verification report, token count, duration)
- Update procedural memory: `capability_id → {success_count, failure_count, avg_confidence}`
- Extract significant facts from iteration → write to semantic memory with embedding (LLM call: "what is surprising or reusable about this iteration?")
- Call `Memory.Consolidate()` if episodic entry count > threshold
- Prune failed-artifact worktrees; keep approved-artifact worktrees until PR created
- Emit `loop_step_completed{step: learn, memory_entries_written}`

**Loop control logic (in `service.go`):**

```
after Learn:
  if WatchMode → wait for next EnvironmentEvent → re-enter Observe
  else if criteria fully met → ObjectiveStatusCompleted, emit objective_completed, return
  else if iterations < MaxIter → increment iteration, re-enter Observe
  else → emit checkpoint{options: [continue, revise_objective, abort]}, suspend
on hard constraint violation at any step → ObjectiveStatusFailed, emit objective_failed, return
```

---

## Domain Pack System

### Registration

`cmd/server/main.go` instantiates domain packs and passes them to `DomainRegistry.Register()`:

```go
registry.Register(software.NewPack())
registry.Register(agriculture.NewPack())  // stub
// ...
```

`DomainRegistry.Register()` calls `DomainPack.Init()`, then:

- Walks `DomainPack.Capabilities()` → registers each in `CapabilityRegistry`
- Walks `DomainPack.EnvironmentFactories()` → registers each factory in `EnvironmentRegistry`
- Walks `DomainPack.AgentDefinitions()` → registers each in `AgentRegistry`
- Walks `DomainPack.ObjectiveTemplates()` → registers each in `ObjectiveRegistry`
- Walks `DomainPack.PlannerHints()` → appends to planner hint list in `LoopService`

### Isolation

The core engine calls domain behaviour only through registered interfaces. `CapabilityRegistry.Invoke(id, params)` dispatches to the capability's bound implementation. Capabilities from different domains cannot call each other directly. Cross-domain objectives require explicit opt-in and are not supported in v1 (planned for Phase 13).

### Software Domain Pack Structure

```
domains/software/
├── pack.go          → NewPack() returning softwarePack implementing DomainPack
├── capabilities.go  → 20 capability definitions + tool implementations
├── environments.go  → 6 EnvironmentFactory entries (no-op defaults)
├── agents.go        → 7 AgentDefinition structs
├── objectives.go    → 7 ObjectiveTemplate structs with criteria and constraints
└── hints.go         → PlannerHint slice (TDD, design-first, review gates, provider hints)
```

Key constraints baked into objective templates (enforced as `Constraint` with `Hard: true`):

- `software.objective.delivery`: `write_design_doc` must precede any `write_code` action
- `software.objective.delivery`: `write_test` must precede the `write_code` it covers
- `software.objective.delivery`: `verify.tech_lead_review` AND `verify.review` must both pass before `create_pr`

### Future Packs

Each stub (`domains/agriculture/pack.go`, etc.) implements `DomainPack` interface with all methods returning empty slices and `Init()` returning nil. They register without error and pass the conformance suite's "valid registration" check. Full implementation requires zero changes to core or feature layers.

### Conformance Suite

Checks (run via `krk domain test <id>`):

1. `DomainPack.ID()` is non-empty, lowercase, no spaces
2. All `Capability.InputSchema` and `OutputSchema` are valid JSON Schema
3. All `EnvironmentFactory.Build()` calls return non-nil without panicking on a zero-value `BuildContext`
4. All `AgentDefinition.Capabilities` IDs are registered in the pack's own `Capabilities()`
5. All `Criterion.Verifier` IDs in objective templates resolve to a registered capability
6. No capability ID collides with universal capabilities or another registered domain's capabilities
7. `DomainPack.Teardown()` does not panic

---

## Current Implementation Status


| Component                                                             | Status                                                                                                                                                                                    |
| --------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Core engine: ReasoningLoop (all six steps)                            | **Fully implemented**                                                                                                                                                                     |
| Core engine: CapabilityRegistry                                       | **Fully implemented**                                                                                                                                                                     |
| Core engine: EnvironmentRegistry                                      | **Fully implemented**                                                                                                                                                                     |
| Core engine: ObjectiveService                                         | **Fully implemented**                                                                                                                                                                     |
| Core engine: AgentFactory (LangChain Go)                              | **Fully implemented**                                                                                                                                                                     |
| Core engine: Memory (all four tiers, sqlite-vec)                      | **Fully implemented**                                                                                                                                                                     |
| Core engine: DigitalTwin (person, team, org)                          | **Fully implemented**                                                                                                                                                                     |
| Core engine: DomainRegistry                                           | **Fully implemented**                                                                                                                                                                     |
| Software domain pack: all 20 capabilities                             | **Fully implemented**                                                                                                                                                                     |
| Software domain pack: all 6 environment interfaces + no-op defaults   | **Fully implemented**                                                                                                                                                                     |
| Software domain pack: all 7 agent definitions                         | **Fully implemented**                                                                                                                                                                     |
| Software domain pack: all 7 objective templates                       | **Fully implemented**                                                                                                                                                                     |
| Software domain pack: planner hints                                   | **Fully implemented**                                                                                                                                                                     |
| Agriculture domain pack (conformance passing)                         | **Fully implemented**                                                                                                                                                                     |
| Git worktree manager (go-git)                                         | **Fully implemented**                                                                                                                                                                     |
| LLM provider: Claude                                                  | **Fully implemented**                                                                                                                                                                     |
| LLM provider: Gemini API                                              | **Fully implemented** (Phase 7) — `langchaingo/llms/googleai`; `GOOGLE_API_KEY` / `GOOGLE_AI_API_KEY`                                                                                     |
| LLM provider: Cursor, Copilot APIs                                    | Stubs (no public API) — use CLI agents instead                                                                                                                                            |
| CLI agents: Claude Code, Cursor CLI, Gemini CLI, Copilot CLI          | **Fully implemented** (Phase 7) — `tools.cli_agents` slot, multi-instance, twin-bound; binary autodetect on PATH                                                                          |
| Executor: local (goroutine-based)                                     | **Fully implemented**                                                                                                                                                                     |
| Executor: Restate (HTTP client)                                       | **Fully implemented** (Phase 11) — submit + status + cancel via REST; degrades to local fallback when `RESTATE_INGRESS_URL` unset                                                         |
| Executor: Celery (Redis broker)                                       | **Fully implemented** (Phase 11) — RPUSH Celery v2 envelopes; polls `celery-task-meta-{id}`; degrades to local fallback when `CELERY_BROKER_URL` unset                                  |
| Durable loop state + server-restart resume                             | **Fully implemented** (Phase 11) — `loop_states` table, `ResumeStoredLoops` at bootstrap                                                                                                  |
| Storage: SQLite + GORM                                                | **Fully implemented**                                                                                                                                                                     |
| Storage: PostgreSQL + GORM                                            | **Fully implemented** (Phase 8) — `gorm.io/driver/postgres`; selected via `database.driver: postgres` or `KARAKURI_DATABASE_DRIVER` env                                                   |
| Storage: MySQL                                                        | Interface-defined only                                                                                                                                                                    |
| Memory: Working, Episodic, Procedural (SQLite)                        | **Fully implemented**                                                                                                                                                                     |
| Memory: Semantic (SQLite keyword fallback)                            | **Fully implemented**                                                                                                                                                                     |
| Memory: Semantic (pgvector)                                           | **Fully implemented** (Phase 8) — `memory.vector_backend: pgvector`; cosine distance recall                                                                                               |
| Migration tooling: `krk migrate --from … --to …`                      | **Fully implemented** (Phase 8) — generic GORM-level row copy                                                                                                                             |
| OTel: local file exporter (JSON, NDJSON)                              | **Fully implemented**                                                                                                                                                                     |
| OTel: local file exporter (Parquet, CSV)                              | **Fully implemented** (Phase 12) — real Parquet via parquet-go; CSV with headers + label flattening; rotation on size + age                                                              |
| OTel: AWS exporter (CloudWatch + S3)                                  | **Fully implemented** (Phase 12) — `PutMetricData` for metrics, NDJSON `PutObject` archive for logs; activates on `AWS_REGION` + `AWS_S3_LOG_BUCKET`                                       |
| OTel: Datadog exporter                                                | **Fully implemented** (Phase 12) — `/api/v1/series` + `/api/v2/logs`; activates on `DD_API_KEY`                                                                                            |
| OTel: NewRelic exporter                                               | **Fully implemented** (Phase 12 extension) — metric-api + log-api with US/EU/staging region URLs; `Api-Key` auth via `NEW_RELIC_LICENSE_KEY`                                              |
| OTel: Elasticsearch (ELK) exporter                                    | **Fully implemented** (Phase 12 extension) — `_bulk` NDJSON; HTTP Basic or `ApiKey` auth; configurable metrics + logs indices                                                            |
| OTel: Loki (Grafana) log exporter                                     | **Fully implemented** (Phase 12 extension) — `/loki/api/v1/push`; streams bucketed by level; multi-tenant via `X-Scope-OrgID`                                                            |
| OTel: OTLP (OpenTelemetry Collector) exporter                         | **Fully implemented** (Phase 12 extension) — OTLP/JSON metrics + logs to any collector; custom headers + service name; opens path to any collector-supported backend                     |
| OTel: Prometheus exporter (scrape + pushgateway)                      | **Fully implemented** (Phase 12 extension) — `GET /metrics` mounted outside bearer auth; in-memory series map; optional pushgateway POST via `PROMETHEUS_PUSHGATEWAY_URL`                |
| Exporter chain isolation                                              | **Fully implemented** (Phase 12) — `OTel.Flush` logs per-exporter failures at WARN; one downstream outage never blocks the others                                                         |
| Exporter retry semantics (exponential backoff)                        | **Fully implemented** (Phase 12 extension) — `RetryExporter` wraps remote exporters; 3 attempts, exponential backoff (capped 30s); `ErrPermanent` short-circuits on 401/403              |
| Tool adapters                                                         | **Fully implemented** (Phase 6, ADR 006) — multi-instance per slot, twin-bound dispatch: GitHub, Linear, Slack, Figma, Playwright, Google Calendar, Email (Gmail/Outlook/SMTP/Apple Mail) |
| ResearchAdapter: HTTP scraper + source registry                       | **Fully implemented**                                                                                                                                                                     |
| API: all defined endpoints                                            | **Fully implemented**                                                                                                                                                                     |
| CLI `krk`: all defined commands                                       | **Fully implemented**                                                                                                                                                                     |
| SSE event stream: all 18 event types                                  | **Fully implemented**                                                                                                                                                                     |
| Domain SDK conformance suite                                          | **Fully implemented**                                                                                                                                                                     |
| Local deployment (Docker Compose, Helm, Minikube, k3s, ArgoCD)        | **Fully implemented**                                                                                                                                                                     |
| Healthcare domain pack (conformance passing, strict authority bounds) | **Fully implemented** (Phase 10)                                                                                                                                                          |
| Other future domain packs (legal, mechanical, consulting)             | Stub modules only                                                                                                                                                                          |
| TypeScript + React frontend                                           | **Fully implemented** (Phase 9) — Vite + React 18; embedded in the server binary via `embed.FS`; SPA fallback + scoped bearer auth                                                        |


---

## Risks and Trade-offs


| Risk                                                         | Severity | Mitigation                                                                                                                                                                                                   |
| ------------------------------------------------------------ | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Non-terminating loop (objective never satisfied)             | High     | Hard `MaxIter` cap (default 50) per objective; criteria completion score tracked per iteration; if score doesn't improve for N consecutive iterations, emit checkpoint rather than burning tokens            |
| Memory bloat degrades semantic recall quality                | High     | Retention TTL + confidence decay (Phase 13); `Forget` runs on schedule; consolidation promotes only high-confidence entries; semantic tier size cap with LRU eviction                                        |
| sqlite-vec performance degrades at scale                     | Medium   | `Memory` interface abstracts vector store; pgvector swap = only `platform/memory/semantic.go` changes (Phase 8); no feature or core changes required                                                         |
| LLM reasoning inconsistency across iterations                | Medium   | Reflexion strategy adds self-critique pass; procedural memory surfaces historical success rates; `ConfidenceThreshold` in `AuthorityBounds` escalates uncertain plans                                        |
| Domain pack quality variance                                 | High     | Conformance suite mandatory for registration; `CapabilityRegistry` validates schemas at registration time; rejects malformed packs with descriptive error                                                    |
| Worktree filesystem conflicts under concurrent load          | Medium   | Branch naming scoped to `<objective-id>/<task-id>` guarantees uniqueness; `WorktreeManager` is the sole path to worktree creation; no direct filesystem writes from agents                                   |
| LangChain Go version drift breaking agent behaviour          | Medium   | All LangChain Go usage confined to `internal/platform/agent/` + `internal/platform/llm/`; `AgentFactory` interface is the sole boundary; swap cost is one package                                            |
| Cross-domain objective complexity exceeds LLM context        | Medium   | Objectives scoped to single domain by default; world state chunked and summarised before reason step if size exceeds provider context limit (Phase 13)                                                       |
| sqlite-vec extension unavailable in deployment               | Low      | Health check verifies sqlite-vec at startup; if unavailable, semantic memory degrades gracefully to keyword-based recall with startup warning                                                                |
| Authority bounds misconfiguration permits unintended actions | High     | Default `AuthorityBounds` is maximally restrictive (`MaxAutonomousActions: 0`, `ConfidenceThreshold: 1.0`); operators must explicitly relax bounds in config; all autonomous actions logged to `tool_events` |


