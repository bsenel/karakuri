# ADR 015 — A desired state is held by a supervisor that calls the loop, not by a loop that never ends

**Status:** Accepted
**Date:** 2026-08-14
**Relates to:** [ADR 004](004-four-tier-memory.md) (memory across runs), [ADR 006](006-multi-instance-tool-adapters.md) (twin-bound adapters), [ADR 008](008-standalone-quota-module.md) (limits), [ADR 012](012-limits-as-resolved-state.md) (resolved state, filtered streams)

## Context

Karakuri converges once. `POST /loops` runs observe → reason → decide → act →
verify → learn until the weighted criteria score reaches 1.0 or the iteration
cap is spent, `finalizeLoop` writes `completed` or `failed` on the objective,
and the goroutine exits. Everything about the engine assumes a task with an
end.

Two things people actually want are not tasks with an end. "Keep this
repository's build green." "Every weekday morning, work through the calendar,
the tickets and the inbox, advance what can be advanced, and tell me what needs
me." Those are *desired states*, and the world moves away from them
continuously — a ticket lands, a PR goes red, a calendar fills. Something has
to notice and converge again, on its own, at 3am, without a human typing a
command.

Most of the parts were already here and unused. `Objective.SuccessCriteria` is
a declarative desired state with weights and verifiers. `Environment.Snapshot`
returns a hash of actual state. `AuthorityBounds` plus checkpoints is an
escalation protocol. Phase 11's `loop_states` is a worked example of durable
background work. What was missing was the thing that decides *when*.

There was one near-miss to replace. `runWatchMode` polled `Snapshot` every
thirty seconds after a loop finished and raised a checkpoint when a hash moved.
It observed drift and then asked a human — it never converged — and its ticker
lived in the goroutine of a completed loop, so it died with the process. It
also subscribed to `env.Subscribe()` channels and never read them.

## Decision 1 — the loop is unchanged, and a supervisor calls it

`internal/feature/reconcile` is a caller of `loop.Service.Run`, not a fork of
it. The six steps are untouched. What the supervisor contributes is when to run
them, how far the objective is trusted this time, and what to do with the
answer.

The alternative — teaching the loop to keep going — was rejected on the shape
of the resulting code. A loop that sometimes terminates and sometimes does not
has two lifecycles inside one function, and the branches multiply through
`finalizeLoop`, `persistState`, `ResumeStoredLoops` and every step that reads
`state.status`. The loop is the most-tested and most-load-bearing code in the
repository; the right place for a new lifecycle is beside it.

### The one loop change

`finalizeLoop` consults `obj.IsStanding()` before writing the objective's
status. "Completed" would say the work is over on something whose point is that
it never is, and "failed" would say the same about one unlucky pass. The
supervisor owns those transitions: `converged` when desired and actual agree,
and `active` — with the failure counted — when they do not.

`objective.StatusConverged` is new. `StatusBlocked` is not: it has been defined
since Phase 1 and assigned by nothing, and the circuit breaker is what it was
waiting for.

### Autonomy becomes enforcement by rewriting bounds, not by a second gate

An objective's earned autonomy is one of four rungs — sense, propose,
act_with_notice, act — and it is applied by writing `agent.AuthorityBounds`
into the `loop.Request` before the call. That struct is what the decide step
has enforced since Phase 1.

A second enforcement path was the obvious alternative and would have been a
mistake. Two places that decide whether an action may run are two places that
can disagree, and the one that disagrees quietly is the one that lets something
through.

## Decision 2 — checking is split into a cheap tier and an expensive one

Every pass senses: `Snapshot` on each environment, sorted, hashed into one
fingerprint, compared against the fingerprint taken when the objective last
converged. That is a handful of adapter calls and no model call. A pass only
runs a loop when the fingerprint moved, a schedule came due, or the resync
horizon expired.

This is the whole economics of the feature. An objective can be checked every
fifteen minutes all year and spend money only on the days something happened.
A design that ran the full loop on every check would cost a hundred times more
and tell the operator the same thing on ninety-nine of those hundred occasions.

Three details are load-bearing, and each is a decision that could have gone the
other way:

- **Environment IDs are sorted before hashing.** Environments are built by
  walking the objective's domains and each domain's factory registry, and
  neither order is stable across builds or replicas. An unsorted hash would
  report drift on deploys and not on commits.

- **An environment that cannot hash itself is blind, not still.** A calendar
  adapter returning no SHA is saying "I don't know", and a system that read
  that as "unchanged" would go quiet exactly when it should not. Blind
  environments contribute nothing to the hash and are named in the outcome;
  objectives over them are driven by their schedule instead, and the docs say
  so rather than implying drift detection covers them.

- **Drift is measured against the last convergence, not the previous
  observation.** An environment that changes and changes back is not drift, and
  a change sitting unaddressed since yesterday still is.

`stepObserve` computes a superficially similar composite over observation
versions and is deliberately left alone. It hashes what the loop read, in the
order it read it, as a record of one iteration's input; this hashes what the
world claims to be, order-independently, as a value to compare against later.
Sharing them would mean giving one an ordering guarantee it does not need or
taking one from the other. What *was* shared is `SelectAgent` and
`BuildEnvironments`, extracted from the runner — if the supervisor built a
different environment set than the loop observes, it would be watching one
world and converging another.

## Decision 3 — the database arbitrates between replicas

`reconcile_states` carries `holder` and `lease_until`, and claiming an
objective is one conditional `UPDATE` with a `RowsAffected` check. Whichever
replica's statement lands first moves the lease into the future and every other
replica's `WHERE` clause stops matching.

Phase 11 deferred this — "cluster-aware leader election is left to operators
using leader-election sidecars" — and for one-shot loops the cost of duplicate
work is a duplicate run somebody notices. For standing objectives it is a
recurring duplicate bill and two copies of the same morning report going to the
same person, forever. That is not a deployment concern.

There is no lock to release and nothing to clean up after a crash: an expired
lease is indistinguishable from an absent one, so a crashed holder recovers by
doing nothing. The supervisor also lets go of the lease when a reconcile
escalates rather than sitting on it — a question to a human could take days,
and holding a lease and a concurrency slot for all of them would starve every
other objective to babysit one.

## Consequences

- **Standing behaviour is opt-in and invisible until used.** `Mode` is empty
  for every objective written before this, and empty is oneshot. A deployment
  that declares no standing objectives is unaffected by all of it.

- **Watch mode is gone and its behaviour is not.** `--watch` and
  `watch_mode: true` still work, now as a standing objective at sense-only
  autonomy: same polling, same checkpoint on change, and it survives a restart.
  Two drift detectors that could disagree is worse than one.

- **Loops are now bounded, for the first time.** They have been unbounded
  detached goroutines since Phase 1 (gosec G118, accepted). Standing objectives
  would have made that a real failure — a hundred coming due together is a
  hundred concurrent model-calling loops — so the supervisor dispatches under a
  semaphore. One-shot loops are still unbounded; nothing about this change made
  that better.

- **An escalation is not a failure anywhere in the accounting.** A loop that
  stopped to ask did the right thing, and a circuit breaker that counted
  questions would trip precisely on the objectives being most careful.

- **The stall detector finally exists.** The Phase 1 risk table promised "if
  score doesn't improve for N consecutive iterations, emit checkpoint rather
  than burning tokens" and nothing implemented it; the only brake was the
  iteration cap and the token budget. It lives on the outer loop rather than
  the inner one, where "no progress across three full reconciles" is a stronger
  signal than "no progress across three iterations of one run".

- **Resolving a checkpoint and resuming a loop are one act now.** They were two
  disconnected paths, each leaving the other's state stale. A human notices and
  uses the other endpoint; a standing objective would have wedged every time.

- **Polling, not events.** The supervisor watches `loop_states` rather than
  holding a channel into the loop, because the row is what survives the
  process. A two-second poll against a reconcile measured in minutes is a
  rounding error, and it buys a wait a restart can simply redo.

## Alternatives considered

**A separate `Controller` entity holding the cadence.** Rejected: the objective
would then have two sources of truth for what it is meant to do, and every
question about "what does this objective do" would need both rows. The
declaration lives on the objective, where the person who wrote it edits it; the
runtime state lives in `reconcile_states`, where nobody does. That is the split
Phase 11 already drew between `Objective` and `loop.State`.

**One goroutine per standing objective.** Rejected: a thousand objectives is a
thousand sleeping goroutines and a thousand independent timers to reason about
during a restart. A single due-wheel over an indexed `next_due_at` is one
statement per tick, and it follows the shape the retention sweeps in
`bootstrap.go` already use.

**`robfig/cron` as a scheduler.** Rejected in favour of using it only as a
parser (`ParseStandard`, `Next`). Its scheduler would be a second timing
authority in the process, each with its own idea of what is running, and none
of its state would survive a restart or coordinate across replicas — which is
most of what the supervisor is for.

**Reviving `internal/platform/executor` (Restate/Celery).** Rejected for now.
It has had no importers since Phase 11, its `Task.Fn` is a Go closure and so
cannot cross a process boundary, and durable *scheduling* is a different problem
from durable *execution*: the supervisor needs to know which objectives are due
and who is working on them, which is a query, not a queue. A future phase that
moves loop execution onto Restate does not change any decision here.

**Reporting drift by event subscription rather than polling.**
`Environment.Subscribe` exists and is implemented; watch mode subscribed to it
and never read the channels. Rejected for this phase because a push signal that
is lossy in-process (the hub drops on slow subscribers) and absent across
replicas cannot be the primary trigger — it can only ever be an optimisation on
top of a poll that has to exist anyway.
