# Karakuri Roadmap

## Context

Karakuri replaced the original role-based workflow simulator with an autonomous platform built on four primitives: **Capabilities, Environments, Objectives, and Agents**. No backward compatibility is maintained. The CLI binary is `krk`. This document records what shipped (Phases 1–22). Starting with Phase 14, the auth and quota engines ship as standalone Go modules under `github.com/bsenel/karakuri/{auth,quota}` and their submodules — fully reusable by other repos without pulling Karakuri itself.

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
| 13    | Cross-Domain Objectives + Hardening        | **Completed** |
| 14    | RBAC + Fine-Grained Authorization (core)   | **Completed** |
| 15    | API Rate Limiting + Quota Management (core)| **Completed** |
| 16    | Federated Identity (OIDC + SAML)           | **Completed** |
| 17    | Hierarchical Resources + Org Units         | **Completed** |
| 18    | Quota Self-Service + Cost Attribution      | **Completed** |
| 19    | Frontend for Auth, Quota, Cost, Audit      | **Completed** |
| 20    | Standing Objectives + Reconciliation       | **Completed** |
| 21    | Digests                                    | **Completed** |
| 22    | The Karakuri Domain Pack                   | **Superseded**|
| 23    | Per-Objective Spend Ceilings               | **Partial**   |
| 24    | Conformance That Tests Behaviour           | **Completed** |
| 25    | Self-Improvement Without a History         | **Completed** |
| 26    | The Write Path                             | **Completed** |


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
| Secrets (`ANTHROPIC_API_KEY`, `KARAKURI_AUTH_JWT_SECRET`)         | Process env at deploy time       | All variants via shared `_secret` Makefile primitive |
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

## Phase 13 — Cross-Domain Objectives + Hardening (Completed)

**Goal:** Lift the v1 single-domain restriction; close out the hardening items flagged in the Risks section.

**What shipped:**

- **Cross-domain Objectives** (`internal/core/objective/objective.go`). Added `AdditionalDomains []string` alongside the legacy `Domain string` primary, with an `AllDomains()` helper that returns the deduplicated union. The loop runner (`internal/feature/loop/runner.go`) walks the union when recruiting agents — first matching pack with an agent wins — and when collecting environment factories, deduping by `EnvID` so packs declaring the same `Factory` only build once. `Criterion.Domain` was added for per-domain criterion attribution; `stepVerify` now emits a `per_domain_score` payload on the `loop.step_completed` event when criteria carry domain tags, while keeping the aggregate weighted score as the authoritative completion gate. Storage migrated by adding `ObjectiveModel.AdditionalDomainsJSON` — existing rows decode cleanly because empty JSON arrays are the default.
- **Cross-pack capability collision audit** (`internal/conformance/suite.go`). New `CheckCrossPackCollisions(packs ...)` returns three independent results (capability, environment, agent ID collisions) so a single audit pass surfaces every overlap instead of stopping at the first. Bootstrap runs it across the set of *active* packs (not stubs) and logs each failed check at WARN — operators can intentionally re-export an ID but never silently. The existing per-pack `Run()` suite (7 checks) is unchanged; this is additive.
- **Memory retention scheduler** (`internal/feature/memory/service.go`). Added `RunRetention(ctx, RetentionPolicySet)` that fans a per-tier policy out to working/episodic/semantic backends; errors from one tier never block the others. Semantic tier now honors `RetentionPolicy.MinScore` in both the SQLite (`internal/platform/memory/semantic.go`) and pgvector (`internal/platform/memory/semantic_pgvector.go`) backends — confidence floor *OR* age cutoff trigger deletion, scoped by agent/twin. Bootstrap goroutine (`startRetentionLoop`) runs the sweep every `memory.retention.interval_minutes`. New config block: `memory.retention.{enabled, interval_minutes, working_ttl_minutes, episodic_ttl_days, semantic_ttl_days, semantic_min_score}`; **disabled by default** — operators must opt in once they've measured growth.
- **Reflexion strategy + benchmark** (`internal/feature/loop/reason.go`, `cmd/krk-bench/main.go`). The `reflexion` `ReasoningStrategy` constant is now wired to a two-pass implementation in `stepReason`: when the agent's `ReasoningStrategy == ReasoningReflexion`, the draft plan is followed by a critique pass ("identify the weakest assumption, the most likely failure mode, missing steps") and a revision pass that consumes the critique. The revised plan replaces the draft only if it parses and contains non-empty actions — Reflexion never regresses below the chain-of-thought baseline. Each iteration emits `reflexion_applied: true` and `reflexion_critique: …` on the step-completed event for observability. The `cmd/krk-bench` harness compares both strategies on 5 fixed-difficulty scenarios (200 trials each, seed-controlled) and writes a markdown table to `docs/benchmarks.md` — synthetic but deterministic; operators can adapt the trial structure to real LLMs without rewriting the comparison logic.
- **Helm chart OCI publishing** (`.github/workflows/release-helm.yml`). New GitHub Actions workflow on `v*.*.*` tag push: checkouts, installs Helm v3.14.4, derives the chart version from the tag (strips leading `v`), runs `helm lint`, packages the chart, logs into GHCR using `GITHUB_TOKEN`, and pushes to `oci://ghcr.io/<owner>/charts`. `workflow_dispatch` is also wired so operators can republish from a branch without cutting a tag. ArgoCD applications can now reference `oci://ghcr.io/bsenel/charts/karakuri` instead of a Git path.
- **Authority-bounds audit log** (`internal/platform/db/schema/models.go`, `internal/feature/loop/decide.go`, `internal/feature/checkpoint/service.go`). `ToolEventModel` gained four audit columns: `kind` (execute|escalation|approval), `escalation_reason`, `approver`, `bounds_violation` (indexed). Every authority-bounds escalation in `decide.go` now writes a `kind=escalation, bounds_violation=true` row capturing the draft plan, the agent's threshold, and the linked checkpoint ID. Every checkpoint resolution writes a `kind=approval` row tagged with the resolver's name. `GET /api/v1/audit` (handler at `internal/api/handler/audit.go`) accepts `objective_id`, `agent_id`, `kind`, `bounds_violation`, `since` (RFC3339), `limit` query params; new CLI subcommand `krk audit` wraps the endpoint with matching flags. `Checkpoint.Decision` gained an optional `Approver` field for the audit chain.

**Acceptance — met:**

- Build clean (`go build ./...`); full suite passes (`go test ./... -count=1`) with new tests covering: AllDomains/CriterionDomains dedup, cross-pack collision detection across the three ID kinds, retention policy fan-out + tier-failure isolation, Reflexion success path + three fallback paths (empty critique, unparseable revision, empty-actions revision), and the `/api/v1/audit` endpoint with kind + tri-state bounds filters.
- A cross-domain objective declaring `Domain: software` + `AdditionalDomains: [healthcare]` recruits the first available agent across the union, builds both packs' environment factories (deduped by EnvID), and tags each Criterion with the responsible domain for per-pack score reporting on the step-completed event.
- Helm OCI publishing workflow lints + packages + pushes cleanly on tag (verified locally via `helm lint deploy` and `helm package deploy`); CI run will be exercised by the first `v*.*.*` push.
- `krk audit --kind escalation --violations-only --since 2026-05-21T00:00:00Z` returns just the bounds-violating escalations from the last day, sorted newest first.

**Operator quickstart (Phase 13 features):**

```bash
# Enable memory retention (semantic tier: 30-day TTL, 0.3 confidence floor)
cat >> config.yaml <<'YAML'
memory:
  retention:
    enabled: true
    interval_minutes: 60
    semantic_ttl_days: 30
    semantic_min_score: 0.3
    episodic_ttl_days: 14
YAML

# Run the Reflexion benchmark
go run ./cmd/krk-bench > docs/benchmarks.md

# Inspect the audit log
krk audit --kind escalation --limit 25
krk audit --violations-only --objective obj-abc123

# Install Helm chart from GHCR (after a tagged release publishes)
helm install karakuri oci://ghcr.io/bsenel/charts/karakuri --version 0.1.0
```

---

## Phase 13.5 — Actionable Checkpoints (Completed)

**Goal:** Close a wired-but-non-functional path in the reasoning loop: today the runner accepts `approve | reject | modify` but only branches on the presence of a signal — `Decision.Choice` is never read, and `modify` behaves identically to `approve`. Phase 13.5 makes the human note a first-class loop input by (a) carrying the planner's draft on the checkpoint so reviewers see what they're approving, and (b) feeding modify-notes into a single Reflexion-style revise pass before the act step. Ships the first vertical slice of Phase 19's UI surface (`/checkpoints` modify dialog + `/audit` viewer) without depending on Phases 14–18.

**What shipped:**

- **Checkpoint payload** (`internal/core/checkpoint/checkpoint.go`). New `Action` type carries the planner draft. `Checkpoint` gains `Actions []Action` + `AuditEventID string`; the existing `Capability` and `Confidence` fields are now populated by the runner. `Decision` gains a structured `Modifications {RemovedActions, AddedConstraints, RevisedConfidence}` block alongside the free-text `Note`. Schema and `gorm_storage.go` round-trip both new columns; old rows decode cleanly because the JSON column defaults to `'[]'` and AuditEventID is a string with empty default.
- **Service.Create signature** (`internal/feature/checkpoint/service.go`). Grew a `CreateOptions{Capability, Confidence, Actions, AuditEventID}` block — passed by the loop runner at escalation time so reviewers can decide without joining tables. Existing callers (watch mode) pass a zero-value struct.
- **Audit-kind matrix** (`internal/platform/storage/adapter.go`, `internal/feature/checkpoint/service.go`). Two new `ToolEventKind` constants: `modification` and `rejection`. `Service.Resolve` now writes one of `{approval, modification, rejection}` based on `Decision.Choice`; the modification payload carries the structured diff (`removed_actions`, `added_constraints`, `revised_confidence`) plus the linked escalation audit ID so the audit log shows *what* changed, not just *that* something did. Wire-shape: JSON tags added to `storage.ToolEvent` so the `/api/v1/audit` response uses snake_case the React frontend can consume directly.
- **Runner consumes Decision** (`internal/feature/loop/runner.go`). The runner now reads `decision.Choice` after `<-state.decisionCh`:
  - `approve` → fall through to act with the (possibly revised) draft.
  - `reject` → write a `kind=rejection` audit row tagged `escalation_reason="rejected_at_checkpoint"`, finalize the loop with `runErr=rejected_at_checkpoint`.
  - `modify` → trim `RemovedActions` from the draft (`trimRemovedActions`), run a single revise pass (`stepReasonRevise`), re-enter `stepDecide`. If the revised plan trips bounds again, wait for one more decision; anything other than `approve` writes a `kind=rejection` row with `escalation_reason="modify_loop_exceeded"` and terminates. Hard-cap at one re-approval — no ping-pong possible.
- **stepReasonRevise** (`internal/feature/loop/reason.go`). New function that runs a single Reflexion-style revise pass driven by the operator's note + `AddedConstraints` as critique input. Never-regress: returns the trimmed draft unchanged on agent error, unparseable JSON, or empty actions — same guarantee as the existing `reflexionPass`.
- **`RevisedConfidence` as per-iteration threshold override** (follow-up after dogfooding). Initial implementation treated `RevisedConfidence` as a *floor on the plan's confidence* — which only helped if the agent reported a confidence below the operator's belief AND that belief still cleared the agent's threshold (a narrow sliver that mostly defeated the feature). Refactored to a proper override: when set, `RevisedConfidence` becomes both the plan's effective confidence (bypassing the procedural-memory bias) AND the per-iteration bounds threshold (via the new `effectiveThreshold` helper in `decide.go`). The operator can only LOWER the threshold via modify — raising it is rejected. The override is per-iteration only; the agent's stated threshold returns on the next iteration. Recorded on the `kind=modification` audit row.
- **Action-layer + verifier honesty** (follow-up after the Phase 14 dogfood "succeeded" with every action a no-op). Three independent lies were stacked: (1) `noopEnv.Act` in all three active packs returned `Success: true` for any capability, (2) `act.go` silently routed an unmatched `EnvID` to `sc.envs[0]`, and (3) `stepVerify` returned `(1.0, true)` for objectives with no `SuccessCriteria`. The combination let the Phase 14 objective complete with all 17 actions reporting noop-success and a trivial verify pass. Fixed: noop envs report `Success=false` with an `unimplemented` reason on `StateDelta`; `act.go` fails an action whose `EnvID` matches nothing (with the available list in the error and `status=unrouted` on `StateDelta`); the no-criteria verify path returns `(0.0, false)` with `reason=no_success_criteria_defined`. All 11 built-in objective templates across the three active packs already declare criteria, so this only affects ad-hoc CLI-created objectives — which now fail honestly instead of fake-completing.
- **Escalation audit row** (`internal/feature/loop/decide.go`). The escalation row is now written *before* the checkpoint so its ID can be threaded back as `AuditEventID`. The checkpoint payload now carries the planner draft (capability, confidence, action list) at creation time.
- **API** (`internal/api/handler/checkpoint.go`, `loop.go`). `POST /api/v1/checkpoints/{id}/resolve` and `POST /api/v1/loops/{id}/resume` both accept the `modifications` block. `GET /api/v1/checkpoints/{id}` now returns the planner draft fields populated.
- **CLI** (`cli/command/checkpoint.go`, `cli/command/loop.go`). New flags on both `krk checkpoint resolve` and `krk loop resume`: `--remove-action` (repeatable), `--constraint` (repeatable), `--revised-confidence`. A shared `buildResolveBody` helper composes the request body — `modifications` is only attached when at least one flag is set so legacy approve/reject calls keep the original shape.
- **Frontend — `/checkpoints` modify dialog** (`web/src/pages/CheckpointsPage.tsx`, new `web/src/components/ModifyCheckpointDialog.tsx`). Pending-checkpoint cards now surface `capability`, `confidence`, a collapsible action list, and a deep-link to the linked audit row. `Modify…` opens a dialog with per-action drop checkboxes, a constraints textarea (one per line), a confidence-floor input, and a rationale note. Submission posts the structured `Modifications` block and refreshes. Approve/Reject stay one-click.
- **Frontend — `/audit` page** (new `web/src/pages/AuditPage.tsx`, new route + nav entry). Filter UI matches `krk audit` (objective, agent, kind, since, violations-only). Filters live in the URL via `useSearchParams` so links are shareable. Row click expands the payload JSON inline; modification rows render a structured diff (dropped capabilities, constraints, confidence floor) above the raw JSON. Kind-coloured pills + bounds-violation badge.

**Acceptance — met:**

- Build clean (`go build ./...`); full suite passes (`go test ./... -count=1`) with new tests covering: `trimRemovedActions` (drop-one-occurrence + drop-all-when-listed + nil-mods noop), `effectiveThreshold` (5 cases: nil, below-threshold lowers, equal noop, above-threshold rejected, zero authority), `stepReasonRevise` success path + 3 fallback paths (unparseable, empty actions, no-feedback skip), constraint-only and note-only critique paths, confidence-not-touched verification, non-modify-choice noop. `checkpoint.Service` tests cover `Create` planner-draft round-trip and the three-way `Resolve` audit-kind matrix plus the structured modification payload.
- A loop that escalates with `confidence 0.84 below threshold 0.90` (the case observed during dogfooding on 2026-06-09) can now be resolved with `--decision modify --remove-action code.write --constraint "scaffold only"` and the revised plan executes without `code.write`.
- `GET /api/v1/checkpoints/{id}` returns a payload containing `capability`, `confidence`, `actions`, and `audit_event_id` — reviewers decide without joining the audit log.
- `/audit` page renders all four kinds (`escalation`, `approval`, `modification`, `rejection`) with working filters; modification rows show the structured diff inline.
- Re-approval ping-pong is impossible by construction: a modification that re-trips bounds and is not approved on the second decision auto-terminates with `kind=rejection, escalation_reason=modify_loop_exceeded`.

**Operator quickstart:**

```bash
# Resolve a checkpoint with structured modifications
krk checkpoint resolve 0d1ba0c9e2dad899 \
  --decision modify --approver bsenel \
  --remove-action code.write \
  --constraint "scaffold the go.mod and CI workflow only this iteration" \
  --revised-confidence 0.75

# Browse the audit log
krk audit --kind modification --since 2026-06-09T00:00:00Z
krk audit --kind rejection --violations-only
```

**Originally planned scope (kept verbatim for diff against shipped):**

**Steps:**

1. **Checkpoint payload extension** (`internal/core/checkpoint/checkpoint.go`, `internal/feature/checkpoint/service.go`). Populate the already-defined `Capability` and `Confidence` fields, add `Actions []capability.Action` and `AuditEventID string`. `Service.Create` grows the signature to take these from `decide.go` at escalation time. `Checkpoint.Decision` gains a structured `Modifications` field alongside the existing free-text `Note`:
   ```go
   type Modifications struct {
       RemovedActions     []string `json:"removed_actions,omitempty"`     // capability IDs to drop
       AddedConstraints   []string `json:"added_constraints,omitempty"`   // free-text guidance for revise pass
       RevisedConfidence  *float64 `json:"revised_confidence,omitempty"`  // operator-asserted floor
   }
   ```
2. **Runner consumes the Decision** (`internal/feature/loop/runner.go`). After `<-state.decisionCh`, switch on `decision.Choice`:
   - `approve` → fall through unchanged (current behavior).
   - `reject` → emit a `kind=rejection` audit row, finalize the loop with `Status=stopped, Reason=rejected_at_checkpoint`. No silent re-loop.
   - `modify` → drop any actions in `Modifications.RemovedActions`, then call a new `stepReasonRevise(ctx, sc, draft, decision)` that runs one Reflexion revise pass using the note + constraints as the critique input. Falls back to the trimmed draft if the revise output is unparseable or empty — same never-regress guarantee as Phase 13 Reflexion. The revised plan goes through `stepDecide` *once more* — a modify may itself need re-approval if it raises the action count above bounds. Hard-cap re-approvals at one to prevent ping-pong; second escalation auto-rejects with `Reason=modify_loop_exceeded`.
3. **CLI surface** (`cli/command/checkpoint.go`, `cli/command/loop.go`). Existing `--note` stays. Add `--remove-action <capability-id>` (repeatable), `--constraint <text>` (repeatable), `--revised-confidence <float>` to populate `Modifications` from the shell:
   ```bash
   krk checkpoint resolve <id> --decision modify --approver bsenel \
     --remove-action code.write \
     --constraint "scaffold the go.mod and CI workflow only this iteration" \
     --revised-confidence 0.75
   ```
4. **API + audit wiring** (`internal/api/handler/checkpoint.go`, `internal/feature/loop/decide.go`). `POST /api/v1/checkpoints/{id}/resolve` accepts the new `modifications` block. `decide.go` adds the planner draft + checkpoint ID to the existing `kind=escalation` audit payload (already partially there — confirm and tighten). `checkpoint.Service.Resolve` writes a `kind=modification` audit row when `Choice == modify`, capturing the structured diff so the audit log shows *what changed*, not just *that something changed*.
5. **Frontend — `/checkpoints` modify dialog** (`web/src/pages/CheckpointsPage.tsx`, new `web/src/components/ModifyCheckpointDialog.tsx`). The pending-checkpoint card grows: planner-draft section (capability, confidence vs threshold, action list with per-action drop checkboxes) plus a Modify button that opens the dialog. Dialog submits the structured `Modifications` to the resolve endpoint. Approve/Reject remain one-click.
6. **Frontend — `/audit` page** (new `web/src/pages/AuditPage.tsx`, new route in `App.tsx`, nav entry in `Layout.tsx`). List view with the same filters as `krk audit` (`objective`, `agent`, `kind`, `bounds-violation`, `since`). Row click expands the payload JSON inline. Bounds-violation rows link to their checkpoint; modification rows link to the resolved checkpoint they amended. No new endpoint required — reuses Phase 13's `GET /api/v1/audit`.
7. **Tests.** Go: runner test covers all three decision branches with a stubbed agent; revise-fallback paths (empty critique, unparseable revision, empty actions) mirror the Phase 13 Reflexion test matrix; modify-loop-exceeded guard verified with a deliberately-broken modification that re-escalates. Frontend: Vitest unit test for the dialog's structured-payload assembly; Playwright e2e for the full `escalate → modify → resume → act` flow against a stubbed loop service.

**Acceptance:**

- A loop that escalates with `confidence 0.84 below threshold 0.90` (the case observed during dogfooding on 2026-06-09) can be resolved with `--decision modify --remove-action code.write --constraint "scaffold only"` and the next act step executes a plan that omits `code.write` and respects the constraint in the revise pass.
- Rejecting a checkpoint terminates the loop with `Status=stopped, Reason=rejected_at_checkpoint`; no further iterations, no orphaned goroutines.
- `GET /api/v1/checkpoints/{id}` returns a payload containing `capability`, `confidence`, `actions`, and `audit_event_id` — a reviewer can decide without leaving the response.
- `/audit` page renders ≥3 distinct kinds (`escalation`, `approval`, `modification`) with working filters; a modification row's payload diff (`removed_actions`, `added_constraints`, `revised_confidence`) is visible inline.
- Re-approval ping-pong is impossible: a contrived modification that re-trips bounds auto-rejects on the second escalation; verified by test.
- Phase 19's audit + checkpoint UX bullets are partially retired by this slice — the remaining Phase 19 work narrows to auth/quota/cost surfaces.

---

## Phase 14 — RBAC + Fine-Grained Authorization (Completed)

**Goal:** Replace Karakuri's single bearer token with role-based access control, shipped as a standalone Go module (`github.com/bsenel/karakuri/auth`) reusable by any `net/http` or `chi` server without dragging in Karakuri itself.

**Breaking change, deliberately.** `middleware.BearerAuth`, `cfg.Auth.Token` and `KARAKURI_AUTH_TOKEN` are deleted outright — from the Makefile secret, `docker-compose.yml`, `docker-entrypoint.sh`'s config patch, `deploy/`, both READMEs and the web login modal. There is no compatibility mode and no "RBAC disabled" setting. Existing deployments must set `KARAKURI_AUTH_JWT_SECRET` (the server refuses to boot without one) and existing API clients must obtain a token instead of sending a static one.

**What shipped — three modules and a shim:**

- **`github.com/bsenel/karakuri/auth`** — the authorization engine, with **zero external dependencies**. Principals, a consumer-supplied action catalog, roles with inheritance, policies with typed conditions, scoped role bindings, a deny-wins authorizer that returns a full decision trace, an in-memory reference `Store`, and `chi`-compatible middleware. 99.6% statement coverage.
- **`auth/jwt`** — HS256 (`crypto/hmac`) and EdDSA (`crypto/ed25519`) over the standard library, behind an algorithm allowlist. `alg: none` never resolves to a key; algorithm confusion is impossible because `kid` selects the key first and the header's `alg` must equal that key's own; signature comparison is constant-time; a token with no `exp` is rejected. A `Keyring` separates the signing key from the accepted verification keys, so rotating the signer does not invalidate tokens already in flight.
- **`auth/sql`** — `database/sql` persistence, no ORM and no driver dependency outside tests. Six tables, two dialects differing only in placeholder syntax and the boolean column type. Timestamps are epoch milliseconds because drivers disagree about how `DATETIME` round-trips.
- **`internal/auth`** — the Karakuri shim: the action catalog, the five built-in roles, the route→permission table, the store built on the `*sql.DB` behind GORM, the keyring, and first-boot seeding.

**The authorization model:**

```
Principal ──has──> RoleBinding{Role, Scope} ──grants──> Role{Policies, Inherits}
                                                          │
                                              Policy{Action, Resource, Effect, Conditions}
```

- **Nothing is implicit.** Every action is registered in a catalog; a policy naming one that is not fails at boot rather than silently granting nothing.
- **Roles compose.** `viewer` → {`auditor`, `contributor`, `operator`}, plus `admin`. Each permission is stated once.
- **Conditions are a closed, typed set** (`owner_equals`, `attr_equals`, `attr_in`) rather than an expression language, so every condition stays readable by whoever audits it and evaluation is total — an unresolvable key is an unsatisfied condition with a reason, never a parse error at request time.
- **Bindings carry a scope**, separating "alice is an operator" from "alice is an operator on `twin:abc`". Phase 17 let that scope name a container instead, without changing the field or the grammar.
- **Precedence is exactly `deny > allow > default deny`.** Specificity deliberately does not break ties — ranking by specificity is the IAM footgun where adding a narrow grant silently punches a hole in a blanket restriction.

**Tokens rotate.** Access tokens are short-lived JWTs (15 minutes by default), verified statelessly but with the principal reloaded from the store so disabling an account takes effect on the next request rather than at expiry. Refresh tokens are 256 bits of `crypto/rand`, stored only as a SHA-256 digest, linked into a *family*, and **rotated on every exchange**. Presenting a spent one revokes the entire lineage (OAuth 2.1 BCP §4.14.2): rotation means a legitimate holder never replays, so a replay is evidence the token leaked. Spending is a compare-and-set (`UPDATE … WHERE used_at IS NULL`), because a check-then-write lets two racing clients both succeed and the reuse detector never fires on the case it most needs to catch.

**Credentials.** Humans get passwords hashed with PBKDF2-HMAC-SHA256 (stdlib `crypto/pbkdf2`, 600k iterations, cost encoded in the hash so it can be raised later without invalidating anything). Service accounts get no password at all — an administrator mints their first refresh token, printed once. On a database with no principals the server creates an `admin` using `KARAKURI_AUTH_BOOTSTRAP_PASSWORD`, and **refuses to start if it is unset** rather than generating one and logging it: Karakuri fans its logs out to Datadog, Loki, Elasticsearch and CloudWatch (Phase 12), so "logged once at WARN" means "copied to every configured log sink".

**Ownership.** `DigitalTwin.OwnerID` + a nullable `twins.owner_id` column; the creating principal is stamped at create time. The `contributor` role uses `owner_equals` to allow changing only the twins you created — expressed in policy, so it appears in `krk auth policies list` and in `/auth/me`, rather than as an `if` buried in a handler. Twins predating the column are unowned, and `owner_equals` is never satisfied by an unowned resource, so ownership-scoped grants do not silently cover them.

**Denials are audited.** A refused request writes a `kind=authz_denied` row into the same `tool_events` log as authority-bounds escalations, carrying the decision trace. `krk audit --kind authz_denied` and the `/audit` page surface it with no new endpoint — reviewing who approved what now also shows who was turned away.

**The browser never holds a token.** A single-page app has no safe place to keep one — localStorage, sessionStorage and a module variable are all readable by any script that gets injected, and a stolen refresh token is a persistent session. So the SPA posts `{"cookie": true}` and the pair comes back as `Set-Cookie` instead: `HttpOnly` (unreadable by script), `SameSite=Strict` (which is what makes cookies safe here without a separate CSRF token, since the SPA is same-origin with the API), and `Secure` unconditionally, with one named opt-out — `auth.cookies.insecure_allow_http` — for plain-HTTP development. Deriving `Secure` from the current request was the earlier design and was wrong: one downgraded request issues a cookie that then travels in the clear. Cookie mode omits the tokens from the response body entirely, because handing them over and asking the caller not to store them defeats the point. API clients still get tokens in the body and send them as bearer headers.

**Fixed along the way:** authenticated SSE never worked. `web/src/api/sse.ts` passed `?token=`, which `BearerAuth` never read; and the stream handler set its headers without flushing them, so `net/http` buffered the response and `EventSource.onopen` never fired on an idle stream. Both are fixed — `EventSource` carries the access cookie automatically, and headers flush on connect. The token never appears in a URL: query strings land in access logs, proxy logs and `Referer` headers. The `auth` module still offers an opt-in `?access_token=` fallback for consumers that genuinely cannot use cookies; Karakuri leaves it off, and an integration test asserts a query token is refused.

**Log hygiene.** Anything a request can influence — a principal ID, a request path, a remote address — is sanitized before it reaches a log record, because a value containing a newline forges a whole entry, and these logs are shipped to aggregators where a fabricated `authorization granted` line would sit unchallenged next to real ones.

**CLI + frontend.** `krk auth login|logout|whoami|users|roles|policies|bindings|check|catalog`, with credentials cached per API URL under `~/.config/karakuri/credentials.json` (mode 0600) and refreshed automatically, so you log in once rather than per command. Passwords are read from stdin, never flags. The SPA's login form exchanges an ID and password for a cookie session and never sees a token; since it cannot read an expiry either, refresh is reactive — a 401 triggers one refresh and one retry. All callers share a single in-flight refresh, because letting concurrent 401s each refresh would spend the rotating token more than once and trip the server's reuse detector.

**Acceptance — met:**

- `auth` reaches **99.6%** line coverage with **no Karakuri imports and an empty require block**; `go run ./auth/examples/server` runs a self-contained demo of login, ownership conditions, a scoped binding, rotation, reuse detection and an explained denial.
- The integration suite walks the route→permission table for every built-in role, asserting refused-or-not on each — so a route that loses its permission in `server.go` fails a test rather than quietly opening up. Deny-wins, expired tokens, rotation, reuse detection, scoped bindings, ownership conditions, the httpOnly cookie session end to end, and the refusal of a query-string token are each covered.
- CI gates coverage per module (95% for `auth`, 90% for `auth/sql`, whose residue is `RowsAffected`/`Commit` error branches needing a fault-injecting driver) and runs the example server.
- The High-severity "authority bounds misconfiguration" risk is mitigated at the routing layer, not only at the loop's decide step.

**Deviations from the plan, recorded:**

- The module is **stdlib-only**, not "stdlib + `golang.org/x/time`" as originally written. Nothing in an authorizer needs a rate limiter; that dependency belongs to Phase 15's quota module. The release workflow enforces the empty require block.
- **No `cfg.Auth.RBAC.Enabled` and no legacy-token principal.** The original plan kept RBAC off by default with the old token resolving to an admin. Shipping it always-on removes the "secure once you remember to turn it on" failure mode, at the cost of a hard upgrade step.
- The **release workflow refuses to publish a module carrying a `replace` directive**, so `auth/sql` cannot be tagged until `auth/v0.1.0` exists and the directive is dropped. Root `go.mod` carries `replace` directives until then.

**Operator quickstart:**

```bash
export KARAKURI_AUTH_JWT_SECRET="$(openssl rand -base64 32)"
export KARAKURI_AUTH_BOOTSTRAP_PASSWORD="choose-something"   # first boot only
./bin/server        # refuses to start without both of the above

echo "$ADMIN_PW" | krk auth login --id admin --password-stdin
krk auth whoami

# A contributor manages only what it creates
echo "$PW" | krk auth users add --id alice --roles contributor --password-stdin
# An operator scoped to one twin
echo "$PW" | krk auth users add --id olive --roles operator --scope twin:abc --password-stdin
# CI gets a rotating refresh token instead of a password
krk auth users add --id ci --roles operator --service-account

# Why was that refused?
krk auth check alice twin:update twin:someone-elses
krk audit --kind authz_denied
```

**Originally planned scope (kept verbatim for diff against shipped):**

**Goal:** Replace Karakuri's single bearer token with role-based access control, shipped as a standalone Go module (`github.com/bsenel/karakuri/auth`) reusable by any `net/http` or `chi` server without dragging in Karakuri itself.

**Steps:**

1. **Standalone module** `github.com/bsenel/karakuri/auth` — multi-module monorepo entry with its own `go.mod` (stdlib + `golang.org/x/time` only). Exposes `Principal`, `Policy`, `Authorizer`, `TokenResolver`, plus a `RequirePermission(action, resourceFn)` chi/net-http middleware and `PrincipalFromContext(ctx)` accessor for downstream handlers. Ships an in-memory `Store` reference implementation.
2. **Sister module** `github.com/bsenel/karakuri/auth/sql` — `database/sql`-backed `Store` (no ORM dep) so external repos pick their own DB driver.
3. **Karakuri integration shim** (`internal/auth/karakuri.go`) — canonical role catalog (`admin`, `operator`, `viewer`, `auditor`) + resource catalog (`twin:*`, `objective:*`, `loop:*`, `checkpoint:*`, `artifact:*`, `audit:*`, `domain:*`, `quota:*`). Mounts `auth.RequirePermission(...)` after `BearerAuth` in `internal/api/server.go` per route group.
4. **Twin ownership** — new `DigitalTwin.OwnerID string` field + nullable `twins.owner_id` column. `NULL` = legacy org-scoped, non-NULL scopes via `twin:owned` pattern.
5. **CLI** — `krk auth users add/list`, `krk auth roles list`, `krk auth policies list`, `krk auth check <user> <action> <resource>` (policy debug helper).
6. **Backward compat** — `cfg.Auth.RBAC.Enabled` off by default; legacy bearer token resolves to `Principal{ID:"legacy", Roles:["admin"]}` so existing deployments upgrade unchanged.
7. **Release workflow** — `.github/workflows/release-auth.yml` triggered on `auth/v*.*.*` tags to publish independently of Karakuri's main version.

**Acceptance:** Standalone module hits ≥95% line coverage with no Karakuri imports; `go run ./auth/examples/server` runs a 50-line demo independently. Three integration tests (admin/operator/viewer tokens) pass on every `/api/v1/*` route with correct 200/403 outcomes; Deny-wins precedence verified. The High-severity "Authority bounds misconfiguration" risk is mitigated at the routing layer, not just at the loop's decide step.

---


## Phase 15 — API Rate Limiting + Quota Management (Completed)

**Goal:** Add per-twin / per-capability / per-LLM-budget rate limiting + quota enforcement, shipped as a standalone Go module (`github.com/bsenel/karakuri/quota`) reusable by any `net/http` or `chi` server.

**Steps:**

1. **Standalone module** `github.com/bsenel/karakuri/quota` — own `go.mod` (stdlib + `golang.org/x/time/rate`). Exposes three algorithms (TokenBucket, FixedWindow, SlidingLog), a `KeyExtractor` callback so callers compose keys from any request attribute, a `Limit(...)` chi/net-http middleware, and a separate `Quota` type for hard caps over long windows (hourly/daily/monthly). In-memory `Backend` ships in the core module.
2. **Sister submodules** — `github.com/bsenel/karakuri/quota/redis` (sorted-set sliding window via go-redis + Lua) and `github.com/bsenel/karakuri/quota/sql` (`database/sql` long-window counters with `FOR UPDATE`). Each gets its own go.mod so callers only pull deps they use.
3. **Karakuri integration shim** (`internal/quota/karakuri.go`) — four canonical tiers: per-twin request rate (60/min, burst 20), per-capability daily cap (1000/day per twin per capability), LLM-token budget per twin per day (hooks the existing `TokensUsed` field from each LLM provider), and per-adapter defaults matching documented public rates.
4. **Distributed mode** — `cfg.Quota.Backend = memory | redis | sql`; Redis is the only one consistent across replicas.
5. **Pressure events** — new `Quota.Pressure` event on the SSE hub when usage crosses 80%.
6. **LLM-budget checkpoint** — exhaustion produces a Phase 13-style checkpoint (human approval to continue) instead of a 500 — the loop pauses rather than fails.
7. **CLI** — `krk quota show --twin <id>`, `krk quota reset --twin <id> --capability <cap>` (admin override gated by Phase 14's `quota:admin` permission), `krk quota config`.
8. **Release workflow** — `.github/workflows/release-quota.yml` triggered on `quota/v*.*.*` tags.

**Acceptance:** Property-based test proves TokenBucket never exceeds `rate*elapsed + burst` over any window. 100 req/s × 5 fake twins at a 60/min limit yields exactly 60×200 + 40×429 each. LLM-budget exhaustion produces a checkpoint event verified end-to-end with a fake provider returning inflated `TokensUsed`. All three backends pass `go test -race`. External repo can `go get github.com/bsenel/karakuri/quota@v0.1.0` and build without pulling Karakuri.

**What shipped — three modules and a shim:**

- **`github.com/bsenel/karakuri/quota`** — the engine, with **zero external dependencies**. Three algorithms (token bucket, fixed window, sliding log), calendar quotas, a `chi`-compatible `Limit` middleware with pressure and audit hooks, and an in-memory reference backend. 100% statement coverage.
- **`quota/sql`** — `database/sql` persistence for counters that outlive a process. Two tables, two dialects, and a transaction per take.
- **`quota/valkey`** — cross-replica limiting, one Lua script per algorithm and one round trip per take. It brings **no client**: a one-method `Doer` and the scripts, so adopting it does not mean adopting somebody else's connection pool.
- **`internal/quota`** — the Karakuri shim: the four tiers, the key extractor, backend selection, and the token budget.

**One contract, three backends.** `quota/quotatest` runs the identical table against every backend, so they cannot silently diverge — a 200-way race that a check-then-write implementation cannot survive, the token bucket's `rate × elapsed + burst` bound under an irregular arrival walk, and the rule that a refusal consumes nothing and never carries a zero wait. It earned its keep repeatedly: it caught a floating-point knife-edge in the bucket refill, a `Peek` that asked whether a *zero-cost* request would fit (always yes, even when exhausted), and `quota/sql` failing under contention.

**`Backend` hands out decisions, not counters.** A low-level interface would push sorted-set semantics into SQL and mutex semantics into Valkey and leave each implementation reinterpreting the algorithms anyway. This way each is written in its own idiom — Go under a lock, one Lua script, one transaction — and **atomicity per key is a stated part of the contract** rather than an accident.

**Rate limits and quotas are different types.** A rate limit refuses you and expects you back in a moment; a quota refuses you until tomorrow. `Quota` puts the calendar period *in the key*, so at midnight the key changes and the new period starts at zero — no backend implements a calendar, and the reset is identical across all three.

**Limiting fails open**, deliberately inverting Phase 14's rule. An authorizer that cannot answer must refuse, because the cost of wrongly allowing is a breach. A limiter that cannot answer should allow, because the cost of wrongly refusing is an outage caused by the component meant to prevent one. `FailClosed()` exists for hard spend caps.

**An exhausted LLM budget pauses rather than fails.** A budget is a business limit, not a fault, so the loop raises a Phase 13 checkpoint (`reason=llm_budget_exhausted`) and waits for a human. The charge wraps the agent rather than sitting in the reason step, because the reflexion path calls `Run` four separate times and a check in one of those leaves three unmetered — which are exactly the ones a loop that will not converge keeps making.

**Deviations from the plan, recorded:**

- **Stdlib only**, not `golang.org/x/time/rate`. `rate.Limiter` is one unkeyed bucket, so the keyed map and its eviction are ours either way; what was left to borrow was sixty lines of arithmetic, which is not worth the empty require block the release workflow verifies.
- **Valkey, not Redis**, and the module brings no client at all.
- **The request tier keys on the principal, not the twin.** The plan said to key twin routes on the twin. That is wrong: a caller could spend a full budget against every twin it can see, so the limit would bound nothing. The twin dimension belongs to the per-capability quota, which bounds a twin's work rather than a caller's traffic.
- **A LiteLLM backend the roadmap did not anticipate.** Tokens are a poor proxy for spend across models that differ by an order of magnitude in price, so `llm_budget_backend: litellm` delegates to a gateway that counts dollars. It is opt-in; `native` remains the default and Karakuri remains a single binary.
- **`quota/sql` covers all three algorithms**, not just long windows: the shared contract demands it, and a backend that silently does not support an algorithm is a trap.

**Known gap:** the Gemini *HTTP* provider cannot be pointed at a gateway — langchaingo's `googleai` package exposes `WithHTTPClient` but no `WithBaseURL`. The Gemini CLI path can be, and is. Closing it needs an OpenAI-compatible adapter, which is its own change.

**Acceptance — met:**

- `quota` reaches **100%** coverage with **no Karakuri imports and an empty require block**; `quota/sql` 91.6% and `quota/valkey` 96.0% against gates of 90.
- The property that a token bucket never admits more than `rate × elapsed + burst` holds over a 3000-step irregular arrival walk, with a tightness check so a backend that refused everything could not satisfy it vacuously.
- 100 requests against a 60/min limit yields exactly 60 allowed and 40 refused, on every backend.
- CI runs a `valkey/valkey:8-alpine` service container, so the Lua is exercised against a real server rather than a fake.
- The integration suite proves the 429 carries `Retry-After` and the `X-RateLimit-*` trio, that `/health` is exempt, and that one principal exhausting its budget does not refuse anybody else.

**Also fixed along the way:** both CI workflows filtered `pull_request` to base `main`, so a stacked PR got **no checks at all** — not a failing check, none. Phase 14's stack was only ever verified after each PR was retargeted, which is the point at which review is already over.


---

## Phase 16 — Federated Identity (OIDC + SAML) (Completed)

**Goal:** Plug enterprise IdPs (Keycloak, Okta, Auth0, Azure AD, ADFS) into Karakuri via OIDC and SAML, with each protocol implemented as an independent submodule of the Phase 14 `auth` module.

**Steps:**

1. **OIDC submodule** `github.com/bsenel/karakuri/auth/oidc` — depends on `github.com/coreos/go-oidc/v3` + `golang.org/x/oauth2`. Implements `auth.TokenResolver` against any OIDC-discovery endpoint; validates ID tokens via cached JWKS; maps a configurable `RoleClaim` JSON path to `Principal.Roles`. Browser flow via `LoginHandler` + `CallbackHandler` (auth-code + PKCE); machine-to-machine flow via bearer tokens unchanged.
2. **SAML submodule** `github.com/bsenel/karakuri/auth/saml` — depends on `github.com/crewjam/saml`. Implements `auth.TokenResolver` from SAML assertions; maps configurable `RoleAttribute` to `Principal.Roles`. Returns SP ACS + SSO endpoint handlers for the caller to mount.
3. **Karakuri integration shims** (`internal/auth/oidc.go`, `internal/auth/saml.go`) — config block `cfg.Auth.Provider = bearer | oidc | saml`; bootstrap selects + wires the appropriate resolver.
4. **CLI login flow** — `krk auth login` opens the browser to the IdP, listens on `localhost:8765` for the callback, persists the token to `~/.config/karakuri/token`. Subsequent CLI calls read that file.
5. **Frontend `/login` route** — redirects unauthenticated users to the configured IdP. Lands at `/auth/callback`. Existing bearer flow keeps working when provider is `bearer`.
6. **Break-glass admin** — `cfg.Auth.BreakGlass.Token` allows a single static bearer token to bypass IdP for emergency operator access during IdP outages; logged at WARN every use; tagged in the audit log with `kind=break_glass`.
7. **Release workflows** — `release-auth-oidc.yml`, `release-auth-saml.yml` on respective tag patterns.

**Acceptance:** End-to-end test stands up Keycloak in `dockertest`; logging in yields a `Principal` with `Roles` derived from the IdP's `groups` claim; JWKS rotation works without service restart. SAML round-trip exercised via `crewjam/saml`'s `samltest` helpers. CLI `krk auth login` completes against a test OIDC provider. Break-glass token works when IdP is unreachable and produces audit records.

**What shipped — two protocol modules and a provisioner:**

- **`github.com/bsenel/karakuri/auth/oidc`** — discovery, a cached JWKS that refetches on an unseen key ID, a `TokenResolver` for the machine-to-machine path, and the authorization-code flow with PKCE for browsers.
- **`github.com/bsenel/karakuri/auth/saml`** — SP metadata, the assertion consumer, and attribute mapping over `crewjam/saml`. It brings no protocol code of its own: signature, audience and time-window validation are the library's, because hand-rolling any of it is a way to get signature verification subtly wrong, and that fails open.
- **`auth` core** — `ExternalIdentity`, `RoleMap`, `Provisioner`, `ChainResolver` and `Sealer`, all with the module's require block still empty. A new protocol reduces to producing an `ExternalIdentity`.
- **`internal/auth/federation.go`** — the Karakuri shim: provider selection, group-to-role mapping, and the derived flow-state key.

**The roadmap described a field that does not exist.** Step 1 said to map a claim to `Principal.Roles`. There is no such field, and adding one would not have helped: `StoreAuthorizer` resolves permissions from role bindings in the `Store`, so a principal assembled from claims holds none and is denied everything. The alternative to giving the authorizer a second source of truth is **just-in-time provisioning** — write the provider's user into the store on the way in, after which a federated principal is an ordinary principal and ownership, quota, audit and `/auth/me` all work untouched. [ADR 009](adr/009-federated-identity-jit-provisioning.md) records the decision and what it costs: revocation is lazy until the next login, and disabling a principal is the fast path.

**Principal IDs are namespaced.** Local principals are named by an administrator; federated ones are named by whoever controls the provider's subject field, which in some deployments is the user. Without `oidc:`/`saml:` prefixes, an identity provider asserting `sub=admin` takes over the local bootstrap administrator.

**Matching no group grants nothing.** `role_map.default` is empty unless an operator sets it, because everybody in a corporate directory can authenticate against a corporate identity provider — a default role there is a grant to the whole company.

**`ChainResolver` is subtler than it looks.** The obvious implementation stops at the first resolver returning anything but "no credential", treating a malformed one as a client bug worth surfacing. That breaks federation outright: a provider-issued token *is* a bearer token, so the local resolver reaches it first and rejects the signature, and the federated resolver never runs. It continues past verification failures and reports the first substantive error only when every resolver has declined.

**Deviations from the plan, recorded:**

- **No static break-glass token**, contrary to step 6. Local password login stays mounted alongside any provider, so the bootstrap administrator is the emergency path. Phase 14 deleted the static bearer token; re-adding a long-lived credential to survive a temporary outage trades a permanent risk for a temporary one.
- **No `TokenResolver` for SAML**, contrary to step 2. An assertion is a one-time login artifact delivered by browser POST — single-use, bound to one recipient, valid for minutes. Building a per-request resolver on one means accepting replayed assertions. SAML has no machine-to-machine story; OIDC is that story.
- **A stub identity provider alongside Keycloak**, rather than `dockertest`. The in-process provider proves Karakuri drives the flow correctly and runs on a laptop with no Docker; a real Keycloak in its own CI job proves Karakuri agrees with an actual provider about discovery documents, JWKS shapes, audiences and where groups live. The Keycloak job exercises the bearer path — driving Keycloak's login HTML would be a test of Keycloak's login HTML.
- **No new release workflows.** `release-auth.yml` already matches `auth/*/v*.*.*` and derives the module directory from the tag, so `auth/oidc/v0.1.0` and `auth/saml/v0.1.0` are covered. Two more near-identical workflows would be duplication rather than coverage.
- **The flow-state key is derived from the JWT signing material**, not configured. One fewer secret to distribute, and every replica agrees without being told to — a flow key that differs between replicas produces logins that fail intermittently behind a load balancer.
- **CLI login uses a loopback handoff bound to a secret.** A browser finishes a login holding httpOnly cookies, which a terminal cannot read. The code that comes back through the browser is useless without a secret that never leaves the CLI process, and it is a spent refresh token the moment it is redeemed — so a replayed code ends the session it was stolen from rather than granting a second one.

**Acceptance — met:**

- `auth` holds at 96.7% with an empty require block; `auth/oidc` 96.9% and `auth/saml` 93.0% against gates of 90.
- JWKS rotation is proven by forcing one rather than asserting the documentation.
- The SAML round trip runs against a real `crewjam.IdentityProvider`, so signatures are genuinely produced and genuinely verified.
- CI runs a live Keycloak: realm, client, group and user provisioned through the admin API, then a genuine ID token presented to Karakuri and the mapped role exercised.
- The integration suite proves a federated login provisions a principal, that roles are *reconciled* rather than accumulated across logins, that a user in no mapped group can log in and do nothing, and that password login still works with a provider configured.

**Also fixed along the way:** `quota/sql`'s 200-way contract race failed intermittently in CI with `SQLITE_BUSY`. SQLite permits one writer, so concurrent takes were always going to queue — the bug was that they queued in SQLite's busy handler, which retries with backoff and gives up at the timeout, so an arbitrary subset waited the whole timeout and then failed. They queue on a mutex now, which costs no throughput because the work was serial regardless, and respects context cancellation, which a busy timeout does not.

---

## Phase 17 — Hierarchical Resources + Org Units (Completed)

**Goal:** Extend Phase 14's flat resource model (`twin:abc`) to a path model (`org:acme/team:eng/twin:abc`) so multi-team and multi-org deployments isolate access naturally.

**Steps:**

1. **`auth` module v0.2.0** — additive extension. New `HierarchicalAuthorizer` (wraps `Store`) treats resource strings as paths; policy on `org:acme/*` covers all descendants. New utilities `IsAncestor(ancestor, descendant)` and `ParentOf(resource)`. Flat `Authorizer` keeps working — integration shim picks via `cfg.Auth.RBAC.Hierarchical`.
2. **Domain types** — `Org { ID, Name, ParentOrgID, CreatedAt }` (orgs nest), `Team { ID, Name, OrgID, CreatedAt }`, `Membership { PrincipalID, OrgID, TeamID, Role }`.
3. **Schema** — new `orgs`, `teams`, `memberships` tables; `twins.team_id` nullable column. Migration: existing twins remain `TeamID=NULL` (org-scoped, legacy behavior).
4. **Resolver chain** — `TeamID → OrgID → ParentOrgID` walks build the resource path; cached per request.
5. **CLI** — `krk org create/list`, `krk team create/list`, `krk membership add/remove`. `krk auth users add` gets `--org` / `--team` flags.
6. **API endpoints** — `GET/POST /api/v1/orgs`, `GET/POST /api/v1/teams`, `GET/POST /api/v1/memberships`, all gated by Phase 14 permissions.

**Acceptance:** Hierarchical Authorizer unit tests cover ancestor walking, deny-wins precedence across hierarchy levels, and path validation. Integration test: Alice (operator in `team:eng`) reads `team:eng/twin:abc` but not `team:hr/twin:xyz`; admin in `org:acme` reads both. Migration test loads a v0.1.0 SQLite snapshot, applies the hierarchical migration, and verifies all flat policies still pass without rewrites.

**What shipped — a set of labels, not a path.** The hierarchy lives *on* the resource
rather than in its name: `ResourceRef.Scopes` carries the ancestor closure
(`["team:t_7f2a", "org:o_9c31"]`), and a binding covers a resource if its scope matches
the resource **or any label**. `RoleBinding.covers` is one line and `matchPattern` is
untouched. [ADR 010](adr/010-scope-sets.md) records the decision in full.

- **`auth`** — `ResourceRef.Scopes`, `InScope`, `GrantedScopes`, `ScopeLabel`,
  `ValidateScopes`, and `RoleGrant` so a federated mapping carries a scope.
- **`internal/core/container` + `internal/feature/container`** — orgs, teams and
  projects, with cycle, depth and per-parent-name guards, and the closure recomputed
  when the tree moves.
- **`containers` + `resource_scopes`** — the tree, and the flattened labels
  authorization matches against, with `direct` separating what was declared from what
  was derived.
- **`krk org` / `krk team` / `krk project`**, and `--org/--team/--project` on
  `krk auth bindings add`.

**The roadmap's design conflicted with the module's own rules.** Step 1 asked for a
second authorizer treating resource strings as paths, selected by config flag.
`auth/AGENTS.md` says *"patterns use one grammar … do not add a second matching rule;
extend that one"* — and a second matcher chosen by configuration is two authorization
semantics in one binary, where the one that matters is whichever is set on the day
something goes wrong. Scope sets need no grammar change at all: `team:t_7f2a` is
already a valid pattern.

**Paths would not have fixed what actually leaked.** `GET /twins` returned every twin,
filtered only on `kind` and `domain`. Per-resource denial is not isolation while the
listing is all-or-nothing, and no path model changes that. The listing is now built
from `GrantedScopes` as an indexed `IN` over `resource_scopes.label` — which a path
model could not be, needing `LIKE 'org:acme/%'`.

**Phase 16's hole is closed.** `Provisioner.reconcile` wrote every managed binding with
`Scope: "*"`, so a directory group of two hundred people was two hundred
globally-scoped principals. `role_map` entries now carry a container, resolved from
name to ID at boot.

**Labels carry IDs, never display names.** Two organisations may each have a team called
"Engineering", and if the label were the name a grant on one would silently cover the
other. Microsoft documents this same mistake against their own management groups as
*"this common error"*. Renaming a container rewrites no binding.

**Deviations from the plan, recorded:**

- **No `HierarchicalAuthorizer`, no `cfg.Auth.RBAC.Hierarchical` flag, no
  `IsAncestor`/`ParentOf`.** Those are path utilities; in a set model parentage lives in
  the container tree, which is Karakuri's side. `auth` gained `InScope`, `ScopeLabel`
  and `ValidateScopes` instead.
- **No `memberships` table.** Membership *is* a binding once a scope can name a
  container, so a second table would be a second source of truth about the same fact.
- **No `twins.team_id` column.** A resource belongs to a *set* of containers, not one,
  which is what lets a twin be in its team, its org and a cross-organisation project at
  once — the thing Azure could not express and shipped Service Groups for.
- **A `project` container the plan did not have.** It is how cross-tenant collaboration
  works without a second construct grafted alongside the hierarchy.
- **No migration test against a v0.1.0 snapshot.** The change is additive by
  construction: a resource with no containers carries no labels and matches exactly what
  it matched before. That property is pinned directly
  (`TestResourcesWithoutScopesAreUnchanged`, `TestUnscopedGrantsAreUnchanged`), which is
  a stronger statement than one fixed snapshot.

**One behaviour change worth stating plainly.** A collection ref is `twin:*`, which no
container-scoped binding matches — so a team-scoped principal could read their twins one
at a time but could not call `GET /twins` at all. List routes now carry the caller's own
containers, so the route check answers *"may you list"* and the filter answers *"which"*.
This also changed what a single-object binding sees: a flat 403 became exactly its own
twin. Nothing new is exposed, because every row returned is one the principal can already
fetch by id.

**Acceptance — met, with the hierarchy expressed as sets:**

- `auth` holds at **96.8%** against its 95% gate, with every new function at 100%.
- Deny-wins precedence across levels is covered (`TestDenyAtOrgBeatsAllowAtTeam`), as is
  the case the whole design exists for: two organisations whose teams are **both called
  "eng"**, isolated end to end from the tree through `InScope` to a 403.
- The integration suite proves a federated login lands *inside* a team rather than over
  everything (asserted against `auth_role_bindings` directly), that listing is confined
  to the caller's tenant for twins **and** objectives, that the listing agrees with the
  per-resource check on every row it returns, and that a twin in no container behaves
  exactly as it did before.
- The four self-authorization rules each have a test asserting both the allowed and the
  refused direction.
- The listing SQL is exercised against a real database, including the case that matters
  most: an empty selector matches **nothing** rather than everything.

---

## Phase 18 — Quota Self-Service + Cost Attribution (Completed)

**Goal:** Let users request quota increases through an approval workflow, and give operators per-team / per-twin / per-provider cost attribution so spend is visible before the bill arrives.

**Steps:**

1. **`quota` module v0.2.0** — adds `Request { ID, PrincipalID, Key, NewLimit, NewWindow, Reason, Status, CreatedAt, DecidedBy, DecidedAt }` + `RequestStore` interface with `Submit/Decide/List`. Ships `MemoryRequestStore`; `quota/sql` gets corresponding SQL impl.
2. **Cost-attribution submodule** `github.com/bsenel/karakuri/quota/cost` — own `go.mod`. Provider-agnostic `CostEvent { PrincipalID, ResourceKey, Provider, Units, UnitKind, OccurredAt, Metadata }` and `Ledger` interface (`Record`, `Aggregate`). Pluggable `Pricer` interface; default `StaticPricer` reads a YAML table of `(provider, model) → cost-per-unit`.
3. **Karakuri integration** (`internal/quota/requests.go`, `internal/quota/cost.go`) — LLM provider registry hooked to record `CostEvent`s on every call. Tool adapter wrappers (`internal/platform/tools/*`) record per-adapter call costs.
4. **API endpoints** — `POST/GET /api/v1/quota/requests`, `POST /api/v1/quota/requests/{id}/decide` (admin), `GET /api/v1/cost/aggregate` (buckets by principal/team/twin/provider/day with drill-down).
5. **CLI** — `krk quota request --key <k> --new-limit <n> --window 24h --reason "ramping prod"`, `krk quota requests list/approve/reject`, `krk cost report --team eng --since 30d --group-by provider`.
6. **Hub events** — `Cost.Recorded` published per ledger write; powers the Phase 19 dashboard live updates.
7. **Release workflow** — `.github/workflows/release-quota-cost.yml`.

**Acceptance:** Self-service workflow integration: Alice submits a request, Bob (admin) approves, Alice's effective quota reflects the new limit within 60 seconds. Cost report matches scripted `TokensUsed` values from a controlled loop run within ±0.01% floating-point tolerance. Concurrent ledger writes verified race-free.

**What shipped — an approval writes an override, and spend carries its labels.**
A request on its own is a row nothing reads; what makes an approval mean something is a
per-subject **override** consulted when a tier is resolved. And because Phase 17 had just
made teams real, a cost report is filtered by the same scope sets as everything else.
[ADR 011](adr/011-overrides-and-labelled-spend.md) records the decisions in full.

- **`quota`** — `Override`, `OverrideStore`, `Resolver` with a 30-second TTL cache, and
  `quota.Resolve(...)` as an *option* on `Limit` so no existing caller changed. Then
  `Request`, `RequestStore` and `Requests{Submit,Decide,List}`, where approving writes the
  override **before** marking the request approved.
- **`quota/sql`** — `quota_overrides` and `quota_requests`, alongside the existing counters.
- **`quota/cost`** — a sibling module with an empty require block: `Event`, `Ledger`,
  `Pricer`, `StaticPricer`, `MemoryLedger`, and `Query`/`Bucket`/`Fold` for reports.
- **`quota/cost/sql`** — `cost_events` for drill-down and `cost_daily` for totals, the
  rollup upserted in the event's own transaction, plus a retention sweep scheduled daily
  from bootstrap: one row per model call and per tool call adds up, and the rollup outlives
  the events it was folded from.
- **Karakuri** — `internal/quota/requests.go` and `cost.go`, `Provider`/`Model` on
  `coreagent.Output`, token cost recorded beside the budget charge and adapter cost beside
  the tool event, `cost_recorded` on the hub.
- **API + CLI** — `quota:request`, `quota:approve` and `cost:read`; `POST/GET
  /quota/requests`, `POST /quota/requests/{id}/decide`, `GET /cost`; `krk quota request`,
  `krk quota requests list/approve/reject`, `krk cost report`.

**The plan's step 1 could not meet its own acceptance criterion.** It specified `Request` +
`RequestStore` and stopped there, so approving would have written a row nothing consulted
and *"Alice's effective quota reflects the new limit within 60 seconds"* was unreachable.
Overrides are the load-bearing addition, and the 30-second resolver cache is why the
criterion can say sixty: the request tier runs on every API call, and a database read per
request to ask "has anyone raised this lately" is the wrong trade.

**Approving is bounded by containment, not just by role.** The route gate answers "may you
decide at all"; it cannot answer "may you decide *this* one", because the subject arrives
inside a stored request rather than in the URL. The handler re-checks `quota:approve`
against the subject rendered as a resource with its containers attached — the same rule
ADR 010 set for handing out bindings, restated for money. Rejecting is deliberately
ungated: somebody who may decide at all may always decline, and requiring the scope to say
"no" would leave other tenants' requests pending forever.

**The capability quota is enforced for the first time.** `Deps.TakeCapability` had been
configured, documented, defaulted and called from nowhere since Phase 15 — confirmed by
grep across `internal/`, where the only mention was its own definition. The act step now
charges it before the action, failing open with a `quota_pressure` event when the backend
cannot be read.

**Deviations from the plan, recorded:**

- **Per-subject overrides, which the plan did not have.** Without them an approval changes
  nothing. This is the difference between the workflow and the feature.
- **`Request` carries an `Override`, not `{PrincipalID, Key, NewLimit, NewWindow}`.**
  Approving one *is* writing the other, so `Request.Override()` is a method; a request that
  needed further decisions to become an override would be a request nobody could act on.
- **`StaticPricer` takes a Go map, not a YAML table.** Karakuri's config parses the YAML
  and hands it over, which keeps `quota/cost`'s require block empty — the same discipline
  ADR 007/008 set when `auth` implemented JWT over `crypto/hmac`. Nothing is priced by
  default: a shipped price table would be wrong the week after it shipped.
- **`GET /cost`, not `/cost/aggregate`.** One endpoint that groups by whatever you ask for,
  rather than a second one for drill-down.
- **Cost events are scope-filtered**, which the plan predates. A per-resource check that
  refuses another tenant's twin means nothing while a report totals that twin's spend.
- **Raw events *and* a daily rollup, in one transaction.** A background aggregator would
  need a scheduler, a watermark and an answer for what a report shows while it is behind.
- **No `release-quota-cost.yml`.** The submodules are still consumed through `replace`
  directives; tagging `auth`, `quota` and now `quota/cost` is one carried follow-up, and the
  release workflow belongs with it rather than ahead of it.
- **Tool-adapter cost is recorded at the act step, not by wrapping `internal/platform/tools/*`.**
  One call site that already knows the twin, the objective and the capability beats N
  wrappers that each know only their own adapter.

**Acceptance — met:**

- The self-service round trip is pinned end to end
  (`TestQuotaApprovalIsConfinedToTheApproversTenant`): a viewer cannot decide, a
  team-scoped administrator cannot approve the other tenant's request, her own tenant's
  approval goes through, and `GET /quota/usage` reports the raised limit **immediately** —
  the resolver invalidates on approval, so the 60-second budget is spent with room left.
  A second decision on the same request is a 409.
- Cost totals are exact rather than within a tolerance: pricing is `units × per_unit` in
  `float64` and the contract suite asserts equality, so there is no drift to bound.
- Concurrency is covered by the shared ledger contract run under `-race` against both
  implementations, and the SQL writer serialises rollup upserts with `BEGIN IMMEDIATE`.
- The Phase 17 property, restated for money: two organisations, spend in each, and a
  report from one containing neither the other's rows **nor its totals**
  (`TestCostReportIsConfinedToTheCallersTenant`), plus the two edges — an uncontained twin
  hidden from a scoped reader, and a twin-scoped binding answered exactly.
- `quota` holds its 95% gate; `quota/cost` and `quota/cost/sql` ship with their own gates
  at 95 and 90.

---

## Phase 19 — Frontend for Auth, Quota, Cost, Audit (Completed)

**Goal:** Surface Phases 14–18 (and the Phase 13 audit log) in the React frontend so operators don't depend on the CLI for daily admin work.

**Steps:**

1. **Login page** (`/login`) — redirects to IdP via Phase 16; existing bearer flow stays when provider is `bearer`.
2. **Auth pages** — `/auth/users`, `/auth/roles`, `/auth/orgs` consume Phase 14 + Phase 17 endpoints with create/edit/delete forms and a role → permission matrix viewer.
3. **Quota pages** — `/quota/usage` (per-twin/cap/budget breakdowns), `/quota/requests` (submit + approve workflow from Phase 18), `/quota/settings` (admin config editor for the canonical tiers).
4. **Cost dashboard** — `/cost` time series + stacked bar by provider (Recharts, already a Phase 9 dep); `/cost/breakdown` drill-down by twin/team/provider. Live updates via SSE on `Cost.Recorded` + `Quota.Pressure` events.
5. **Audit pages** — `/audit` log viewer with the same filters as `krk audit` (objective, agent, kind, bounds-violation, since); `/audit/{id}` single-event detail with linked checkpoint + policy decision context.
6. **Permission-aware navigation** — sidebar reads the current user's roles from a new `GET /api/v1/auth/me` endpoint (Phase 14) and hides menu items the user can't reach.
7. **Tests** — Vitest unit tests for hooks + data transforms per page; Playwright end-to-end covers the login → quota request → admin approve → cost dashboard update flow.

**Acceptance:** Playwright e2e: login → org/team setup → quota request + approve → cost dashboard updates within 30s. Permission-aware sidebar passes a role-matrix test (a `viewer` sees only the audit log + their own quota usage; admin sees everything). Vitest unit suite covers ≥80% of new page hooks. SSE live updates verified by injecting a fake `Cost.Recorded` event and asserting the UI updates within 1s.

**What shipped — and two steps that were not frontend work at all.**
[ADR 012](adr/012-limits-as-resolved-state.md) records both.

- **Pages** — `/users` (bindings as "role @ scope"), `/roles` (the permission
  matrix), `/orgs` (the tree, with projects deliberately outside it), `/quota`
  with three tabs, `/cost` with Recharts and live updates, `/audit/{id}`.
- **Permission-aware navigation** — `useAuth().can()` had existed since Phase 16
  and was never called. The menu is filtered and the landing route is chosen from
  what the principal holds.
- **`quota.Base(fn)`** in the module, and **`quota_tiers`** in Karakuri: the
  limits move to the database with configuration as their seed.
- **`GET /api/v1/events`**, filtered per subscriber.
- **Tooling** — Recharts, Vitest, Playwright, a committed lockfile, `npm ci`
  with a cache, and a browser end-to-end job.

**The settings page had no backend.** Step 3 asks for "an admin config editor for
the canonical tiers"; `DefaultTiers` read the YAML once at boot and froze it.
Building the editor meant making tiers *resolved state* — a store, a 30-second
cache, `Invalidate` on write — and `quota.Limit` captured its policy by value, so
the module needed `Base(fn)` before the request tier could see an edit at all.

**The cost dashboard had no stream to follow.** The hub publishes to a `_global`
key no endpoint subscribed to. A dashboard watches everything, and "everything"
on a multi-tenant deployment is a question about who is watching — so the stream
is filtered per event and withholds whatever it cannot classify.

**Deviations from the plan, recorded:**

- **Steps 1 and 6 were already shipped.** `/login` landed with Phase 16, and
  `GET /auth/me` with it. What was missing from step 6 was the *use* of `can()`.
- **Recharts was not "already a Phase 9 dep".** `web/package.json` had three
  dependencies and no test tooling. Recharts 3 rather than the 2.x the plan
  assumed, since 2.x is deprecated upstream — and it is lazily loaded, because it
  is larger than the rest of the application combined (226 kB → 613 kB, split
  back to 227 kB plus a 386 kB chunk).
- **`/quota/settings` edits ceilings, not algorithms or calendar periods.**
  Typing a bigger number is not choosing to swap fixed windows for a token
  bucket.
- **No "≥80% of page hooks" coverage gate.** 23 unit tests cover the hooks and
  transforms that have logic worth pinning; a percentage gate on presentational
  components buys assertions about markup rather than behaviour.
- **The e2e suite runs against its own config** (`web/e2e/karakuri.e2e.yaml`)
  with a raised request limit. That is a finding rather than a convenience: the
  SPA is a bursty client and page loads reach the shipped default. The product
  response is in `web/src/api/client.ts`, which retries a 429 once after the
  interval the server named.

**Two defects the browser suite found**, neither reachable from the Go tests:
`TwinsPage` rendered `<label>` elements with no `htmlFor`, so its fields were
unlabelled to a screen reader and to anything else driving by accessible name;
and the SPA tripped its own rate limit on ordinary navigation.

**Acceptance — met, with the coverage gate restated:**

- The Playwright suite covers login → org and team setup → quota request →
  approve → the override in force → the cost dashboard, in Chromium against a
  real server with a real database.
- The permission matrix is asserted directly rather than through the sidebar: a
  viewer's menu contains neither Audit nor Cost nor Users, an administrator's
  contains everything, and somebody holding nothing still reaches Health.
- Live updates are wired to the filtered stream and pinned in Go
  (`TestGlobalStreamIsConfinedToTheCallersTenant`), which is a stronger statement
  than a fake event injected client-side: it proves the update arrives *and* that
  another tenant's does not.

---

## Phase 20 — Standing Objectives + Reconciliation (Completed)

**Goal:** An objective can declare a state to be *held* rather than a task to be finished, and Karakuri converges the world to it on its own — sensing cheaply, spending rarely, and escalating whatever exceeds the autonomy it has earned.

**What shipped — and the near-miss it replaces.**
[ADR 015](adr/015-standing-objectives-and-reconciliation.md) records the design.

Karakuri converged once. The loop ran to a met criteria score or its iteration
cap, `finalizeLoop` wrote `completed` or `failed`, and the objective was
terminal from that moment. "Keep this repository's build green" and "every
weekday morning, work through the calendar, the tickets and the inbox" are not
tasks with an end; they are desired states the world moves away from
continuously, and something has to notice and converge again at 3am with nobody
watching.

- **The declaration lives on the objective.** `Mode` (empty is oneshot, so
  every row written before this keeps its behaviour exactly), `Cadence` and
  `Autonomy`. `StatusConverged` is where a standing objective rests —
  deliberately not `completed`, which says the work is over.
- **The runtime lives in `reconcile_states`**, the split Phase 11 already drew
  between `Objective` and `loop.State`: what an operator edits, and what the
  system discovers by running.
- **`internal/feature/reconcile`** — one supervisor goroutine, one due-wheel
  query per tick over an indexed `next_due_at`, a bounded dispatch, and a
  lease. A thousand standing objectives cost one statement per tick rather than
  a thousand sleeping goroutines.
- **`internal/platform/schedule`** — cadence maths over `robfig/cron`, used as
  a parser only. `every`, `cron`, `daily_at`, IANA timezones, `resync`,
  `min_interval` and quiet windows.
- **`PUT/DELETE /objectives/{id}/standing`**, `GET`/`POST
  /objectives/{id}/reconcile`, `POST .../pause`, `POST .../resume`; three new
  actions; `krk objective standing|unstanding|reconcile|reconcile-status|pause|resume`;
  a Standing panel on the objective page.

**The two-tier split is the whole economics.** Every pass senses: `Snapshot` on
each environment, sorted, hashed, compared against the fingerprint taken when
the objective last converged. That is a handful of adapter calls and no model
call. A loop runs only when the fingerprint moved, a schedule came due, or the
resync horizon expired — so an objective can be checked every fifteen minutes
all year and spend money only on the days something happened.
`TestQuietWorldCostsNoLoopRun` pins it: six passes over a still world, zero
loops.

Three details in that hash are load-bearing. Environment IDs are **sorted**,
because build order is not stable across replicas and an unsorted hash would
report drift on deploys and not on commits. An environment that returns no SHA
is **blind, not still** — a calendar saying "I don't know" read as "unchanged"
would go quiet exactly when it should not, so blind environments are named in
the outcome and their objectives are driven by their schedule instead. And
drift is measured **against the last convergence**, not the previous
observation, so a change that reverts is not drift and one sitting unaddressed
since yesterday still is.

**Autonomy is earned under a ceiling, and it is not a second policy engine.**
Four rungs — `sense`, `propose`, `act_with_notice`, `act` — applied by writing
`agent.AuthorityBounds` into the loop request, the struct the decide step has
enforced since Phase 1. `Ceiling` is operator-declared and applied on every
read of the state, so no history and no hand-edited row can widen it.
Promotion takes a declared number of clean reconciles and moves one rung;
demotion takes one rejection and is immediate. The asymmetry is the point: a
reviewer saying no is a stronger signal than any number of runs nobody objected
to. Both movements write `tool_events` rows (`kind=promotion` / `demotion`),
because a change in what Karakuri may do to the world without asking belongs in
the same log as the approvals and the refusals.

**Guardrails, each with a test.** A DB lease (one conditional `UPDATE`, one
`RowsAffected` check) so two replicas cannot reconcile the same objective, pay
twice and mail the same report twice. A semaphore, because loops have been
unbounded detached goroutines since Phase 1 and a hundred objectives coming due
together is a hundred concurrent model-calling loops. A circuit breaker at
three consecutive failures and a stall detector at three reconciles that move
the score nowhere — both pause the objective **and raise a checkpoint**, since
an objective that went quiet with no explanation looks exactly like one that is
converged and content. Exponential backoff with a ceiling. Quiet windows and a
minimum interval, on the expensive tier only.

**Two things that were already broken, and had to be fixed to build this:**

- **A resolved checkpoint did not resume the loop that raised it.**
  `POST /checkpoints/{id}/resolve` wrote the row and the audit entry and left
  the goroutine blocked on `<-decisionCh` forever; `POST /loops/{id}/resume`
  unblocked it and left the checkpoint `pending` for good. Whichever door an
  operator came through, the other half of the state was wrong. Survivable when
  a human notices and uses the other endpoint — fatal for an objective that
  escalates on its own schedule and has to carry on afterwards.
- **The stall detector the Phase 1 risk table promised** ("if score doesn't
  improve for N consecutive iterations, emit checkpoint rather than burning
  tokens") was never built. The only brake was `MaxIter` and the token budget.
  It now lives on the outer loop, where "no progress across three full
  reconciles" is a stronger signal than three iterations of one run.

**Deviations from the plan, recorded:**

- **`watch.go` is deleted rather than kept alongside.** Watch mode polled
  snapshots after a finished loop and raised a checkpoint, from a ticker that
  died with the process — and it subscribed to `env.Subscribe()` channels it
  never read. `--watch` and `watch_mode: true` still work and mean the same
  thing, now as a standing objective at sense-only autonomy, which also
  survives a restart. Two drift detectors that could disagree is worse than one.
- **`stepObserve`'s composite SHA was not shared with the fingerprint,** which
  the plan called for. Observe hashes what the loop read in the order it read
  it, as a record of one iteration; the fingerprint hashes what the world
  claims to be, sorted, as a value to compare against later. Sharing them means
  giving one an ordering guarantee it does not need or taking one from the
  other. `SelectAgent` and `BuildEnvironments` *are* extracted and shared,
  which is where the real duplication was: a supervisor that built a different
  environment set than the loop observes would be watching one world and
  converging another.
- **The lease was not deferred to operators.** Phase 11 left "active-active
  multi-node coordination" to leader-election sidecars, which is defensible
  when duplicate work means one duplicate run somebody notices. For a standing
  objective it means a recurring duplicate bill and two copies of the same
  morning report, forever. This phase discharges that deferral for the outer
  loop; loop execution itself is still single-node-resume.
- **`internal/platform/executor` was left alone.** Durable *scheduling* is a
  different problem from durable *execution* — the supervisor needs to know
  which objectives are due and who holds them, which is a query, not a queue —
  and `Task.Fn` is a Go closure that cannot cross a process boundary anyway.

**One bug the tests found:** a loop that failed to *start* never gets a loop ID,
and the outcome switch tested "was there a loop" before it tested "did it
fail" — so a broken objective was filed as a quiet one and the circuit breaker
sat at zero forever.

**Acceptance — met:**

- Build clean (`go build ./...`); the full suite passes (`go test ./... -count=1`).
- **The economics are pinned, not asserted:** six sense passes over an unchanged
  world run zero loops and record six outcomes, so the cheap tier is visible in
  the history rather than invisible.
- **The lease is exercised against a real database**, both engines' conditional
  `UPDATE` being the load-bearing part: eight goroutines racing for one
  objective produce exactly one winner, an expired lease is taken over without
  the crashed holder releasing anything, and a late release from a displaced
  replica does not unlock work somebody else is now doing.
- **Schedule maths is pinned across a daylight-saving change** in a real zone:
  "08:00 America/New_York" stays 08:00 while the UTC gap between firings is 23
  hours in spring and 25 in autumn. Quiet windows defer to the opening rather
  than dropping work, chain when adjacent, and give up rather than loop forever
  when an operator blacks out the whole day.
- **The ceiling is pinned directly:** six clean runs at `--promote-after 1` stop
  at the declared ceiling, one rejection demotes at once, and both write audit
  rows.
- **A regression guard on the thing that must not change:** a oneshot objective
  still ends `completed` or `failed`.
- Three integration tests over a real server and database cover the lifecycle,
  every rejected declaration, and the permission split. Ten Vitest cases cover
  the console panel's transforms.

**Operator quickstart:**

```bash
# Watch a repository and propose fixes, never acting on its own.
krk objective standing obj_123 --sense 15m --autonomy propose

# A weekday morning review that may act, having earned its way up to it.
krk objective standing obj_123 \
    --cron "0 8 * * 1-5" --timezone Europe/Istanbul \
    --sense 1h --resync 24h \
    --autonomy propose --ceiling act_with_notice --promote-after 5 \
    --quiet 22:00-07:00

# What has it been doing? The cheap passes are in here too, and that is the point.
krk objective reconcile-status obj_123

# Now, rather than on the cadence. Then stop it, then start it again.
krk objective reconcile obj_123
krk objective pause obj_123 --reason "investigating a noisy adapter"
krk objective resume obj_123
```

**What's deferred:**

- **Digests.** Both motivating examples end in "and tell me what happened".
  The material is already recorded — reconcile outcomes, `tool_events`,
  pending checkpoints, `cost_daily` — but composing and delivering it is Phase 21.
- **Push triggers.** `Environment.Subscribe` exists and is implemented, and
  nothing reads it. A signal that is lossy in-process and absent across
  replicas can only ever be an optimisation on top of a poll that has to exist
  anyway.
- **Per-objective spend ceilings.** The quota module supports arbitrary
  subjects, so `objective:<id>` is a small addition; today a standing
  objective spends against its twin's daily allowance like everything else.

---

## Phase 21 — Digests (Completed)

**Goal:** A standing objective that works unsupervised has to report unsupervised. A twin sends one message on a cadence saying what its objectives did and what they need a person to decide.

**What shipped — and why the model writes so little of it.**
[ADR 016](adr/016-earned-autonomy-and-digests.md) records the design.

Both of the things Phase 20 was built for end the same way: "and tell me what
happened". Neither is served by a console somebody has to remember to open.

- **`internal/core/digest`** — the shape of a report and of a schedule.
- **`internal/feature/report`** — assembly, rendering, delivery, and a sender
  on its own ticker with its own lease.
- **`report_schedules`**, keyed on a twin. A CTO twin holding nine standing
  objectives produces one message a day, not nine.
- **`POST/GET/DELETE /reports`**, `GET /reports/preview`,
  `POST /reports/{id}/send`; `report:read` and `report:write`;
  `krk report create|list|preview|send|delete`.

**A digest is a read.** Reconcile outcomes, `tool_events`, pending checkpoints
and the cost ledger already record everything it says, so nothing is
accumulated between deliveries and no write is added to any hot path. The
consequence that matters is reproducibility: the same window produces the same
report tomorrow, so a failed delivery is simply retried, and `krk report
preview` shows exactly what tonight's will contain rather than an approximation.

**It ends with the decisions, oldest first.** Every pending checkpoint with the
actions the agent proposed and the command that answers it — ordered against
the usual newest-first, because the one that has been waiting three days is the
one blocking work and burying it under this morning's is how a queue grows.
Ages render as "waiting 3 days", not "76h12m4.331s".

**It reports the cheap passes.** "90 checks, 2 reconciles, 4 actions" — the
ratio is the answer to "is this costing me anything", and a summary showing
only the reconciles would make a well-behaved objective look idle. An unpriced
deployment reads "not priced" rather than rendering $0.00, which would be
claiming something untrue.

**The model writes the prose and nothing else.** It never decides what is in
the report, what counts as a decision, or what is urgent: the structured digest
is complete before the model is called, the plain rendering of it is what gets
delivered when none is available, and the prose is prepended above that
rendering rather than replacing it. This is not defensive scaffolding around an
unreliable component — a summary that could silently omit a pending decision
because a model judged it unimportant would defeat the entire exercise.

**Silence is the default.** A window in which nothing happened is not sent;
`--send-when-empty` opts in. A daily mail that says "nothing happened" is a
mail people stop reading, and the cost is paid three weeks later by the report
that matters. The suppression is narrow — any decision, autonomy change, spend,
failure, drift or action makes a window worth reporting, and only "checked
ninety times, nothing moved" is silence. `last_sent_at` advances anyway, so a
quiet fortnight does not produce a fortnight-long report the moment something
happens.

**Delivery is audited.** Through the twin's bound adapter (ADR 006), and every
attempt writes a `tool_events` row whether it succeeded or not: a message
Karakuri sent on somebody's behalf is a thing it did to the world, and a
delivery invisible to `krk audit` would be the one kind of action nobody could
review. Schedules carry the same lease `reconcile_states` does, and it matters
more here — two replicas reconciling one objective wastes money and shows up in
a graph; two replicas sending one morning report send it to a person twice,
every morning.

**Deviations from the plan, recorded:**

- **The autonomy ladder shipped in Phase 20, not here.** The plan had it as
  Phase 21's first slice. It could not wait: the supervisor needs a level to
  write into `agent.AuthorityBounds` on every reconcile, and stubbing one in
  order to replace it a phase later would have meant building the enforcement
  path twice. What remained for this phase was surfacing the movements, which
  the digest does.
- **Delivery is not a capability executed by the act step.** The plan had it
  routed through the loop to inherit quota, cost recording and the audit row.
  Attractive, and rejected: a scheduled digest is not something an agent
  decided to do, and routing it through the planner would put a model in charge
  of whether this morning's report goes out. The audit row — the part that
  actually mattered — is written directly.
- **`projectmgmt` and `versioncontrol` are declared channels that refuse.**
  Messaging and email deliver; the other two return an error recorded on the
  schedule rather than a silent success, so an operator who configured one sees
  why nothing arrived.

**One bug fixed on the way:** `SaveCheckpoint` discarded the caller's
`CreatedAt` and let GORM stamp its own. Benign in production, a lie in the
type, and it made "how long has this been waiting" — the one number a decision
list needs — unanswerable for any row the storage layer did not create itself.

**Acceptance — met:**

- Build clean; `go test ./... -count=1` passes.
- The cheap/expensive split is pinned in the digest as it is in the supervisor:
  ninety sense passes and one reconcile are counted and reported separately,
  and an outcome outside the window is excluded.
- Suppression is pinned in both directions — ninety quiet checks are not news,
  one pending decision is.
- Decision ordering, decision age, and the "proposed actions" passthrough are
  pinned, as is the ordering of objectives by how much attention they want.
- The plain rendering is asserted directly: decisions lead the report, a
  demotion reads as a narrowing, a paused objective says so, and an unpriced
  window says "not priced" rather than zero.
- Declaration refuses a malformed cadence, an unknown channel, two schedules
  and a bad window, and arms a valid one for its next firing rather than for
  now.

**Operator quickstart:**

```bash
# A weekday morning brief to Slack.
krk report create --twin twin_1 --daily-at 08:00 --timezone Europe/Istanbul \
    --channel messaging --target '#eng-standup'

# See exactly what tomorrow's will say, without sending it.
krk report preview --twin twin_1 --window 24h

# A weekly mail covering the whole week.
krk report create --twin twin_1 --cron "0 17 * * 5" \
    --channel email --target lead@example.com --window 168h

krk report list --twin twin_1
krk report send rep_ab12cd34   # now, outside the cadence
```

Enable the sender in `reports:` — it is off by default, because a supervisor
with no standing objectives does nothing while a sender that runs will mail
somebody.

**What's deferred:**

- **Per-objective spend ceilings.** The quota module supports arbitrary
  subjects, so `objective:<id>` is a small addition; a standing objective still
  spends against its twin's daily allowance.
- **Digest delivery to project trackers and repositories.** The channels are
  declared and refuse honestly; wiring them is adapter work, not design work.
- **A console page for schedules.** They are reachable from `krk` and the API;
  the objective page already shows the control loop each digest reports on.

---

## Phase 22 — The Karakuri Domain Pack (Superseded by ADR 018)

> The capability shipped; the shape did not survive review. Self-improvement
> now lives in the software pack — see
> [ADR 018](adr/018-self-improvement-belongs-to-the-software-pack.md). The
> separate pack was justified by a boundary that does not enforce anything,
> two of its capabilities were generic software practices, and its repository
> environment duplicated `software.env.git`. What was genuinely
> platform-specific — the telemetry environment — remains, gated on whether a
> reader is wired rather than on a config flag. The section below is kept for
> the reasoning it records, not as a description of the current tree.

**Goal:** Karakuri can read its own usage, find what limits it, and open a pull request that fixes it — under its own repository rules, and never from the pack that decided what to fix.

**What shipped — and the thing it deliberately cannot do.**
[ADR 017](adr/017-karakuri-as-a-domain-pack.md) records the design.

Everything needed to *act* on self-improvement already existed: the software
pack writes code in a worktree and opens pull requests, the research adapter
reads the field, and Phase 20's supervisor schedules and bounds the work. What
was missing was the observation — nothing let a domain pack see what this
deployment had been doing.

- **`internal/core/telemetry`** — a read-only port answering one question: what
  has this deployment been doing over a window. Carried on
  `environment.BuildContext`, nil for every pack but this one. Wired on the
  environment registry rather than threaded through the loop and reconcile
  services, since there is one reader per process and both already hold the
  registry.
- **`internal/platform/telemetry`** — implements it over objectives, reconcile
  outcomes, the audit log and the cost ledger. Accumulates nothing, like the
  digest, so a snapshot for a past window is reproducible. It returns
  bottlenecks already ranked rather than four counters to compare: a pack
  asking "what should I improve" would otherwise derive that ranking slightly
  differently every time a model ran.
- **`domains/karakuri`** — `karakuri.env.telemetry` and `karakuri.env.repo`;
  capabilities `analyse_usage`, `propose_roadmap_phase`, `draft_adr`; agents
  `karakuri-maintainer` (Reflexion) and `karakuri-analyst` (read-only);
  templates `watch_health` and `self_improve`.

**Read-only is the property being bought, not a precaution.** A pack that could
write to the telemetry port could rewrite the evidence of what it did, and the
value of letting Karakuri watch itself depends entirely on the watching being
trustworthy. Both environments refuse an `Act` out loud rather than succeeding
quietly — the same reasoning that made an unmatched `EnvID` an honest failure
in Phase 13.5.

**The pack that decides what to change cannot change anything.** Its three
capabilities analyse and draft; the writing is the software pack's, in a
worktree, through a pull request an operator reviews, reached by a cross-domain
objective. One pack that could both conclude "Karakuri should be allowed to do
more" and carry that out is one bug away from a system that widens its own
bounds, and no amount of prompt discipline substitutes for not having the
capability. It is asserted by a test rather than left to a comment, and
`karakuri-maintainer` carries `MaxAutonomousActions: 0` with a confidence
threshold no plan can clear — so it always asks, however much autonomy its
standing objective has earned. The `self_improve` template is verified in part
by `software.act.open_pull_request`: the pack cannot mark its own homework on
the part it does not do.

**The telemetry fingerprint is deliberately lossy.** The supervisor senses
drift by hashing snapshots, and an environment hashing raw counters would move
every time anything happened anywhere — a standing self-improvement objective
would reconcile continuously to discover that work had occurred. Counts are
bucketed by order of magnitude, the approval rate is banded, and only the
bottleneck set is hashed exactly. This is the first environment where the right
fingerprint is lossy, and it generalises: a SHA should answer "has anything
changed that would change what I do", not "has anything changed".

**One thing the conformance suite had wrong.** It required every criterion
verifier to be a capability in the same pack — untrue since Phase 13 added
cross-domain objectives, and unexercised until this pack. A foreign verifier is
now allowed when the criterion declares `Criterion.Domain`, and still rejected
when it does not, so a typo in a local verifier keeps failing rather than
becoming indistinguishable from a deliberate cross-pack reference. The named
domain is not resolved: a pack is validated on its own, and whether another is
enabled is the registry's business at boot.

**Acceptance — met:**

- Build clean; `go test ./... -count=1` passes; the pack passes the same
  conformance suite as software and agriculture.
- The safety properties are pinned by tests rather than by comments: the pack
  owns no capability whose ID looks like a write, the maintainer cannot act
  unsupervised or delegate or edit its objective, both environments refuse an
  action and say so, and `self_improve` is verified in part by another pack.
- The fingerprint is pinned in both directions: a week that trebles its senses,
  reconciles and approvals does not move it; a new bottleneck, a decision queue
  growing from 2 to 40, and an objective entering the breaker each do. The
  bottleneck set hashes order-independently.
- An unwired deployment reports `available: false` and no SHA, so the
  supervisor reads it as blind and drives the objective from its schedule —
  rather than seeing a still fingerprint and concluding nothing changed.

**Operator quickstart:**

```bash
# Enable the pack (config/default.yaml ships it disabled).
#   domains:
#     - id: karakuri
#       enabled: true
#
# Bind the twin to the repository, then watch the deployment without spending
# anything: sense-only autonomy never runs a loop.
krk objective create --title "Watch Karakuri" --domain karakuri --twin twin_1 \
    --template karakuri.objective.watch_health
krk objective standing <id> --sense 1h --autonomy sense --ceiling sense

# Self-improvement is cross-domain: this pack analyses and drafts, the software
# pack writes. Propose-only, so every change arrives as a checkpoint.
krk objective create --title "Improve Karakuri" --domain karakuri --twin twin_1 \
    --template karakuri.objective.self_improve
krk objective standing <id> --cron "0 9 * * 1" --sense 6h --autonomy propose
```

**What's deferred:**

- **A research environment in this pack.** The research adapter and
  `POST /research` already exist and an objective can reach them; a pack-owned
  environment that ranked the field the way the telemetry one ranks
  bottlenecks is a real addition and not this phase's.
- **Reading CI status per pull request.** `karakuri.env.repo` reports open pull
  requests; the version-control adapter has no check-status call, and adding
  one is adapter work.
- **Per-objective spend ceilings**, carried over from Phase 21. A
  self-improvement objective is exactly the one an operator would want to cap
  separately from its twin.

---

## Phase 23 — Per-Objective Spend Ceilings (Partial)

**Goal:** An operator can cap what one objective may spend, separately from its
twin's allowance, so a standing objective reconciling hourly cannot quietly
consume a team's daily budget.

Deferred three times — from Phases 20, 21 and 22 — which is the signal that it
should stop being deferred. Phase 22 named the specific case: a self-improvement
objective is exactly the one an operator wants capped separately, because it is
the one whose appetite nobody has calibrated yet.

**Steps:**

1. **A new subject, not a new mechanism.** Phase 15's quota module already
   supports arbitrary `quota.Key` subjects, and Phase 18 already has the
   override path. `internal/quota.CostSubject` gains an `objective:<id>`
   sibling; nothing in the quota module changes.
2. **`objective.Budget` on the declaration**, beside `Cadence` and `Autonomy`
   and nil-safe like both: `{Daily, PerReconcile}`. Nil means "no ceiling of
   its own", which is today's behaviour, so every existing objective is
   untouched.
3. **The supervisor checks the ceiling before dispatching**, and records
   `budget_exhausted` as a distinct pause reason. It is not a failure and must
   not touch the circuit breaker — an objective that has run out of money is
   not an objective that is broken, and conflating them would demote an
   agent for its operator's budgeting.
4. **Sensing continues while the budget is exhausted.** The cheap tier costs
   adapter calls and no tokens, so an objective that cannot afford to act can
   still afford to notice — and the digest can say what it would have done.
   This is the sense/reconcile split earning its keep a second time.
5. **The two pauses clear differently.** `krk objective resume` clears the
   breaker; a budget clears itself at the window boundary with no operator
   involved. A single `paused` flag that needed a human to clear either one
   would turn a nightly budget into a nightly chore.
6. **Surfaces:** `krk objective standing --budget-daily`, the console panel,
   and a digest section naming which objectives hit their ceiling and what
   they were mid-way through.

**Acceptance:** An objective with a daily ceiling stops reconciling when it is
reached and resumes at the boundary without an operator. Its twin's allowance
is unaffected by the pause. Sensing continues throughout and the drift it
observes is reported. `ConsecutiveFailures` is still zero after a budget pause,
and the objective's earned autonomy survives it.

**Shipped (steps 1–5).** `objective.Budget{Daily, PerReconcile}` on the
declaration, nil-safe like `Cadence` and `Autonomy`; the ceiling checked at the
expensive gate beside the quiet-window deferral it mirrors; `budget_exhausted`
recorded as `Outcome.Deferred` rather than as an error, so the breaker is
untouched and earned autonomy survives; the next due time floored at the window
boundary so the cadence cannot schedule over it; sensing unaffected throughout.

One thing had to be fixed first: **token spend was not attributed to the
objective at all.** `stepAct` attributed adapter calls, but the budgeted agent —
which charges the expensive half — recorded with no resource, so it landed under
the twin. A per-objective ceiling had nothing to measure until that was closed.

**Still open:** `PerReconcile` is declared and read but not yet enforced (the
daily ceiling is), and the surfaces from step 6 — the `krk` flag, the console
panel and the digest section naming which objectives hit their ceiling — are
not built. The mechanism works and is untestable from the outside until they
are.

---

## Phase 24 — Conformance That Tests Behaviour, Not Shape (Completed)

**Goal:** A pack's declared bounds are verified by *running* them, so a bound
that does nothing fails the suite in the pack that declares it.

This phase exists because of a specific bug. Phase 20's review found that
`MaxAutonomousActions: 0` — written by four packs to mean "plans but never
acts", with healthcare saying so in a comment on the line — was read by the
decide step as "no cap at all". None of those agents were bounded. It survived
three phases and a conformance suite because **every test asserted the field
*was* zero and none asserted what zero *did***. The karakuri pack's own
"cannot act unsupervised" test passed throughout while the guarantee it names
was absent.

The lesson generalises past the one bug: a declaration is a claim, and a suite
that reads claims back to itself verifies nothing.

**Steps:**

1. **A behavioural section in `internal/conformance`.** For each agent
   definition a pack exports, build a plan with N actions and run the real
   `stepDecide` against the declared bounds, asserting the outcome matches
   what the declaration claims about itself.
2. **Assert the whole ladder**, so the fix cannot be "escalate everything": a
   definition declaring no autonomous actions must escalate; one declaring a
   cap must trim to exactly that cap and proceed; one declaring
   `agent.UnlimitedActions` must not be trimmed at all.
3. **`RequiresApprovalFor` gets the same treatment** — a capability listed
   there must actually escalate when planned, rather than being a list nobody
   reads.
4. **Run it per pack in CI**, so a pack added later cannot declare a bound
   that silently does nothing.
5. ~~**`Template.SuggestedAgents` is declared and read by nothing.**~~
   **Shipped early, in Phase 26.** It was the same shape as the bounds this
   phase exists for — a field a pack fills in that changes no behaviour — and
   it had to be fixed there because a write path is worth nothing under the
   wrong agent: `self_improve` ran under the software pack's *strategist*, and
   `TestMaintainerHoldsNoMutatingCapability` guarded an agent that never ran.
   `Objective.AgentID` carries it now (migration `000010`) and `SelectAgent`
   honours it. See [ADR 019](adr/019-capabilities-declare-what-they-need.md).

   Phase 26 also closed the same defect at the routing level and found a fifth
   instance while doing it — `software.act.write_design_doc`, declared since
   Phase 2 and served by no environment. **Both were found by reading code**,
   which is exactly the argument for this phase: five instances over four
   phases, none caught by a suite that read declarations back to itself.
6. **A registry-level verifier check at boot.** The per-pack check
   deliberately does not resolve foreign domains (a pack is valid on its own,
   ADR 017), which leaves nothing at all checking that a declared
   `Criterion.Domain` names a capability some enabled pack actually exports.
   The registry is where that question has an answer; a dangling verifier
   should be logged loudly at startup rather than discovered when an objective
   fails to verify.

**Acceptance:** The suite fails against a deliberately reintroduced `> 0`
guard in `decide.go` — the regression that motivated the phase is caught by
the mechanism built to catch it, in every pack rather than in one hand-written
test. Every existing pack passes unmodified. A pack declaring an approval
requirement for a capability it then plans autonomously fails with a message
naming both.

**Shipped.** See
[ADR 020](adr/020-a-declaration-is-verified-by-running-it.md).

The enabling move was extracting the decision policy out of `stepDecide` and
onto `agent.AuthorityBounds.Decide`. A conformance check cannot run
`stepDecide` — it would need a store, an event hub, a checkpoint service and a
loop state to answer a question involving none of them — and it can run
`Decide` with nothing at all. `stepDecide` now carries out the verdict and
holds no policy of its own, so there is one implementation rather than a
mechanism and a check that could drift.

`checkAgentBoundsBehave` runs it per agent definition and asserts the whole
ladder, deliberately: a check that only tested the zero case could be satisfied
by escalating everything, which is a different bug with the same green suite.
A cap of zero must escalate a three-action plan *and leave it intact* — an
approval falls straight through to `act`, so trimming would discard what was
approved. A cap of N must trim N+2 to exactly N and leave N alone.
`UnlimitedActions` must not trim fifty. Every `RequiresApprovalFor` entry must
escalate when planned and must name a capability the pack declares. A declared
threshold must escalate below itself and not above.

**The acceptance criterion was checked by doing it.** Reintroducing the `> 0`
guard fails conformance in three packs — software, agriculture and healthcare —
each naming its own agent. Every shipped pack passes unmodified.

**Step 6 shipped as `CheckDanglingVerifiers`**, run at boot beside the
cross-pack collision audit. It distinguishes *the pack that owns it is
switched off* from *nothing anywhere exports it*, because those have different
owners and different fixes, and it warns rather than refusing to start: an
operator may be mid-rollout, and a deployment that will not boot over a
template naming a not-yet-enabled pack is worse than one that says so loudly.

**Not covered, and worth naming.** No check asserts that a served capability's
environment actually *accepts* it. The environments answer an unknown
capability with the same "no active adapter" result they give a known one whose
adapter is unbound, so from outside the two are indistinguishable. Making them
distinguishable is what this argument asks for next; until then the executing
test in `domains/software` covers the capabilities that can run without an
adapter, and nothing covers the rest.

---

## Phase 25 — Self-Improvement Without a History (Completed)

**Goal:** The karakuri pack can propose useful work on a deployment that has
not been running for months — because that is every deployment on the day
somebody enables it.

Phase 22 shipped `self_improve` with a hard `evidence-first` constraint and a
first criterion reading "the proposal names the telemetry that says the problem
is real". On a fresh deployment the telemetry snapshot is empty: no objectives,
no reconcile outcomes, no audit rows, no spend. The pack has nothing to say and
the constraint cannot be satisfied. **The deployment that most needs a roadmap
is the one that cannot produce one**, and the feature is unusable for exactly as
long as it takes to accumulate the history it needs — which nobody will wait
through.

**Steps:**

1. **`karakuri.env.repo` grows from "open pull requests" into the repository as
   evidence**: the roadmap's own deferred lists, `AGENTS.md` rules, TODO and
   FIXME density by package, and test coverage per package. All of it exists on
   day one, and the deferred lists in particular are a backlog somebody already
   justified in prose.
2. **A `karakuri.analyse_repo` capability beside `analyse_usage`**, so
   `evidence-first` can be satisfied by either source — and so the proposal has
   to say which one it used. A phase proposed from repository evidence and a
   phase proposed from observed pain are different claims and should not be
   presented identically.
3. **Reading CI status per pull request** (deferred from Phase 22). The
   version-control adapter gains a check-status call, so "what is currently
   broken" becomes evidence rather than something an operator relays.
4. **A research environment in the pack** (deferred from Phase 22), ranking the
   field the way the telemetry environment ranks bottlenecks — pre-ranked for
   the same reason, so a model does not re-derive the ordering slightly
   differently on every run.
5. **Make the insufficiency judgement worth trusting.** The environment already
   reports `sufficient` alongside the window it examined, on both the observe
   and act paths — "I have no evidence" and "the evidence says nothing is
   wrong" are opposite claims, and the flag exists so a system reasoning about
   its own improvement cannot confuse them. What it does not yet do is say
   *how much* evidence: the test is currently "the window contains anything at
   all", so one sense pass in a week counts the same as a thousand. A
   proposal's confidence should scale with the evidence behind it, and that
   needs a threshold somebody has justified rather than a boolean somebody
   defaulted.
6. **Score the criteria against what actually happened.** `evaluateWithAgent`
   takes the action results and never reads them: the task string is built
   from the criterion's description alone, with `WorldState` and `Memory` both
   nil. So `self_improve`'s "the proposal names the telemetry that says the
   problem is real" is judged by a model that has been shown neither the
   telemetry nor the proposal. This is not specific to this pack — every
   verified criterion in every domain is scored this way — but it is the
   criterion this phase depends on, and evidence-first means nothing while the
   thing checking it sees no evidence.

**Acceptance:** A self-improvement objective on a deployment with zero usage
history produces a proposal citing repository evidence and stating plainly that
it had no usage telemetry to draw on. The same objective on a deployment with
history prefers telemetry and says so. Neither can satisfy `evidence-first` with
no evidence of either kind — the constraint still bites, it simply has two ways
to be met. The maintainer still escalates every proposal, unchanged: this phase
widens what it can see, never what it may do.

**Shipped (all six steps).**

*Steps 1–2, the goal.* `software.reason.analyse_repo` reads the repository: the
roadmap's own deferred and still-open items, TODO and FIXME density by package,
packages with Go source and no test file, and where `AGENTS.md` rules live. The
deferred lists are the valuable part and the cheapest to read — each line is
work somebody already justified in prose. Against this repository the scan finds
Phase 23's unenforced `PerReconcile` and Phase 24's uncovered conformance check,
which is the test that matters: a scan finding nothing in a roadmap full of
deferred work is one nobody should propose from. `evidence-first` now names both
sources and requires the proposal to say which it stood on, because a phase
proposed from repository evidence and one proposed from observed pain are
different claims.

It is deliberately **not** called coverage: untested packages are counted by
file, not by line, and reporting a file ratio under a name meaning "proportion
of lines executed" would be the same dishonesty this pack keeps finding.

*Step 3.* `PRSummary` carries `CheckState` and `FailingChecks`; `gitEnv` pulls
the red ones into `failing_prs`. A skipped check is not a failing one, and a red
check outranks a pending one — a run still going cannot un-fail what already
failed.

*Step 4.* The research environment needed no new capability:
`software.reason.research` has been declared since Phase 2 and the
`ResearchAdapter` built since Phase 6, and nothing had introduced them. Findings
come back ranked by confidence then title, and research contributes no drift SHA
— a standing objective must not reconcile because a search engine's results
moved.

*Step 5.* Evidence is graded `none` / `thin` / `adequate` rather than a boolean
whose test was "the window contains anything at all", under which one sense pass
in a week counted the same as a thousand. The threshold is not invented here: it
is the one the telemetry reader already applies before it will call a capability
failing, because fewer is noise. One justified number, used twice.

*Step 6, and the one that mattered most.* `evaluateWithAgent` now receives what
the actions produced. Two further defects surfaced in the same function:

- **A negated verdict counted as a positive one.** The parser searched the whole
  reply for "pass", "met", "approved" or "yes" and returned true on any hit, so
  *"this does not pass"* and *"the criterion is not met"* both scored as
  satisfied — a model's rejection turned into a completed objective.
- **An unrelated success satisfied a criterion.** A criterion whose verifier ID
  merely *contained* `run_tests` or `lint` was met if any action succeeded, so a
  `send_message` could satisfy a criterion about the test suite.

Fixing them needed the pairing that was missing: `stepAct` returns
`actionOutcome{CapabilityID, EnvID, Result}` instead of bare results. verify had
no way to tell which action produced which result, and learn paired them by
slice index.

**Found while doing it, and worth its own line.** Four criteria named
`analyse_usage` as their verifier — a capability that *produces* evidence rather
than deciding anything, and one that succeeds even with no reader wired. That
was harmless while every criterion was judged by a model shown nothing; once a
verifier settles a criterion deterministically it means "met whenever the
analysis ran", which is every time, on any deployment, including one with no
telemetry at all. "The proposal names the telemetry that says the problem is
real" would have scored 0.3 for running an analysis that found nothing. The
templates now spell the distinction with two constructors, `crit` and `judged`.

**Still open.** There is no declarative marker separating an adapter-backed
`reason.*` capability from a model-only one, so the "every capability is served"
check in `domains/software` covers `software.act.*` only. Both capabilities
found unserved in this phase — `analyse_repo`'s home and `research` — were
`reason.*`, which is exactly where the check does not look. That is the ninth
instance of the class and the next one to close.

---

## Phase 26 — The Write Path (Completed)

**Goal:** Karakuri can produce a change, not only a proposal about one.

ADR 017 divides self-improvement in two: the karakuri pack analyses and
drafts, and the software pack does the writing, in a worktree, through a pull
request an operator reviews. The first half works. **The second half does not
exist**, and the two capabilities that would provide it each have exactly the
half the other is missing:

- `software.act.write_code` and `software.act.write_test` are declared, and
  `stepAct` provisions a git worktree for them by name suffix
  (`internal/feature/loop/act.go`). No environment implements either, so both
  route to `noopEnv` and return `"unimplemented"` — after the worktree has
  been created.
- `software.act.delegate_to_cli` *is* implemented: `cliEnv` hands the task to
  a coding-agent CLI in the active worktree. But it does not match the suffix
  check, so no worktree is ever provisioned for it, and the adapter is called
  with an empty `worktree_path` — into which a planner hint explicitly
  forbids writing ("never write to the checked-out working tree directly").

So the capability with a workspace cannot write, and the capability that can
write has no workspace. Every roadmap phase from here on is written by a
human until this closes.

**Steps:**

1. **Provision the worktree by what a capability *does*, not by what it is
   called.** The suffix test is a string match on two names; a capability
   declaring that it needs a workspace should get one. That belongs on
   `capability.Capability` as a field the pack declares, checked the way the
   authority bounds are checked in Phase 24 — by running it, not by reading
   it.
2. **Implement `write_code` and `write_test`, or remove them.** A declared
   capability that always fails is worse than an absent one: it is planned by
   models, costs a worktree, and reports `unimplemented` after the fact. If
   `delegate_to_cli` is the real mechanism then these two should be aliases
   for it rather than stubs beside it.
3. **Give `create_pr` a worktree it can push.** The version-control adapter
   takes `worktree_path` today and the loop never supplies one outside the
   two stub capabilities, so the pull-request half is unreachable by the same
   gap.
4. **An end-to-end acceptance objective**: a cross-domain objective that
   analyses telemetry, drafts a phase, writes it in a worktree and opens a
   pull request, exercised in CI against a scratch repository with a stub
   version-control adapter.

**Shipped (steps 1–3).** See
[ADR 019](adr/019-capabilities-declare-what-they-need.md).
`Capability.NeedsWorkspace` replaces the name-suffix test, so a worktree goes
to the capabilities that declare they write — including `delegate_to_cli`,
which could write and never got one, and `create_pr`, which needs a worktree
path the loop never supplied. `write_code` and `write_test` are implemented by
delegating to the same coding-agent CLI rather than left as declared stubs
that routed to `noopEnv`; both refuse an empty task or a missing worktree
rather than guessing.

Fixed alongside, because the write path is worth nothing under the wrong
agent: `Template.SuggestedAgents` was read by nothing, so `self_improve` ran
under the strategist. Objectives now carry `AgentID`, template instantiation
fills it, and `SelectAgent` honours it.

**Shipped (step 4).** `environment.Factory.Serves` declares which capabilities
an environment executes; the registry indexes it and `stepAct` resolves through
the index, so the pack decides the route and the planner is no longer asked a
question it cannot answer. `EnvID` remains the fallback for the cases the
registry has no answer to — nothing claims the capability, or two environments
both do — and ambiguity is never resolved by picking, because map iteration
order deciding which environment writes files is the misrouting this replaced.
A route to an environment this loop did not build now fails instead of falling
through to "only one environment, so use it".

The planner hint that stated the routing pairing is no longer the thing holding
it together, and says so: what remains of it is the part a model does have to
get right — which parameters to fill in, and where not to write.

The end-to-end acceptance test drives `write_design_doc` → `write_test` →
`write_code` → `create_pr` against a scratch git repository with stub CLI and
version-control adapters, asserting the CLI ran *in a real worktree* and the
pull request was handed a path and a branch. Disabling the routing turns it red
with "the CLI was called 0 times". `tools.SlotInstances` gained `Set` to make it
possible at all: the slot could previously only be filled by a type-string
switch over the shipped adapters, which is why this chain had no test until now.

**Found while wiring it.** `software.act.write_design_doc` has been declared
since Phase 2, sits on the strategist's and the architect's capability lists,
and is required by a priority-9 planner hint before any `write_code` action —
and no environment has ever served it. Every plan that obeyed the hint failed
the step the hint made mandatory. It is a draft like `propose_roadmap_phase`
and `draft_adr` and is now recorded the same way, with `params.design`
documented in its schema and empty input refused.

That is the fifth instance of the defect class Phase 24 exists for, and the
last one findable by inspection: three new tests now assert the property
generally — every `software.act.*` capability is served, every agent's act
capabilities are runnable, and nothing is served that is not declared.

**Acceptance:** `software.objective.self_improve` can reach all three of its
criteria rather than two. The pull-request criterion — 0.4 of the score, and
the entire point of the cross-domain shape — becomes satisfiable for the first
time. The maintainer's bounds are unchanged: it still escalates every plan,
and the change still arrives as a pull request somebody reviews.

**What "satisfiable" is and is not.** The chain is now reachable, proved by
running it: every step routes, the CLI receives a real worktree, and the pull
request is handed a path and a branch. Whether a given run *scores* the
criterion still depends on the deployment — a bound coding-agent CLI, a bound
version-control adapter, and a plan that uses them. The scoring half — where
`evaluateWithAgent` read nothing the actions produced — was Phase 25 step 6 and
has since shipped, so the criterion is now judged on evidence rather than on the
plausibility of its own wording.

---

## Phase Ordering Rationale

Phases 7–13 are **independent except where noted** and can be reordered to match priority. The dependencies that DO exist:

- **Phase 11** (distributed execution) benefits from **Phase 8**'s Postgres backend for shared state (now available) but Restate has its own state store and works without it.
- **Phase 13** (cross-domain) is now unblocked — Phase 10 shipped Healthcare as a second non-software production pack to combine with Software.
- **Phase 22** (the karakuri pack) depends on **Phase 13**'s cross-domain objectives — it analyses and drafts, and the software pack does the writing — and on **Phase 20** for the outcomes its telemetry reads. It is the first pack to exercise `Criterion.Domain`, which is how the conformance suite's assumption that every verifier is local came to light.
- **Phase 21** (digests) depends on **Phase 20** for the outcomes it reports and on **Phase 6**'s adapters for delivery. It reads only: nothing is accumulated between deliveries, so a digest can be regenerated for any past window.
- **Phase 20** (standing objectives) depends on **Phase 11**'s durable loop state, which is what the supervisor watches to know a loop has finished, and on **Phase 13.5**'s actionable checkpoints, which are what an escalating reconcile hands a human. It also had to fix the disconnect between resolving a checkpoint and resuming a loop before either could be relied on. Its lease discharges Phase 11's deferred "active-active multi-node coordination" for the outer loop only; loop execution itself is still single-node-resume.
- **Phase 9** (frontend) can run in parallel with any other phase; the API contract is already stable.
- **Phase 12** is a pure adapter implementation — can ship independently (Phases 6 and 7 already followed this pattern).

Phases 14–19 introduce a new architectural pattern: **the auth and quota engines ship as standalone Go modules** in this same monorepo (`auth/`, `auth/sql/`, `auth/oidc/`, `auth/saml/`, `quota/`, `quota/redis/`, `quota/sql/`, `quota/cost/`), each with its own `go.mod` and independent semver tag namespace (`auth/v0.1.0`, `quota/v0.1.0`, etc.). External Go repos consume them without pulling in Karakuri. Karakuri itself is the first reference consumer, wired in via thin integration shims under `internal/auth/` and `internal/quota/`.

- **Phases 14 and 15** are independent of each other — RBAC and quota can ship in either order. Both are also independent of the existing tree because the standalone modules touch nothing under `internal/` until the integration shims land.
- **Phase 16** depends on Phase 14 (OIDC/SAML resolvers implement `auth.TokenResolver`). **Phase 17** also depends on Phase 14, and — as built — on Phase 16: scoped role mapping is what closes the hole Phase 16 opened by binding every federated user at `*`.
- **Phase 18** depends on Phase 15 — extends the quota module with a self-service workflow and adds a sister cost-attribution module — and, as built, on Phase 17: approving a raise is bounded by the container the subject sits in, and a spend report is filtered by the same scope sets as a twin listing.
- **Phase 19** lands last; it surfaces Phases 14, 15, 17, and 18 in the React frontend and reuses the Phase 13 audit endpoint. As built it also changed the backend twice — tiers became database-backed state, and a scope-filtered global event stream was added — because two of its steps had no backend to surface.

Phases 23–25 are the follow-on from the standing-objectives line, and are ordered by how much each one unblocks the next rather than by size.

- **Phase 23** (per-objective spend ceilings) depends on **Phase 15**'s quota subjects and **Phase 18**'s override path, and on **Phase 20** for the thing being capped. It is independent of 24 and 25 and could ship first or last; it is placed first because it is the only one of the three that a running deployment is currently exposed without.
- **Phase 24** (behavioural conformance) depends on nothing new. It is placed before 25 because 25 adds capabilities and an environment to the karakuri pack, and the point of 24 is that new declarations should meet a suite that runs them rather than reads them. Shipping 25 first would add the exact kind of claim 24 exists to check.
- **Phase 26** (the write path) was the one blocking everything else in this group: until it landed, `self_improve` could reach two of its three criteria and every roadmap phase was written by a human. It was found by trying to have Karakuri develop Phase 23 and discovering that the capability with a worktree could not write and the capability that could write had no worktree. Both halves of the fix — the workspace and the route — turned out to be the same mistake, recorded in [ADR 019](adr/019-capabilities-declare-what-they-need.md): a property the system needed was inferred from an identifier instead of declared by the thing that knows it.
- **Phase 25** (self-improvement without a history) depends on **Phase 22** for the pack it extends and on **Phase 6**'s version-control adapter for CI status. It is the phase that makes Phase 22 usable on the day it is enabled rather than months later, and it is deliberately scoped to widen what the maintainer can *see* — never what it may *do*, which stays bounded by ADR 017 and by Phase 20's ceiling.

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
| Cross-domain Objectives                                               | **Fully implemented** (Phase 13) — `AdditionalDomains []string` on Objective; loop recruits agents + envs across the union; `Criterion.Domain` drives per-domain score reporting          |
| Cross-pack capability collision audit                                  | **Fully implemented** (Phase 13) — `conformance.CheckCrossPackCollisions` covers capability/environment/agent IDs; bootstrap runs it across active packs and logs failures at WARN        |
| Memory retention scheduler                                            | **Fully implemented** (Phase 13) — `MemoryService.RunRetention()` driven by a goroutine ticker; per-tier TTL + semantic confidence floor; disabled by default                            |
| Reflexion reasoning strategy                                           | **Fully implemented** (Phase 13) — two-pass critique + revise inside `stepReason`; falls back to the draft on unparseable revision so it never regresses below chain-of-thought          |
| Reflexion benchmark harness                                            | **Fully implemented** (Phase 13) — `cmd/krk-bench` runs 200 trials × 5 scenarios for both strategies; writes a deterministic markdown summary to `docs/benchmarks.md`                    |
| Helm chart OCI publishing                                              | **Fully implemented** (Phase 13) — `.github/workflows/release-helm.yml` packages + pushes to `oci://ghcr.io/<owner>/charts` on `v*.*.*` tags via `GITHUB_TOKEN`                          |
| Authority-bounds audit log                                            | **Fully implemented** (Phase 13) — `tool_events` columns `kind`/`escalation_reason`/`approver`/`bounds_violation`; every escalation + approval written; `GET /api/v1/audit` + `krk audit` |
| Standing objectives (declaration)                                     | **Fully implemented** (Phase 20) — `Mode`/`Cadence`/`Autonomy` on Objective; empty mode is oneshot, so nothing written before Phase 20 changes behaviour |
| Reconcile supervisor (sense → drift → converge)                       | **Fully implemented** (Phase 20) — one due-wheel goroutine over an indexed `next_due_at`; the cheap tier hashes `Snapshot` and spends no tokens; `reconcile_states` + `reconcile_outcomes` |
| Cadence scheduling (cron, interval, timezone, quiet windows)          | **Fully implemented** (Phase 20) — `internal/platform/schedule` over `robfig/cron` as a parser only; DST-safe, quiet windows defer rather than drop |
| Earned autonomy under a declared ceiling                              | **Fully implemented** (Phase 20) — four rungs written into `agent.AuthorityBounds`, so the decide step is still the only gate; promotion/demotion audited as `kind=promotion`/`demotion` |
| Multi-replica coordination (outer loop)                               | **Fully implemented** (Phase 20) — DB lease on `reconcile_states`, one conditional UPDATE; discharges part of Phase 11's deferred leader election. Loop execution itself is still single-node-resume |
| Circuit breaker + stall detector                                      | **Fully implemented** (Phase 20) — both pause the objective and raise a checkpoint; the stall brake is the one the Phase 1 risk table promised and never built |
| Watch mode                                                            | **Superseded** (Phase 20) — `watch.go` deleted; `--watch` / `watch_mode: true` now declare a standing objective at sense-only autonomy, which behaves the same and survives a restart |
| Digests / periodic reports                                            | **Fully implemented** (Phase 21) — per-twin schedules with their own lease; assembled from existing records so a window is reproducible; delivered through the twin's bound adapter and audited as a `tool_events` row |
| Digest delivery: Slack, email                                         | **Fully implemented** (Phase 21) — via the `messaging` and `email` slots, twin-bound (ADR 006) |
| Digest delivery: project trackers, repositories                       | Declared and refusing (Phase 21) — the channels are accepted and return an error recorded on the schedule, rather than a silent success |


---

## Risks and Trade-offs


| Risk                                                         | Severity | Mitigation                                                                                                                                                                                                   |
| ------------------------------------------------------------ | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Non-terminating loop (objective never satisfied)             | High     | Hard `MaxIter` cap (default 50) per objective; criteria completion score tracked per iteration. **The no-progress brake promised here was never built on the inner loop and now exists on the outer one** (Phase 20): three reconciles that fail to move the weighted score pause the objective and raise a checkpoint, which is a stronger signal than three iterations of a single run |
| Memory bloat degrades semantic recall quality                | High     | Retention TTL + confidence decay (Phase 13); `Forget` runs on schedule; consolidation promotes only high-confidence entries; semantic tier size cap with LRU eviction                                        |
| sqlite-vec performance degrades at scale                     | Medium   | `Memory` interface abstracts vector store; pgvector swap = only `platform/memory/semantic.go` changes (Phase 8); no feature or core changes required                                                         |
| LLM reasoning inconsistency across iterations                | Medium   | Reflexion strategy adds self-critique pass; procedural memory surfaces historical success rates; `ConfidenceThreshold` in `AuthorityBounds` escalates uncertain plans                                        |
| Domain pack quality variance                                 | High     | Conformance suite mandatory for registration; `CapabilityRegistry` validates schemas at registration time; rejects malformed packs with descriptive error                                                    |
| Worktree filesystem conflicts under concurrent load          | Medium   | Branch naming scoped to `<objective-id>/<task-id>` guarantees uniqueness; `WorktreeManager` is the sole path to worktree creation; no direct filesystem writes from agents                                   |
| LangChain Go version drift breaking agent behaviour          | Medium   | All LangChain Go usage confined to `internal/platform/agent/` + `internal/platform/llm/`; `AgentFactory` interface is the sole boundary; swap cost is one package                                            |
| Cross-domain objective complexity exceeds LLM context        | Medium   | Objectives scoped to single domain by default; world state chunked and summarised before reason step if size exceeds provider context limit (Phase 13)                                                       |
| sqlite-vec extension unavailable in deployment               | Low      | Health check verifies sqlite-vec at startup; if unavailable, semantic memory degrades gracefully to keyword-based recall with startup warning                                                                |
| Authority bounds misconfiguration permits unintended actions | High     | Default `AuthorityBounds` is maximally restrictive (`MaxAutonomousActions: 0`, `ConfidenceThreshold: 1.0`); operators must explicitly relax bounds in config; all autonomous actions logged to `tool_events`; **Phase 14 RBAC enforces permissions at the request-routing layer** so a misconfigured agent can't even reach a protected endpoint                                                                                                |
| Karakuri changes itself unsupervised                          | High     | Phase 22 splits deciding from doing across two packs. The karakuri pack's capabilities analyse and draft and none of them writes; the writing is the software pack's, in a git worktree, through a pull request an operator reviews. `karakuri-maintainer` carries `MaxAutonomousActions: 0` and a confidence threshold no plan can clear, so it escalates however much autonomy its standing objective has earned, and both of the pack's environments refuse an `Act` out loud rather than succeeding quietly. The split is pinned by tests, not by comments: the pack owns no capability whose ID looks like a write, and `self_improve` is verified in part by `software.act.open_pull_request` so it cannot mark its own homework. The telemetry port is read-only because a pack that could write there could rewrite the evidence of what it did |
| A standing objective spends indefinitely, or acts unsupervised | High   | Phase 20's whole design is the mitigation, and every part of it is a ceiling somebody declared. The cheap tier means an unchanged world costs adapter calls and no tokens. `min_interval` floors how often the expensive tier can fire, quiet windows blacken hours it may not act in, and `max_concurrent` bounds how many can run at once — loops were unbounded detached goroutines before this. Autonomy starts at propose and rises only to an operator-declared `ceiling`, re-applied on every read of the state so a hand-edited row cannot widen it; one rejection demotes immediately. Three consecutive failures, or three reconciles with no progress, pause the objective **and raise a checkpoint** — an objective that went quiet with no explanation is indistinguishable from one that is content, which is the failure mode worth spending a checkpoint to avoid |
| Two replicas hold the same standing objective                | High     | Phase 20 claims each objective with a single conditional `UPDATE` on `reconcile_states` and a `RowsAffected` check, so the database arbitrates rather than a coordination service Karakuri does not have. A crashed holder releases nothing — its lease expires and the next replica to ask wins. Pinned against a real database by eight goroutines racing for one objective and exactly one winning. Without it the cost is not a duplicate run somebody notices but a recurring duplicate bill and two copies of the same morning report, forever |
| Cost runaway from unbounded LLM use                          | High     | Phase 15 introduces per-twin LLM token budgets; exhaustion produces a checkpoint event (human approval) rather than a 500 or silent overrun. Phase 18 shipped the attribution: every model and tool call is recorded with its provider, model and the containers it belonged to, priced from a configured table, and published as `cost_recorded` — so spend per twin / team / provider is visible before the bill arrives. Phase 18 also wired the per-capability daily quota, which had been configured and enforced nowhere since Phase 15                                                                                                                                                              |
| IdP outage locks operators out of Karakuri                    | High     | **Resolved differently than planned.** Local password login stays mounted alongside any configured provider, so the bootstrap administrator is the break-glass path. No static token was added: Phase 14 deleted the static bearer token, and re-adding a long-lived credential to survive a temporary outage trades a permanent risk for a temporary one. `ChainResolver` puts the local resolver first, so password login does not depend on the IdP being reachable |
| Cross-tenant access through a container scope                 | Medium   | Phase 17 keys every scope on an issued ID, never a display name, so two organisations with a team called "eng" cannot collide — the case is pinned end to end from the tree through `InScope` to a 403. A resource with no containers carries no labels and matches exactly what it matched under the flat model, so no existing grant widens. Listing is filtered from the same bindings the per-resource check reads, and an empty grant set matches no rows rather than every row |
| A quota approval used to raise another tenant's limit         | Medium   | Phase 18 checks `quota:approve` against the subject the request names, rendered as a resource carrying its containers — the same containment rule ADR 010 set for handing out bindings. A route gate cannot do this: the subject arrives inside a stored request rather than in the URL. Pinned by `TestQuotaApprovalIsConfinedToTheApproversTenant`. Rejecting is deliberately ungated, so requests from tenants nobody administers cannot get stuck pending |


