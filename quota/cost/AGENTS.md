# `quota/cost` — standalone cost-attribution module

Overrides parent rules for `quota/cost/` only. This directory is a **separate Go module**
(`github.com/bsenel/karakuri/quota/cost`). See
[ADR 011](../../docs/adr/011-quota-overrides-and-cost-attribution.md).

## Hard rules

1. **No Karakuri imports.** Same rule as `quota` and for the same reason: this has to build
   for a consumer who has never heard of Karakuri.
2. **No external dependencies beyond `quota`.** The require block names the parent module
   and nothing else — including for pricing, which takes a Go map rather than reading YAML.
   A price table is configuration, and parsing configuration is the host's job.
3. **No application vocabulary.** A subject is a `quota.Key`, a resource is two opaque
   strings, a label is a string. If a change would require naming a twin or a team here, it
   belongs in `internal/quota/` instead.

## Why this is not part of `quota`

The two answer opposite questions. A quota is asked *before* the work — may this proceed —
and a cost is recorded *after* — this is what it took. Merging them produces a limiter that
has to know prices and a ledger that can refuse things, and neither is better at its job for
it. They share a key space so that "what did this twin spend" and "what is this twin's
remaining budget" name the same subject.

## Conventions

- **Labels are copied onto the event, never derived at query time.** Deriving them would
  mean a report changes retroactively when a resource moves between teams — last month's
  spend migrating to the new team is wrong for money already spent, and makes two runs of
  the same report disagree.
- **A multi-label event counts under each label**, so `ByLabel` buckets deliberately do not
  sum to the grand total. It is the only place anything is double-counted, and it is what
  makes "by team" and "by org" both correct.
- **Ranges are half-open**, `[Since, Until)`, so two adjacent months cannot both claim the
  same event.
- **An empty filter does not narrow.** A caller meaning "nothing" must not reach the ledger
  at all — the same rule the scoped listing in `internal/auth` follows, and for the same
  reason: a filter that widens to everything on empty input is how a report leaks.
- **An unpriced model records its units and no money.** Providers ship models faster than
  anybody updates a rate table, and losing the tokens would make the ledger useless exactly
  when a new model is the thing worth watching. `Pricer` returns zero rather than an error
  for the same reason.
- **Time is a parameter**, as in `quota`. Nothing here calls `time.Now`.

## Adding a ledger

Implement `Ledger`, then run the shared contract from your package:

```go
func TestContract(t *testing.T) {
    costtest.Run(t, func(t *testing.T) cost.Ledger { return New(...) })
}
```

`Fold` is exported so an implementation that groups in the database still agrees with the
reference about bucket keys, ordering and how a multi-label event is counted.

## Testing

`go test ./... -race` from this directory. Coverage is gated at 90% in CI
(`.github/workflows/ci.yml`, job `modules`). `costtest` is excluded from the measurement for
the reason `quotatest` is: its uncovered statements are assertion branches that only run
when a ledger is broken.
