# ADR 009 — Federated identity maps to local principals

**Status:** Accepted
**Date:** 2026-08-12
**Supersedes / relates to:** [ADR 007](007-standalone-auth-module.md) (standalone `auth` module)

## Context

Phase 14 gave Karakuri a complete authorization model: principals, roles, policies,
and role bindings that say who may do what to which resource. Every principal in it
is created by hand — `krk auth users add`, or the bootstrap administrator on first
boot.

That does not survive contact with an organisation. Enterprises already keep their
users, their groups and their joiner/leaver process in Keycloak, Okta, Auth0, Azure AD
or ADFS, and will not maintain a second copy in Karakuri. Phase 16 lets those identity
providers authenticate, and lets group membership drive Karakuri roles.

The question this ADR settles is what an identity provider's user *is*, once it is
inside Karakuri.

## The decision

**A federated user becomes an ordinary local principal, written to the store on the
way in.** Authentication resolves an assertion into an `ExternalIdentity`; a
`Provisioner` upserts the principal it names and reconciles its role bindings from the
mapped groups. Everything after that point is unchanged.

### Why not carry roles in the token

The obvious alternative — and what the roadmap originally described — is to put roles
on the `Principal` and let the authorizer read them from the credential.

`StoreAuthorizer.Authorize` resolves permissions by listing a principal's
`RoleBinding`s from the `Store`. A principal assembled purely from claims holds none,
and is denied everything with `principal %q holds no role bindings`. Making that work
would mean teaching the authorizer a second source of truth, and a second source of
truth in an authorizer is where deny-wins precedence stops being total: a deny
expressed as a binding and an allow arriving in a token have no defined ordering
between them.

Provisioning instead means there is exactly one place that decides what a principal
may do. Ownership conditions, quota keys, audit records, `krk auth users list` and the
`/auth/me` permission list all keep working with no code changes at all, because a
federated principal is not a special kind of principal.

### What it costs

A store write per login — but only when something differs. `Provision` compares the
principal and its managed bindings before writing, and the overwhelmingly common
login, where nothing about the user changed, performs reads only.

Reconciliation is also the mechanism for revocation, and it is *lazy*: a user removed
from a group at the identity provider keeps the corresponding binding until their next
login. For faster revocation, disable the principal — that is checked on every request
and outranks the provider still authenticating them.

## Two rules the implementation exists to enforce

**Principal IDs are namespaced** — `oidc:<sub>`, `saml:<nameID>`. Local principals are
named by an administrator; federated ones are named by whoever controls the provider's
subject field, which in some deployments is the user. Without the prefix, a provider
asserting `sub=admin` would resolve to the local bootstrap administrator.
`ValidatePrefix` is the single enforcement point, and the reserved `idp:` namespace
that managed bindings live under cannot be used as one.

**Matching no group grants nothing.** `RoleMap.Default` is empty unless an operator
fills it in. Everybody in a corporate directory can authenticate against a corporate
identity provider, so a default role is a grant to the whole company. A user who
matched no group logs in successfully and can do nothing, which is the correct shape:
authentication is not authorization.

## Consequences

- **Two protocol submodules**, `auth/oidc` and `auth/saml`, each with its own `go.mod`,
  for the reason ADR 007 gave: the core module's require block stays empty, and a
  consumer who wants neither protocol pulls neither dependency.
- **Only OIDC contributes a `TokenResolver`.** A SAML assertion is a one-time login
  artifact delivered by browser POST — single-use, bound to one recipient, valid for
  minutes. It is not a credential a client can present per request, and building a
  resolver on one would mean accepting replayed assertions.
- **Local password login stays mounted alongside any provider.** It is the break-glass
  path when the identity provider is unreachable. This is deliberately *not* a second
  static token: Phase 14 removed the static bearer token, and re-adding a long-lived
  credential to survive an outage trades a permanent risk for a temporary one.
- **Hand-made grants survive.** Reconciliation only touches bindings carrying the
  reserved `idp:` prefix, so `krk auth bindings add` is not undone by somebody logging
  in. The binding ID carries that provenance, which is why `RoleBinding` gained no
  field and `auth/sql` needed no migration.
- **A role map naming an unregistered role fails at boot**, not at the login it would
  have matched.

## Alternatives considered

**Claims-only principals, no store write.** Rejected above: it splits the authorizer's
source of truth, and it makes federated users invisible to every read path that lists
principals.

**Mirroring the directory on a schedule.** A background sync would revoke faster than
lazy reconciliation, but it needs directory-read credentials, a scheduler, and an
answer for providers with no group-enumeration API. Reconciling what the user just
proved about themselves needs none of that. Disabling a principal covers the urgent
case.

**A static break-glass token, per the original roadmap step.** Rejected: it
re-introduces exactly what Phase 14 deleted. Keeping password login mounted serves the
same purpose with a credential that already exists, already rotates, and is already
audited.
