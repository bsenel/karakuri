# ADR 030 — Proposal streams act within their template limits

**Status:** Proposed
**Date:** 2026-11-18
**Relates to:** [ADR 015](015-standing-objectives-and-reconciliation.md) (authority is written into the request), [ADR 017](017-karakuri-as-a-domain-pack.md) (Decision 2, superseded), [ADR 018](018-self-improvement-belongs-to-the-software-pack.md) (the maintainer's bounds enforce the property)

## Context

ADR 017 Decision 2 said the part of Karakuri that decides what to change
cannot change anything. ADR 018 kept the property and moved it: a pack is a
namespace, `stepAct` resolves environments across every domain an objective
names, so what holds the line is `MaxAutonomousActions: 0` on
`software.agent.maintainer` and the checkpoint path it forces every plan
through. The maintainer's authority says so in a comment: zero, "deliberately
not a small number", because nothing else bounded what promotion could ever
mean. The strategist carries zero as well.

So nothing in this pack proposes without a checkpoint. The
`market_discovery` and `engineering_backlog` streams name those two agents,
and a pass that has already done its research or read its telemetry still
stops before it can write the roadmap text and open its pull request. Each
pass waits for a person, and the person's only decision is whether a
mechanical check would have passed.

Phase 40 steps 1 to 3 added that check:

- `agent.AuthorityBounds` holds `ActionLimits`, each a capability, a
  parameter and the patterns it may take (step 1).
- A template's limits are added beside the agent's and every one must hold,
  so a template narrows authority and never widens it; at `act_with_notice`
  and `act` a plan outside them is refused with a bounds row and no
  checkpoint (step 2, `internal/feature/reconcile/authority.go`).
- A delegated action that pushed to a ref outside its declared branch fails
  after it ran (step 3).

## Decision

Zero was the only guarantee available when nothing checked what an action
holds. Limits checked on every action now give the same guarantee: a pass
can reach only the paths, branch and pull request its template declares,
and an action outside them is refused before it runs or failed after it
ran. The guarantee no longer depends on permitting no action at all.

1. **A new agent proposes, and the maintainer and strategist keep zero.**
   `software.agent.proposer` has a small `MaxAutonomousActions` above zero.
   The maintainer and the strategist are unchanged, so every objective that
   names them still escalates every action, at every level.
2. **The two proposal streams name the proposer.** `market_discovery` and
   `engineering_backlog` suggest `software.agent.proposer`.
3. **Their template limits are the bound.** A pass may write only
   `docs/roadmap.md`, and `docs/research/**` for discovery; it may push only
   to the stream's own branch; it may open a pull request only against main.
4. **The merge is still the owner's approval.** No limit admits a merge tool,
   and the `no-merge` constraint stands.

## Consequences

- At `act_with_notice`, a discovery pass and a backlog pass run to an open
  pull request with no checkpoint. At `propose`, `effectiveAuthority` still
  pins the count to zero, so a stream that has not earned the rung still
  asks.
- The bound moves from one number on the agent to the limits on the
  template. A template that drops a limit widens what the proposer may do,
  so the limits are held by tests on the templates, not by a comment.
- A path limit is new: steps 1 to 3 limit branches and tools, not the paths
  a coding action writes. Until the decide step checks a declared path
  parameter and the step 3 check reads the paths a push touched, the paths
  rule is held by the `paths` criterion after the pull request opens, which
  fails the pass but does not stop the write.

## Alternatives considered

**Give the maintainer a small number.** It is the agent ADR 018 relies on,
and every objective that names it would lose the guarantee, not only the
two streams.

**Keep zero and let a person resolve each pass.** The current state: a
checkpoint per pass whose answer a machine already knows.
