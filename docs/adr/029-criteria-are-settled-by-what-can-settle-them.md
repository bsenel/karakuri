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
- Completion is `metCount == len(criteria)` (:127). An objective with no
  criteria never gets there: the guard at :37 publishes
  `no_success_criteria_defined` and `finalizeLoop` sets `StatusFailed`. The
  guard counts all criteria, not required ones (fact 6).
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
     `not_run`, `unsupported`, `no_baseline` or `measure_changed`.
   - A verifier error is a reason, not a fourth state. It is never handed to
     the agent as feedback and never counts as an attempt.
   - The run outcome (converged, budget spent, errored) is recorded apart from
     the criterion states and settles none of them. A criterion's state comes
     from its verifier reading the world after the run.
   - `Met` stays in JSON, derived from the state.
   - `Threshold any` is removed.
   - A criterion has a `description` a person writes. A `measured` or
     `verified` one is bound to its verifier by explicit ID, never by matching
     the prose. A `judged` one names an observable outcome and an `evidence`
     field saying what the judge will look at; create rejects it without one.
2. **Completion is "all required criteria met".**
   - A criterion is required unless it says otherwise.
   - Weight scores progress only. `required: false, weight: 0` is a shadow
     criterion: evaluated, shown and audited, never gating. A stretch target
     with an expected partial grade is `required: false` with a weight and is
     shown as progress, never as a failure.
   - The rule is read over the states the verifiers settled after the same
     run, not over flags latched at different moments.
   - Zero required criteria never completes. The guard at verify.go:37
     counts required criteria, so an objective whose criteria are all
     `required: false` fails as an empty one does, rather than completing
     vacuously at :127. Create also rejects such an objective, to save the
     iterations it would spend before failing.
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
   - A verifier that reports skipped or neutral, or has not reported when the
     verify step's bounded wait ends, gives `inconclusive: not_run`: never
     `met`, never pending forever, never `not_met`.
   - A criterion is settled only by a measurement from the verifier it names,
     taken at the commit being judged. Any other measurement is audited and
     settles nothing.
5. **No data is inconclusive (ADR 026 read for measurements).** An empty
   result, a NaN or `ErrUnsupported` from a verifier is `inconclusive`, never
   zero and never a pass. A measurement taken from text somebody outside the
   deployment wrote is `TrustThirdParty` (ADR 021) and cannot settle a
   required criterion.
6. **Targets belong to their author.** Targets, baselines (fixed, or
   `at_start` measured once and frozen), reducers, sample counts and sustain
   budgets are fixed at create time. Only a person changes them, through the
   API. The change is audited and restarts the criterion's sustain window.
   When a pass changes the paths a verifier declares as its own definition,
   the criterion is `inconclusive: measure_changed`. A relative target whose
   baseline is absent is `inconclusive: no_baseline`. The baseline is never
   approximated from another commit or a later measurement, and its value is
   shown beside the target.
7. **Time lives in reconcile (ADR 015, rule 8).**
   - The loop evaluates once and converges.
   - Reconcile keeps a per-criterion `sustain` budget beside `ScoreStreak`:
     `sustain.window` (rolling, never calendar), `sustain.budget` (the share
     of passes in the window that may end `not_met`) and
     `sustain.min_passes`, below which the sustain state is
     `inconclusive: too_few_samples`.
   - Only `not_met` on a required criterion counts towards `BreakerFailures`
     and spends budget. An `inconclusive` pass is left out of both counts.
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
- **An all-optional objective can no longer complete vacuously.** The
  verify.go:37 guard changes from counting all criteria to counting required
  ones, and create rejects such an objective.
- **A recovered criterion is seen.** Sustain spends a budget over a rolling
  window instead of counting a streak, so one `not_met` pass does not keep a
  criterion broken for N more passes. A thin window reads
  `inconclusive: too_few_samples`, not healthy.
- **A check that never reports cannot pass or hang.** A skipped, neutral or
  unreported verifier is `not_run` after a bounded wait, and a stale or
  foreign result settles nothing.
- **A relative target can wait on a person.** With no baseline it stays
  `inconclusive: no_baseline` until a person sets one, so the digest must
  show it.
- **Judged criteria cost their author a line.** Each names an observable
  outcome and its `evidence`, and existing templates' judged criteria must
  gain one.
- **The status endpoint is not covered.** `POST /objectives/{id}/status` can
  still write `completed` outside the loop (fact 6). This ADR does not decide
  that path.

## Alternatives rejected

- **A fourth `error` state.** Argo keeps Error apart from Failed and
  Inconclusive, but its pages do not say what causes an Error or how it
  counts. What the gate and the breaker need is that an error is told apart
  from `not_met`, and the `verifier_error` reason does that.
- **Treating skipped as passing, or waiting forever.** GitHub does both: an
  `if`-skipped job reports success and a path-skipped workflow stays pending.
  `not_run` after a bounded wait is neither.
- **Settling criteria from how the run ended.** A run that stopped at its
  limit has ended, not succeeded. AutoGen and LangGraph send both through one
  path and leave the difference to a free-text reason.
- **Feeding a verifier error back as a failed validation.** CrewAI's
  guardrail does this and spends a retry on an outage.
- **`sustain: N` consecutive passes.** A streak is only the long-window half
  of multiwindow alerting and keeps a recovered criterion broken until N more
  passes. Burn-rate alerting is not adopted either; the breaker stays the
  alert.
- **Approximating a missing baseline.** Codecov's pseudo-comparison
  approximates a missing base, and users report "Missing base commit".
- **Binding prose to code by matching text.** Gherkin matches steps by text,
  and an undefined step was skipped and then counted as passed.
- **Capping the number of criteria.** The OKR guide's three key results is
  advice, not a rule.

## Sources

From the research report, as revised by its second pass:

- D1: section 1, Argo and Kayenta, and its second pass on Argo's Error,
  Failed and Inconclusive; section 2, Keptn's no-data history; section 3's
  takeaways; section 4, Inspect and SWE-bench; section 5, CrewAI #7150,
  AutoGen termination and LangGraph `END`; section 7.
- D2: section 1, Kayenta's `critical` and Argo's `dryRun`; section 2, Keptn's
  `keyObjective`; section 4, promptfoo and SWE-bench; section 5, AutoGen
  takeaway 3; section 6, re:Work OKRs; Code reading fact 6.
- D3: section 1, Flagger and Argo; section 2, Keptn's objectives; section 3,
  Codecov and SonarQube; section 4, Inspect's `Score`; facts 1, 2 and 5.
- D4: section 1, Argo and Kayenta; section 2, SRE workbook (takeaway 4),
  OpenSLO and Keptn; section 3, Codecov and SonarQube; section 4, Inspect;
  section 7, Luo et al.
- D5: section 1, Argo; section 2, SRE workbook (error budgets, multiwindow
  alerting, too few events) and Pyrra; section 7; facts 4 and 5.
- D6: section 2, OpenSLO's README; section 4, promptfoo; section 6, Gherkin
  and re:Work OKRs.
- D7: section 3, GitHub required status checks; section 4, SWE-bench and
  Inspect; section 5, LangGraph, AutoGen and AutoGen #6826; section 6,
  Gherkin; section 7, Luo et al.; fact 1.

## What was read

- **Read:** the research report's sections 1 to 7, its Code reading
  including fact 6, and D1 to D8 as revised by its second pass; Phase 38 in
  full; and the head of ADR 028.
- **Not read:** OpenSLO's `timeWindow` and `budgetingMethod` pages, so the
  rolling window rests on the SRE workbook alone, and Keptn v1's comparison
  with previous evaluations (report section 2). Also not read: the full text
  of ADRs 015, 021, 022 and 026 for this ADR (the report's summaries and
  AGENTS.md rules 8 to 10 were relied on).
