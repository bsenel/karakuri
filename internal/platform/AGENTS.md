# Platform (`internal/platform`)

Overrides parent rules for `internal/platform/` only.

## Role

**Infrastructure implementations** of ports defined in `internal/core/` and interfaces used by `internal/feature/`. All third-party SDKs live here.

## Subsystems

| Path | Technology |
|------|------------|
| `db/`, `storage/` | GORM, SQLite/PostgreSQL |
| `git/` | go-git worktrees |
| `llm/` | langchaingo provider adapters |
| `agent/` | `core/agent` implementation |
| `executor/` | Local goroutine executor (stubs: Celery, Restate) |
| `observability/` | OTel, NDJSON/CSV/Parquet exporters |
| `tools/` | Category adapters (versioncontrol, messaging, testing, …) with `noop` defaults |

## Rules

1. **Implement, don't leak** — Feature code sees `storage.StorageAdapter`, not `gorm.DB`. Map DB models in `db/schema` ↔ `entity` at the storage boundary.
2. **Registries** — LLM providers and tool adapters register in bootstrap; support fallback provider from config.
3. **Stubs** — New external integrations ship as interface + `noop` until credentials and behavior are defined (see README adapter table).
4. **langchaingo** — Confined to `llm/` and `agent/` (and any future platform subpackages). Run `scripts/check_langchaingo_imports.sh` after changes.

## Child docs

- [llm/AGENTS.md](llm/AGENTS.md) — Provider adapters
- [agent/AGENTS.md](agent/AGENTS.md) — Agent factory and runtime

## Adding an adapter

1. Define or reuse interface in `tools/<category>/adapter.go`.
2. Add `noop.go` for offline operation.
3. Wire real implementation in `internal/app/bootstrap.go` behind config flags.
4. Do not call the adapter from `internal/core/`.
