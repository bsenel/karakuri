# `auth` — standalone authorization module

Overrides parent rules for `auth/` only. This directory is a **separate Go module**
(`github.com/bsenel/karakuri/auth`), not part of the Karakuri module. See
[ADR 007](../docs/adr/007-standalone-auth-module.md).

## Hard rules

1. **No Karakuri imports.** Nothing under `github.com/bsenel/karakuri/internal`, `/domains`,
   `/config` or `/cli` may be imported here. The module has to build for a consumer who has
   never heard of Karakuri.
2. **No external dependencies.** `auth/go.mod` has an empty require block and stays that
   way — including for JWT, which is implemented over `crypto/hmac` and `crypto/ed25519`.
   A dependency that only one submodule needs belongs in that submodule's own `go.mod`
   (as `auth/sql` does for its test-only SQLite driver).
3. **No application vocabulary.** This module does not know what a twin, an objective or a
   loop is. Action names arrive through `Catalog.Register` at startup; resource types are
   opaque strings. If a change would require naming a Karakuri concept here, it belongs in
   `internal/auth/` instead.

## Conventions

- **Stores return deep copies.** The authorizer reads concurrently and must never be able
  to mutate stored state through a returned value. Every `Get`/`List` clones.
- **Patterns use one grammar** — exact, `<prefix>:*`, or bare `*` — matched by
  `matchPattern` for actions, resources and binding scopes alike. Do not add a second
  matching rule; extend that one.
- **Conditions are a closed set.** Adding a `ConditionKind` means adding a case to
  `Validate` and to `Evaluate`, and `Evaluate` must stay total — an unresolvable key is an
  unsatisfied condition with a `Detail`, never an error.
- **Decisions explain themselves.** Any new denial path sets `Decision.Reason` to something
  an operator can act on. "forbidden" is not a reason.
- **Fail closed.** An authorizer that cannot answer returns an error, and the middleware
  turns that into a 500. Never fall through to the handler.

## Testing

`go test ./... -race` from this directory. Coverage is gated at 95% in CI
(`.github/workflows/ci.yml`, job `auth-modules`) — the module is a security boundary, so
untested branches are treated as defects rather than debt.
