# ADR 007: Authorization as a Standalone, Dependency-Free Module

## Status

Accepted

## Context

Through Phase 13.5 the entire Karakuri API sat behind a single shared secret — `middleware.BearerAuth(cfg.Auth.Token)`. Anyone holding that token could start loops, resolve checkpoints, and read the authority-bounds audit log. The token never expired, was `sed`-ed into the runtime config by `docker-entrypoint.sh`, and could not distinguish an operator from an auditor.

Phase 14 replaces it with role-based access control. Two structural questions had to be settled before writing any of it:

1. **Where does the authorization engine live?** Karakuri is not the only Go service that needs "roles, policies, and a `RequirePermission` middleware". Phases 16–18 also plan to extend it (OIDC/SAML resolvers, hierarchical org paths, quota admin permissions), which only works if the extension points are a published, versioned surface rather than an internal package.

2. **How granular is the model?** Flat `role → permission` RBAC cannot express "Alice may edit the twins she owns" or "operators may do anything except edit the auth model" without pushing those rules into handler bodies, where they cannot be audited or listed.

## Decision

1. **The engine ships as its own Go module**, `github.com/bsenel/karakuri/auth`, inside this monorepo but with its own `go.mod` and its own `auth/v*.*.*` tag namespace. Karakuri is its first consumer, wired in through a thin shim under `internal/auth/`. Persistence lives in a sister module, `github.com/bsenel/karakuri/auth/sql`, so callers pick their own driver and only pull the deps they use.

2. **The core module has zero external dependencies.** Stdlib only — including the JWT implementation, which uses `crypto/hmac` and `crypto/ed25519` behind a strict algorithm allowlist. The roadmap originally pencilled in `golang.org/x/time`; nothing in an authorizer needs a rate limiter, so it was dropped. (That dependency belongs to the Phase 15 quota module.)

3. **The permission catalog is supplied by the consumer, not hard-coded.** The module knows nothing about twins or objectives. A deployment registers its own action set at startup, and a policy naming an unregistered action is rejected at seed time. Nothing is implicit: a typo in a role definition fails on boot instead of silently granting or withholding access.

4. **Four composable concepts, not one flat mapping:**

   - `Role` — a named policy set with `Inherits`, so `admin ⊃ operator ⊃ viewer` is stated once. Built-ins are flagged `System` and are immutable.
   - `Policy` — an action pattern and a resource pattern with an `allow`/`deny` effect.
   - `Condition` — a closed set of typed predicates (`owner_equals`, `attr_equals`, `attr_in`) that narrow a policy against the acting principal and the target resource.
   - `RoleBinding` — grants a principal a role **over a scope** (`*`, `twin:*`, `twin:abc`). This is what separates "Alice is an operator" from "Alice is an operator on this one twin".

5. **Deny wins, and specificity does not break ties.** Precedence is exactly `explicit deny > explicit allow > default deny`. A narrower allow can never override a broader deny. This is deliberately unlike IAM systems that rank by specificity, where adding a narrow grant silently punches a hole in a blanket restriction.

6. **Every decision is explainable.** `Authorize` returns a `Decision` carrying the matched policy, the role it came through, the binding scope in play, and the per-condition evaluation trace. That trace powers `krk auth check`, the `403` response body, and an audit row on every denial.

7. **Authorization fails closed.** A store outage or an unresolvable role produces a `500`, never a pass-through.

## Consequences

- External Go services can `go get github.com/bsenel/karakuri/auth` and get roles, conditions, scoped bindings and a `chi`-compatible middleware without pulling Karakuri, GORM, LangChain Go, or OpenTelemetry.
- The monorepo is now multi-module. `go build ./...` at the root does not cover `auth/`, so CI runs a dedicated job per module and each module is released on its own tag.
- Because the catalog is consumer-supplied, the module cannot ship a useful default policy set — every consumer writes its own role definitions. Karakuri's live in `internal/auth/roles.go` alongside the route→permission table.
- Conditions are a closed set. Deployments needing a predicate we do not ship must extend the module rather than write policy in an expression language. That is the intended trade: every condition in a policy stays readable by whoever is auditing it, and evaluation is total — no parse errors at request time.
- Ownership checks move out of handler bodies and into policy, which means they show up in `krk auth policies list` and in the effective-permissions endpoint instead of being invisible until someone reads the handler.
- Phase 17 extends `RoleBinding.Scope` from a flat pattern into a hierarchical path. The shape does not change, so that phase is additive rather than a rewrite.
