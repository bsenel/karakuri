# HTTP API (`internal/api`)

Overrides parent rules for `internal/api/` only.

## Role

Transport layer: Chi routing, handlers, middleware, SSE. **Translate HTTP ↔ feature services**; no workflow or agent logic here.

## Structure

- `server.go` — Router, middleware chain, handler registration, `App` struct holding services.
- `handler/` — One file per resource area (session, artifact, events, promote, etc.).
- `middleware/` — Auth, logging, request IDs.

## Handler conventions

```go
// Decode → call feature service → writeJSON or http.Error
func (h *XHandler) Action(w http.ResponseWriter, r *http.Request) {
    // chi.URLParam for :sha, :id
    // 400 on bad JSON, 404 on missing entity, 500 only for unexpected errors
}
```

- Use `entity` types in responses where they match OpenAPI schemas.
- Long operations: return 202 + session reference, or stream via `handler/events` SSE — do not block the handler for full agent runs unless already established.
- Run orchestration in a goroutine only when the existing session `Run` handler pattern does; preserve event ordering guarantees.

## Do not

- Import `internal/platform/` except through services already injected into `App`.
- Add SQL, LLM calls, or workflow YAML parsing in handlers.
- Change route paths without updating `docs/openapi.yaml` and CLI client paths.

## Auth

- Respect `middleware` auth for protected routes; health endpoints stay unauthenticated unless config says otherwise.
