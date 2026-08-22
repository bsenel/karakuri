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
and **the wired reader is the gate**: with none, the environment reports
`available: false` and `sufficient: false` rather than zeroes that read as a
healthy deployment, and `analyse_usage` says so instead of inventing an
answer. Gating is therefore per deployment — decided by whether the port is
present — rather than by the pack-level `domains[].enabled` flag it replaces.

A first attempt had the factory *refuse to build* without a reader, on the
theory that a deployment which had not opted in should not get the environment
at all. The conformance suite rejected it: a declared factory must be
constructible, and every other adapter-backed environment in this pack builds
and degrades honestly rather than failing construction. The suite was right,
and the property that mattered survives without it — an unwired deployment
learns nothing about the platform, because there is nothing behind the port to
learn it from.

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

- **The conformance suite now runs in CI, for every pack.** It was reachable
  only by a human typing `krk domain test <pack>`: bootstrap runs the
  cross-pack collision check and nothing else, and no pack had a test of its
  own — the deleted karakuri pack held the only one in the tree. This refactor
  shipped a capability with no `OutputSchema` and an unbuildable factory with
  the full suite passing throughout, which is exactly the gap. All six shipped
  packs are checked now, plus an assertion that the checked set matches the
  set bootstrap registers.

- **The maintainer is no longer selected by default, and that is a real
  regression.** `SelectAgent` takes the first agent a domain declares when an
  objective does not name one. In a two-agent pack the maintainer was first;
  in the nine-agent software pack the strategist is. So an objective created
  from `self_improve` runs under the strategist unless the caller names an
  agent — and `TestMaintainerHoldsNoMutatingCapability` then guards an agent
  that did not run. A live run confirmed it: the escalation came from the
  strategist's 0.90 confidence threshold, not from the maintainer's bounds.

  The escalation property survived by luck rather than design — the strategist
  also carries `MaxAutonomousActions: 0`, so every plan still escalated — but
  the stricter threshold and the no-delegate and no-modify-objective flags did
  not apply.

  **Resolved by [ADR 019](019-capabilities-declare-what-they-need.md).**
  `Objective.AgentID` carries the template's suggested agent and `SelectAgent`
  honours it, so `self_improve` runs under the maintainer and the test
  guarding its bounds guards the agent that runs. Left in this ADR rather than
  edited out: the regression is the clearest evidence for what ADR 019
  concludes, which is that a property inferred from an identifier is a
  property nobody is enforcing.

## Alternatives considered

**Keeping the pack.** The status quo, and defensible only if the boundary
enforced something. It does not.

**Merging everything into software with the telemetry environment reporting
like any other.** Nearly what shipped, and the difference is only that the
environment is explicit about having no reader rather than returning zeroes. A
deployment reading zeroes cannot tell "nothing is wrong" from "I cannot see",
and a system reasoning about its own improvement must not confuse them.

**Keeping a `karakuri` pack holding only the telemetry environment.** A whole
pack, an entry in the domain registry, and a config flag, to carry one
environment whose gating is better expressed by whether a port is wired. The
pack would exist to hold a namespace prefix.
