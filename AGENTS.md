# Karakuri — Agent Instructions

Guidance for AI coding agents working in this repository. Follow the **nearest** `AGENTS.md` on the path from leaf to root; **child files override parent** where they conflict. Non-conflicting parent rules still apply.

## Principles

- **YAGNI** — Ship only what the task needs. No speculative abstractions, flags, or adapters.
- **KISS** — Prefer straight-line code and explicit wiring over frameworks and indirection.
- **DRY** — Extract duplication only after a second real use case; avoid premature shared helpers.

## Clean architecture

Dependency direction is inward only:

```
cmd/ → internal/api/ → internal/feature/ → internal/core/
                              ↓
                    internal/platform/
```

| Layer | Path | Responsibility |
|-------|------|----------------|
| Entry | `cmd/` | `main`, wiring, config path |
| Delivery | `internal/api/` | HTTP, SSE, auth, JSON — no business rules |
| Use cases | `internal/feature/` | Orchestrator, strategy, discovery, delivery, session, etc. |
| Domain | `internal/core/` | Interfaces, entities, events — **no vendor imports** |
| Infrastructure | `internal/platform/` | GORM, go-git, langchaingo, OTel, tool adapters |

**Thin CLI, thick server** ([ADR 001](docs/adr/001-thin-cli-thick-server.md)): `cli/` and future UI talk HTTP only; all orchestration lives in the API.

## Hierarchical AGENTS.md

| Path | Focus |
|------|--------|
| [cli/AGENTS.md](cli/AGENTS.md) | HTTP client CLI |
| [internal/core/AGENTS.md](internal/core/AGENTS.md) | Domain types and ports |
| [internal/feature/AGENTS.md](internal/feature/AGENTS.md) | Use-case services |
| [internal/api/AGENTS.md](internal/api/AGENTS.md) | Handlers and routing |
| [internal/platform/AGENTS.md](internal/platform/AGENTS.md) | Adapters and vendors |
| [internal/platform/llm/AGENTS.md](internal/platform/llm/AGENTS.md) | LLM providers (langchaingo) |
| [internal/platform/agent/AGENTS.md](internal/platform/agent/AGENTS.md) | Agent runtime |
| [workflows/AGENTS.md](workflows/AGENTS.md) | Workflow YAML |

Add a new `AGENTS.md` in a subdirectory when that area has **distinct** conventions (adapter boundaries, vendor SDKs, DSLs). Keep each file focused; link to ADRs in `docs/adr/` instead of duplicating rationale.

## Global rules

1. **Import boundary** — `github.com/tmc/langchaingo` only under `internal/platform/`. Enforced by `scripts/check_langchaingo_imports.sh`.
2. **Storage** — Sessions and artifacts via `storage.StorageAdapter`; content-addressed blobs per [ADR 002](docs/adr/002-vfs-artifact-store.md).
3. **Git** — Delivery worktrees via `git.WorktreeManager`; see [ADR 003](docs/adr/003-git-worktrees.md).
4. **No-op adapters** — External integrations (Linear, Slack, etc.) use noop implementations until wired; the system must run without them.
5. **Tests** — `go test ./...` must pass. Add tests for non-trivial logic in `internal/feature/` and `internal/platform/`.
6. **API contract** — REST shapes in [docs/openapi.yaml](docs/openapi.yaml); do not break paths or JSON fields without updating the spec.
7. **Config** — Defaults in `config/default.yaml`; secrets via environment (e.g. `ANTHROPIC_API_KEY`), never committed.

## Before finishing a change

- Match existing naming, package layout, and constructor injection style.
- Run `make build` and `make test`.
- Run `scripts/check_langchaingo_imports.sh` if you touched imports.
- Update the nearest child `AGENTS.md` only when you introduce a **new recurring pattern** for that area.
