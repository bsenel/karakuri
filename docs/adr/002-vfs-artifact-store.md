# ADR 002: VFS Artifact Store

## Status

Accepted

## Context

Agents produce outputs (design docs, code, research reports, review comments) that must be passed between loop steps and loop runs without coupling agents to each other's internals. Outputs must be auditable and diffable.

## Decision

All agent outputs flow through a content-addressed blob store (`internal/feature/artifact/`). Each blob is SHA-256 addressed, stored in SQLite, and associated with an objective and agent ID. Agents write artifacts via `ArtifactService.Write()` and read them via `ArtifactService.Get(sha)`.

## Consequences

- Full immutable audit trail of every artifact produced by every loop run
- Diffs between two artifact versions available via `GET /artifacts/:sha/diff/:other`
- No in-process memory sharing between loop steps; all state flows through the store or memory tiers
- `krk artifact list --objective <id>` provides full provenance for any objective
