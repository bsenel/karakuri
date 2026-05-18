# LLM providers (`internal/platform/llm`)

Overrides [platform/AGENTS.md](../AGENTS.md) for LLM code only.

## Role

Wrap **langchaingo** (`github.com/tmc/langchaingo`) behind `ProviderAdapter` so feature and agent code never import vendor LLM packages.

## `ProviderAdapter`

- `Name()`, `Complete()`, `Stream()`, `Available()`
- Map `CompletionRequest` / `CompletionResponse` to langchaingo `llms` APIs inside each provider file (`claude.go`, etc.).
- **Claude** is the v1 production path (`llms/anthropic`); Gemini, Cursor, Copilot remain stubs until needed.

## Registry

- `NewRegistry(fallback string)` — register providers in bootstrap; resolve by name from `agent.Input.Provider`.
- If API key missing, `Available()` returns false and callers may use mock/fallback behavior already in providers — do not panic at registration time.

## Rules

1. **Imports** — Only files under `internal/platform/llm/` (and `agent/` for orchestration) may import `langchaingo`.
2. **No domain leakage** — Do not reference `entity.Session` or orchestrator types here.
3. **Tokens and errors** — Return `TokensUsed` when the SDK provides it; wrap errors with provider name for observability.
4. **Streaming** — Send chunks on a channel; close on completion or error; respect `ctx.Done()`.

## Adding a provider

1. New file `internal/platform/llm/<name>.go` implementing `ProviderAdapter`.
2. Register in `internal/app/bootstrap.go`.
3. Document in README adapter table and `config/default.yaml` if configurable.
