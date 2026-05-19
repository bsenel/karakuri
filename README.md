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

## License

Apache 2.0 — see [LICENSE](LICENSE).
