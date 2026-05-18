# ADR 001: Thin CLI, Thick Server

## Status

Accepted

## Context

Karakuri must support a future React frontend without backend structural changes.

## Decision

All orchestration logic lives in the API server. The `krk` CLI is an HTTP client only.

## Consequences

- Single source of truth for business rules
- CLI and future UI share identical API and SSE contracts
