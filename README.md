# Karakuri

An autonomous decision-making platform built on four primitives: **Capabilities**, **Environments**, **Objectives**, and **Agents**. The engine runs a continuous observe→reason→decide→act→verify→learn loop, accumulates cross-run memory, and escalates to humans only when confidence or authority bounds require it.

## Quick start

```bash
# Build
make build

# Start the server.
# Auth: set ANTHROPIC_API_KEY (or GOOGLE_API_KEY for Gemini). If unset and
# the matching CLI (`claude` / `gemini`) is installed and authenticated,
# Karakuri routes through the CLI as a transparent fallback — operators
# with `claude /login` already done can skip the env var entirely.
./bin/server

# Create a twin and objective, then run the loop
krk twin create --name "dev-team" --kind team --domain software
krk objective create --twin <twin-id> --title "implement JWT auth" --domain software
krk loop start <objective-id> --twin <twin-id>
krk loop status <loop-id>

# Approve a checkpoint when the loop escalates
krk loop resume <loop-id> --decision approve

# Continuous watch mode (creates watcher twin and streams SSE until Ctrl+C)
krk auto --domain software
```

## Architecture

Three-layer Go monolith + thin CLI:

```
cmd/              → server and krk binaries
internal/core/    → domain types and interfaces (zero vendor imports)
internal/feature/ → business logic (loop, memory, checkpoint, artifact, …)
internal/platform/→ vendor bindings (LangChain Go, GORM, go-git, OTel)
internal/api/     → HTTP handlers; delegates to feature services
domains/          → pluggable domain packs
cli/              → krk commands; thin HTTP client
```

See [docs/architecture.md](docs/architecture.md) for the full design and loop step breakdown.

## The Reasoning Loop

```
OBSERVE → REASON → DECIDE → ACT → VERIFY → LEARN
   ↑                                          │
   └──────────────────────────────────────────┘
```

- **Observe** — fan-out across all registered environments; recall episodic + semantic memory
- **Reason** — call the agent with world state + memory; parse JSON plan. Strategy is per-agent: ChainOfThought (default), ReAct, or **Reflexion** (Phase 13 — two-pass critique + revise; never regresses below the draft if the revision is unparseable)
- **Decide** — check authority bounds; bias confidence from procedural memory history; emit checkpoint if escalating. Every escalation writes a `tool_events` row tagged `kind=escalation, bounds_violation=true` for the audit log
- **Act** — execute actions; create isolated git worktrees for code-writing capabilities
- **Verify** — evaluate weighted success criteria via agent or environment results; emits a `per_domain_score` payload when criteria carry `Domain` tags
- **Learn** — write episodic + procedural memory; consolidate to semantic tier

Synthetic Reflexion-vs-ChainOfThought comparison lives in [`docs/benchmarks.md`](docs/benchmarks.md); regenerate with `go run ./cmd/krk-bench`.

## Domain Packs

Domain packs encapsulate all field-specific knowledge. The core engine imports none of it.

| Pack | Status | Capabilities | Agents | Templates |
|------|--------|-------------|--------|-----------|
| `software` | Active (v1) | 20 | 7 | 7 |
| `agriculture` | Active (v1) | 8 | 2 | 2 |
| `healthcare` | Active (Phase 10) — strict authority bounds, compliance-aware verifiers | — | — | — |
| `legal`, `mechanical`, `consulting` | Stub | — | — | — |

Cross-domain objectives (Phase 13): set `additional_domains` on an Objective to have the loop runner recruit agents and environment factories across multiple packs in a single iteration; `Criterion.Domain` tags per-domain verify scores on the `loop.step_completed` event.

Validate any pack with:
```bash
krk domain test software
krk domain test agriculture
```

See [docs/domain-packs.md](docs/domain-packs.md) to author your own pack.

## Memory

Four tiers, all persisted across loop runs:

| Tier | Storage | Purpose |
|------|---------|---------|
| Working | in-process map | In-flight state within a loop run |
| Episodic | SQLite or Postgres | Iteration traces; recalled at observe step |
| Semantic | SQLite (keyword) or **pgvector** (Phase 8 — cosine-distance vector recall) | Consolidated facts; promoted from episodic |
| Procedural | SQLite or Postgres | Per-capability success rates; biases plan confidence at decide step |

**Retention scheduler** (Phase 13, disabled by default): a goroutine ticker calls `MemoryService.RunRetention(ctx, ...)` every `memory.retention.interval_minutes`. Per-tier TTLs (`working_ttl_minutes`, `episodic_ttl_days`, `semantic_ttl_days`) and a `semantic_min_score` confidence floor drop stale entries; enable it after measuring growth.

## CLI Commands

```
# Twins
krk twin create --name <name> --kind <person|team|org> --domain <id>
krk twin list
krk twin get <id>

# Objectives
krk objective create --twin <id> --title <title> --domain <id> \
                     [--description <text>] [--max-iter N] [--priority N] \
                     [--template <id>]
krk objective list [--twin <id>] [--status <pending|active|completed|failed>]
krk objective get <id>

# Loop (start returns JSON `{"loop_id":"..."}` with --output json; jq
# selector is `.loop_id`, not `.id`)
krk loop start <objective-id> --twin <id> [--max-iter N] [--watch]
krk loop status <loop-id>
krk loop resume <loop-id> --decision <approve|reject|modify> \
                          [--note <text>] [--approver <id>]

# Checkpoints — --note + --approver populate the Phase 13 audit row
krk checkpoint list [--twin <id>]
krk checkpoint get <id>
krk checkpoint resolve <id> --decision <approve|reject|modify> \
                            [--note <text>] [--approver <id>]

# Memory
krk memory store --agent <id> --tier episodic --content "..."
krk memory recall --query "..." [--tier episodic]
krk memory forget --before <date>

# Artifacts
krk artifact list [--objective <id>]
krk artifact get <sha>
krk artifact write --objective <id> --agent <id> --content "..."

# Domains
krk domain list
krk domain capabilities [--domain <id>]
krk domain test <domain-id>

# Research
krk research --topic "..." [--depth shallow|deep]

# Watch mode
krk auto [--domain <id>]

# Audit log (Phase 13)
krk audit [--kind execute|escalation|approval] [--objective <id>] \
          [--agent <id>] [--violations-only] [--since <RFC3339>] [--limit N]
```

## Configuration

Copy `config/default.yaml` and set `ANTHROPIC_API_KEY`. Key options:

```yaml
server:
  addr: ":8080"
database:
  driver: sqlite
  dsn: karakuri.db
providers:
  default: claude
auth:
  token: ""        # set to require Bearer token on all endpoints
memory:
  semantic_top_k: 5
```

## Deployment

Karakuri ships five interchangeable ways to run locally. All five share one Docker image (`karakuri:latest`) and one canonical runtime config (`deploy/karakuri.yaml`), so switching between them never requires re-templating values.

| Variant | Best for | Up | Down |
|---|---|---|---|
| **Docker Compose** | Simplest single-machine dev | `make docker-up` | `make docker-down` |
| **Helm (direct)** | Any existing Kubernetes cluster | `make helm-up` | `make helm-down` |
| **Minikube** | Local single-node K8s with a built-in image registry | `make minikube-up` | `make minikube-down` |
| **k3s** | Lightweight K8s (edge / VMs / Raspberry Pi) | `make k3s-up` | `make k3s-down` |
| **ArgoCD** | GitOps continuous sync from this repo's `deploy/` | `make argocd-up` | `make argocd-down` |

Every variant reaches the API at `localhost:8080` (for Helm/k3s, after `kubectl port-forward svc/karakuri 8080:8080 -n karakuri`).

### Required env vars

```bash
export ANTHROPIC_API_KEY=sk-ant-...
export KARAKURI_AUTH_TOKEN=""   # optional; empty disables auth
```

The Makefile injects these as a Kubernetes Secret (`karakuri-secrets`) for the K8s variants and as Compose environment for Docker.

### Helm chart

`deploy/` is the chart root (chart name `karakuri` from `Chart.yaml`). The same chart drives Helm direct, Minikube, k3s, and ArgoCD. Tunable values live in [`deploy/values.yaml`](deploy/values.yaml); k3s-specific overrides in [`deploy/values-k3s.yaml`](deploy/values-k3s.yaml).

```bash
helm template karakuri deploy                       # render manifests
helm upgrade --install karakuri deploy -n karakuri  # install/upgrade
helm package deploy                                 # produce karakuri-0.1.0.tgz
```

### Single source of truth

`deploy/karakuri.yaml` is the canonical server config (`/data/`-paths). The Dockerfile `COPY`s it into the image; the chart's ConfigMap template reads the same file via `.Files.Get`. No drift is possible. The local-dev config (`./karakuri.db` paths) remains at `config/default.yaml` for `go run`.

## Adapter Ecosystem

| Category | Adapter | Status |
|----------|---------|--------|
| LLM | Claude (claude-sonnet-4-6 default) | **Active** |
| LLM | Gemini (vertex AI) | **Active** (Phase 7) |
| LLM | Cursor + Copilot CLI agents | **Active** (Phase 7 — wraps external CLIs) |
| Storage | SQLite | **Active** |
| Storage | PostgreSQL + pgvector | **Active** (Phase 8) |
| Storage | MySQL | Interface only |
| Migration tooling | `krk migrate --from … --to …` | **Active** (Phase 8 — generic GORM row copy) |
| Git worktrees | go-git | **Active** |
| Version Control | GitHub (multi-instance, twin-bound) | **Active** (Phase 6) |
| Version Control | GitLab | Stub |
| Project Management | Linear | **Active** (Phase 6) |
| Project Management | Jira | Stub |
| Messaging | Slack | **Active** (Phase 6) |
| Design | Figma | **Active** (Phase 6) |
| Testing | Playwright, Go test runner | **Active** (Phase 6) |
| Calendar / Email | Google Calendar, Gmail / Outlook / SMTP / Apple Mail | **Active** (Phase 6) |
| OTel Exporter | Local file (NDJSON / CSV / **Parquet**) with size+age rotation | **Active** (Phase 12) |
| OTel Exporter | AWS (CloudWatch metrics + S3 NDJSON logs) | **Active** (Phase 12) |
| OTel Exporter | Datadog (`/api/v1/series` + `/api/v2/logs`) | **Active** (Phase 12) |
| OTel Exporter | NewRelic, Elasticsearch (ELK), Loki, OTLP Collector, Prometheus (scrape + pushgateway) | **Active** (Phase 12 extension) |
| OTel Exporter | RetryExporter wrapper (exponential backoff, `ErrPermanent` short-circuit) | **Active** (Phase 12 extension) |
| Executor | Local goroutines | **Active** |
| Executor | Restate (durable workflows) | **Active** (Phase 11) |
| Executor | Celery (Python workers via Redis) | **Active** (Phase 11) |
| Frontend | Embedded React SPA (Vite + React 18) served from the Go binary | **Active** (Phase 9 — via `web/embed.go`) |

## Observability

Karakuri's in-process metrics + logs fan out simultaneously to any subset of eight destinations (Phase 12 + extension), with chain isolation — one downstream outage logs at WARN but never blocks the others. Remote exporters are wrapped in `RetryExporter` (3 attempts, exponential backoff capped at 30s, `ErrPermanent` short-circuits on 401/403). Configure under `observability.exporters.{local,aws,datadog,newrelic,elasticsearch,loki,otlp,prometheus}.enabled` in [`deploy/values.yaml`](deploy/values.yaml); credentials flow through the `karakuri-secrets` Kubernetes Secret. The Prometheus exporter mounts `/metrics` outside bearer auth so scrapers don't need a token. See [`docs/roadmap.md`](docs/roadmap.md#phase-12--observability-fan-out-completed) Phase 12 for env var details.

## Development

```bash
make build          # build server and krk
make test           # run all tests including integration
go test ./test/integration/... -v   # integration tests only

# CI runs the same matrix on every PR (.github/workflows/ci.yml):
# Frontend → Build → Vet → Test, plus CodeQL static analysis (Go + JS/TS).
```

Import boundary rules are enforced by golangci-lint (see `.golangci.yml`):
- LangChain Go only in `internal/platform/`
- Domain packs only in `cmd/` and `internal/app/`

## Repository governance

`main` is protected — direct pushes are blocked, all changes land via squash-merged pull requests. Active rules:

- 1 approving review required; code-owner review required; stale reviews dismissed on new push; last-push approval required
- All four required status checks (Frontend, Build, Vet, Test) must pass and the branch must be up to date
- Linear history; signed commits required; force-pushes and deletions blocked

Additional security stack:

- **Secret scanning** + **push protection** enabled
- **CodeQL** static analysis for Go and JS/TS, on every PR + weekly schedule
- **Dependabot** alerts + security updates + version updates across `gomod`, `npm`, and `github-actions`; major-version bumps excluded — those land via maintainer-opened PRs after compat testing
- **Private vulnerability reporting** open at [`/security/advisories/new`](https://github.com/bsenel/karakuri/security/advisories/new)

See [SECURITY.md](./SECURITY.md) for vulnerability reporting, [CONTRIBUTING.md](./CONTRIBUTING.md) for the Dependabot review policy and merge workflow, and [`.github/CODEOWNERS`](./.github/CODEOWNERS) for ownership.

## Philosophy

Karakuri is built for **human augmentation**.

The project is free for organizations using AI to
empower employees and improve productivity.

Use intended primarily for workforce replacement
is restricted under the
[Karakuri Human Augmentation License Addendum (HALA)](./HALA.md).

## Roadmap

Phases 1–13 have shipped (core engine through cross-domain objectives + hardening). Phases 14–19 are queued and introduce a multi-team production layer:

- **Phase 14:** RBAC + fine-grained authorization, shipped as a standalone module `github.com/bsenel/karakuri/auth` reusable by any net/http or chi server
- **Phase 15:** API rate limiting + quota management, shipped as `github.com/bsenel/karakuri/quota` with Redis/SQL backend submodules
- **Phase 16:** Federated identity (OIDC + SAML) as `auth` submodules
- **Phase 17:** Hierarchical resources + org units (`auth` v0.2)
- **Phase 18:** Quota self-service workflow + cost attribution (`quota` v0.2 + sibling `quota/cost` module)
- **Phase 19:** Frontend pages for auth, quota, cost, and audit

Full per-phase status, acceptance criteria, and architecture rationale in [docs/roadmap.md](docs/roadmap.md).

## License

Licensed under Apache 2.0.

See:

- [LICENSE](./LICENSE)
- [HALA.md](./HALA.md)
- [SECURITY.md](./SECURITY.md)
- [CONTRIBUTING.md](./CONTRIBUTING.md)
