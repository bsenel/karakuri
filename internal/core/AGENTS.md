# Domain core (`internal/core`)

Overrides parent rules for `internal/core/` only.

## Role

Pure domain: **entities**, **ports** (interfaces), and **shared types**. This layer defines *what* the system is, not *how* it is persisted or called.

## Packages

| Package | Purpose |
|---------|---------|
| `entity` | Session, artifact, task, checkpoint structs and enums |
| `agent` | `Agent`, `AgentFactory`, `Input`/`Output` — vendor-agnostic agent contract |
| `event` | Event types and hub interface for SSE |
| `vfs` | Virtual filesystem / blob addressing concepts |
| `errors` | Domain error values and helpers |

## Rules

1. **No vendor imports** — No GORM, chi, langchaingo, go-git, or OTel in this tree.
2. **Interfaces stay small** — Prefer one clear method group per port; implement in `internal/platform/`.
3. **JSON tags** on entities exposed via API; keep field names stable (see OpenAPI).
4. **No I/O** — No `context` calls to databases, HTTP, or filesystem here.

## Adding types

- New session modes or states → `entity` constants + document in OpenAPI.
- New agent capabilities → extend `agent.Input`/`Output` only when a use case needs them; map from platform in `internal/platform/agent/`.
