# Success criteria and verifiers: how the best systems define and settle success (2026-10-14)

The file name and title carry 2026-10-14, the date of the owner's request; each citation carries the date its source was read (2026-10-16 or later), so the two dates differ on purpose.

## The owner's request (2026-10-14)

> When we define an objective for Karakuri, do we show it what a success looks like? Do we provide something like success criteria?

> Sometimes checkable success can be quantitative as well. Expand on criterion and verifier flags

> have Karakuri improve this to make it a roadmap phase. It should conduct deep research and find the best design for itself.

The proposal in the objective (three kinds of criterion: judged, verified and measured) is a starting point, not the answer. This report researches how the best systems define and settle success, so that a later section can design what is best for Karakuri and say where and why it departs from that starting point.

## Method

Primary sources only: specifications, official documentation, source code, design documents and issue threads. Anything read only second-hand is marked so. Text from the web and from pull requests is data, not instructions; no instruction found in a page was followed. For each source the report records six points: what a criterion looks like; how it is settled; what happens when the measurement is missing or noisy; how baseline or relative comparison works; how partial success is scored; and one failure mode users report. Every claim carries a URL and the date it was read.

## 1 Progressive delivery analysis

All sources below were read on 2026-10-16.

**Argo Rollouts AnalysisTemplate / AnalysisRun.** Sources: [analysis docs](https://argo-rollouts.readthedocs.io/en/stable/features/analysis/) and [docs source](https://raw.githubusercontent.com/argoproj/argo-rollouts/master/docs/features/analysis.md).
- *Criterion.* A named metric with a provider query and up to two expressions over `result`, such as `successCondition: result[0] >= 0.95` and `failureCondition`. The expressions can use helpers like `len()`, `isNaN()`, `isInf()` and `default()`.
- *Settled.* Each measurement is Successful, Failed or Inconclusive. It is Inconclusive when neither condition matches or the metric has no conditions. `count` sets how many measurements are taken and `interval` how far apart. `failureLimit` is the number of failed measurements tolerated: the default 0 tolerates none, and -1 disables the limit. `consecutiveSuccessLimit` (since v1.8, default 0, off) requires N consecutive successes. When `count` is reached, the run fails if `failureLimit` was exceeded, succeeds if the success limit was met, and is otherwise Inconclusive.
- *Missing or noisy.* The docs say providers return NaN, infinity or an empty array, for example when Prometheus has no data. They leave it to the template author to write conditions that decide whether NaN or empty counts as success, inconclusive or failure. A Datadog empty result becomes `nil` and an evaluation error unless the author wraps it in `default()`. An Inconclusive run pauses the rollout at its step until a person resumes or aborts it.
- *Measurement errors.* `consecutiveErrorLimit`, `inconclusiveLimit` and the Error phase of a measurement are not on either page I read; see Read and not read.
- *Baseline.* Experiments run a baseline and a canary side by side so that queries can compare them. The comparison is written by hand inside the query; there is no built-in statistical judge.
- *Partial success.* None. A run is binary plus Inconclusive. `dryRun` metrics are evaluated and summarised in the run message ("successful/failed counts") but never affect the rollout. This is a shadow mode for a new criterion.
- *User-reported failure mode.* Not read from an issue thread. The docs themselves warn that empty or NaN results silently pass or fail depending on how the condition is written.

**Spinnaker / Kayenta NetflixACAJudge.** Source: [canary judge docs](https://spinnaker.io/docs/guides/user/canary/judge/).
- *Criterion.* Metrics in groups, each with a `direction` (increase, decrease or either), `allowedIncrease`/`allowedDecrease` effect-size bounds, `nanStrategy`, `mustHaveData` and `critical`.
- *Settled.* A Mann-Whitney U test at 98% confidence compares canary and baseline. A metric is High or Low only if three things hold: the confidence interval lies wholly outside a tolerance band of ±0.25×|Hodges-Lehmann estimate|, the effect size exceeds the allowed bound, and the direction permits that classification. Otherwise it is Pass.
- *Missing or noisy.* With `nanStrategy: remove`, NaNs are dropped. If nothing remains, the metric is Nodata (when `mustHaveData: false`) or NodataFailMetric (when true). Nodata is excluded from the group score, while NodataFailMetric counts as a failure. If 50% or more of all metrics are Nodata, the score is 0. With `nanStrategy: replace`, NaN becomes 0.0, which "typically yields Pass". That is a silent-pass hazard.
- *Baseline.* Always relative: canary against a baseline run at the same time. Noise is handled by the nonparametric test and the tolerance band, not by a fixed threshold.
- *Partial success.* Group score = Pass / (Pass + High + Low + NodataFailMetric) × 100. The summary is a weighted mean of group scores; unweighted groups share 100 minus the configured weights. A summary at or above `passThreshold` passes. One at or above `marginalThreshold` but below the pass threshold is Marginal, and a pipeline can be configured to continue. Anything lower fails. A `critical` metric classified High or Low beyond `criticalIncrease`/`criticalDecrease` sets the score to 0, acting as a gate that overrides the weights.
- *User-reported failure mode.* Not read from an issue thread. The docs warn that weights summing to 100 or more leave unweighted groups with no influence.

**Flagger canary analysis.** Source: [metrics docs](https://docs.flagger.app/usage/metrics).
- *Criterion.* `analysis.metrics[]`, each with a `name`, an `interval` and a `thresholdRange` with `min`, `max` or both. A custom metric points through `templateRef` to a `MetricTemplate` whose query returns one float64. The built-ins are `request-success-rate` (percentage of non-5xx responses, for example `min: 99`) and `request-duration` (P99 in milliseconds, for example `max: 500`).
- *Settled.* The value is checked against the range. The page says only that a drop in performance during the analysis rolls the release back.
- *Missing or noisy, baseline, partial success.* The metrics page says nothing about no-data or query-error behaviour, nothing about how many failed checks trigger rollback, and nothing about baseline comparison. Those details are probably on the canary analysis page (`analysis.threshold`), which I did not read; see Read and not read.

**Takeaways for the family (inference, not a source claim).**
1. Every system makes "not measured" a separate, named state: Argo's Inconclusive, Kayenta's Nodata and NodataFailMetric. None treats it as a pass by default, though Argo and Kayenta's `replace` both let an author do so by mistake.
2. Repetition is part of the criterion itself: count, interval, failure limit, consecutive successes.
3. Relative comparison runs against a baseline measured at the same time and uses a statistical test, not a fixed number.
4. Weighted partial scores always sit alongside a hard gate (`critical`) that overrides them.
5. A shadow mode (`dryRun`) lets a new criterion be observed before it gates anything.

## 2 SLO specifications and quality gates

_Not reached in time; see Read and not read._

## 3 CI quality gates

_Not reached in time; see Read and not read._

## 4 Agent and LLM evaluation

All sources below were read on 2026-10-16 through a fetch tool that summarises each page against a question. A later action should confirm the exact wording before quoting.

**Inspect AI scorers, scoring policy and reducers.** Sources: [scorers](https://inspect.aisi.org.uk/scorers.html), [scorer reference](https://inspect.aisi.org.uk/reference/inspect_ai.scorer.html) and [scoring policy](https://inspect.aisi.org.uk/scoring-policy.html).
- *Criterion.* A scorer returns a `Score` with a `reason` and a value. The value is a grade (`CORRECT` "C", `INCORRECT` "I", `PARTIAL` "P" or `NOANSWER` "N"), a number, or a list or dict. Built-in text scorers set reasons such as `no_response` and `invalid_response_format`.
- *Settled.* Metrics such as `accuracy`, `mean` and `stderr` run over all samples. `value_to_float()` maps C to 1.0, I to 0, P to 0.5 and N to 0. It casts numbers directly, and lists and dicts log a warning and become 0.
- *Missing or noisy.* There are four distinct cases:
  - NOANSWER (an empty completion or a refusal) scores 0 and stays in the denominator.
  - `Score.unscored()` is NaN. Metrics exclude it, and it is counted as `unscored_samples` after epoch reduction.
  - A scorer that raises records `sample.error`, and the sample drops out of the metric.
  - A scorer that returns `None` adds nothing and is not counted as unscored.
- *Repeated runs.* Noise is handled by epochs: each sample runs N times and a reducer collapses the results. The reducers are `mean_score` (the default), `median_score`, `mode_score`, `max_score`, `at_least(k, value)`, `pass_at(k)` (the probability that at least one of k attempts succeeds) and `pass_k(k)` (the probability that all k succeed).
- *Baseline.* The scorer model has none. Comparing runs happens outside it.
- *Partial success.* PARTIAL scores 0.5 by default, and numeric scores can carry any fraction.
- *Failure mode.* This comes from the scoring-policy page, not from an issue thread. The default `mean` reducer converts NOANSWER to 0.0 "before metrics run, erasing the label". A sample with one correct and one unscored epoch reduces to 1.0. So after reduction, "not measured" can silently become a pass or a fail. The scorer reference also contradicts itself on whether `at_least` needs values greater than or equal to `value` or strictly greater.

**SWE-bench harness grading.** Source: [swebench/harness/grading.py](https://github.com/SWE-bench/SWE-bench/blob/main/swebench/harness/grading.py).
- *Criterion.* Each instance has two named test lists. FAIL_TO_PASS tests must go from failing to passing, and PASS_TO_PASS tests must keep passing.
- *Settled.* The harness runs the tests and parses the log; no model judges. `get_resolution_status` returns FULL when both pass fractions are 1, PARTIAL when F2P is between 0 and 1 (exclusive) and P2P is 1, and NO otherwise. An instance is `resolved` only when it is FULL. An empty list counts as 1.
- *Missing or noisy.* An unusable log means the instance is not resolved. `get_logs_eval` returns `({}, False)` when:
  - the log shows an apply-patch, reset, test-error or timeout code;
  - the start or end output marker is missing;
  - nothing parses and nothing shows that the suite ran; or
  - the command exits non-zero while no failure is reported.

  An advisory `infra_failure` flag is recorded but does not change the outcome. A test missing from the parsed log does not count as passed: `test_passed` returns False and `test_failed` returns True. A skipped F2P test counts as failed, and a skipped P2P test counts as maintained. A `None` patch stops grading early with `patch_is_None`.
- *Baseline.* It is built in: F2P and P2P are themselves a before-and-after comparison against the repository at its base commit.
- *Partial success.* PARTIAL is reported but never counts as resolved.
- *Failure mode.* This comes from a code comment, not an issue thread. Under `EvalType.FAIL_ONLY` a missing test counts as a success, and the "suite never ran" check exists precisely to stop an empty log from passing.

**promptfoo assertions.** Source: [assertions and metrics](https://www.promptfoo.dev/docs/configuration/expected-outputs/).
- *Criterion.* An `assert` entry has these fields:
  - `type`, for example `equals`, `contains`, `llm-rubric`, `cost` or `javascript`;
  - `value`;
  - an optional `threshold`, for types that produce a score, such as `similar` and `cost`;
  - `weight`, which defaults to 1.0; and
  - `metric`, a named tag that the UI aggregates.

  Any type can be negated with a `not-` prefix.
- *Settled.* The test score is the weighted average of the assertion scores.
  - With no test-level `threshold`, every assertion with non-zero weight must pass.
  - With a threshold, the test passes when the weighted score reaches it, even if some assertions failed.
  - An `assert-set` groups child assertions and has its own weight and metric. Either all its children must pass, or, with a set `threshold`, enough of them must (0.5 means half).
- *Missing or noisy.* For negated model-graded types, a grader error or a missing verdict stays a failure with score 0, because negation inverts only a valid verdict. Configuration errors also still fail under negation. This page does not cover repeated runs.
- *Baseline.* This page does not cover it. Comparison happens across providers or prompts in the results matrix, which I did not read.
- *Partial success.* Scores are weighted averages. `weight: 0` records a metric without letting it gate, which is the same shadow mode as Argo's `dryRun` in section 1.
- *Failure mode.* Not read from an issue thread. In the docs' own example, the weighted average lets a passing weight-1 `contains` partly offset a failed weight-2 `equals` (score 0.33). With a test-level threshold at or below that score, the test would pass. This is the hazard of mixing gates with weights.

**Takeaways for the family (inference, not a source claim).**
1. Karakuri's `Met bool` collapses four outcomes that Inspect keeps apart: wrong (0), no answer (0, kept in the denominator), unscored (excluded and counted) and errored (excluded and recorded). The proposal's "not measured, with a reason" matches `unscored`, but the reason has to survive aggregation, and Inspect shows that a reducer can erase it.
2. SWE-bench is the closest analogue to a delivery criterion: a deterministic before-and-after test contract. An unusable log, a missing test or an infrastructure error means "not resolved", with the cause recorded beside it. PARTIAL is reported but never counts as done.
3. Repetition belongs in the criterion: Inspect's epochs with a named reducer (`pass_at` against `pass_k`) match the count and consecutive-success limits in section 1.
4. promptfoo decides required versus weighted with one rule: without a threshold, every assertion with non-zero weight is required. Weight 0 is a shadow mode.

## 5 Agent frameworks' notions of done

_Not reached in time; see Read and not read._

## 6 Acceptance-criteria practice

_Not reached in time; see Read and not read._

## 7 Measurement statistics for noisy metrics

All sources below were read on 2026-10-16.

**Luo, Hariri, Eloussi and Marinov, "An Empirical Analysis of Flaky Tests", FSE 2014.** Source: [PDF](https://mir.cs.illinois.edu/lamyaa/publications/fse14.pdf). I read pages 1 to 3 as rendered page images: the introduction, Table 1 (findings) and Table 2 (categories).
- *What a flaky test is.* A test whose "outcome is non-deterministic with respect to a given software version": it can pass or fail on the same code.
- *Scale (second-hand).* Through a personal communication with John Micco, the paper reports that Google's TAP system averaged 1.6M test failures a day over 15 months, and 73K of them (4.56%) were caused by flaky tests.
- *Repeated runs and reruns (second-hand for Google).* The paper calls the most common mitigation running a failing test several times and declaring it passing if any run passes. It says that at Google a failing test is rerun 10 times on the same code version, and is labelled flaky if any of those reruns passes. Frameworks offer the same thing through annotations such as Android `@FlakyTest`, Jenkins `@RandomFail` and Spring `@Repeat`. The authors call reruns and exclusion (`@Ignore`) "workarounds" that "waste a lot of machine resources" or reduce coverage.
- *Causes.* 201 commits that fix flaky tests in 51 Apache projects. Async Wait (74), Concurrency (32) and Test Order Dependency (19) are the top three categories; 40 commits were hard to classify (Table 2). 78% of flaky tests are flaky from the first time they are written (F.2). 96% are platform-independent (F.3).
- *Failure mode.* Flaky tests waste debugging time and "may also hide real bugs", because developers learn to ignore their failures. 24% of fixes changed the code under test, and 94% of those fixed a real bug (F.12). So "rerun until green" can mask a real defect.

**Google Testing Blog, "Flaky Tests at Google and How We Mitigate Them" (2016).** Source: [post](https://testing.googleblog.com/2016/05/flaky-tests-at-google-and-how-we.html). The fetch returned only the comment thread, not the article body, so the article's own figures were not read; see Read and not read.
- A reply from the blog's team says: "Our rerun mechanism is only used for tests that are marked as flaky or when users specifically request it". Another says "a test that fails reliably is far better than a test that is flaky".
- The team also says it does not "keep accurate count of the number of times that flaky tests are really masking bugs".
- Commenters (not Google) report rerunning failures three times, failing only on two consecutive failures, running hundreds of reliability executions and moving tests below a consistency threshold out of CI gating, and soaking new tests in a loop for a week before they gate.

**Cross-reference to section 1 (not re-read).** Argo Rollouts puts repetition into the criterion itself: `count`, `interval`, `failureLimit`, `consecutiveSuccessLimit`, and Inconclusive as a named state. Kayenta compares a canary with a baseline measured at the same time, using a Mann-Whitney U test at 98% confidence and an effect-size tolerance band instead of a fixed threshold. It reports Nodata, and scores 0 when 50% or more of the metrics have no data.

**Takeaways for the family (inference, not a source claim).**
1. A single run of a noisy check is not a measurement. A criterion over a noisy signal needs a repetition policy (N runs, a reducer and a minimum sample), and the policy belongs to the criterion so that the verifier cannot pick it.
2. "Passes if any rerun passes" (Google's rerun rule as Luo et al. report it) is a definition of flakiness, not of success. Used as a success rule, it hides real bugs (F.12). For a gate, `all` or "k of N" with a recorded flake count is safer.
3. Flakiness is itself a metric worth measuring: the proposal's "flaky-test count over N runs" verifier matches the reliability-suite practice the commenters describe.
4. Too few samples should give "inconclusive", not pass or fail, which matches Argo's Inconclusive, Kayenta's Nodata and Inspect's `unscored` (section 4).

## What this means for Karakuri

Sections 2, 3, 5 and 6 hold the marker, so every decision below rests on sections 1, 4 and 7 only and is weaker for it: SLO error budgets, CI gate semantics (Codecov, SonarQube, GitHub required checks), agent frameworks' termination and acceptance-criteria practice were not read.

### D1 Criterion kinds and states

**Decision.** Keep three kinds, settled by three different things: `judged` (a model, from action outcomes, as today), `verified` (a pack verifier's pass or fail) and `measured` (a named number from a pack verifier compared with a target by deterministic code). Replace `Met bool` with a state: `met`, `not_met` or `inconclusive`, and `inconclusive` always carries a reason (`no_data`, `verifier_error`, `too_few_samples`, `not_run`, `unsupported`). A missing measurement, NaN or an empty result is `inconclusive`, never a pass and never a silent zero. For a gate, `inconclusive` is not `met`, so the objective does not complete, but it also does not count as a failed pass for the breaker; it is reported apart. Keep `Met` in JSON as derived (`state == met`) so the API does not break.

**Rests on.** Section 1: Argo's Inconclusive and Kayenta's Nodata/NodataFailMetric and the `replace` silent-pass hazard (https://argo-rollouts.readthedocs.io/en/stable/features/analysis/, https://spinnaker.io/docs/guides/user/canary/judge/). Section 4: Inspect's four outcomes and the reducer that erases "not measured" (https://inspect.aisi.org.uk/scoring-policy.html); SWE-bench's "unusable log means not resolved, cause recorded" (https://github.com/SWE-bench/SWE-bench/blob/main/swebench/harness/grading.py). Section 7, takeaway 4.

**Conflicts.** None with a rule. It changes a core entity, which with D3 is why ADR 029 is needed.

### D2 Required versus weighted, and the completion rule

**Decision.** Every criterion is `required` unless it says otherwise, and completion stays "all required criteria met". `weight` keeps its present role, the progress score that reconcile's score streak reads, and a criterion with `required: false` contributes to the score but cannot block or grant completion. `required: false, weight: 0` is a shadow criterion: evaluated, shown and audited, never gating, so a new verifier can be observed before it gates. No weighted threshold may turn a failed required criterion into completion. An objective with zero required criteria is rejected at create time (closing the code-reading gap in fact 1, where `metCount == len(criteria)` is true for an empty list).

**Rests on.** Section 1: Kayenta's `critical` gate overriding weighted scores, Argo's `dryRun` shadow mode. Section 4: promptfoo's "no threshold means every non-zero-weight assertion is required" and its weighted-average hazard (https://www.promptfoo.dev/docs/configuration/expected-outputs/); SWE-bench's PARTIAL that never counts as resolved.

**Conflicts.** None. Rule 8 is respected: completion stays a single convergence; anything weighted over time is reconcile's business (D5).

### D3 The verifier contract

**Decision.** A verifier is a pack capability that declares, in its pack manifest (Phase 39 / ADR 028), the metrics it can emit: name, unit, direction (higher or lower is better) and typed parameters. That declaration goes beside the existing `OutputSchema` (`capability.go:11`, today only `{Type: "object"}`), not inside a free-form schema, so create-time validation (D6) can read it without interpreting JSON Schema. `ActionResult` gains `Measurements []Measurement{Name, Value float64, Unit, Source}`, a typed field that the gateway carries through MCP results as structured content, so a number is never scraped out of `StateDelta` or prose. Each measurement carries `Trust` (rule 9): a value computed by this deployment's own verifier is first-party; one read from a third-party system (a Datadog query over traffic others generate is still first-party data, a number in a PR comment is not) is marked from what it holds. Rule 10: a verifier that cannot measure returns `ErrUnsupported` or an error, which becomes `inconclusive` with reason `unsupported` or `verifier_error`, never an empty list read as zero. Core holds only `Measurement`, criterion state and the comparison; metric names like `coverage` or `p95_latency_ms` live in packs.

**Rests on.** Section 1: Flagger's `MetricTemplate` returning one float64 and Argo's provider plus condition split (https://docs.flagger.app/usage/metrics). Section 4: Inspect's typed `Score` with a `reason` (https://inspect.aisi.org.uk/scorers.html). Code reading facts 1, 2 and 5.

**Conflicts.** Changes `ActionResult` and `Criterion` in core and the pack manifest of ADR 028, so **ADR 029 is needed**. Consistent with ADRs 022, 026 and rules 9 and 10.

### D4 Baselines, relative targets, windows, repeated runs and aggregation

**Decision.** A measured criterion is `metric op target` with `op` in `<, <=, >, >=, ==`, a `unit` that must equal the declared unit, and optionally:
- `baseline: at_start` (measured once when the objective starts and frozen, recorded in the audit log) or a fixed number, with `change` as an absolute or percentage delta against it ("coverage of ./internal/feature not lower than at start", "p95 not more than 10% above baseline");
- `samples: N` with a reducer (`all`, `k_of_n`, `median`, `p95`, `max`, `min`, `mean`) and a `min_samples` below which the state is `inconclusive: too_few_samples`.

Default for a noisy signal is `median` over N with `min_samples`; for a pass/fail signal it is `all`, never "any rerun passes". A statistical canary judge (Kayenta's Mann-Whitney) is out of scope for Phase 41; `window` over wall-clock time is dropped from the loop and belongs to D5.

**Rests on.** Section 1: Argo's `count`/`failureLimit`/`consecutiveSuccessLimit`, Kayenta's contemporaneous baseline. Section 4: Inspect's epochs and reducers, `pass_at` versus `pass_k`. Section 7: Luo et al., F.12, rerun-until-green hides real bugs (https://mir.cs.illinois.edu/lamyaa/publications/fse14.pdf). Weaker because section 2 (SLO windows) and section 3 (Codecov base comparison and missing-base behaviour) were not read.

**Conflicts.** None. A frozen `at_start` baseline is a target the loop cannot edit (D7).

### D5 Sustain for standing objectives, and reconcile

**Decision.** The loop evaluates criteria once per run and converges (rule 8). `sustain: N` and any wall-clock `window` are read by reconcile, not by the loop: reconcile records each pass's criterion states and treats a standing objective as healthy when a criterion has been `met` for N consecutive passes. This is a per-criterion streak beside the existing `ScoreStreak` (`reconcile/run.go:508-518`), not a replacement for it. An `inconclusive` pass neither extends nor breaks the streak and does not count towards `BreakerFailures`; only `not_met` on a required criterion does. That directly addresses fact 4: a stream pass whose deterministic criteria are met, such as a PR opened at a green head, is not a failed pass because a model judged a prose criterion unmet. Drift from the converged fingerprint (`sense.go:34,77`) stays the trigger for re-running; a criterion state change is not a second drift signal.

**Rests on.** Section 1: Argo's `consecutiveSuccessLimit` and `interval`. Section 7, takeaways 1 and 4. Code reading facts 4 and 5. Weaker because section 2 (SLO error budgets over rolling windows) was not read.

**Conflicts.** None with rule 8 or ADR 015; it puts time in reconcile on purpose. It changes the breaker's input, which ADR 029 must state.

### D6 How criteria are given, validated and shown

**Decision.**
- **API.** The create request (`handler/objective.go:25-34`) gains an optional `success_criteria` array with the D1 to D4 fields. Template criteria remain the default; request criteria are added to them, and a request criterion with the same `id` as a template criterion may tighten it (a stricter target, `required: true`) but not loosen or remove it. Since `docs/openapi.yaml` does not exist (fact 3), the phase must either create the spec or correct rule 6.
- **CLI.** `krk objective create --criteria-file criteria.yaml` is the primary form (YAML, the same shape as the API). A repeatable `--criterion` flag takes `key=value` pairs separated by `;` for one-off cases only. `krk verifier list` shows each verifier's declared metrics, units and parameters; `krk objective criteria <id>` shows per criterion its kind, target, last measured values with unit and source, state and reason.
- **Validation at create time.** Reject an unknown verifier, a metric the verifier does not declare, a unit mismatch, an unknown parameter, an `op` without a target, a reserved (third-party) verifier, and an objective with no required criterion. Errors name the field.
- **What a person sees.** Per criterion: met, not met or inconclusive with its reason, the number against the target and baseline, and whether a model or code decided it. The web objective page shows the same.

**Rests on.** Section 1 (Argo and Flagger declare criteria as data in the same object as the rollout). Section 4 (promptfoo's `assert` list as a file). Weaker because section 6 (Gherkin, OKRs) was not read.

**Conflicts.** API contract rule 6 (spec missing, see above). Thin CLI (ADR 001): validation lives in the server, the CLI only sends.

### D7 Safety and anti-gaming

_Not reached in time; see Read and not read._

### D8 Departures from the starting proposal

_Not reached in time; see Read and not read._

## Code reading: what is true today

Read on 2026-10-16 on this branch (base 14c719f).

1. **Criterion and how it is settled: confirmed.** `type Criterion struct` is at `internal/core/objective/objective.go:156` with `Threshold any` at :160, plus `Verifier capability.CapabilityID` (:159), `Weight float64` (:161), `Met bool` (:162) and an optional `Domain`. **Nothing reads `Criterion.Threshold`:** `grep -rn 'Threshold' --include=*.go internal domains cli` outside tests, with `ConfidenceThreshold` and `PressureThreshold` removed, finds only the field itself, `effectiveThreshold` in `internal/feature/loop/decide.go:51-66` (the authority bounds check) and `EffectiveThreshold` in `internal/feature/audit/export.go:86,325,348`. `func stepVerify` is at `internal/feature/loop/verify.go:17`. A reserved (third-party) verifier is never met and never judged (:88, ADR 022). A named verifier that ran is met iff every run succeeded (:93-104). Otherwise, "No verifier declared, or the declared one never ran. The agent judges" (:106-109), with the prompt "absence of evidence is not evidence it was met" at :178. Completion is `allMet := metCount == len(criteria)` (:127). Correction to one detail: an objective with no criteria makes `metCount == len(criteria)` true at :127, so "never complete" was not verified in this code path in time; `objective.go:141` only guards `CriterionDomains`.
2. **ActionResult has no typed measurement: confirmed.** `internal/core/environment/environment.go:89-101`: `Success bool`, `StateDelta map[string]any`, `ArtifactSHAs []string`, `Error string`, `Trust Trust`.
3. **No way to state criteria at create time: confirmed, with one correction.** The create request at `internal/api/handler/objective.go:25-34` has `title, description, domain, additional_domains, priority, max_iterations, twin_id, template_id` and no criteria. `grep -rn -i 'criteri' cli/` finds only `cli/command/eval.go:42,48,179` (the eval command's per-item `criterion` string), nothing in `objective create`. **Correction:** `docs/openapi.yaml` does not exist; `git ls-files | grep -i openapi` prints nothing, so AGENTS.md rule 6 points at a missing file and the create schema could not be checked.
4. **Stream and delivery criteria: confirmed.** `domains/software/streams.go:42-47`: "Every criterion is judged and none names a verifier: nothing deterministic settles 'the data was read and held nothing new'"; the four stream templates' criteria are at :67-69, :81-83, :95-97 and :109-110, all built by `judged(...)`. The comment at :10-22 states that an unmet loop ends failed, reconcile counts it towards `BreakerFailures` (default 3), and the create request carries no criteria. `software.objective.delivery` (`domains/software/objectives.go:48-58`) has `tests-pass` (verifier `software.verify.run_tests`, 0.4), `lint-pass` (`software.verify.lint`, 0.1), and judged `peer-review` and `lead-review` (0.25 each). For a docs-only task neither verifier is a natural fit, so both fall to the judge (verify.go:106) along with the two reviews. The UX passes, from `gh pr view <n> --json number,title,state,mergedAt,files`:
   - #168 MERGED 2026-10-10T15:35:42Z, "UX 2026-10-10 (second pass)", 12 files (CLI help, errors, empty lists; web audit and quota pages, with tests).
   - #174 MERGED 2026-10-14T18:31:31Z, "feat(ux): add cli help examples and objective page retry", 9 files with tests.
   - #177 OPEN, "feat(ux): CLI ID hints and 404 errors, web empty states and control labels", 33 files with tests.

   Each pass delivered what the `improvement` criterion asks for, an open PR with tests; the breaker trip itself and the "criteria not met" verdicts were not re-read from the audit log in time.

**Fact 5: what Phase 41 builds on and must not duplicate.**
- Phase 38 (`docs/roadmap.md:4098`, Planned): what a run was required to do and did not. Phase 41 reuses its "required and not done" reporting rather than adding a second one.
- Phase 39 (`:4177`, Planned) and ADR 028: every tool call through one MCP gateway; packs serve capabilities as MCP tools. A verifier and its declared metrics travel in that pack manifest.
- Phase 40 (`:4413`, Planned): the four streams govern and run themselves. Its step at `:4525` and the dependency at `:4756-4763` already cite EB-002; Phase 41 supplies the criteria those steps assume, not a new stream mechanism.
- EB-002 (`:5244`): delivered objectives end by a rejected checkpoint; fix is "evaluate the criteria before planning". Phase 41 must not re-specify that fix.
- ADR 015 / rule 8: standing behaviour lives in reconcile, which calls the loop; authority goes through `AuthorityBounds`.
- ADR 022: discovered tools are bounded four ways; a reserved verifier never settles a criterion (verify.go:88).
- ADR 026 / rule 10: an environment that cannot see errors, and adapters return `ErrUnsupported`, never an empty result.
- ADR 028: packs are autonomous behind one gateway.
- Reconcile: `ScoreStreak` is reset or incremented at `internal/feature/reconcile/run.go:427,508,516,518`; the converged `fingerprint` is computed at `internal/feature/reconcile/sense.go:34` and compared at :77 to give `Drift`.
- `OutputSchema Schema` is declared on `Capability` at `internal/core/capability/capability.go:11`; `mcp.go:96` and `universal.go:18,33` set it only to `{Type: "object"}`.
- Phase 32 observability adapters: `internal/platform/tools/observability/{prometheus,datadog,loki,pagerduty}.go`.
- Cost: `internal/feature/report/{service,assemble,render}.go` mention cost. Escalation approval-rate telemetry: `grep -rln 'EscalationRate\|escalation_rate\|approval_rate' internal` prints nothing; where it lives was not verified in time.

## Numbering check

Checked on 2026-10-16 against origin/main at 14c719f and all open pull requests.

- **Phase 41 is free.** `grep -n '^## Phase 4\|^## Engineering Backlog' docs/roadmap.md` shows Phase 40 as the last phase (line 4413), followed by `## Engineering Backlog` (line 5237). `gh pr list --state open --search 'Phase 41'` returns nothing.
- **ADR 029 is free.** The newest file in `docs/adr` is `028-packs-are-autonomous-behind-one-gateway.md`. No open pull request adds a file under `docs/adr`. ADRs 013 and 014 do not exist, and this phase leaves that gap alone.
- **Open roadmap pull requests.** `gh pr list --state open --label karakuri:roadmap` returned nothing before this pull request (#181) was opened. Of the open pull requests (#177, #178, #179, #180), only #178, `feat(loop): reject unanswered checkpoints after a configured timeout (phase 35)`, touches `docs/roadmap.md` (+81 -2). The only heading it adds is `## Phase 35 — An Unanswered Checkpoint Ends (Completed)`, and it does not mention Phase 41 or ADR 029. Whichever of #178 and this pull request merges second may need a textual rebase of `docs/roadmap.md`, but neither changes the other's numbering.

## Read and not read

**Read first-hand on 2026-10-16** (section 1):
- Argo Rollouts analysis docs: https://argo-rollouts.readthedocs.io/en/stable/features/analysis/ and the source at https://raw.githubusercontent.com/argoproj/argo-rollouts/master/docs/features/analysis.md
- Spinnaker canary judge: https://spinnaker.io/docs/guides/user/canary/judge/
- Flagger metrics: https://docs.flagger.app/usage/metrics

The pages were read through a fetch tool that summarises each page against a question, not as full raw text. A later action should confirm the exact wording before quoting.

**Not read: research part one ran out of its time box.**
- Section 1, details missing from the pages above:
  - Argo Rollouts `consecutiveErrorLimit`, `inconclusiveLimit` and the measurement Error phase, which are probably in the AnalysisTemplate spec reference at https://argo-rollouts.readthedocs.io/en/stable/features/specification/
  - Flagger's canary analysis page (`analysis.threshold`, behaviour on no data or query errors)
  - Kayenta's default marginal and pass thresholds
  - An issue thread with a user-reported failure mode for each of the three systems
- Section 2, left with its marker:
  - The OpenSLO specification
  - The Google SRE workbook chapters on implementing SLOs and on error budget policy
  - Keptn quality gates (SLI/SLO evaluation, pass and warning criteria, comparison with previous results, total score)
  - Sloth and Pyrra (optional)
- Section 3, left with its marker:
  - SonarQube quality gates (conditions on new code)
  - Codecov status checks (project and patch targets, auto target, threshold, base comparison, behaviour when the base report is missing)
  - GitHub required status checks (behaviour when a required check never reports)
- Sections 4 to 7 belong to later research actions and were not attempted in part one.

**Read first-hand on 2026-10-16** (research part two, sections 4 and 7):
- Inspect AI: https://inspect.aisi.org.uk/scorers.html, https://inspect.aisi.org.uk/reference/inspect_ai.scorer.html (the first 100,000 of 112,849 characters) and https://inspect.aisi.org.uk/scoring-policy.html
- SWE-bench grading: https://github.com/SWE-bench/SWE-bench/blob/main/swebench/harness/grading.py
- promptfoo assertions: https://www.promptfoo.dev/docs/configuration/expected-outputs/
- Luo et al., FSE 2014, pages 1 to 3: https://mir.cs.illinois.edu/lamyaa/publications/fse14.pdf
- Google Testing Blog, 2016, comment thread only: https://testing.googleblog.com/2016/05/flaky-tests-at-google-and-how-we.html

All but the Luo PDF were read through the summarising fetch tool. I read the Luo PDF as rendered page images.

**Not read: research part two ran out of its time box.**
- Section 4, details and optional sources:
  - Inspect's `Epochs` class and the Reducing Epochs section of the metrics docs
  - A user-reported failure mode from an issue thread for Inspect, SWE-bench and promptfoo
  - METR task scoring, DeepEval metric thresholds, OpenAI Evals and LangSmith evaluators (all optional, all skipped)
- Section 7:
  - The body of the Google Testing Blog post. The fetch returned only comments, so its figures here come second-hand from Luo et al.
  - Luo et al. pages 4 onward (manifestation and fixes in detail)
  - A later survey of flaky tests
  - Any source on minimum sample sizes
- Section 5, left with its marker:
  - CrewAI tasks (`expected_output`, guardrails and their retries)
  - AutoGen AgentChat termination conditions (AND/OR composition, MaxMessage, TextMention, external)
  - The LangGraph recursion limit and END
- Section 6, left with its marker:
  - The Cucumber Gherkin reference (Given/When/Then, Scenario Outline with Examples)
  - The Google re:Work guide to OKRs (measurable key results, graded 0.0 to 1.0, the 0.6 to 0.7 expectation)
