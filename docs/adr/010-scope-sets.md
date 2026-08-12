# ADR 010 — Hierarchy is a set of labels on the resource, not a path in its name

**Status:** Accepted
**Date:** 2026-08-12
**Relates to:** [ADR 007](007-standalone-auth-module.md) (standalone `auth` module), [ADR 009](009-federated-identity-jit-provisioning.md) (federated identity)

## Context

Phase 14 gave role bindings a **scope**, so "alice is an operator" and "alice is an
operator on `twin:abc`" differ by one field. The scope is a pattern in one grammar —
exact, `<prefix>:*`, or bare `*` — matched by `matchPattern` for actions, policy
resources and binding scopes alike.

That is enough for one team and not enough for two organisations. Phase 17's job was
to make a multi-tenant deployment isolate naturally, and three things shaped how.

**The roadmap's design conflicted with the module's own rules.** It proposed Azure-style
resource paths (`org:acme/team:eng/twin:abc`) matched by a new `HierarchicalAuthorizer`
behind a config flag. `auth/AGENTS.md` says: *"patterns use one grammar … do not add a
second matching rule; extend that one."* A second matcher for the same field, selected
by configuration, is two authorization semantics in one binary — and the one that
matters is whichever is configured on the day something goes wrong.

**Paths do not fix what actually leaks.** `TwinHandler.List` returned every twin,
filtered only on `kind` and `domain`. No list endpoint filtered by principal, so
per-resource denial was never isolation: a tenant refused another tenant's twin by id
could read the whole list. A path model changes how a single check matches and leaves
that untouched.

**Phase 16 opened a hole.** `Provisioner.reconcile` wrote managed bindings with
`Scope: "*"`, so every federated user landed with their mapped role over *everything*.
A directory group of two hundred people was two hundred globally-scoped principals.

## What the research says

| System | Mechanism | Lesson |
|---|---|---|
| [Zanzibar / Leopard](https://authzed.com/zanzibar) | *"Recursive pointer chasing during check evaluation has difficulty maintaining low latency with groups that are deeply nested"* — so it **flattens group-to-group paths** into a materialised index | Materialise the closure; never walk a tree per request |
| Zanzibar, again | The index answers *membership*, not enumeration — the paper does not address listing what a user can access | Listing is a separate, harder problem |
| [OpenFGA ListObjects](https://openfga.dev/docs/interacting/relationship-queries) | Default cap of **1,000 results**; *"quite database and CPU intensive"*, needs throttling | A general relationship graph makes listing expensive |
| [Kubernetes HNC](https://github.com/kubernetes-retired/hierarchical-namespaces/blob/master/docs/user-guide/concepts.md) | *"The primary way that HNC implements this policy inheritance is simply to copy them."* Reparenting deletes objects no longer in the ancestry | Propagate at **write** time; keep evaluation flat |
| [Azure RBAC scopes](https://learn.microsoft.com/en-us/azure/role-based-access-control/scope-overview) | Path scopes keyed on `{subscriptionId}` — a GUID, not a name | Identity is an ID; names are display |
| [Azure management groups](https://learn.microsoft.com/en-us/azure/governance/management-groups/overview) | *"Use the management group's ID and not the management group's display name. This common error happens because both are custom-defined fields."* Six-level depth cap; one parent; hierarchy **cached up to 30 minutes** | Even the vendor caps depth and caches rather than resolving live |

Everyone who runs a hierarchy at scale materialises or caches it. Nobody gets listing
free. And every one of them keys scopes on IDs — Microsoft calls the name/ID mix-up
*"this common error"*.

## The decision

**Put the ancestor closure on the resource as a set of labels, and leave the pattern
grammar alone.**

```go
type ResourceRef struct {
    Type, ID, Owner string
    Attrs  map[string]string
    Scopes []string   // ["team:t_7f2a", "org:o_9c31"]
}
```

A binding covers a resource if its scope matches the resource **or any of its labels**.
That is the entire evaluation change:

```go
func (b RoleBinding) covers(r ResourceRef) bool {
    return r.InScope(b.EffectiveScope())
}
```

`matchPattern` is untouched, because `team:t_7f2a` is *already* a valid pattern. The
one-grammar rule is satisfied by not touching the grammar.

### Labels carry IDs, never display names

Acme's Engineering is `team:t_7f2a`; Globex's is `team:t_be04`. They cannot collide,
because the database issues the IDs. Names are unique **per parent** and nowhere else,
which is exactly the multi-tenancy case stated as a schema constraint.

Renaming an organisation then changes nothing about authorization — the binding still
points at the same ID. Under a path model, `org:acme/team:eng` would have to be
rewritten across every binding on rename, and any that was missed would be a silent
grant or a silent denial. A name collision across organisations would be a
**cross-tenant grant**.

Readability is recovered where it belongs: the CLI resolves names to IDs before
sending anything, and an ambiguous name is an error rather than a guess.

### A set, not a path — which is what makes sharing work

A resource can be **multi-homed**. A twin carrying
`["team:t_7f2a", "org:o_acme", "project:p_delta"]` is simultaneously acme's and part of
a cross-organisation project both tenants are bound to. A path model cannot express
this at all, because a path is a single location.

Azure ran into exactly this and could not solve it inside the hierarchy: their own
comparison marks *"grouping of resources that are shared across scope boundaries"* as
**Not Supported** for resource groups, subscriptions and management groups alike —
management groups are *"one parent only"* — so they shipped a separate construct,
[Service Groups](https://learn.microsoft.com/en-us/azure/governance/service-groups/overview),
because *"your Azure resource hierarchy … may not reflect how your teams actually
work."*

The split costs them the thing that matters: *"Role assignments on the Service Group
can be inherited to the **child Service Groups only**. There's **no inheritance**
through the memberships to the resources."* So an Azure customer picks **either** a
container that grants permissions **or** one that crosses boundaries. Scope sets give
both from one mechanism, because a label is a label whether it came from the ancestry
or from a shared project.

That unification has a price Azure pays deliberately: their service-group access
*"doesn't grant access to the underlying resources"*, which is a real least-privilege
valve. We give it up, so two guards replace it — adding a resource to a project
requires holding that resource (Azure requires the same pairing: *"Service Group
Contributor on the service group **and** `Microsoft.Relationship/write` on the
resource"*), and project-scoped roles stay narrow.

### The closure is materialised by the caller

`auth` never walks, recurses, or bounds depth during a check. The tree lives in
Karakuri (`internal/core/container`, `internal/feature/container`), which computes the
closure and stores it in `resource_scopes` at write time — the Leopard and HNC lesson.
Nesting is therefore free at request time, and reparenting recomputes what moved:
a resource that leaves an organisation stops being visible to it the moment the move
commits, not the next time somebody reads it.

`resource_scopes` keeps **declared** membership alongside **derived**, separated by a
`direct` column, because a closure cannot be recomputed from itself. HNC keeps the same
split between a namespace's parent and the objects it propagates.

### Listing is a query, not a boolean

Authorization answers a boolean about one resource. Listing asks the opposite question,
and answering it by checking every row does not scale — this is why OpenFGA caps
`ListObjects` at 1,000 results.

`GrantedScopes(principal, action)` hands back the scopes a principal holds, from **one
indexed read of their bindings**, and the caller turns them into a `WHERE`: an indexed
`IN` over `resource_scopes.label`. A path model would need `LIKE 'org:acme/%'` —
unindexable, and unable to express "these three teams".

Allow and deny are kept apart rather than resolved against each other, because
resolving them needs the tree, which `auth` deliberately does not know. Conditional
denies are absent and cannot be otherwise: whether one bites depends on the resource,
which is not in hand when building a query. **A filtered listing is therefore a
narrowing, and the per-resource check stays authoritative** — the same caveat
`ExpandGrants` already carries, for the same reason.

## The hierarchy authorizes changes to itself

Four rules, each closing a specific escalation:

- **Creating inside a container requires holding it**, so an acme administrator cannot
  create a team in globex. Creating a **root** is checked against the unrestricted
  scope instead: minting a tenant is a different privilege from running one, and
  without that distinction anyone who could create a team could create an organisation
  beside their own and grow from there.
- **Reparenting requires a covering grant at both ends.** Holding only the destination
  would let somebody pull a team, and everything in it, out of a tenant they have no
  claim on. Azure enforces the same pairing.
- **Granting requires already holding the scope.** Without it, the permission to manage
  bindings is the permission to manage every tenant.
- **Sharing a resource into a project requires holding the resource and the project.**

Creating principals stays an unscoped privilege, deliberately: `POST /auth/users`
upserts a principal and sets its password, so a tenant-scoped administrator who could
reach it could reset the bootstrap administrator's. Bindings are the part containment
makes safe to delegate; identities are not.

## Consequences

- **Perfectly additive.** A resource with no containers carries no labels, so only
  `twin:<id>`, `twin:*` and `*` match — byte-identical to Phase 14. Every existing
  pattern test passes untouched, and a deployment that never creates an organisation
  behaves exactly as it did.
- **Deny-wins stays total and unchanged.** A deny on `org:o_9c31` beats an allow on a
  single twin with no new precedence rule. Better than Azure, where an inherited
  assignment cannot be denied lower down.
- **A collection ref carries the caller's own containers, on list routes only.** A
  collection is `twin:*`, which no container-scoped binding matches — so without this a
  team-scoped principal could read their twins one at a time and could not call
  `GET /twins` at all. The route check answers *"may you list"*; the filter answers
  *"which"*. This also changed what a single-object binding sees: a flat 403 became
  exactly its own twin. Nothing new is exposed — every row returned is one the
  principal can already fetch by id.
- **Depth is capped at six**, Azure's own limit, and cycles and per-parent name
  collisions are refused at write time.
- **No intermediate wildcard.** There is no `org:acme/team:*`; binding the organisation
  covers everything beneath it, which is what people want.
- **No canonical display path from a label alone.** Resolve one for the interface;
  never match on it.
- **The closure needs recomputing when the tree changes.** Organisations and teams
  number in the tens and change rarely.
- **Federated grants carry a scope.** `role_map` entries accept
  `{role: operator, org: acme, team: eng}`; names resolve to IDs once, at boot, where a
  typo is something an operator is reading output about. The bare `[operator]` form
  still means the role over everything, so no existing configuration file changes.

## Alternatives considered

**Azure-style resource paths, per the original roadmap step.** Rejected: it adds a
second matching rule to a field shared with action matching, it cannot express a
resource in two places, it makes listing an unindexable prefix scan, and it forces
every binding naming a container to be rewritten when that container is renamed.

**A relationship graph (Zanzibar/OpenFGA-shaped).** More expressive than either, and
the expressiveness is exactly what makes listing expensive — a graph walk per query,
hence the 1,000-result cap. Karakuri's question is "which containers does this
principal hold", which is one indexed read of their bindings. Buying generality we do
not need with a listing story we cannot afford is the wrong trade.

**A separate sharing construct alongside the hierarchy**, as Azure shipped. Rejected
because it is two mechanisms where one suffices, and because the version that crosses
boundaries is the version that grants nothing — a customer picks isolation or sharing.
A label is a label whichever way it was acquired.

**`IsAncestor` / `ParentOf` in `auth`, per the original roadmap step.** Not built.
They are path utilities; in a set model parentage lives in the container tree, which is
Karakuri's side. `auth` gained `InScope`, `ScopeLabel` and `ValidateScopes` instead.
