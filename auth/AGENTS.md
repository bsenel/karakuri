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

## Federated identity

The protocols live in submodules (`auth/oidc`, `auth/saml`) because they need dependencies
and rule 2 says the core cannot have any. What lives here is everything both share:
`ExternalIdentity`, `RoleMap`, `Provisioner` and `ChainResolver`. A new protocol should
reduce to producing an `ExternalIdentity` and nothing else.

- **Federated users become real principals.** Authorization reads role bindings from the
  `Store`, so an identity that exists only as claims holds none and is denied everything.
  `Provisioner` writes it in on the way through. Do not add a second source of roles to the
  authorizer to avoid this — one place decides what a principal may do.
- **Principal IDs are namespaced** (`oidc:<sub>`), enforced in `ValidatePrefix`, which is
  the single enforcement point. Without it a provider asserting `sub=admin` takes over the
  local bootstrap administrator. Never derive a principal ID from a provider's subject
  without going through `ExternalIdentity.PrincipalID`.
- **Reconciliation only touches bindings under `ManagedBindingPrefix`.** A grant an
  administrator made by hand is not the provider's to revoke, and the binding ID carries
  that provenance so no column has to.
- **Matching no group grants nothing.** `RoleMap.Default` stays empty unless an operator
  fills it in: everyone in a corporate directory can authenticate, so a default role is a
  grant to the whole company.
- **A local credential path always remains.** `ChainResolver` puts the local `JWTResolver`
  ahead of the provider's, so the bootstrap administrator can still log in when the IdP is
  unreachable. That is the break-glass path — deliberately not a second static token, which
  is what Phase 14 removed.
- **`Sealer` carries flow state through the browser**, signed rather than encrypted: a
  state token, a nonce, a PKCE verifier or a SAML request ID is not a secret from the
  browser holding it, and what matters is that a browser cannot mint its own. It lives here
  rather than in either protocol module because both need it, and the crypto is worth
  reviewing once. Its key is required, never generated — a generated key lives in one
  process, so behind a load balancer a flow started on one replica and finished on another
  fails intermittently.

## Testing

`go test ./... -race` from this directory. Coverage is gated at 95% in CI
(`.github/workflows/ci.yml`, job `auth-modules`) — the module is a security boundary, so
untested branches are treated as defects rather than debt.
