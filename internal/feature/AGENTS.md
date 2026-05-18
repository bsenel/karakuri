# Use cases (`internal/feature`)

Overrides parent rules for `internal/feature/` only.

## Role

Application services: orchestrate **core** types and **platform** ports to fulfill workflows (strategy → discovery → delivery → autonomous).

## Packages

| Package | Responsibility |
|---------|----------------|
| `orchestrator` | Load workflow YAML, plan tasks, schedule agents, session run lifecycle |
| `strategy` | Strategy-mode sessions and artifacts |
| `discovery` | Discovery-mode analysis |
| `delivery` | Delivery-mode implementation and review |
| `autonomous` | Autonomous validation loops |
| `session` | Session CRUD and lineage |
| `artifact` | Artifact metadata and resolution |
| `checkpoint` | Human-in-the-loop checkpoints |
| `research` | Research artifacts and promotion |

## Rules

1. **Depend on ports** — Use `storage.StorageAdapter`, `core/agent`, `executor.Executor`, etc.; do not import langchaingo or GORM models directly (use platform storage APIs).
2. **Constructor injection** — `NewService(...)` with explicit dependencies; wire in `internal/app/bootstrap.go`.
3. **Context** — Pass `context.Context` as first parameter; respect cancellation on long runs.
4. **Events** — Publish session/task progress via `event.Hub` for SSE consumers.
5. **State transitions** — Session `entity.State*` updates go through storage; keep transitions explicit in one place per mode.

## Orchestrator specifics

- Workflow definitions live in `workflows/*.yaml`; planner reads hints, does not hardcode role lists in Go when YAML can express them.
- Delivery tasks that need isolated repos use `git.WorktreeManager` from platform, not raw `os/exec git` in feature code.

## Tests

- Table-driven unit tests for planners, schedulers, and pure helpers.
- Integration tests belong in `test/integration/`, not buried in feature packages unless small and fast.
