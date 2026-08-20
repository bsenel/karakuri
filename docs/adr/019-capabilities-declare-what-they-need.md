# ADR 019 — A capability declares what it needs; the loop does not infer it from the name

**Status:** Accepted
**Date:** 2026-08-20
**Relates to:** [ADR 005](005-domain-pack-isolation.md) (domain pack isolation), [ADR 018](018-self-improvement-belongs-to-the-software-pack.md) (self-improvement in the software pack)

## Context

Self-improvement could analyse and draft but never write. Tracing why produced
two findings that turned out to be the same finding.

**The loop chose who gets a workspace by string matching.** `stepAct`
provisioned a git worktree for any capability whose ID ended in `.write_code`
or `.write_test`. Neither of those had an implementation: no environment
served them, so they fell through to `noopEnv`, which returned
`"unimplemented"` — *after* a worktree and a branch had been created for them.
Meanwhile `software.act.delegate_to_cli`, the one capability that can actually
write (it hands the task to a coding-agent CLI), did not match the suffix and
so was invoked with an empty `worktree_path`, into a tree a planner hint
explicitly forbids writing to.

The capability with a workspace could not write, and the capability that could
write had no workspace.

**The agent that runs an objective was chosen the same way.** `SelectAgent`
took the first agent the objective's domain declared. `Template.SuggestedAgents`
existed to say otherwise and was read by nothing — an objective created from a
template kept no reference back to it, so the field could not be honoured even
in principle. That was harmless while packs had two agents. After ADR 018 moved
self-improvement into the nine-agent software pack, `self_improve` ran under
the strategist, and the test asserting the maintainer's bounds guarded an agent
that never ran. The escalation property survived only because the strategist
also happens to carry `MaxAutonomousActions: 0`.

Both are the same mistake: **a property the system needed was inferred from an
identifier instead of being declared by the thing that knows it.**

## Decision 1 — `Capability.NeedsWorkspace`

A capability says whether it writes files. `stepAct` asks the capability
registry rather than the capability's name, and provisions a worktree for
those that say yes — putting its path and branch in the action's params.

A capability the registry has never heard of gets no workspace. A name a model
invented is not a reason to create a branch.

The four capabilities that write now declare it: `write_code`, `write_test`,
`delegate_to_cli` and `create_pr`. A test asserts that set exactly, in both
directions — a capability that writes without declaring it fails, and so does
one that declares it without writing.

## Decision 2 — `write_code` and `write_test` delegate rather than duplicate

They are not reimplemented. They route to the same coding-agent CLI that
`delegate_to_cli` uses, because they are the same act with different emphasis,
and a second code path would be a second thing to keep correct.

`write_test` says "tests only, no implementation changes" in the prompt. The
CLI receives text; the distinction from `write_code` is not implied by an ID it
never sees.

Both refuse rather than guess:

- **No task** → refused, so a plan that names the capability and forgets the
  prompt does not bill a CLI run to produce nothing. A capability that succeeds
  on empty input feeds a perfect success rate into procedural memory and biases
  the next plan's confidence *up* for having done nothing.
- **No worktree** → refused, and named as the reason. Arriving without one
  means provisioning failed, and writing into the checked-out tree instead is
  the single outcome the planner hints forbid.

## Decision 3 — the objective carries its agent

`Objective.AgentID` names the agent the objective runs under; empty keeps the
old behaviour exactly. Template instantiation copies the template's first
suggested agent into it, which is what finally makes `SuggestedAgents` mean
something.

`SelectAgent` checks it before falling back to first-declared. A name no
enabled pack declares falls back rather than failing the objective — an
objective should not be unrunnable because of a typo — but it logs, because
silently running a different agent than the one named is how a bounds
guarantee evaporates without trace.

## Decision 4 — an environment declares what it serves, and that is the route

*Added when Phase 26 step 4 shipped. It was written up as the remaining work
under Consequences, and the same argument applies one level out: `stepAct`
resolved an action's environment from `Action.EnvID`, which the **model**
chooses. A plan that wrote code without naming `software.env.cli_agent` reached
`noopEnv`. A priority-10 planner hint stated the pairing, which is guidance
rather than a guarantee — the same shape as a name-suffix test, differing only
in who does the inferring.*

`environment.Factory.Serves` lists the capabilities an environment executes.
The registry indexes it, and `stepAct` resolves through the index. Which
environment runs a capability is a fact about the pack, fixed when it is
written; a model re-deriving it per plan can only get it wrong.

The fallback order is the point. `EnvID` still decides where the registry has
no answer — nothing claims the capability, or two environments both do — and
**ambiguity is never resolved by picking**. A capability two environments claim
is a pack bug that conformance names; until it is fixed, deferring to the
plan's preference is better than letting map iteration order decide which
environment writes files.

A route to an environment this loop did not build is not a route either. That
case used to fall through to "only one environment, so use it", which runs a
capability on an environment its own pack never said could run it.

## Consequences

- **A worktree is now created for `create_pr`**, which previously had none —
  the version-control adapter takes a `worktree_path` the loop never supplied.

- **The maintainer's bounds are load-bearing again**, rather than being
  guarded on an agent that did not run.

- **The declarations are checked by running them, not by a second copy.** A
  declaration kept honest by a mirror is two declarations that can disagree;
  `selfimprove.go` had exactly that — a hand-written `servedBy` map beside the
  environments, correct only while somebody remembered to update it. It now
  reads the `Serves` declarations the loop routes on. Conformance rejects a
  `Serves` entry naming a capability the pack does not declare, two
  environments claiming one capability, and a `NeedsWorkspace` capability that
  routes nowhere. An end-to-end test drives the whole chain against a scratch
  repository; disabling the routing turns it red.

- **This is the fifth instance of one defect class in this line of work**:
  `MaxAutonomousActions: 0` that meant unlimited, `write_code` declared and
  unimplemented, `open_pull_request` verified against a capability nothing
  exports, `SuggestedAgents` read by nothing, and — found while wiring the
  routing — `software.act.write_design_doc`, declared since Phase 2, listed by
  two agents, required by a priority-9 hint before any `write_code` action, and
  served by no environment for four phases. Every plan that obeyed the hint
  failed the step the hint made mandatory.

  Each looked correct because a field held the right value. The five were found
  by reading code, which does not scale and did not catch them for four phases;
  Phase 24 exists to make declarations testable by running them, and should be
  read as the general answer to all five.

## Alternatives considered

**Implementing `write_code` and `write_test` directly.** A second path to the
same place, with its own prompt handling and its own bugs.

**Deleting them and keeping only `delegate_to_cli`.** Defensible, and rejected
because the two names carry intent a planner uses — "write a test" and "write
the code" are different instructions, and collapsing them loses the TDD
ordering hint the pack already declares.

**Inferring the workspace need from the capability's verb segment** (`.act.`).
The same mistake one level up: `draft_adr` is an `.act.` that touches no
repository, and `create_pr` needs a worktree without writing files itself.

**Declaring the route on the capability rather than the environment**
(`Capability.ServedBy`). Symmetrical with `NeedsWorkspace` and wrong for a
different reason: a capability can be served by more than one pack — that is
what makes cross-domain objectives work — and a capability naming its
environment would have to name one, hard-coding a pairing that is properly a
property of whichever pack is enabled.

**Failing registration when two environments claim one capability.** It makes
the ambiguity impossible rather than merely visible, and was rejected because
the registry holds every enabled pack: one badly-declared third-party pack
would take down a deployment at boot. Conformance fails the pack that created
the ambiguity, which puts the failure on whoever can fix it.

**Routing by capability and ignoring `EnvID` entirely.** Tempting, since
`EnvID` is what got this wrong. Rejected because most capabilities are served
by no environment at all — they are reasoned about or verified — and removing
the fallback would make every one of those unroutable.
