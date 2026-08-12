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
and rule 2 says the core cannot have any. See
[ADR 009](../docs/adr/009-federated-identity-jit-provisioning.md) for why a federated user
becomes a local principal rather than carrying its roles in a token.

What lives here is everything both protocols share: `ExternalIdentity`, `RoleMap`,
`Provisioner`, `ChainResolver` and `Sealer`. A new protocol should reduce to producing an
`ExternalIdentity` and nothing else.

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
- **A mapped grant carries a scope.** `RoleGrant` is a role *and* a scope, and reconciliation
  keys on the pair, so somebody in two teams holds two bindings and losing one group takes
  one away. Without the scope every federated user lands over everything, and a directory
  group of two hundred people is two hundred globally-scoped principals. An unset scope still
  means `*`, which is what a binding with no scope has always meant.
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

## Containers and multi-tenancy

Hierarchy is expressed as a **set of labels on the resource**, not a path in its name.
`ResourceRef.Scopes` carries the ancestor closure (`["team:t_7f2a", "org:o_9c31"]`) and a
binding covers the resource if its scope matches the resource *or any label*. See
[ADR 010](../docs/adr/010-scope-sets.md) for why, and what it gives up.

- **Do not add path syntax.** `matchPattern` is untouched by containers and must stay that
  way — it is shared by actions, resources and scopes alike, so a `/` rule would change how
  permissions match. A label is an ordinary `<type>:<id>` pattern, which the grammar already
  accepts.
- **Labels carry IDs, never display names.** Two organisations may both have a team called
  "Engineering"; if the label were the name, a grant on one would silently cover the other.
  Build them with `ScopeLabel`, and keep names for the interface.
- **The closure is materialised by the caller**, because the caller owns the tree. This
  package never walks, recurses, or bounds depth during a check — that is what makes nesting
  free at request time.
- **A set, not a path, is the point.** A resource can be multi-homed — its team, its org, and
  a project spanning two organisations — which is what lets cross-tenant collaboration work
  without a second construct grafted alongside the hierarchy.
- **Authorization is a boolean; listing is a query.** `GrantedScopes` hands back the scopes a
  principal holds so a caller can build a `WHERE`, and keeps allow and deny apart because
  resolving them needs the tree. Conditional denies cannot appear there, so a filtered list
  is a narrowing and the per-resource check stays authoritative.

## Testing

`go test ./... -race` from this directory. Coverage is gated at 95% in CI
(`.github/workflows/ci.yml`, job `modules`) — the module is a security boundary, so
untested branches are treated as defects rather than debt.
