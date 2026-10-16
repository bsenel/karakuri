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

_Not reached in time; see Read and not read._

## What this means for Karakuri

_Not reached in time; see Read and not read._

## Code reading: what is true today

_Not reached in time; see Read and not read._

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
