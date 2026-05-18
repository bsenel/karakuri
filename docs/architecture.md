# Karakuri Architecture

Karakuri is a Go API server with a thin `krk` CLI client. All business logic lives in the API.

## Layers

- `cmd/` — entrypoints
- `internal/api/` — HTTP handlers and routing
- `internal/feature/` — use-case services (orchestrator, strategy, discovery, delivery, autonomous)
- `internal/core/` — interfaces and domain types (no vendor imports)
- `internal/platform/` — implementations (LangChain Go, GORM, go-git, OTel)

## Key flows

1. CLI creates session via `POST /sessions`
2. CLI triggers `POST /sessions/:sha/run`
3. Orchestrator loads workflow YAML, plans tasks, dispatches agents
4. Delivery mode provisions Git worktrees per implementation task
5. Events stream via SSE at `GET /sessions/:sha/events`

See ADRs in `docs/adr/` for design decisions.
