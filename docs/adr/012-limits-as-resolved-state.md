# ADR 012 — Limits are resolved state, and a live stream is filtered per subscriber

**Status:** Accepted
**Date:** 2026-08-13
**Relates to:** [ADR 008](008-standalone-quota-module.md) (standalone `quota` module), [ADR 010](010-scope-sets.md) (scope sets), [ADR 011](011-overrides-and-labelled-spend.md) (overrides and labelled spend)

## Context

Phase 19 is described as a frontend phase — pages for what Phases 14 through 18
built. Two of its seven steps could not be built as pages at all, and both turned
out to be decisions rather than plumbing.

**The settings page had no backend.** The roadmap asks for "an admin config editor
for the canonical tiers". `DefaultTiers` read the YAML once at boot and froze the
result into `Deps`; raising a limit meant editing a file and restarting a process.
ADR 011 had deliberately made *per-subject overrides* the runtime mechanism and
left the tiers themselves as boot state. The instruction for this phase was
explicit: build the real editor, and make the database the source of truth.

**The cost dashboard had no stream to follow.** The hub publishes to a `_global`
key that no endpoint subscribed to; only `/twins/{id}/events` and
`/objectives/{id}/events` existed. A dashboard watches everything, and
"everything" on a multi-tenant deployment is a question about who is watching.

## Decision 1 — tiers become resolved state, seeded by configuration

A stored tier replaces a ceiling for everybody. It is deliberately **the base**
rather than an override:

| | scope | mechanism | table |
|---|---|---|---|
| "everybody gets a million tokens" | all subjects | tier | `quota_tiers` |
| "this twin gets five million until Friday" | one subject | override | `quota_overrides` |

Conflating them would put two kinds of thing in one table with no way to tell
them apart in a report, and "who has been granted an exception" is exactly the
question an override list has to answer.

The shape is ADR 011's, reused rather than reinvented: a store, a thirty-second
TTL cache, `Invalidate` on write. Resolution composes in one direction —
configuration, then the stored tier, then a subject's override — because those
are three different statements and only that order makes each of them mean what
it says.

### The precedence rule, and what it costs

**A stored row wins. Configuration is the seed and the fallback.** The file still
matters: it is what a fresh database starts from, and what `krk quota unset`
returns a tier to.

The cost is real and worth naming. An operator reads `llm_tokens_per_day:
1000000` in a YAML file and believes it, and after this change that belief may be
wrong. Three things pay for it:

- the server logs one line per diverging tier at startup, naming the configured
  value, the value in force, who set it and why;
- `GET /quota` returns `configured` beside what is enforced, and the settings
  page renders both, never one without the other;
- both shipped config files say in comments that they are the seed.

A design that made the database authoritative *without* these would be strictly
worse than the config file it replaced, because the file would still look
authoritative.

### `quota.Limit` gained an option, not a signature

The module captures its `Policy` by value at wire-up — honest for a limit read
from a file, stale for one in a database. `Base(fn)` resolves the configured
limit per request; `Resolve` applies the subject's override on top. Additive, so
every existing caller compiles untouched, consistent with ADR 008.

A base that cannot be read, or reads back invalid, falls back to **the policy
`Limit` was constructed with** — never to the zero `Policy`, which admits
everything. A malformed row must not be a way to switch the limiter off.

### What is editable, and what is not

Ceilings, and the request tier's window and refill. **Not** the algorithm and not
a quota's calendar period. Somebody typing a bigger number is not choosing to
swap fixed windows for a token bucket — the same line `Override.Apply` already
draws, for the same reason.

Editing is `quota:admin` and **unscoped**: approving a raise for a team you
administer is a tenant decision, and moving everybody's ceiling is not. A
deployment with no database answers `editable: false` and refuses the write with
a 501, rather than accepting a limit that would vanish on restart.

## Decision 2 — a global stream is filtered per subscriber, and fails closed

`GET /api/v1/events` is gated on `twin:read` and then tested per event.

Filtering a stream is not filtering a list. A listing asks the database one
question and gets rows back already narrowed; a stream is handed events one at a
time by a hub that knows nothing about who is watching. And **a stream is the one
place where getting it wrong leaks continuously rather than once** — a bad
listing is one response, a bad stream keeps going for as long as the tab is open.

Four rules, cheapest first:

1. **Labels the event carries** are used as-is. A cost event copied them at write
   time (ADR 011), which makes this both the cheapest case and the most accurate:
   they say where the spend sat *when it happened*.
2. **A twin or objective** is resolved through the tenancy tree, and the verdict
   is memoised — a loop emits dozens of events about one twin, and a lookup each
   would put the container tree on the path of every loop step in the system.
3. **Quota pressure** names a key rather than a resource. `principal|alice` is
   that principal's own business; `twin|id|capability` is the twin case.
4. **Everything else is withheld** from a scoped reader.

Rule 4 is the load-bearing one. Broadcasting the unclassifiable means every event
type added later leaks by default, and the person adding it would have no reason
to think about tenancy at all. An unreadable tenancy tree also withholds — the
one place in this code that fails closed rather than open, because the costs are
asymmetric: a missing event is a gap in a dashboard, and an extra one is another
tenant's activity.

Grants are read **once**, when the subscription opens. A binding revoked
mid-stream is honoured until the client reconnects. That is the same staleness
the quota resolver accepts, bounded by the life of one HTTP connection, and the
alternative is a database read per event.

## Consequences

- **The config file is no longer the answer to "what is the limit".** Anyone
  debugging a limit reads `krk quota config` or the settings page. The startup
  log is the safety net for somebody who does not know that yet.
- **Two caches now sit between an edit and its effect**, both thirty seconds and
  both invalidated by the writing process. Cross-replica propagation is bounded
  by the TTL rather than immediate.
- **`GET /events` is a long-lived connection per viewer.** The hub drops on a
  full channel rather than blocking, so a slow reader loses events rather than
  stalling a publisher.
- **The interface hides what a principal cannot reach, and that is a courtesy.**
  The server refuses either way. Nothing secret may rely on a hidden menu item;
  it has to be absent from the API's response.
- **Recharts is 386 kB.** The cost page is lazily loaded so only somebody opening
  it pays. A second charting page wants the same treatment or the split stops
  helping.

## Alternatives considered

**Keeping tiers in YAML and calling overrides the answer.** This was ADR 011's
position and it is defensible — an override can raise any subject, including
every subject one at a time. Rejected because "raise the deployment's limit"
should not be N writes, and because the phase asked for the editor explicitly.

**A `LimitStore` in the `quota` module.** Rejected: the four tiers and their names
are Karakuri's vocabulary. The module enforces a `Policy` it is handed and has no
opinion about how many kinds of limit a host has. `quota.Base` is the seam, and
it leaves the module's storage surface unchanged.

**Making the stored tier an override with an empty subject.** Tempting — it
reuses the table, the cache, the CLI and the API. Rejected because a default is
not an exception, and a magic sentinel subject is the kind of thing that becomes
a bug the first time somebody has a real subject that matches it.

**An unfiltered global stream gated on an existing read action.** Fastest, and it
hands every authenticated viewer every other tenant's activity. Not a trade worth
making after two phases spent building the isolation it would undo.

**Polling instead of streaming for the cost dashboard.** Genuinely viable: a
thirty-second refresh meets the acceptance criterion and needs no new endpoint or
tenancy rule. Rejected because the stream is also what the checkpoint and loop
views want next, and building it once with the filter in place is cheaper than
retrofitting the filter later.
