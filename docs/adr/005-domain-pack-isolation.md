# ADR 005: Domain Pack Isolation

## Status

Accepted

## Context

Karakuri must support multiple unrelated fields — software development, agriculture, healthcare, legal — without the core engine knowing anything about any of them. Domain knowledge must be addable without modifying the engine, and a badly-authored pack must not crash the server or corrupt other domains.

## Decision

All domain-specific knowledge is encapsulated in a `domain.Pack` implementation registered at startup via `DomainRegistry.Register()`. The interface (`internal/core/domain/`) declares five read-only accessors (Capabilities, EnvironmentFactories, AgentDefinitions, ObjectiveTemplates, PlannerHints) plus lifecycle methods (Init, Teardown).

**Import discipline** (enforced by golangci-lint `depguard`):
- `internal/core/` and `internal/feature/` must not import any `domains/` package
- `internal/platform/` must not import any `domains/` package
- Only `cmd/` and `internal/app/` import domain packs

**Conformance suite** (`internal/conformance/`) validates every registered pack at startup and on demand via `GET /domains/:id/conformance`. Seven checks cover ID format, schema validity, factory safety, cross-reference integrity, ID uniqueness, and teardown safety.

Domain capabilities are namespaced by convention (`<domain>.<step>.<name>`) and registered into a shared `CapabilityRegistry`. The engine dispatches actions by ID, never by domain-specific logic.

## Consequences

- Adding a new domain requires zero changes to `internal/core/`, `internal/feature/`, `internal/platform/`, or `internal/api/`
- Domain packs are validated before the server accepts traffic; malformed packs are logged and skipped
- `krk domain test <id>` gives pack authors immediate pass/fail feedback on all 7 checks
- Cross-domain objectives are not supported in v1; objectives are scoped to a single domain
- The agriculture pack ships as a fully conformant v1 reference implementation; other stubs pass registration but return empty capability/env/agent lists
