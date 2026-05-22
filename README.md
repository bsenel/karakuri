# Karakuri

An autonomous decision-making platform built on four primitives: **Capabilities**, **Environments**, **Objectives**, and **Agents**. The engine runs a continuous observe→reason→decide→act→verify→learn loop, accumulates cross-run memory, and escalates to humans only when confidence or authority bounds require it.

## Quick start

```bash
# Build
make build

# Start the server (requires ANTHROPIC_API_KEY)
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
- **Reason** — call the agent (Claude) with world state + memory; parse JSON plan
- **Decide** — check authority bounds; bias confidence from procedural memory history; emit checkpoint if escalating
- **Act** — execute actions; create isolated git worktrees for code-writing capabilities
- **Verify** — evaluate weighted success criteria via agent or environment results
- **Learn** — write episodic + procedural memory; consolidate to semantic tier

## Domain Packs

Domain packs encapsulate all field-specific knowledge. The core engine imports none of it.

| Pack | Status | Capabilities | Agents | Templates |
|------|--------|-------------|--------|-----------|
| `software` | Active (v1) | 20 | 7 | 7 |
| `agriculture` | Active (v1) | 8 | 2 | 2 |
| `healthcare`, `legal`, `mechanical`, `consulting` | Stub | — | — | — |

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
| Episodic | SQLite | Iteration traces; recalled at observe step |
| Semantic | SQLite | Consolidated facts; promoted from episodic |
| Procedural | SQLite | Per-capability success rates; biases plan confidence at decide step |

## CLI Commands

```
# Twins
krk twin create --name <name> --kind <person|team|org> --domain <id>
krk twin list
krk twin get <id>

# Objectives
krk objective create --twin <id> --title <title> --domain <id> [--max-iter N]
krk objective list [--twin <id>]
krk objective get <id>
krk objective status <id> <status>

# Loop
krk loop start <objective-id> --twin <id> [--max-iter N] [--watch]
krk loop status <loop-id>
krk loop resume <loop-id> --decision <approve|reject|modify>

# Checkpoints
krk checkpoint list
krk checkpoint get <id>
krk checkpoint resolve <id> --decision <approve|reject|modify>

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
| LLM | Claude (claude-sonnet-4-6) | **Active** |
| LLM | Gemini, Cursor, Copilot | Stub |
| Storage | SQLite | **Active** |
| Storage | PostgreSQL, MySQL | Interface only |
| Git worktrees | go-git | **Active** |
| Version Control | GitHub, GitLab | No-op |
| Project Management | Linear, Jira | No-op |
| Messaging | Slack | No-op |
| Testing | Playwright, Go test runner | No-op |
| OTel Exporter | Local file (JSON/NDJSON) | **Active** |
| OTel Exporter | AWS, Datadog | Stub |
| Executor | Local goroutines | **Active** |
| Executor | Celery, Restate | Interface only |

## Development

```bash
make build          # build server and krk
make test           # run all tests including integration
go test ./test/integration/... -v   # integration tests only
```

Import boundary rules are enforced by golangci-lint (see `.golangci.yml`):
- LangChain Go only in `internal/platform/`
- Domain packs only in `cmd/` and `internal/app/`

## Philosophy

Karakuri is built for **human augmentation**.

The project is free for organizations using AI to
empower employees and improve productivity.

Use intended primarily for workforce replacement
is restricted under the
[Karakuri Human Augmentation License Addendum (HALA)](./HALA.md).

## License

Licensed under Apache 2.0.

See:

- [LICENSE](./LICENSE)
- [HALA.md](./HALA.md)
