# ADR 003: Git Worktrees for Code-Writing Isolation

## Status

Accepted

## Context

When the loop's act step executes `software.act.write_code` or `software.act.write_test` capabilities, multiple iterations or parallel objective runs must not conflict on the filesystem or in the git index.

## Decision

Each code-writing action gets a dedicated Git worktree created by `WorktreeManager.Create()`. The worktree path follows the convention `worktrees/<objective-id>/<task-id>/` and the branch is `karakuri/<objective-id>/<task-id>`. Worktrees are tracked in the `worktrees` table and pruned after PR creation or on objective failure.

The act step detects code-writing capabilities by suffix (`.write_code`, `.write_test`) and injects `worktree_path` and `branch` into the action params before dispatching to the environment.

## Consequences

- Safe parallel act steps: each action writes to an isolated working tree
- `worktree_created` SSE event fired on each allocation for real-time visibility
- Worktree lifecycle (create → PR → prune) fully tracked in DB and observable via `GET /health`
- `WorktreeManager` is the sole path to worktree creation; no direct filesystem writes from agents
