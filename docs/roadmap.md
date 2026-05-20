# Karakuri Roadmap

## Context

Karakuri replaced the original role-based workflow simulator with an autonomous platform built on four primitives: **Capabilities, Environments, Objectives, and Agents**. No backward compatibility is maintained. The CLI binary is `krk`. This document records what shipped (Phases 1–7) and what is queued (Phases 8–13).

## Status Summary

| Phase | Title | Status |
|---|---|---|
| 1 | Core Engine Foundation | **Completed** |
| 2 | Reasoning Loop + Software Domain Pack | **Completed** |
| 3 | Memory Intelligence + Watch Mode | **Completed** |
| 4 | Domain Pack SDK + Hardening | **Completed** |
| 5 | Local Deployment Variants | **Completed** |
| 6 | Real Tool Adapters | **Completed** |
| 7 | Multi-LLM Provider Parity + CLI Agents | **Completed** |
| 8 | Production Storage (PostgreSQL + pgvector) | Planned |
| 9 | React Frontend | Planned |
| 10 | Domain Pack Expansion | Planned |
| 11 | Distributed & Durable Execution | Planned |
| 12 | Observability Fan-out | Planned |
| 13 | Cross-Domain Objectives + Hardening | Planned |

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

14. **`internal/core/domain/registry.go`** — `DomainRegistry` that calls `DomainPack.Init()` at startup.

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

4. **`software.objective.autonomous_watch` fully operational** — watcher agent subscribes to all configured environments via `Environment.Subscribe()`; on `EnvironmentEvent` received, evaluates against promotion rules; emits `checkpoint` with suggested objective template for human approval.

5. **Research pulse** — integrate `ResearchService` into watcher loop: periodically invoke `software.reason.research` via ResearchAdapter; detect threats/opportunities; emit checkpoint with promotable research objective if significance threshold met.

6. **`krk auto` command** — shorthand for creating a watcher twin and starting watch mode; streams environment events and checkpoint prompts to terminal.

7. **OTel metrics** — add memory hit rate, recall latency, consolidation frequency to LocalFileExporter output.

**Acceptance:**
- Second run of `software.objective.delivery` on same repo produces reasoning trace referencing prior episodic memory entries.
- Simulated environment change (push a commit) triggers watcher → `environment_changed` SSE event → `checkpoint` emitted asking to promote to `software.objective.code_review`.
- Research pulse produces trend report artifact; similarity score visible in `krk memory recall` output.

---

## Phase 4 — Domain Pack SDK + Hardening (Completed)

**Goal:** External domain authors can build and register packs; system is production-hardened.

**Steps:**

1. **`karakuri-domain-sdk` Go module** — extract DomainPack scaffolding, capability primitives, environment base types into a publishable Go module. Include conformance test suite: validates capability schemas, environment factory outputs, objective template structure.

2. **`krk domain add <pack-path>`** — load Go plugin or local module; call `DomainPack.Init()`; register capabilities and environments; validate via conformance suite.

3. **`krk domain test <pack-path>`** — run conformance suite against pack in dry-run mode; report pass/fail per check.

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
- Five `make` targets composed from internal `_secret`, `_image-load-*`, `_helm-install*` primitives — image tag, namespace, release name, and chart path each declared exactly once

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

| Setting | Lives in | Consumed by |
|---|---|---|
| Server config (DB path, providers, memory thresholds) | `deploy/karakuri.yaml` | Dockerfile `COPY`; chart ConfigMap via `.Files.Get` |
| Image, replicas, service, storage, resources | `deploy/values.yaml` | All four K8s variants |
| k3s deltas (`pullPolicy: IfNotPresent`, ClusterIP, `local-path`) | `deploy/values-k3s.yaml` | k3s target only |
| Secrets (`ANTHROPIC_API_KEY`, `KARAKURI_AUTH_TOKEN`) | Process env at deploy time | All variants via shared `_secret` Makefile primitive |
| ArgoCD Application | `deploy/argocd/application.yaml` | ArgoCD only |

**Variants:**

| Variant | Up | Down |
|---|---|---|
| Docker Compose | `make docker-up` | `make docker-down` |
| Helm (direct) | `make helm-up` | `make helm-down` |
| Minikube | `make minikube-up` | `make minikube-down` |
| k3s | `make k3s-up` | `make k3s-down` |
| ArgoCD | `make argocd-up` | `make argocd-down` |

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

| Slot | Adapter `type:` values | Package(s) |
|---|---|---|
| `versioncontrol` | `github` | `tools/versioncontrol/github.go` |
| `projectmgmt` | `linear` | `tools/projectmgmt/linear.go` |
| `messaging` | `slack` | `tools/messaging/slack.go` |
| `design` | `figma` | `tools/design/figma.go` |
| `testing` | `playwright` | `tools/testing/playwright.go` |
| `calendar` | `google` (Google Calendar v3) | `tools/calendar/google.go` |
| `email` | `gmail`, `outlook`, `smtp`, `apple_mail` | `tools/email/{gmail,outlook,smtp,applemail}.go` |

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

- **`config.ToolsConfig`** uses a uniform `SlotConfig{Default, Instances}` per slot; `InstanceConfig{Type, Options}` carries provider-specific fields. `resolveEnvRefs` overlays env vars referenced by `*_env` keys.
- **`tools.Registry`** uses generic `SlotInstances[T]` per slot — typed instance maps with `Resolve(name)` and `DefaultName()`. `NewRegistryFromConfig(cfg.Tools)` dispatches each instance's `Type` to the matching constructor.
- **`environment.Factory.Build(BuildContext)`** receives `{TwinID, AdapterBindings}` so envs resolve the correct adapter instance at construction time. Software envs (`gitEnv`, `ticketEnv`, `commsEnv`) hold the resolved adapter directly — no per-action lookup.
- **`DigitalTwin.AdapterBindings map[string]string`** — slot → instance name. Persisted in the `adapter_bindings_json` column on `twins`.
- **`/health`** returns `adapters` as one row per `(slot, instance, type, active, is_default)` so operators see the full topology.

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
5. **`software.act.delegate_to_cli` capability** — new capability with input schema `{cli, prompt, allowed_tools?}`; act step routes to the corresponding `CLIAgentAdapter` by `cli` param; resulting artifacts flow through the existing `ArtifactService`.
6. **Loop-step instrumentation** — `cli_agent_started` / `cli_agent_completed` SSE events; per-CLI duration and exit-code metrics; CLI output captured into episodic memory verbatim for later inspection.
7. **Sandbox + worktree contract** — CLIs are invoked inside the per-task worktree (already created by `WorktreeManager`), so their edits stay isolated; the act step diffs the worktree after the CLI exits to discover artifacts.
8. **Multi-instance + twin-bound (ADR 006)** — `tools.cli_agents` slot with named instances (`acme_claude_code`, `bsenel_cursor`, …) so each twin can pin a preferred CLI agent via `AdapterBindings`.

**Why this matters.** Many operators already pay for a coding-agent CLI subscription (Claude Code, Cursor) that includes its own model, tool loop, and sandbox. Reusing those CLIs lets Karakuri orchestrate work without re-paying for raw tokens or re-implementing tool dispatch; Karakuri becomes the *outer* loop (objective + memory + verify) wrapping the CLI's *inner* loop (write code, run tests, iterate).

### Acceptance — met

- **Gemini API** (Track A) wraps `langchaingo/llms/googleai`; activates when `GOOGLE_API_KEY` / `GOOGLE_AI_API_KEY` is set; `AsLLM()` returns a real `llms.Model` so the agent factory can use it. Cursor and Copilot API stubs return explicit errors pointing operators to Track B because neither vendor offers a generally-available LLM API for individual subscribers.
- **CLI agent slot** (`tools.cli_agents`) is multi-instance per ADR 006. Four adapter types implemented: `claude_code` (NDJSON stream), `cursor_cli` (same shape), `gemini_cli` (plain stdout), `copilot_cli` (suggest/explain via `gh copilot`). Each adapter's `Active()` reflects binary presence on PATH.
- **`software.act.delegate_to_cli` capability** is registered; the new `software.env.cli_agent` environment resolves the twin's bound CLI instance at construction and dispatches `Delegate(...)` inside the per-task worktree.
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

## Phase 8 — Production Storage (PostgreSQL + pgvector) (Planned)

**Goal:** Production-grade backends so Karakuri runs beyond a single SQLite file. Semantic memory uses pgvector for true vector recall (replacing SQLite keyword fallback).

**Steps:**

1. **PostgreSQL GORM dialect** — switch `internal/platform/storage/gorm_storage.go` to use `gorm.io/driver/postgres` when `database.driver: postgres`; SQLite remains the default for local dev.
2. **pgvector semantic backend** (`internal/platform/memory/semantic_pgvector.go`) — new implementation behind the existing `Memory` interface; selected by config `memory.vector_backend: pgvector`. Embedding generation via Claude (already in Phase 3) feeds the `vector(1536)` column.
3. **Migration tooling** — `krk migrate sqlite-to-postgres` walks all tables (twins, objectives, loop_iterations, memory_*, artifacts, worktrees, tool_events) and replays them through `StorageAdapter` so the same constraints apply.
4. **Helm values** — extend `deploy/values.yaml` with `postgresql.host/port/database/user`; `postgresql.passwordSecret` reference. Optional `dependencies` block to include the Bitnami postgresql sub-chart for one-command Postgres + Karakuri.
5. **Integration test matrix** — `test/integration/` runs against both SQLite and Postgres (matrix var `KARAKURI_DB`); same suite must pass.

**Acceptance:** `make helm-up` against a cluster with Postgres provisioned brings the full system online; the same conformance + integration tests pass against Postgres; semantic recall returns true similarity matches (not keyword).

---

## Phase 9 — React Frontend (Planned)

**Goal:** Browser UI for non-CLI users. Consumes the existing REST + SSE endpoints; no backend changes required (the API was designed frontend-ready in v1).

**Steps:**

1. **Project scaffold** — `web/` directory, Vite + React + TypeScript; `krk` CLI gains `krk web` to launch dev server with proxy to localhost:8080.
2. **Twin dashboard** — list, create, drill-down; child-twin tree visualization.
3. **Objective board** — create, list, status; criteria progress bars from `Verification.WeightedScore`.
4. **Loop runner view** — SSE-driven live event timeline; per-step expandable cards (Observe → Learn); world state diff between iterations.
5. **Checkpoint inbox** — pending checkpoints with approve / reject / modify actions; reason/decision context from `Checkpoint.Context`.
6. **Memory + artifact browsers** — recall queries with similarity scores; artifact diff viewer for line-text blobs.
7. **Auth** — Bearer token via login modal stored in localStorage; same `KARAKURI_AUTH_TOKEN` as CLI.
8. **Static build embedded into server** — `cmd/server/main.go` serves `web/dist/` at `/` (preserving `/api/v1/*` paths) so the binary stays self-contained.

**Acceptance:** Complete a full `software.objective.delivery` run end-to-end (create twin → objective → loop → approve checkpoint → review artifacts) without touching the CLI; SSE stream renders within 200 ms of event emission.

---

## Phase 10 — Domain Pack Expansion (Planned)

**Goal:** Ship a second non-software production pack to prove the `karakuri-domain-sdk` SDK and conformance suite scale. Healthcare is the suggested first candidate (highest signal for "domain isolation actually works").

**Steps:**

1. **Pick one pack to ship fully** — recommend `healthcare` (clinical decision support is high-stakes, exercises authority bounds rigorously).
2. **Capabilities** — minimum 10 across observe/reason/decide/act/verify/learn (e.g. `healthcare.observe.lab_results`, `healthcare.reason.differential_diagnosis`, `healthcare.verify.guideline_adherence`).
3. **Environments** — at least 2 (EHR mock, lab system mock) with no-op defaults.
4. **Agents** — 3+ definitions (e.g. triage, clinician, auditor) with strict `AuthorityBounds` (e.g. `MaxAutonomousActions: 0` for prescription actions).
5. **Objective templates** — 2+ (e.g. `healthcare.objective.diagnosis_support`, `healthcare.objective.guideline_check`).
6. **Conformance** — passes all 7 checks via `krk domain test healthcare`.
7. **Reference end-to-end** — sample objective runs in CI against mock environments; produces a `clinical_review` artifact.

**Acceptance:** A clinician (or stand-in) walks through a diagnosis-support objective end-to-end; all critical actions escalate to checkpoint; full audit trail visible via `krk artifact list`.

---

## Phase 11 — Distributed & Durable Execution (Planned)

**Goal:** Loops survive server restarts and parallelize across nodes. Replaces the local-goroutine `Executor` for production workloads.

**Steps:**

1. **Restate executor** — `internal/platform/executor/restate.go`; loop iterations checkpointed as durable workflow steps; server restart resumes from last completed step (no lost work).
2. **Celery executor** — `internal/platform/executor/celery.go`; fan-out act-step parallelism across worker nodes; uses Redis or RabbitMQ broker.
3. **Loop state externalized** — `LoopService.states` map (currently in-process) moves behind a `LoopStateStore` interface; SQLite/Postgres implementation; Restate workflow ID maps to loop ID.
4. **Worker image** — `karakuri-worker:latest` separate image; Helm chart adds `worker.replicaCount`.
5. **Verification suite** — kill server mid-loop, restart, observe resume; run 10 concurrent delivery objectives across 3 workers, verify no worktree collisions.

**Acceptance:** A 30-iteration objective survives 3 server restarts and resumes from the last completed step each time; 10 parallel objectives complete on a 3-worker cluster with deterministic worktree allocation.

---

## Phase 12 — Observability Fan-out (Planned)

**Goal:** Production observability beyond local files. Activates the OTel exporter interfaces already defined in v1.

**Steps:**

1. **Parquet + CSV file formats** — replace the JSON-fallback stubs in `LocalFileExporter`; use `parquet-go` and stdlib `encoding/csv`; DuckDB-queryable.
2. **AWS exporter** (`internal/platform/observability/aws.go`) — CloudWatch Metrics + S3 Parquet archive for logs.
3. **Datadog exporter** — metrics via Datadog API; logs via HTTP intake.
4. **Exporter chain** — multiple exporters active simultaneously; failure in one does not block others.
5. **Helm values** — `observability.exporters.[]` accepts the same schema as the in-repo config; secrets via existing `karakuri-secrets`.
6. **File rotation hardening** — size + age limits enforced under load; tested with 24h soak run.

**Acceptance:** Same loop emits identical metric series visible in local Parquet (queried via DuckDB), CloudWatch, and Datadog simultaneously; 24h soak shows no exporter back-pressure on the loop.

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

- **Phase 11** (distributed execution) benefits from **Phase 8** (Postgres state externalization) but does not strictly require it (Restate has its own state store).
- **Phase 13** (cross-domain) benefits from **Phase 10** (a second real pack exists to combine with software).
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

| Component | Package | Responsibility | Depends On |
|---|---|---|---|
| CapabilityRegistry | `internal/core/capability/` | Registers and validates capabilities; enforces schema | nothing |
| EnvironmentRegistry | `internal/core/environment/` | Registers environment factories by domain | nothing |
| ObjectiveService | `internal/feature/objective/` | CRUD, status transitions, criteria progress | core/objective, StorageAdapter |
| LoopService | `internal/feature/loop/` | Drives observe→reason→decide→act→verify→learn | all core, Memory, AgentFactory, WorktreeManager |
| TwinService | `internal/feature/twin/` | CRUD for DigitalTwin; assigns objectives; tracks child twins | core/twin, ObjectiveService |
| MemoryService | `internal/feature/memory/` | Recall orchestration, consolidation scheduling | core/memory, StorageAdapter |
| CheckpointService | `internal/feature/checkpoint/` | Lifecycle: create → pending → resolved | core/checkpoint, StorageAdapter |
| ArtifactService | `internal/feature/artifact/` | VFS blob writes; SHA addressing; diff | core/vfs, StorageAdapter |
| ResearchService | `internal/feature/research/` | Spawns research sub-objectives via loop | LoopService, ResearchAdapter |
| AgentFactory | `internal/platform/agent/` | Builds LangChain Go agents from AgentDefinition | LangChain Go, ProviderRegistry |
| ProviderRegistry | `internal/platform/llm/` | Resolves provider by LLMHints; applies fallback chain | LangChain Go |
| WorktreeManager | `internal/platform/git/` | Creates/removes isolated git worktrees via go-git | go-git |
| StorageAdapter | `internal/platform/storage/` | Single GORM-backed impl; all DB ops | GORM, SQLite |
| MemoryTier impls | `internal/platform/memory/` | Working (map), Episodic (SQLite), Semantic (sqlite-vec), Procedural (SQLite) | StorageAdapter |
| LocalFileExporter | `internal/platform/observability/` | Writes OTel metrics/logs in JSON/NDJSON/Parquet/CSV | OTel SDK |
| DomainRegistry | `internal/core/domain/` | Registers DomainPack instances; validates conformance | nothing |
| Software Domain Pack | `domains/software/` | Capabilities, environments, agent defs, objective templates | core interfaces only |
| API Server | `internal/api/` | chi router; all REST + SSE endpoints | feature services |
| CLI `krk` | `cli/` | cobra commands; thin HTTP client | net/http |

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
- Fan-out: invoke all `observe.*` capabilities in `AgentDefinition.Capabilities` via `CapabilityRegistry`
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

| Component | Status |
|---|---|
| Core engine: ReasoningLoop (all six steps) | **Fully implemented** |
| Core engine: CapabilityRegistry | **Fully implemented** |
| Core engine: EnvironmentRegistry | **Fully implemented** |
| Core engine: ObjectiveService | **Fully implemented** |
| Core engine: AgentFactory (LangChain Go) | **Fully implemented** |
| Core engine: Memory (all four tiers, sqlite-vec) | **Fully implemented** |
| Core engine: DigitalTwin (person, team, org) | **Fully implemented** |
| Core engine: DomainRegistry | **Fully implemented** |
| Software domain pack: all 20 capabilities | **Fully implemented** |
| Software domain pack: all 6 environment interfaces + no-op defaults | **Fully implemented** |
| Software domain pack: all 7 agent definitions | **Fully implemented** |
| Software domain pack: all 7 objective templates | **Fully implemented** |
| Software domain pack: planner hints | **Fully implemented** |
| Agriculture domain pack (conformance passing) | **Fully implemented** |
| Git worktree manager (go-git) | **Fully implemented** |
| LLM provider: Claude | **Fully implemented** |
| LLM provider: Gemini API | **Fully implemented** (Phase 7) — `langchaingo/llms/googleai`; `GOOGLE_API_KEY` / `GOOGLE_AI_API_KEY` |
| LLM provider: Cursor, Copilot APIs | Stubs (no public API) — use CLI agents instead |
| CLI agents: Claude Code, Cursor CLI, Gemini CLI, Copilot CLI | **Fully implemented** (Phase 7) — `tools.cli_agents` slot, multi-instance, twin-bound; binary autodetect on PATH |
| Executor: local (goroutine-based) | **Fully implemented** |
| Executor: Celery, Restate | Interface-defined only (Phase 11) |
| Storage: SQLite + GORM | **Fully implemented** |
| Storage: PostgreSQL, MySQL | Interface-defined only (Phase 8) |
| Memory: Working, Episodic, Procedural (SQLite) | **Fully implemented** |
| Memory: Semantic (sqlite-vec) | **Fully implemented** |
| OTel: local file exporter (JSON, NDJSON) | **Fully implemented** |
| OTel: local file exporter (Parquet, CSV) | Format stubs (Phase 12) |
| OTel: AWS, Datadog exporters | Interface-defined only (Phase 12) |
| Tool adapters | **Fully implemented** (Phase 6, ADR 006) — multi-instance per slot, twin-bound dispatch: GitHub, Linear, Slack, Figma, Playwright, Google Calendar, Email (Gmail/Outlook/SMTP/Apple Mail) |
| ResearchAdapter: HTTP scraper + source registry | **Fully implemented** |
| API: all defined endpoints | **Fully implemented** |
| CLI `krk`: all defined commands | **Fully implemented** |
| SSE event stream: all 18 event types | **Fully implemented** |
| Domain SDK conformance suite | **Fully implemented** |
| Local deployment (Docker Compose, Helm, Minikube, k3s, ArgoCD) | **Fully implemented** |
| Other future domain packs (healthcare, legal, mechanical, consulting) | Stub modules only (Phase 10) |
| TypeScript + React frontend | Not implemented; API and SSE stream frontend-ready (Phase 9) |

---

## Risks and Trade-offs

| Risk | Severity | Mitigation |
|---|---|---|
| Non-terminating loop (objective never satisfied) | High | Hard `MaxIter` cap (default 50) per objective; criteria completion score tracked per iteration; if score doesn't improve for N consecutive iterations, emit checkpoint rather than burning tokens |
| Memory bloat degrades semantic recall quality | High | Retention TTL + confidence decay (Phase 13); `Forget` runs on schedule; consolidation promotes only high-confidence entries; semantic tier size cap with LRU eviction |
| sqlite-vec performance degrades at scale | Medium | `Memory` interface abstracts vector store; pgvector swap = only `platform/memory/semantic.go` changes (Phase 8); no feature or core changes required |
| LLM reasoning inconsistency across iterations | Medium | Reflexion strategy adds self-critique pass; procedural memory surfaces historical success rates; `ConfidenceThreshold` in `AuthorityBounds` escalates uncertain plans |
| Domain pack quality variance | High | Conformance suite mandatory for registration; `CapabilityRegistry` validates schemas at registration time; rejects malformed packs with descriptive error |
| Worktree filesystem conflicts under concurrent load | Medium | Branch naming scoped to `<objective-id>/<task-id>` guarantees uniqueness; `WorktreeManager` is the sole path to worktree creation; no direct filesystem writes from agents |
| LangChain Go version drift breaking agent behaviour | Medium | All LangChain Go usage confined to `internal/platform/agent/` + `internal/platform/llm/`; `AgentFactory` interface is the sole boundary; swap cost is one package |
| Cross-domain objective complexity exceeds LLM context | Medium | Objectives scoped to single domain by default; world state chunked and summarised before reason step if size exceeds provider context limit (Phase 13) |
| sqlite-vec extension unavailable in deployment | Low | Health check verifies sqlite-vec at startup; if unavailable, semantic memory degrades gracefully to keyword-based recall with startup warning |
| Authority bounds misconfiguration permits unintended actions | High | Default `AuthorityBounds` is maximally restrictive (`MaxAutonomousActions: 0`, `ConfidenceThreshold: 1.0`); operators must explicitly relax bounds in config; all autonomous actions logged to `tool_events` |
