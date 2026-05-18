# ADR 003: Git Worktrees for Delivery Isolation

## Status

Accepted

## Context

Parallel implementation agents must not conflict on the filesystem.

## Decision

Each delivery implementation task gets a dedicated Git worktree at `worktrees/delivery-<session>/task-<id>/`.

## Consequences

- Safe parallel delivery
- Worktree lifecycle tracked in DB and SSE events
