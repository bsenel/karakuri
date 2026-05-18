# Workflows (`workflows/`)

Overrides parent rules for workflow YAML only.

## Role

Declarative hints for the orchestrator (`strategy`, `discovery`, `delivery`, `autonomous`). Go code loads and interprets these files; **business rules stay in `internal/feature/orchestrator`**, not in YAML alone.

## Files

| File | Mode |
|------|------|
| `strategy.yaml` | Strategy pipeline roles and steps |
| `discovery.yaml` | Discovery pipeline |
| `delivery.yaml` | Delivery / implementation pipeline |
| `autonomous.yaml` | Autonomous validation loop |

See [workflows/README.md](README.md) for field semantics.

## Editing conventions

- Prefer **additive** changes (new optional keys) over renaming required fields until orchestrator supports both.
- Role names and task kinds must match what `Planner` and `Scheduler` expect — grep orchestrator code before renaming.
- Keep prompts and role descriptions in YAML; avoid duplicating the same prompt string in Go.
- No secrets in YAML; use config/env for API keys and endpoints.

## When to change Go instead

- New session mode or state machine edge → `entity` + orchestrator + this YAML together.
- Conditional branching that YAML cannot express cleanly → planner logic in Go, minimal new YAML keys.
