# ADR 016 — A digest is a read, and the model writes only its prose

**Status:** Accepted
**Date:** 2026-08-14
**Relates to:** [ADR 006](006-multi-instance-tool-adapters.md) (twin-bound adapters), [ADR 011](011-overrides-and-labelled-spend.md) (labelled spend), [ADR 015](015-standing-objectives-and-reconciliation.md) (standing objectives)

## Context

Phase 20 gave Karakuri objectives it holds rather than finishes. Both of the
things people asked for end the same way — "and tell me what happened" — and
neither is served by a console somebody has to remember to open. A system that
works unsupervised has to report unsupervised.

The material was already there. Reconcile outcomes record every pass, cheap and
expensive. `tool_events` records every action, escalation, approval, rejection
and autonomy movement. `checkpoints` holds what is still waiting on a person.
The cost ledger holds what it all cost. Nothing new needed to be written as
work happened; what was missing was the reading.

## Decision 1 — the digest is assembled from existing records, never accumulated

`Assemble` runs queries. There is no counter incremented as work happens and no
"digest buffer" filled between deliveries.

That makes a digest reproducible: the same window produces the same report
tomorrow, so a delivery that failed can simply be retried, a report can be
regenerated for a past window, and `preview` shows exactly what tonight's
delivery will contain rather than an approximation of it. An accumulator would
have made all three impossible and added a write to every hot path to do it.

It also decides the failure mode. Individual sources are allowed to fail: a
report that arrives without its cost line is worth far more than one that does
not arrive because the ledger was briefly unreachable, and the missing line is
visible in the output rather than silently rendered as zero.

## Decision 2 — the model writes the prose and nothing else

An agent renders a narrative. It does not decide what is in the report, what
counts as a decision, or what is urgent — the structured digest is complete
before the model is called, and the plain rendering of it is what gets
delivered when no model is available.

This is not defensive scaffolding around an unreliable component. It is what
the report is *for*. A digest exists to tell somebody what an autonomous system
did while they were not watching, and a summary that could silently omit a
pending decision because a model judged it unimportant would defeat the whole
exercise. The prose is a convenience laid on top of a complete answer, so it is
appended above the plain rendering rather than replacing it: summaries lose
things, and the reader who wants the numbers should not have to ask twice.

## Decision 3 — silence is the default

A window in which nothing happened is not delivered. `send_when_empty` opts in.

A daily mail that says "nothing happened" is a mail people stop reading, and
the cost of that is paid by the report that matters, three weeks later, when it
is skimmed like all the others. The suppression is deliberately narrow: any
pending decision, any autonomy change, any spend, any failure or drift or
action makes a window worth reporting. Only "the objective was checked ninety
times and nothing moved" is silence, which is exactly the case the two-tier
sense/reconcile split makes common.

`last_sent_at` advances even on a suppressed window, so the next report starts
from there rather than accumulating a week of silence into one message. The
opposite rule — window from the last *delivered* report — would mean a quiet
fortnight produced a fortnight-long report the moment anything happened.

## Decision 4 — schedules belong to twins, and carry a lease

One schedule names a twin, not an objective. A twin holding nine standing
objectives produces one message a day.

The lease is the same conditional `UPDATE` on the schedule row that ADR 015
put on `reconcile_states`, for the same reason and with more at stake. Two
replicas reconciling one objective wastes money and somebody notices in a
graph. Two replicas sending one morning report send it to a person twice, every
morning, and the failure is in somebody's inbox rather than in a metric.

Delivery goes through the twin's bound adapter, so a multi-tenant deployment
reaches the right Slack workspace without this code knowing tenants exist. And
it writes a `tool_events` row whether it succeeded or failed: a message
Karakuri sent on somebody's behalf is a thing it did to the world, and a
delivery invisible to `krk audit` would be the one kind of action nobody could
review.

## Consequences

- **Off by default,** unlike the reconcile supervisor. A supervisor with no
  standing objectives does nothing; a sender that runs will mail somebody, and
  that is a decision an operator makes rather than one they discover.

- **`report:read` is a viewer permission.** Somebody receiving a daily brief
  should be able to find out why they are receiving it and where else it goes.
  Writing one is an operator's, because a schedule makes Karakuri message a
  named address on a recurring basis.

- **A declared schedule is armed for its next firing, not for now.** The
  scheduler treats a never-run thing as due immediately, which is right for an
  objective — the first useful answer is whether the state already holds — and
  wrong for a report, whose first delivery would otherwise cover a window
  nobody asked about.

- **`projectmgmt` and `versioncontrol` are declared channels that refuse.** An
  honest error recorded on the schedule rather than a silent success, so an
  operator who configured one sees why nothing arrived.

- **`SaveCheckpoint` now passes `CreatedAt` through.** It discarded the
  caller's value and let GORM stamp one, which was benign in production and a
  lie in the type — and made "how long has this been waiting", the one number a
  decision list needs, unanswerable for any row storage did not create itself.

## Alternatives considered

**Delivering through a capability, executed by the loop's act step.** It would
have inherited the capability quota, the cost recording and the audit row for
free, which is genuinely attractive. Rejected because a scheduled digest is not
something an agent decided to do — it is the system reporting on itself, and
routing it through the planner would mean a model deciding whether this
morning's report goes out. The audit row is written directly instead, which is
the part that actually mattered.

**One report per standing objective.** Simpler to build and wrong for the
reader. The unit of attention is a person's remit, not a row.

**A digest table, written as work happens.** Rejected: it adds a write to every
reconcile to save queries on a path that runs once a day, and it makes the
report unreproducible — a bug in the accumulator is undetectable and
uncorrectable, while a bug in a query is fixed by fixing the query.
