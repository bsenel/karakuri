# Domain Pack Authoring Guide

A domain pack encapsulates all knowledge for a specific field — software, agriculture, healthcare, etc. — as a self-contained Go package that registers with the Karakuri engine at startup.

The engine never imports domain knowledge directly. Every domain-specific capability, environment, agent, and objective template is expressed through the four core primitives and registered via the `domain.Pack` interface.

## File Structure

```
domains/<your-domain>/
├── pack.go           → implements domain.Pack; delegates to sibling files
├── capabilities.go   → []capability.Capability definitions
├── environments.go   → []environment.Factory + no-op env implementation
├── agents.go         → []agent.Definition
├── objectives.go     → []objective.Template
└── hints.go          → []domain.PlannerHint
```

## 1. pack.go

```go
package yourdomain

import (
    "context"
    "github.com/bsenel/karakuri/internal/core/agent"
    "github.com/bsenel/karakuri/internal/core/capability"
    "github.com/bsenel/karakuri/internal/core/domain"
    "github.com/bsenel/karakuri/internal/core/environment"
    "github.com/bsenel/karakuri/internal/core/objective"
)

type Pack struct{}

func New() *Pack { return &Pack{} }

func (p *Pack) ID() string          { return "yourdomain" }   // lowercase, no spaces
func (p *Pack) Name() string        { return "Your Domain" }
func (p *Pack) Version() string     { return "1.0.0" }
func (p *Pack) Description() string { return "..." }

func (p *Pack) Init(_ context.Context, _ domain.Config) error { return nil }
func (p *Pack) Teardown(_ context.Context) error              { return nil }

func (p *Pack) Capabilities() []capability.Capability        { return yourCapabilities() }
func (p *Pack) EnvironmentFactories() []environment.Factory  { return yourEnvironmentFactories() }
func (p *Pack) AgentDefinitions() []agent.Definition         { return yourAgentDefinitions() }
func (p *Pack) ObjectiveTemplates() []objective.Template     { return yourObjectiveTemplates() }
func (p *Pack) PlannerHints() []domain.PlannerHint           { return yourPlannerHints() }
```

## 2. capabilities.go

Each capability must have a unique prefixed ID (`<domain>.<step>.<name>`), valid JSON Schema input and output, and an optional `LLMHints` struct.

```go
// Example from agriculture pack:
{
    ID:          "agriculture.observe.soil_conditions",
    Name:        "Observe Soil Conditions",
    Domain:      "agriculture",
    Description: "Observe soil moisture, pH, and nutrient levels from field sensors",
    InputSchema: capability.Schema{
        Type: "object",
        Properties: map[string]capability.SchemaProperty{
            "field_id": {Type: "string", Description: "Unique field identifier"},
            "depth_cm": {Type: "number", Description: "Sensor depth in centimetres"},
        },
        Required: []string{"field_id"},
    },
    OutputSchema: capability.Schema{
        Type: "object",
        Properties: map[string]capability.SchemaProperty{
            "moisture_pct": {Type: "number", Description: "Volumetric water content %"},
            "ph":           {Type: "number", Description: "Soil pH value"},
        },
    },
},
```

**Rules:**
- `ID` must be unique across all registered packs
- Both `InputSchema.Type` and `OutputSchema.Type` must be non-empty (conformance check)
- Use the convention `<domain>.<observe|reason|decide|act|verify|learn>.<name>`

## 3. environments.go

Environments are built by factories. Ship a no-op default so the pack registers without real infrastructure. The `Build` closure receives an `environment.BuildContext` carrying the twin context (twin ID and `AdapterBindings`) for the current loop run — packs that integrate with tool adapters use this to resolve the right instance at construction time (see "Packs that integrate with tool adapters" below).

```go
func yourEnvironmentFactories() []environment.Factory {
    return []environment.Factory{
        {
            EnvID:       "yourdomain.env.field",
            Domain:      "yourdomain",
            Description: "Field sensor network",
            Build: func(_ environment.BuildContext) (environment.Environment, error) {
                return &noopEnv{id: "yourdomain.env.field"}, nil
            },
        },
    }
}

type noopEnv struct{ id environment.EnvironmentID }

func (e *noopEnv) ID() environment.EnvironmentID { return e.id }
func (e *noopEnv) Domain() string                { return "yourdomain" }
func (e *noopEnv) Observe(_ context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
    return environment.Observation{EnvID: e.id, Data: map[string]any{"status": "noop"}}, nil
}
func (e *noopEnv) Act(_ context.Context, _ environment.Action) (environment.ActionResult, error) {
    return environment.ActionResult{Success: true, StateDelta: map[string]any{"note": "noop"}}, nil
}
func (e *noopEnv) Subscribe(_ context.Context, _ environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
    return nil, nil
}
func (e *noopEnv) Snapshot(_ context.Context) (environment.EnvironmentSnapshot, error) {
    return environment.EnvironmentSnapshot{SHA: "noop", EnvID: e.id}, nil
}
```

### Packs that integrate with tool adapters

Packs whose environments delegate to platform-level tool adapters (GitHub, Slack, Gmail, …) accept a `*tools.Registry` via a second constructor and capture the resolved adapter at `Build` time using `ctx.AdapterBindings`. The software pack is the reference:

```go
// pack.go: two constructors — one for tests/conformance, one for prod wiring
func New() *Pack { return &Pack{} }
func NewWithTools(reg *tools.Registry) *Pack { return &Pack{tools: reg} }

// environments.go: resolve the bound instance, fall back to no-op if absent
Build: func(ctx environment.BuildContext) (environment.Environment, error) {
    var vc versioncontrol.VersionControlAdapter = versioncontrol.NewNoOp()
    if reg != nil {
        if a, ok := reg.VC.Resolve(ctx.AdapterBindings["versioncontrol"]); ok {
            vc = a
        }
    }
    return &gitEnv{id: "yourdomain.env.git", vc: vc}, nil
},
```

The env struct holds the resolved adapter directly — `Act()` is a direct call with no per-action lookup. Empty bindings or unknown instance names fall through to the slot's default; missing default → no-op. See ADR 006 for the full design.

## 4. agents.go

Agent definitions list the capabilities an agent can invoke and define its authority bounds.

```go
{
    ID:     "agriculture.agent.field_manager",
    Name:   "Field Manager",
    Domain: "agriculture",
    Capabilities: []capability.CapabilityID{
        "agriculture.observe.soil_conditions",
        "agriculture.act.irrigate",
    },
    ReasoningStrategy: agent.ReasoningReAct,
    Authority: agent.AuthorityBounds{
        MaxAutonomousActions: 10,
        ConfidenceThreshold:  0.75,
        RequiresApprovalFor:  []capability.CapabilityID{"agriculture.act.apply_treatment"},
    },
    LLMHints: capability.LLMHints{PreferredProvider: "claude", TemperatureMax: 0.4},
},
```

**Rules:**
- All capability IDs in `Capabilities` must appear in `p.Capabilities()` (conformance check 4)

## 5. objectives.go

Templates define the criteria and constraints for an objective type.

```go
{
    ID:          "agriculture.objective.optimize_yield",
    Title:       "Optimize Crop Yield",
    Domain:      "agriculture",
    Description: "Observe conditions, forecast yield, apply treatments, verify target",
    SuccessCriteria: []objective.Criterion{
        {
            ID:          "yield-target",
            Description: "Forecasted yield meets target",
            Verifier:    "agriculture.verify.yield_target",  // must be a registered capability
            Weight:      1.0,
        },
    },
},
```

**Rules:**
- `Criterion.Verifier` must be a capability ID registered in the pack (conformance check 5)

## 6. hints.go

Planner hints guide the loop's action ordering. They are advisory — the agent may override them.

```go
{
    Condition: "capability.id startswith 'agriculture.act'",
    Guidance:  "Always observe soil conditions before executing any act capability",
    Priority:  10,
},
```

## Registering the Pack

In `cmd/server/main.go` (via `internal/app/bootstrap.go`), add:

```go
import yourdomain "github.com/bsenel/karakuri/domains/yourdomain"

// In the packs slice:
yourdomain.New(),
```

The bootstrap function calls `DomainRegistry.Register()`, which calls `Init()` then registers all capabilities, environment factories, and objective templates.

## Testing with the Conformance Suite

```bash
krk domain test <domain-id>
```

This runs 7 checks server-side and returns pass/fail per check:

| Check | What it verifies |
|-------|-----------------|
| `id_format` | Pack ID is non-empty, lowercase, no whitespace |
| `capability_schemas` | All capabilities have non-empty InputSchema.Type and OutputSchema.Type |
| `environment_factories` | All factories Build a non-nil environment without error when called with a zero-value `BuildContext` |
| `agent_capability_refs` | All capability IDs referenced by agents exist in the pack |
| `criterion_verifier_refs` | All Criterion.Verifier IDs exist in the pack |
| `no_capability_id_collision` | No two capabilities share the same ID |
| `teardown_no_panic` | Teardown() completes without panicking |

All 7 checks must pass before a pack is considered conformant.

## Observing Karakuri itself

A pack may ask to see this deployment's own behaviour — how often it escalates,
what it spends, which objectives keep failing, what nobody has decided — by
reading `BuildContext.Telemetry` in its environment factory (Phase 22,
[ADR 017](adr/017-karakuri-as-a-domain-pack.md)).

```go
Build: func(bc environment.BuildContext) (environment.Environment, error) {
    // Nil unless the deployment wired one. Report that rather than
    // returning zeroes, which read as a perfectly healthy system.
    return &myEnv{telemetry: bc.Telemetry}, nil
},
```

The port is read-only and stays that way. A pack that could write there could
rewrite the evidence of what it did, and the value of Karakuri watching itself
depends on the watching being trustworthy.

Two rules follow, and `domains/karakuri` is the worked example of both:

- **An environment that observes should refuse to act, out loud.** Return
  `ActionResult{Success: false, Error: "..."}` rather than succeeding quietly.
  A silent success is the failure mode Phase 13.5 spent a dogfood run finding.
- **Keep deciding and doing in separate packs.** The karakuri pack analyses and
  drafts; the software pack writes, in a worktree, through a pull request
  somebody reviews. A pack that could both conclude something about its own
  authority and act on it is one bug away from widening its own bounds.

### Cross-pack criteria

A template may be verified by another pack's capability — that is what
cross-domain objectives are for — by declaring the domain on the criterion:

```go
objective.Criterion{
    ID:       "pull-request",
    Verifier: "software.act.open_pull_request",
    Domain:   "software", // required; without it this is a dangling reference
    Weight:   0.4,
}
```

The conformance suite accepts a foreign verifier only when `Domain` says so, so
a typo in a local one still fails. It does not check that the named pack is
enabled: a pack is validated on its own, and which others are configured is the
registry's business at boot.

### Fingerprints should be lossy on purpose

`Snapshot().SHA` is what the reconcile supervisor uses to decide whether
anything changed. It should answer *"has anything changed that would change
what I do"*, not *"has anything changed"*.

`karakuri.env.telemetry` buckets its counts by order of magnitude and bands its
rates, so an ordinary busy week does not move the hash while a new bottleneck
or a decision queue growing tenfold does. An environment that hashed raw
counters would wake a standing objective continuously to discover that work had
occurred.
