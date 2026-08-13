# ADR-014 — Uniform API error envelope with correlation IDs

**Status:** Proposed (audit findings E-03, E-06 / SECURITY_AUDIT F-09) · **Date:** 2026-08-13

## Context

Two related gaps degrade both security and operability:

1. **Internal errors leak to clients.** 71 handler sites do `http.Error(w, err.Error(), …)`
   (`internal/api/handler/`), returning raw GORM/storage text — including a refresh-state
   oracle at `handler/auth.go:149`. This couples clients to internal error strings and
   makes a stable error contract impossible.
2. **No correlation ID.** `internal/api/middleware/logging.go` logs method/path/duration but
   stamps **no request ID**, and none propagates into downstream `slog` records or OTel
   spans. A single request cannot be followed across log lines, despite six observability
   exporters being wired.

## Decision

Introduce a request-scoped correlation ID and a uniform error envelope:

- A **request-ID middleware** (chi ships `middleware.RequestID`) generates or accepts an
  `X-Request-Id`, stores it in the context, adds it to every `slog` record and the OTel
  span, and echoes it on the response.
- A single **`writeError(w, r, status, publicMessage)`** helper replaces the ad-hoc
  `http.Error(w, err.Error(), …)` calls. It:
  - returns a stable JSON envelope `{"error": "<code>", "message": "<safe>", "request_id": "<id>"}`;
  - logs the full internal error against the request ID at the appropriate level;
  - never puts internal error text in the client-facing `message`.

The existing deliberately-opaque responses (login, SSO — `handler/auth.go:99-111`,
`handler/sso.go:153-159`) already follow this shape and become the template.

## Consequences

- **+** No internal-detail disclosure; the refresh-state oracle closes (F-09).
- **+** Every client error carries a `request_id` an operator can grep across all sinks.
- **+** A stable, documented error contract (feeds the OpenAPI work, E-04).
- **−** A mechanical sweep of ~71 call sites (low risk, high count) — best done as one
  focused, test-backed refactor commit.

## References
[CWE-209](https://cwe.mitre.org/data/definitions/209.html) · [Error Handling Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Error_Handling_Cheat_Sheet.html) · [OpenTelemetry](https://opentelemetry.io/docs/) · [chi middleware](https://github.com/go-chi/chi)
