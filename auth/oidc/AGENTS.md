# `auth/oidc` — OpenID Connect authentication

Overrides parent rules for `auth/oidc/` only. This directory is a **separate Go module**
(`github.com/bsenel/karakuri/auth/oidc`), a submodule of `auth` in the sense of ADR 007 —
its own `go.mod` so a consumer of `auth` does not pull `go-oidc` and `oauth2` unless they
want OIDC.

## Hard rules

1. **No Karakuri imports**, same as the parent. The only dependency on this repo is
   `github.com/bsenel/karakuri/auth`.
2. **Nothing here decides what anybody may do.** This module establishes *who* someone is
   and hands the result to `auth.Provisioner`. Roles come from the host's `RoleMap`. If a
   change would require knowing what a role means, it belongs in the host.
3. **No protocol reimplementation.** Signature verification, JWKS caching and discovery are
   `go-oidc`'s job; the code exchange and PKCE are `oauth2`'s. This package is the glue and
   the claim mapping, and should stay small enough to read in one sitting.

## Conventions

- **Claim locations are configuration, not constants.** There is no standard OIDC claim for
  groups: Keycloak nests them under `realm_access.roles`, Okta and Auth0 use `groups`,
  Azure AD emits object IDs. `auth.ClaimPath` addresses them; do not add a provider-specific
  branch, add a default.
- **The flow cookie is signed, not encrypted, and single-use.** State, nonce and the PKCE
  verifier belong to the browser holding them, so secrecy from that browser buys nothing —
  what matters is that a browser cannot mint its own. It is cleared on the way into the
  callback whatever the outcome, because it carries a live verifier.
- **`SameSite=Lax`, deliberately.** Strict withholds the cookie on the provider's redirect
  back, which is the one request that needs it.
- **The state key is required, never generated.** A generated key lives in one process, so
  behind a load balancer a login started on one replica and returned to another fails —
  intermittently, which is the worst way to find out.
- **A missing credential returns `auth.ErrNoCredential`.** Anything else and an
  `auth.ChainResolver` would stop here rather than trying the local resolver, which is what
  keeps password login working when the provider is down.

## Testing

`go test ./... -race` from this directory. Coverage is gated at 90% in CI
(`.github/workflows/ci.yml`, job `modules`).

Tests run against an in-process provider (`stub_test.go`) that wraps `go-oidc`'s `oidctest`
with the `/auth` and `/token` endpoints it does not ship, so the authorization-code flow is
driven end to end with no Docker and key rotation is forced rather than waited for. A real
Keycloak covers what an actual provider does differently; that lives in the repo's
integration suite, not here.

## Releasing

`.github/workflows/release-auth.yml` already matches `auth/*/v*.*.*` and derives the module
directory from the tag, so `auth/oidc/v0.1.0` needs no workflow of its own. Drop the
`replace` directive before tagging — the workflow refuses to publish a module carrying one.
