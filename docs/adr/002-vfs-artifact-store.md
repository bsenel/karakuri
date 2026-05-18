# ADR 002: VFS Artifact Store

## Status

Accepted

## Context

Agents must not share in-process memory across roles.

## Decision

All cross-agent context flows through content-addressed blobs in SQLite via `StorageAdapter`.

## Consequences

- Full audit trail of artifacts
- Promotion flows compose via manifest nesting
