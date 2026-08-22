# ADR 017 — Karakuri watches itself through a read-only port, and cannot change itself from the pack that decides what to change

**Status:** Partially superseded by [ADR 018](018-self-improvement-belongs-to-the-software-pack.md)

> Decision 2 ("the pack that decides what to change cannot change anything")
> was justified by a pack boundary that does not enforce anything: `stepAct`
> resolves environments across every domain an objective names. The property
> is real and now lives where it always actually lived — the maintainer
> agent's own bounds. Decisions 1 (a read-only telemetry port on
> BuildContext) and 3 (the fingerprint hashes the shape, not the counters)
> stand unchanged; the environment moved to the software pack and gates on
> whether the port is wired.
**Date:** 2026-08-14
**Relates to:** [ADR 005](005-domain-pack-isolation.md) (domain pack isolation), [ADR 006](006-multi-instance-tool-adapters.md) (twin-bound adapters), [ADR 015](015-standing-objectives-and-reconciliation.md) (standing objectives), [ADR 016](016-earned-autonomy-and-digests.md) (digests)

## Context

Phases 20 and 21 gave Karakuri objectives it holds and a way to report on
them. The example that motivated the whole line of work was Karakuri improving
itself: reading its own usage, finding what limits it, extending the roadmap,
and implementing it under the repository's own rules.

Everything needed to *act* on that already existed. The software pack writes
code in a git worktree and opens pull requests. The research adapter reads the
field. The supervisor schedules and bounds the work. What was missing was the
observation: nothing let a domain pack see what this deployment had been doing.

The obvious shortcut — give the pack the storage adapter — fails ADR 005. The
engine imports no domain knowledge and packs reach the world through
environments, and a pack holding `StorageAdapter` would have both the whole
persistence surface and a write path to the record of its own behaviour.

## Decision 1 — self-observation is a read-only port on BuildContext

`internal/core/telemetry.Reader` answers one question: what has this deployment
been doing over a window. It is defined in core, implemented in platform, and
handed to factories on `environment.BuildContext` — nil for every pack that
does not want it, which is all of them but this one.

Read-only is not a precaution, it is the property being bought. A pack that
could write here could rewrite the evidence of what it did, and the value of
letting Karakuri watch itself depends entirely on the watching being
trustworthy. Both environments the pack exposes refuse an `Act` out loud rather
than succeeding quietly, for the same reason the loop's act step made an
unmatched `EnvID` an honest failure: a silent success is worse than a refusal
nobody can miss.

It lives on the registry rather than being threaded through the loop and
reconcile services. There is exactly one reader per process, both services
already hold the registry, and passing it any other way would mean two
constructors growing a parameter to carry a value neither of them uses.

### The reader ranks, rather than returning counters

`Snapshot` returns bottlenecks already ordered, not four numbers to be compared.
A pack asking "what should I improve" would otherwise derive that ranking
itself, and derive it slightly differently each time a model ran — which is the
opposite of what an evidence-backed proposal is for.

## Decision 2 — the pack that decides what to change cannot change anything

The pack's three capabilities analyse and draft. None of them writes. The
writing is the software pack's, in a worktree, through a pull request an
operator reviews — reached by a cross-domain objective, which is what Phase 13
built.

This is the load-bearing decision in the phase. A single pack that could both
conclude "Karakuri should be allowed to do more" and carry that conclusion out
is one bug away from a system that widens its own bounds, and no amount of
prompt discipline substitutes for not having the capability. The split is
asserted by a test rather than left to a comment, and `karakuri-maintainer`
carries `MaxAutonomousActions: 0` with a confidence threshold no plan can
clear, so the agent always asks however much autonomy its objective has earned.

The self-improvement template is verified in part by
`software.act.open_pull_request` — the pack cannot mark its own homework on the
part it does not do.

## Decision 3 — the telemetry fingerprint hashes the shape, not the counters

The supervisor senses drift by hashing environment snapshots. A telemetry
environment that hashed its raw numbers would move its fingerprint every time
anything happened anywhere in the deployment, and a standing self-improvement
objective would reconcile continuously to discover that work had occurred.

Counts are therefore bucketed by order of magnitude, the approval rate is
banded, and only the bottleneck set is hashed exactly. A busy week is not news;
a new bottleneck, a decision queue growing tenfold, or an objective entering the
circuit breaker are.

This is the first environment where the right fingerprint is deliberately
lossy, and it generalises: an environment's SHA should answer "has anything
changed that would change what I do", not "has anything changed".

## Consequences

- **Off by default.** Like the digest sender and unlike the supervisor: it does
  nothing until somebody declares an objective in it, but pointing Karakuri at
  its own repository is a decision to make rather than to discover.

- **`Criterion.Domain` finally means something.** The conformance suite
  required every verifier to be a capability in the same pack, which Phase 13's
  cross-domain objectives made untrue and nothing exercised until this pack.
  A foreign verifier is now allowed when the criterion declares its domain, and
  still rejected when it does not — so a typo in a local verifier keeps failing
  rather than becoming indistinguishable from a deliberate reference.

- **The conformance check does not resolve the named domain.** A pack is
  validated on its own; whether another pack is enabled and exports the
  capability is the registry's business at boot. Otherwise every pack's
  validity would depend on which others happen to be configured.

- **An unwired deployment reports itself unavailable rather than healthy.** The
  telemetry environment with no reader returns `available: false` and no SHA,
  so the supervisor reads it as blind and drives the objective from its
  schedule — rather than seeing a still fingerprint and concluding nothing has
  changed.

## Alternatives considered

**Giving the pack the storage adapter.** Rejected: it violates ADR 005, hands a
pack the entire persistence surface for four aggregates, and gives the thing
being observed a write path to the observation.

**Exposing telemetry over the HTTP API and having the pack call it.** Rejected:
it would make a pack's access to the deployment depend on a token and a network
round trip to itself, and the authorization question — which principal is a
domain pack? — has no good answer.

**One pack that both analyses and writes.** Simpler by one cross-domain
objective, and the specific thing this phase exists not to build.

**Hashing the raw telemetry snapshot.** Correct in the narrow sense and useless
in practice: it would fire on every counter tick, which for a busy deployment
means a self-improvement objective that reconciles constantly and concludes
nothing.
