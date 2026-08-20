# ADR 018 — A pack is a namespace, not a boundary, so self-improvement belongs to the software pack

**Status:** Accepted
**Date:** 2026-08-20
**Supersedes:** [ADR 017](017-karakuri-as-a-domain-pack.md) Decision 2. Decisions 1 and 3 stand.
**Relates to:** [ADR 005](005-domain-pack-isolation.md) (domain pack isolation), [ADR 013](../roadmap.md) (cross-domain objectives), [ADR 015](015-standing-objectives-and-reconciliation.md) (standing objectives)

## Context

Phase 22 put self-improvement in its own `karakuri` domain pack: telemetry and
repository environments, three capabilities that analyse and draft, two agents,
and two objective templates. The writing was the software pack's, reached by a
cross-domain objective.

ADR 017 justified the split on separation of powers: *"a single pack that could
both conclude 'Karakuri should be allowed to do more' and carry that conclusion
out is one bug away from a system that widens its own bounds."*

That justification does not survive inspection.

## Decision 1 — the pack boundary was never the thing enforcing it

**A pack is a namespace and a registry entry. It is not a sandbox.**

`stepAct` resolves an action's `EnvID` against the environments registered by
*every* domain the objective names, and that union is precisely what makes
cross-domain objectives work. Nothing stops an agent declared in one pack from
planning a capability another pack serves — `self_improve` was *designed* to do
exactly that. The boundary constrained what the pack **declared**, never what an
agent could **reach**.

What actually enforces the property is `MaxAutonomousActions: 0` on the
maintainer agent, plus the checkpoint path it forces every plan through. That is
agent-level, and it survives both capability sets living in one pack.

Worse, the mechanism was inert for the whole of Phase 22's life: the decide step
read a cap of zero as "no cap at all", so neither the boundary nor the bounds
were doing anything. ADR 017 leaned on a boundary that does not enforce while
the thing that does enforce was broken.

`TestPackOwnsNoWriteCapability` matched capability IDs against substrings like
`.act.` and `.write`. It is replaced by `TestMaintainerHoldsNoMutatingCapability`,
which checks the agent's own capability list against an explicitly named set of
repository-mutating capabilities. Naming them beats a substring heuristic:
drafting an ADR is an `.act.` that changes nothing, and the distinction that
matters is what a capability *does*.

## Decision 2 — most of the pack was not platform-specific

Taken piece by piece, three of the five parts had no claim to separation:

- **`draft_adr`** and **`propose_roadmap_phase`** are software practices. Every
  project has a roadmap and writes decision records; nothing about either is
  specific to this platform.
- **`karakuri.env.repo`** was a thin wrapper over `VersionControlAdapter` —
  which is exactly what `software.env.git` already is. A second environment
  over the same adapter.

They are now plain software capabilities, served by the existing git
environment. Drafting is answered *before* the adapter check, because it
touches no repository: a deployment with no version control wired can still
draft a phase for a human to read, and routing it behind the adapter would
report a draft that never happened.

There is also a taxonomy argument. Every other pack names a problem domain —
healthcare, agriculture, legal, mechanical, consulting, software. `karakuri`
named the product. It did not belong in that list.

## Decision 3 — the telemetry environment stays distinct, and gates on wiring

One part is genuinely different in kind, and Decision 1 of ADR 017 stands
because of it: the telemetry environment's **subject** is the deployment
running the loop, where every other software environment's subject is the
codebase the loop is working on. A customer using Karakuri to build their
product should not casually acquire an environment exposing the platform's
internals to their objectives.

That distinction needs a name, not a pack. It is `software.env.platform_telemetry`,
and its factory **returns an error when no telemetry reader is wired** — so the
environment does not exist on deployments that have not opted in. This is
strictly finer than the pack-level `domains[].enabled` flag it replaces: gating
is per deployment, decided by whether the port is present, and a plan naming
`analyse_usage` where it is absent fails honestly through the unmatched-`EnvID`
path Phase 13.5 built for this case.

## Consequences

- **The cross-domain reference is gone, and with it a whole bug class.**
  `self_improve`'s pull-request criterion named `software.act.open_pull_request`,
  which nothing exports — the software pack calls it `create_pr`. It carried 0.4
  of the template's score and could never be satisfied. Nothing caught it
  precisely *because* it was cross-domain: the conformance suite deliberately
  does not resolve foreign domains, since a pack must be valid on its own. Now
  the verifier is same-pack, and the suite checks it. The split created the
  bug and the split hid it.

- **Phase 26's write path gets simpler.** The analysing and writing capabilities
  no longer sit on opposite sides of a domain reference, so wiring a worktree to
  a capability that can use one stops being a cross-pack question.

- **Capability IDs changed.** `karakuri.analyse_usage` →
  `software.reason.analyse_usage`; `karakuri.propose_roadmap_phase` and
  `karakuri.draft_adr` → `software.act.*`. Nothing depended on the old names:
  the pack shipped disabled and no deployment had declared an objective in it.
  Doing this after somebody's config referenced them would have been a
  migration; doing it now was a rename.

- **`domains/karakuri` is deleted.** The engine registers one fewer pack, and
  the platform stops appearing in a list of industries.

## Alternatives considered

**Keeping the pack.** The status quo, and defensible only if the boundary
enforced something. It does not.

**Merging everything into software, telemetry environment included, with no
gate.** Simpler by one factory error path, and it would hand every software
deployment an environment describing a platform its objectives have no business
reading. The nil-reader check costs three lines and keeps the subjects apart.

**Keeping a `karakuri` pack holding only the telemetry environment.** A whole
pack, an entry in the domain registry, and a config flag, to carry one
environment whose gating is better expressed by whether a port is wired. The
pack would exist to hold a namespace prefix.
