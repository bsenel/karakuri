# ADR 001: Thin CLI, Thick Server

## Status

Accepted

## Context

Karakuri must support a future React frontend without backend structural changes. The `krk` CLI must be distributable as a single binary that works against any server deployment.

## Decision

All business logic lives in the API server. The `krk` CLI is a pure HTTP client — it issues requests and renders responses, but contains no domain logic, no state, and no direct database or LLM access.

## Consequences

- Single source of truth for business rules; CLI and future UI share identical REST + SSE contracts
- CLI binary has no dependency on LangChain Go, GORM, or go-git
- All endpoints return structured JSON; `krk` renders as pretty-printed JSON or table by default
- SSE endpoints (`/objectives/:id/events`, `/twins/:id/events`) are consumed by `krk auto` for streaming output
