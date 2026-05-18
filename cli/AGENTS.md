# CLI (`krk`)

Overrides parent rules for `cli/` only.

## Role

The CLI is an **HTTP client**. It must not contain orchestration, workflow planning, agent logic, or direct database/git/LLM access.

## Structure

- `cli/client/` — shared `Client` with auth header, JSON encode/decode, base URL from env/config.
- `cli/command/` — Cobra commands; each command maps to one or more API calls.

## Conventions

- Parse flags → build request DTO → call `client.Client` → print JSON or human-readable summary to stdout.
- Errors: print a clear message to stderr; exit non-zero. Do not swallow API error bodies.
- Long-running work: poll `GET /sessions/:sha` or consume SSE if the command already supports streaming; do not reimplement server-side state machines in the CLI.
- New commands need a subcommand under `cli/command/root.go` and a matching route documented in `docs/openapi.yaml`.

## Do not

- Import `internal/feature/`, `internal/platform/`, or `internal/core/` (except nothing from internal — CLI stays outside `internal/`).
- Add business validation that belongs on the server (mode transitions, artifact promotion rules, etc.).
