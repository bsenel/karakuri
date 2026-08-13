# `quota` — standalone rate-limiting and quota module

Overrides parent rules for `quota/` only. This directory is a **separate Go module**
(`github.com/bsenel/karakuri/quota`), not part of the Karakuri module. See
[ADR 008](../docs/adr/008-standalone-quota-module.md).

## Hard rules

1. **No Karakuri imports.** Nothing under `github.com/bsenel/karakuri/internal`, `/domains`,
   `/config` or `/cli` may be imported here. The module has to build for a consumer who has
   never heard of Karakuri.
2. **No external dependencies.** `quota/go.mod` has an empty require block and stays that
   way — including for the token bucket, which is arithmetic rather than a reason to depend
   on `golang.org/x/time/rate`. A dependency only one submodule needs belongs in that
   submodule's own `go.mod`.
3. **No application vocabulary.** This module does not know what a twin, a capability or a
   loop is. Keys are opaque strings a caller's `KeyExtractor` produces. If a change would
   require naming a Karakuri concept here, it belongs in `internal/quota/` instead.

## Conventions

- **`Backend` is high-level.** It hands out decisions, not counters. Do not add
  `Incr`/`ZAdd`-shaped methods: that pushes sorted-set semantics into SQL and mutex
  semantics into Valkey, and every implementation ends up reinterpreting the algorithms
  anyway.
- **Atomicity per key is the contract**, not an implementation detail. A check-then-write
  backend passes almost every case in `quotatest` and fails the one that matters.
- **A refusal consumes nothing**, and always carries a positive `RetryAfter`. Zero tells a
  client to retry immediately, which turns a limiter into a busy loop. `Decision.Normalize`
  enforces both; call it on the way out of `Take` and `Peek`.
- **Time is a parameter.** Every backend method takes `now`. Nothing here calls
  `time.Now` except the middleware's default clock, which is why the tests are
  deterministic and take milliseconds rather than sleeping.
- **Exhaustion is a decision, not an error.** Reserve errors for "I could not find out".
- **Fail open, loudly.** `Limit` lets a request through when its backend is unreachable and
  reports it via `OnError`. This is the opposite of `auth`, deliberately: an authorizer
  that cannot answer must refuse, while a limiter that converts its own store's outage
  into a site outage has done more damage than the traffic it was guarding against.
  `FailClosed()` is there for the cases where the budget is the point.

## Overrides

A limit is configuration; an override is the exception one subject gets. `Resolver` sits
between them, so `Policy` and `Quota` stay values a caller composes at startup.

- **An override replaces a ceiling and nothing else.** Not the algorithm — that is a
  decision about the shape of the traffic, and somebody approving "more" is not choosing to
  swap how it is counted. `Apply` clears `Rate` when the window moves, so a raise cannot
  leave a bigger bucket refilling at the old speed.
- **The cache is the design, not an optimisation.** The request tier runs on every call, so
  reading a store each time makes the limiter a second hot-path dependency. `DefaultCacheTTL`
  bounds how stale a decision can be; whoever writes an override calls `Invalidate` so their
  own process sees it at once.
- **Resolution fails to the configured limit**, never to the cached one and never upward. A
  store that cannot be read must not be able to raise a ceiling, and the failure is not
  cached, so recovery is immediate.
- **A nil `*Resolver` resolves to the base.** A deployment with no overrides never branches,
  and `Limit` without the `Resolve` option never consults one at all.
- **The base can move too, and it is still the base.** `Limit` captures its `Policy` by
  value, which is honest for a limit read from a file at boot and stale for one a host keeps
  in a database. `Base(fn)` resolves the configured limit per request; `Resolve` then applies
  the subject's override on top, in that order — "what everybody gets", then "except this
  one". A base that cannot be read, or that reads back invalid, falls to the policy `Limit`
  was constructed with and reports through `OnError`. It never falls to *no* limit: a
  malformed row must not be a way to switch the limiter off.

## Adding a backend

Implement `Backend`, then run the shared contract from your package:

```go
func TestContract(t *testing.T) {
    quotatest.Run(t, func(t *testing.T) quota.Backend { return New(...) })
}
```

The suite is the definition of correct. A backend is finished when it passes, and a new
behavioural rule belongs in `quotatest` — added once, enforced everywhere — rather than in
one backend's own tests.

## Testing

`go test ./... -race` from this directory. Coverage is gated at 95% in CI
(`.github/workflows/ci.yml`, job `modules`). `quotatest` is excluded from the measurement:
its uncovered statements are assertion branches that only run when a backend is broken, and
it is executed in full by the package under test.

## Karakuri's own wiring

Everything Karakuri-specific lives outside this module, in `internal/quota`:
the four tiers, the key extractor, backend selection, and the token budget. If
a change here would require naming a twin, a capability or a principal, it
belongs there instead.
