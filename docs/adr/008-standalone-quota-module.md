# ADR 008: Rate Limiting as a Standalone, Dependency-Free Module

## Status

Accepted

## Context

Phase 14 gave every `/api/v1` route an answer to *who is calling* and *whether they may*. Nothing answers *how often* or *how much*. Three specific holes:

- A principal holding `loop:start` can start loops as fast as HTTP allows. `internal/api/server.go` runs `Recoverer` and `Logging` in front of the handlers and nothing else.
- Every LLM provider already reports `TokensUsed` (`internal/platform/llm/provider.go`), which flows through `coreagent.Output` into `LoopIteration.TokensUsed` and the `loop_iterations` table — and is then only ever *recorded*. A reflexion loop that will not converge bills until a human notices.
- Nothing caps per-capability usage, so a misconfigured watcher can hammer one capability indefinitely.

Phase 15 closes them. The same two structural questions as ADR 007 had to be settled first, plus one the auth module never faced.

## Decision

1. **The engine ships as its own Go module**, `github.com/bsenel/karakuri/quota`, inside this monorepo with its own `go.mod` and its own `quota/v*.*.*` tag namespace, mirroring ADR 007. Karakuri is its first consumer through a shim under `internal/quota/`. Persistent and cross-replica backends are sister modules — `quota/sql`, `quota/valkey` — so a caller pulls a database driver or a cache client only if they use one.

2. **The core module has zero external dependencies.** The roadmap pencilled in `golang.org/x/time/rate`. It was dropped. `rate.Limiter` is a single unkeyed bucket, so the keyed map and its eviction are ours either way; the fixed-window and sliding-log algorithms are ours entirely; and what remains to borrow is about sixty lines of token arithmetic. Paying a dependency for that would also cost the empty require block the release workflow verifies.

3. **`Backend` is high-level.** It exposes `Take`/`Peek`/`Reset` returning a `Decision`, not `Incr`/`ZAdd`-shaped primitives. A low-level interface would push sorted-set semantics into SQL and mutex semantics into Valkey, and each implementation would reinterpret the algorithms anyway. This way each is written in its own idiom — Go under a lock, one Lua script per round trip, one transaction — and **atomicity per key is a stated part of the contract** rather than an accident of implementation.

4. **One shared contract suite, in `quota/quotatest`.** Every backend runs the identical table, so a difference surfaces as a failing test in whichever implementation is wrong rather than as a production surprise. A backend is finished when the suite passes; a new behavioural rule is added once, there, and enforced everywhere. The suite includes the property that a token bucket never admits more than `rate × elapsed + burst`, driven by an irregular pseudo-random arrival walk, and a 200-way race proving a check-then-write backend cannot pass.

5. **Rate limits and quotas are different types.** `Policy` smooths traffic over seconds and minutes; `Quota` is a hard cap over a calendar period. A rate limit refuses you and expects you back in a moment; a quota refuses you until tomorrow. They want different messages and usually different responses from the caller. `Quota` puts the period *in the key* — `twin:t1|writes|2026-08-10` — so the reset is exact and identical on every backend: at midnight the key changes and the new period starts at zero without any backend having to implement a calendar.

6. **Limiting fails open; authorization fails closed.** This is the deliberate inversion of ADR 007's rule 7. An authorizer that cannot answer must refuse, because the cost of wrongly allowing is a security breach. A limiter that cannot answer should allow, because the cost of wrongly refusing is an outage caused by the very component meant to prevent one — and one request over the limit is recoverable. `FailClosed()` exists for the cases where the budget is the point, such as a hard spend cap.

7. **The LLM budget is pluggable.** Karakuri's token budget goes through a `TokenBudget` interface with two implementations: `native`, counting `TokensUsed` through this module, and `litellm`, delegating dollar-denominated budgets to a LiteLLM gateway keyed on `x-litellm-customer-id: twin:<id>`. `native` is the default so Karakuri stays a single binary plus optional Postgres.

## Consequences

- External Go services can `go get github.com/bsenel/karakuri/quota` and get three algorithms, calendar quotas, pressure hooks and `chi`-compatible middleware without pulling Karakuri, GORM, LangChain Go, or a Redis client.
- The monorepo gains three more modules. CI's per-module job becomes a matrix (`modules`) covering `auth`, `auth/sql` and `quota` today.
- A distributed deployment must choose a backend deliberately. The in-memory default is per-replica, so a limit of 60/min across three replicas admits 180. This is documented at the type, in the config file, and in the README rather than left to be discovered.
- Because `Backend` is high-level, adding a fourth algorithm means implementing it in every backend rather than composing it from primitives. That is the accepted cost of each backend being atomic and idiomatic; the shared contract suite is what keeps the cost honest.
- LiteLLM is an option, never a requirement. It cannot rate-limit Karakuri's own API routes, cap per-capability usage, or enforce per-adapter limits — three of the four tiers — and its refusal is an HTTP error where the loop wants a Phase 13 checkpoint. That translation stays Karakuri's either way.
- Phase 18 (quota self-service and cost attribution) builds on `Quota` and the `quota:admin` permission this phase registers. The shapes do not change, so that phase is additive.
