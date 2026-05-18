# Agent runtime (`internal/platform/agent`)

Overrides [platform/AGENTS.md](../AGENTS.md) for agent factory/runtime only.

## Role

Implement `core/agent.Agent` and `AgentFactory` using `llm.ProviderAdapter`, `event.Hub`, and `observability.OTel`.

## Components

- `Factory` — `New` / `NewWithSession`: resolve provider from registry, return `langchainAgent`.
- `langchainAgent` — `Run` / `Stream`: build prompts from `coreagent.Input`, call provider, emit events/metrics.
- `toolregistry.go` — Tool binding deferred until v2; do not add half-wired langchaingo tools without adapter backing.

## Rules

1. **Map at the edge** — Convert `coreagent.Input` → `llm.CompletionRequest` and response → `coreagent.Output` in this package only.
2. **Session context** — Use `NewWithSession` when orchestrator needs session-scoped events; set `sessionSHA` on the agent struct.
3. **Observability** — Record token usage and latency via `otel`; publish notable steps to `events` when the orchestrator expects UI updates.
4. **Errors** — Return errors to feature layer; do not log-and-nil unless existing code already does for a specific recoverable case.

## Do not

- Import `internal/feature/`.
- Call langchaingo LLM constructors from `internal/feature/orchestrator` — always go through `Factory`.
