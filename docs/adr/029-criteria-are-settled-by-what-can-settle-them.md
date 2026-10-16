# ADR 029 — A success criterion is settled by what can settle it

**Status:** Proposed
**Date:** 2026-10-16
**Relates to:** [ADR 015](015-standing-objectives-and-reconciliation.md) (standing behaviour lives in reconcile), [ADR 021](021-observations-carry-provenance.md) (a payload says who wrote it), [ADR 022](022-discovered-tools-are-bounded-four-ways.md) (a reserved verifier never settles a criterion), [ADR 026](026-an-environment-that-cannot-see-says-so.md) (blind is an error, not an empty result), [ADR 028](028-packs-are-autonomous-behind-one-gateway.md) (the pack manifest)

## Context

On 2026-10-14 the owner asked whether an objective shows Karakuri what success
looks like, and asked for quantitative criteria and verifiers. The research is
in [docs/research/success-criteria-2026-10-14.md](../research/success-criteria-2026-10-14.md),
and the plan is Phase 41 in [docs/roadmap.md](../roadmap.md).

Today, from that report's Code reading:

- `Criterion` (`internal/core/objective/objective.go:156`) has a `Threshold any`
  that nothing reads, and a `Met bool`.
- `stepVerify` (`internal/feature/loop/verify.go`) meets a criterion whose
  verifier ran when every run succeeded (:93-104). Otherwise a model judges it,
  including when the declared verifier never ran (:106-109).
- Completion is `metCount == len(criteria)` (:127).
- `ActionResult` (`internal/core/environment/environment.go:89-101`) carries no
  typed number.
- Every stream criterion is judged (`domains/software/streams.go:42-47`). On
  2026-10-14 the UX stream's breaker tripped after three passes that each opened
  a pull request with tests (#168, #174, #177).

## Decision

1. **Three kinds and three states.**
   - A criterion is `judged` (a model decides), `verified` (a pack verifier's
     pass or fail) or `measured` (a named number from a pack verifier compared
     with an author's target by deterministic code).
   - Its state is `met`, `not_met` or `inconclusive`. An `inconclusive` state
     carries a reason: `no_data`, `verifier_error`, `too_few_samples`,
     `not_run`, `unsupported` or `measure_changed`.
   - `Met` stays in JSON, derived from the state.
   - `Threshold any` is removed.
2. **Completion is "all required criteria met".**
   - A criterion is required unless it says otherwise.
   - Weight scores progress only. `required: false, weight: 0` is a shadow
     criterion: evaluated, shown and audited, never gating.
   - An objective with no required criterion is rejected at create time.
3. **The core holds the measurement, not its meaning.**
   `internal/core/environment` gains
   `Measurement{Name, Value float64, Unit, Source, Trust}` on `ActionResult`,
   and the core holds the comparison operators. Metric names, units and
   parameters are declared by the verifier in its pack manifest (ADR 028),
   beside `OutputSchema`. The gateway carries measurements through MCP results
   as structured content.
4. **A model never settles a measured or verified criterion.** When the
   verifier did not run, the state is `inconclusive: not_run`. A discovered
   (reserved-namespace) tool is rejected as a verifier at create time, which
   extends ADR 022.
5. **No data is inconclusive (ADR 026 read for measurements).** An empty
   result, a NaN or `ErrUnsupported` from a verifier is `inconclusive`, never
   zero and never a pass. A measurement taken from text somebody outside the
   deployment wrote is `TrustThirdParty` (ADR 021) and cannot settle a
   required criterion.
6. **Targets belong to their author.** Targets, baselines (fixed, or
   `at_start` measured once and frozen), reducers and sample counts are fixed
   at create time. Only a person changes them, through the API. The change is
   audited and resets sustain streaks. When a pass changes the paths a
   verifier declares as its own definition, the criterion is
   `inconclusive: measure_changed`.
7. **Time lives in reconcile (ADR 015, rule 8).**
   - The loop evaluates once and converges.
   - Reconcile keeps a per-criterion `sustain` streak beside `ScoreStreak`.
   - Only `not_met` on a required criterion counts towards `BreakerFailures`.
     `inconclusive` neither counts nor resets.
   - Drift still comes only from the converged fingerprint.

## Consequences

- **Passes that did the work stop failing.** A stream pass that opened a PR at
  a green head, with the declared paths, is `met` without a model's reading.
- **The breaker counts real failures only.** A pass that could not be measured
  no longer counts towards it. A criterion that stays `inconclusive` must
  therefore be shown in the digest, or it would hide.
- **Named verifiers no longer fall back to the judge.** Templates that name a
  verifier which does not fit a task, such as `run_tests` on a docs-only task,
  must stop naming it. Phase 41 adds a docs template.
- **Rule 6 gets a real file.** `docs/openapi.yaml` does not exist, so
  Phase 41 creates it for the objective paths it touches.

## What was read

- **Read:** the research report's sections 1, 4 and 7, its Code reading and
  D1 to D8; Phase 38 in full; and the head of ADR 028.
- **Not read:** sections 2, 3, 5 and 6 of the report, which were not reached
  in time, so the window and CI-gate semantics rest on thinner evidence. Also
  not read: the full text of ADRs 015, 021, 022 and 026 for this ADR (the
  report's summaries and AGENTS.md rules 8 to 10 were relied on).
