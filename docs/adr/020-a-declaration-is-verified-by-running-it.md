# ADR 020 — A declaration is verified by running it, not by reading it back

**Status:** Accepted
**Date:** 2026-08-20
**Relates to:** [ADR 005](005-domain-pack-isolation.md) (domain pack isolation), [ADR 017](017-karakuri-as-a-domain-pack.md) (a pack is validated on its own), [ADR 019](019-capabilities-declare-what-they-need.md) (capabilities declare what they need)

## Context

Karakuri is configured by declaration. A pack declares its capabilities, its
environments, its agents' authority bounds; the engine reads those declarations
and behaves accordingly. That is the design, and it is a good one — until a
declaration is read by nothing, at which point the system behaves as though the
operator had written something else entirely, and everything that inspects the
configuration agrees it is correct.

Five of these shipped, over four phases:

| Declaration | What it claimed | What happened |
|---|---|---|
| `MaxAutonomousActions: 0` | "plans but never acts" — four packs, one with a comment on the line | `decide.go` guarded on `> 0`, so zero meant *no cap at all*. None of those agents were bounded. |
| `software.act.write_code` | a capability that writes implementation | No environment served it. Planned by models, given a git worktree, answered `"unimplemented"`. |
| `open_pull_request` | the verifier for a template's 0.4-weighted criterion | Named a capability no pack exports. The criterion could never be met. |
| `Template.SuggestedAgents` | which agent an objective should run under | Read by nothing. `self_improve` ran under the strategist; the test guarding the maintainer's bounds guarded an agent that never ran. |
| `software.act.write_design_doc` | required before any `write_code` action, by a priority-9 hint | Served by no environment since Phase 2. Every plan that obeyed the hint failed the step the hint made mandatory. |

The conformance suite existed throughout and passed throughout. It checked that
IDs were well-formed, that schemas were populated, that references resolved —
**that the declarations were present and well-shaped**. Not one check asked what
a declaration *did*.

The karakuri pack's own test was the clearest case: it asserted
`MaxAutonomousActions == 0` and was named for the guarantee "cannot act
unsupervised". It passed for three phases while that guarantee was absent.

All five were found by reading code. That does not scale, and it demonstrably
did not work.

## Decision 1 — the policy is a function, so a check can call it

`AuthorityBounds.Decide(confidence, threshold, plannedCapabilities) Verdict`
lives in `internal/core/agent` and holds the entire question of what an agent
may do without asking: the confidence threshold, the approval set, the
autonomous-action cap, the trim.

`stepDecide` calls it and carries out the verdict — the audit row, the
checkpoint, the trim on the plan. It has no policy of its own.

This is what makes the rest possible. A conformance check cannot run
`stepDecide`: it would need a store, an event hub, a checkpoint service and a
loop state. It can run `Decide` with nothing at all. **The extraction is not
tidying; it is the difference between a suite that can test behaviour and one
that can only test shape.**

## Decision 2 — conformance runs each pack's bounds and asserts the outcome

`checkAgentBoundsBehave` walks a pack's agent definitions and, for each, calls
the same `Decide` the loop calls:

- A cap of **zero** must escalate a three-action plan — and must *not* trim it,
  because an approval falls straight through to `act` and an emptied plan would
  discard the work the operator approved.
- A cap of **N** must trim an N+2 plan to exactly N, and must leave a plan of
  exactly N alone without escalating.
- **`UnlimitedActions`** must not trim a fifty-action plan.
- Every capability in **`RequiresApprovalFor`** must escalate when planned, and
  must be one the pack declares.
- A declared **`ConfidenceThreshold`** must escalate below itself and not above.

The ladder is asserted whole on purpose. A check that only tested the zero case
could be satisfied by escalating everything, which would be a different bug
with the same green suite.

Reintroducing the `> 0` guard in the policy now fails conformance in three
packs, each naming its own agent — the regression is caught by the mechanism
built to catch it, in every pack that declares the bound rather than in one
hand-written test somebody remembered to write.

## Decision 3 — the registry answers what a pack cannot

The per-pack suite deliberately does not resolve a foreign domain: a pack is
valid on its own (ADR 017), and making one pack's validity depend on which
others happen to be enabled would be wrong.

That left nobody checking whether a criterion's verifier is exported by a pack
this deployment actually enabled. `CheckDanglingVerifiers` runs at boot, beside
the cross-pack collision audit, and distinguishes the two cases because they
have different owners and different fixes: *the pack that owns it is switched
off* is a deployment decision, *nothing anywhere exports it* is a bug in the
pack.

It warns rather than refuses to start. An operator may be mid-rollout, and a
deployment that will not boot because one template names a pack that is not
enabled yet is worse than one that says so loudly.

## Consequences

- **A bound that does nothing fails the suite in the pack that declares it.**
  That is the property the phase was for, and it is now true of every shipped
  pack and every pack added later.

- **`internal/core/agent` gained behaviour.** It held only types before. The
  policy has no vendor imports and no I/O, so `internal/core/AGENTS.md` still
  holds; but the layering claim "core is types, features are behaviour" is now
  false, and the honest version is that core owns behaviour that is pure policy
  over its own types.

- **Two checks are not five.** This ADR covers authority bounds and dangling
  verifiers. `Serves` and `NeedsWorkspace` got the same treatment in
  [ADR 019](019-capabilities-declare-what-they-need.md), by the same argument
  arrived at from the other direction. What is *not* covered: no check asserts
  that a served capability's environment actually accepts it — the environments
  answer an unknown capability with the same "no active adapter" result they
  give a known one whose adapter is unbound, so the two are indistinguishable
  from outside. Making them distinguishable is the next thing this argument
  asks for.

- **The suite is slower to write and worth more.** A shape check is three lines
  over a slice. A behaviour check has to construct a plausible input and know
  what the right answer is, which means whoever writes it has to understand the
  bound. That is the cost, and it is the point.

## Alternatives considered

**Testing `stepDecide` directly from conformance.** Rejected on layering:
`internal/conformance` is consumed by bootstrap and by `krk domain test`, and
having it import `internal/feature/loop` inverts the dependency. It would also
need a database and a checkpoint service to answer a question that involves
neither.

**A per-pack test in each pack's own package.** This is what `software` had,
and it is why the gap persisted — the check exists only in packs whose author
thought to write it, and a new pack ships with none. The suite runs against
every pack by construction.

**Failing boot on a dangling verifier.** Considered and rejected in Decision 3:
correctness at the cost of a deployment that cannot start during a rollout.

**Making the checks assert exact reason strings.** Tempting for precision, and
rejected because it would couple conformance to the wording of operator-facing
messages, which should be free to improve. The checks assert *which* bound
fired only where the distinction matters — the confidence case, where a more
specific reason is the whole point.
