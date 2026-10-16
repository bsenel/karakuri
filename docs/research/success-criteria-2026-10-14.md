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

_Not reached in time; see Read and not read._

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
