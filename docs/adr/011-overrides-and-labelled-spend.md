# ADR 011 — An approval writes an override; spend carries the labels it was spent under

**Status:** Accepted
**Date:** 2026-08-12
**Relates to:** [ADR 008](008-standalone-quota-module.md) (standalone `quota` module), [ADR 010](010-scope-sets.md) (scope sets)

## Context

Phase 15 shipped four limits, read from configuration at boot and frozen into `Tiers`.
Raising one for a single team meant editing YAML and restarting the process. Phase 18's
job was to make that self-service, and to make what a twin spends visible before the
invoice arrives.

Three things shaped how.

**The roadmap's step 1 does not close its own loop.** It specifies `Request` +
`RequestStore` with `Submit/Decide/List`, and stops there. Approving a request would
write a row that nothing reads, so the acceptance criterion — *"Alice's effective quota
reflects the new limit within 60 seconds"* — could not be met by it. Something has to
consult the decision when a limit is resolved.

**The roadmap predates Phase 17.** It buckets cost "by team", and Phase 17 had just made
teams real. A report that is not filtered by the same scope sets is a way around the
isolation that phase existed to build: a per-resource check that refuses another tenant's
twin means nothing while a report totals that twin's spend.

**`Deps.TakeCapability` had no callers.** The per-capability daily quota was configured,
documented, defaulted — and enforced nowhere. Confirmed by grep across `internal/`: the
only mention was its own definition.

## The decision

### An override is the thing an approval writes

```go
type Override struct {
    Subject   Key           // "principal|alice", "twin|t_7f2a" — opaque, as always
    Name      string        // which tier: "llm-tokens", "capability", "request", "adapter"
    Cap       int
    Window    time.Duration // rate policies only; quotas keep their calendar period
    ExpiresAt time.Time     // zero means until revoked
    Reason    string
}
```

A `Resolver` wraps an `OverrideStore` with a TTL cache and answers `Policy(subject, base)`
and `Quota(subject, base)`. `Request.Override()` is a method, because approving a request
is exactly writing the override it describes — a request that could not be turned into
one without further decisions would be a request nobody can act on.

An override **replaces** rather than multiplies. An operator approving a limit should be
able to read the number they are approving; "2× the default" is a number that changes when
somebody edits the config file, which is the opposite of what an approval means.

`quota.Limit` did **not** change signature. It gained an option — `quota.Resolve(resolver,
name)` — so every existing caller compiles and behaves identically, which is the same
additive discipline ADR 008 set for the module.

### The 30-second cache is why the criterion says 60 seconds

The request tier runs on **every** API call. A database read per request to answer "has
anyone raised this limit lately" is the wrong trade against a limiter whose entire job is
to be cheaper than the work it guards. A 30-second TTL lands an approval well inside the
minute the roadmap asks for, and the process that approved invalidates its own entry, so
an operator watching `krk quota show` sees the new number immediately rather than
wondering whether it took.

A store that cannot be read resolves to the configured limit and **does not cache the
failure**, so a database blip does not pin every subject to the default for the next
thirty seconds. It is logged, because falling back silently would mean an approved raise
quietly not applying.

### Approving is bounded by the same containment as granting

The module owns the record; **who may approve is Karakuri's question**. It is answered
where every other scoped decision is: the subject a request would raise is rendered as a
resource carrying its containers, and the approver must hold `quota:approve` over it. An
acme administrator approves acme's requests and nobody else's.

Without that rule, the permission to approve is the permission to raise anybody's limit,
including one's own in a tenant one has no claim on — the same escalation ADR 010 closed
for bindings, restated for money. `MayGrant` and this check are now one function with the
action as a parameter.

Rejecting is deliberately **not** gated that way. Somebody who may decide at all may
always decline, and requiring the scope to say "no" would leave requests from tenants
nobody administers pending forever.

### Cost is a sibling module, not part of `quota`

`github.com/bsenel/karakuri/quota/cost`, with its own `go.mod` and a require block naming
only `quota` itself. The two answer opposite questions: a quota is asked **before** the
work — may this proceed — and a cost is recorded **after** — this is what it took.
Conflating them produces a limiter that has to know prices and a ledger that can refuse
things, and neither is better at its job for it.

**Deviation from the roadmap:** `StaticPricer` takes a Go map, not a YAML file. Karakuri's
config already parses YAML and hands the table over. This keeps the module's require block
empty, exactly as `auth` implements JWT over `crypto/hmac` rather than taking a dependency
(ADR 007/008). A price table is configuration, and configuration is the host's job.

Nothing is priced by default. A shipped price table would be wrong the week after it
shipped, and a report that invents money is worse than one that reports units.

### Labels are copied at write time, never derived at read time

An event carries the scope labels the resource sat in **when the spend happened**:

```go
type Event struct {
    Subject    quota.Key
    ResourceType, ResourceID string
    Provider, Model          string
    Units      float64
    UnitKind   string     // "tokens", "calls"
    Cost       float64
    OccurredAt time.Time
    Labels     []string   // ["team:t_7f2a", "org:o_9c31"] — copied, not derived
}
```

Copied is the load-bearing word. Deriving the team at query time would mean a report
changes retroactively when a twin is reparented: last month's spend would migrate to the
new team, and two runs of the same report would disagree. That is wrong for money that
has already been spent.

It also makes "by team" an indexed `IN` over the same labels Phase 17 already
materialises, and lets `/cost` filter through `ListFor` exactly as the twin listing does.

### Raw events **and** a daily rollup, written in one transaction

`cost_events` answers "which objective spent this"; `cost_daily`, keyed on
`(day, subject, resource, provider, model)`, answers "what did we spend". The rollup is
upserted in the same transaction as the event.

A background aggregator would need a scheduler, a watermark, and an answer for what a
report shows while it is behind — three new failure modes to save one write on a path that
is already writing. Raw events age out on a daily sweep; the rollup is kept, so a shorter
horizon costs the drill-down and not the totals. Pruning is the one thing here that *is*
scheduled, and it can be: a sweep that runs late deletes the same rows a day later, which
is not a correctness question the way a lagging total would be.

Both ledgers group through the **same exported `cost.Fold`**, so the SQL implementation
and the in-memory one cannot drift on tie-breaking, label expansion or bucket ordering.

### Recording is best-effort; the work is already paid for

A ledger that cannot be written must not fail the request that generated the charge.
Losing a record is bad; losing work somebody has already been billed for is worse. This is
the same trade the loop already makes when charging tokens, and the same one every limiter
here makes when a counter cannot be read.

## Consequences

- **The capability quota is enforced for the first time.** The act step charges it before
  the action, and fails open with a `quota_pressure` event when the backend cannot be
  read — a quota outage turning into a halted loop does more damage than the calls it
  would have bounded.
- **`coreagent.Output` gains `Provider` and `Model`.** Without them "group by provider" is
  impossible, because the loop knew what a call cost and not who served it. One small core
  change, and the only one.
- **Self-service and cost need a database.** Both are wired only when one is present, on
  any counter backend — an approval that vanished on restart and a spend report that
  started empty every morning are worse than not offering either, so a memory-only
  deployment gets neither and is told why in a log line.
- **A cost report is filtered, and a caller's own filter can only narrow it.** `--org` and
  `--team` intersect with the tenancy filter rather than replacing it, so naming another
  tenant returns nothing rather than their spend.
- **Overrides are not swept.** They are live configuration, not counters; an expired one
  stops applying by its `ExpiresAt` and stays readable, because "why is this one different"
  is a question asked after the fact.
- **Module records gained JSON tags.** They are the wire shape now that the API returns
  them, and `{"ID":…,"RequestedBy":…}` beside every other snake_case response would be a
  difference with no reason behind it.

## Alternatives considered

**A multiplier instead of a replacement** (`cap × 2`). Rejected: the approved number stops
being knowable from the approval, and changes under everyone when the base config is
edited.

**Resolving overrides per request with no cache.** Rejected: the request tier runs on
every call. The 60-second criterion exists precisely because nobody wants a database read
in that path.

**A background rollup job.** Rejected above: a scheduler, a watermark, and an unanswerable
question about what a lagging report means.

**Deriving labels at query time from the current tree.** Rejected: spend that already
happened would move between teams when a twin is reparented, and a report would not
reproduce.

**Putting cost in the `quota` module.** Rejected: opposite questions, opposite hot paths
(`Record` is per model call; `Aggregate` is not), and a limiter that knows prices is a
limiter with a bigger blast radius.

**A YAML price table read by `quota/cost`.** Rejected: it would put a parser and a
dependency into a module whose require block is otherwise empty, to read something the
host already parses.
