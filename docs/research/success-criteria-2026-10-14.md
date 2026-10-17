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

All sources below were read on 2026-10-17 through a fetch tool that summarises each page against a question. A later action should confirm the exact wording before quoting.

### Google SRE workbook: error budgets and burn rate

Sources: [Implementing SLOs](https://sre.google/workbook/implementing-slos/), [Alerting on SLOs](https://sre.google/workbook/alerting-on-slos/) and [pyrra-dev/pyrra#1925](https://github.com/pyrra-dev/pyrra/issues/1925), all read 2026-10-17. The full example error budget policy is in the workbook's Appendix B, which was not read.
- *Criterion.* An SLI is a ratio: the page recommends "the number of good events divided by the total number of events", for example successful HTTP requests over all HTTP requests. (The summary I read says "total", not "valid"; a later action should check the exact word.) An SLO is a target on that ratio over a window. The page's starter SLOs for an API are 97% availability, 90% of requests under 450 ms and 99% under 900 ms; several latency thresholds on one SLI capture the tail (https://sre.google/workbook/implementing-slos/, read 2026-10-17).
- *Settled.* "The error budget is 100% minus the SLO": at 99.9% and 3 million requests over four weeks, 3,000 errors are allowed. The window is either rolling, which tracks user experience, or calendar, which fits business planning; the page recommends rolling windows of "an integral number of weeks" and says "a four-week rolling window" is "a good general-purpose interval", with weekly summaries and quarterly reports beside it. When the budget is spent "(or come close to exhausting it), you should do something": prioritise reliability bugs, work only on reliability until the service is back within SLO, or "a production freeze halts certain changes to the system until there is sufficient error budget to resume changes" (https://sre.google/workbook/implementing-slos/, read 2026-10-17). Alerting is settled by burn rate, "how fast, relative to the SLO, the service consumes the error budget". The recommended form is multiwindow, multi-burn-rate: for a 99.9% SLO, page at burn rate 14.4 (2% of budget) over 1 hour with a 5-minute short window, page at 6 (5%) over 6 hours with a 30-minute short window, and open a ticket at 1 (10%) over 3 days with a 6-hour short window. Both windows must exceed the rate; "a good guideline is to make the short window 1/12 the duration of the long window" (https://sre.google/workbook/alerting-on-slos/, read 2026-10-17).
- *Missing or noisy.* Neither page addresses missing or absent data. Noise is treated as a property of the alert, measured by precision ("the proportion of events detected that were significant"), recall, detection time and reset time. A plain threshold on the error rate has low precision: "A 0.1% error rate for 10 minutes would alert, while consuming only 0.02% of the monthly error budget". A single burn-rate alert has low recall: a 35x burn never alerts yet spends a 30-day budget in about 20.5 hours. The short window exists to cut reset time, so the alert stops once the burn has stopped. Low traffic is the noisy case: "a single failed request results in an hourly error rate of 10%". The page offers artificial traffic, grouping small services, client retries and fallbacks, or a lower SLO or longer window, and warns that lowering the SLO "involves a product decision" (https://sre.google/workbook/alerting-on-slos/, read 2026-10-17). Per-customer SLOs have the same problem: customers sending very few requests show either 100% or very low availability (https://sre.google/workbook/implementing-slos/, read 2026-10-17).
- *Baseline.* The page warns against setting an SLO from current performance, which "can commit you to unnecessarily strict SLOs". Current performance is acceptable as a starting point when nothing else is known and there is an iteration process; measured values are rounded down to make starter SLOs. Because an SLO set from past data is met by construction, the page suggests looking at the distribution of past days that missed it. With no history at all, a remote health checker is a low-fidelity start (https://sre.google/workbook/implementing-slos/, read 2026-10-17). There is no comparison against a concurrent baseline: the target is absolute, and the budget is what is relative.
- *Partial success.* The SLO itself is met or missed per window, and the decision matrix on the page uses met and missed outcomes. The error budget is the graded quantity: how much of it is left, and how fast it is burning, decides the response. An "aspirational SLO" is tracked without triggering the error budget policy, a shadow mode like Argo's `dryRun` (https://sre.google/workbook/implementing-slos/, read 2026-10-17).
- *User-reported failure mode.* Low-traffic SLOs page on one error. A Pyrra user saw "too few requests" messages in the UI and reported that "prometheus alerts could be triggered instantly when just single error event is received", and asked for an optional minimum request rate below which ErrorBudgetBurn alerts stay silent. The maintainer declined: "I don't think Pyrra should add a traffic threshold to the alerts", because it "hides a real problem instead of fixing it". They pointed to the existing "Too few requests!" warning, to matchers that filter low-traffic series, to `alerting.burnrates: false` with the recording rules kept, and to synthetic traffic. The issue was opened 2026-09-01 and closed 2026-10-03 (https://github.com/pyrra-dev/pyrra/issues/1925, read 2026-10-17).

**Takeaways for the family (inference, not a source claim).**
1. The error budget turns a pass/fail target into a graded, spendable quantity over a window, and the policy says what changes when it is spent. For D5 that is a standing objective's health as "budget left", read by reconcile, not a streak alone.
2. Multiwindow alerting needs both a long window (significance) and a short one (still happening). D5's `sustain: N` is the long-window half; it has no short-window confirmation, so a criterion that has recovered keeps its broken streak until N more passes.
3. Too few events is named as the noisy case, not as missing data, and the workbook's answers are product decisions. Pyrra shows "Too few requests!" beside an alert that still fires. That supports D4's `min_samples` with `inconclusive: too_few_samples` as a criterion state rather than a hidden alert threshold.
4. An SLO set from past performance is met by construction. D4's `baseline: at_start` has the same risk if the target is "not worse than at start" and the start was already bad; the person should see the baseline value.
5. "Aspirational SLO" is a third shadow mode, beside Argo's `dryRun` and Codecov's `informational`.

### OpenSLO specification

Pages tried, all read 2026-10-17: the [repository page](https://github.com/OpenSLO/OpenSLO), the [raw README](https://raw.githubusercontent.com/OpenSLO/OpenSLO/main/README.md) and the [OpenSLO site](https://openslo.com/). All three opened, but none holds the specification. The README is now only an introduction: OpenSLO "is an open specification for defining service level objectives (SLOs) in YAML", "Platform-specific implementation details are purposefully excluded from the scope of this specification", and it is "designed to be extended where needed to meet the needs of the implementation". For more it says "visit openslo.com" (https://raw.githubusercontent.com/OpenSLO/OpenSLO/main/README.md, read 2026-10-17). The site's landing page calls OpenSLO "a service level objective (SLO) language that declaratively defines reliability and performance targets using a simple YAML specification". It links to a `specification/` page and to schema pages for three versions, v1, v2alpha and v1alpha, for example `schema/v1/slo/`, `schema/v1/sli/` and `schema/v1/alertcondition/` (https://openslo.com/, read 2026-10-17). That used the family's three-page limit, so none of the spec pages was read.
- *Criterion, settled, missing or noisy, baseline, partial success.* Not read. `objectives[]` with `target`, `timeWindow` (rolling against calendar), `budgetingMethod` (Occurrences against Timeslices), composite weights and alert-condition fields are all on the specification and schema pages above, which a later action should read. Nothing about them is stated here from memory.
- *User-reported failure mode.* Not read.

**Takeaway for the family (inference, not a source claim).** The one thing read bears on D6: OpenSLO keeps "platform-specific implementation details" out of the spec and leaves extension to the implementation. That is the same split as a criterion declaration and the verifier that settles it.

### Keptn SLO evaluation (Analysis)

Sources: [Keptn SLO guide](https://keptn.sh/stable/docs/guides/slo/), [AnalysisDefinition reference](https://keptn.sh/stable/docs/reference/crd-reference/analysisdefinition/) and [keptn-contrib/dynatrace-service#728](https://github.com/keptn-contrib/dynatrace-service/issues/728), all read 2026-10-17. Keptn's docs are not gone: the guide is live, with a date of November 4, 2024 near the bottom. It describes the current Analysis feature, which it calls "similar to the functionality provided by the Keptn v1" Quality Gates, and it offers converters for "most" v1 SLIs and SLOs. It does not say that v1 Quality Gates are deprecated (https://keptn.sh/stable/docs/guides/slo/, read 2026-10-17). The v1 quality-gate pages, which hold the `comparison` block and `key_sli`, were not tried.
- *Criterion.* "A Service Level Input (SLI) identifies the data to be analysed as a query to a data provider", and an SLO sets quality criteria for each SLI (https://keptn.sh/stable/docs/guides/slo/, read 2026-10-17). An `AnalysisDefinition` lists `objectives[]`, each with an `analysisValueTemplateRef` (the query), a `target` holding `failure` and `warning` conditions, a `weight` and a `keyObjective` flag. The operators are `lessThan`, `lessThanOrEqual`, `greaterThan`, `greaterThanOrEqual`, `equalTo`, and `inRange`/`notInRange` with `lowBound` and `highBound` (https://keptn.sh/stable/docs/reference/crd-reference/analysisdefinition/, read 2026-10-17). As in SonarQube, the condition states the failure: the guide's example says "The objective fails if the percentile 95 is less than 600" (https://keptn.sh/stable/docs/guides/slo/, read 2026-10-17).
- *Settled.* Three outcomes: "At the end of the analysis, the status returns whether your objective failed, passed, or passed with a warning" (https://keptn.sh/stable/docs/guides/slo/, read 2026-10-17). An objective with no `failure` or `warning` value passes (https://keptn.sh/stable/docs/reference/crd-reference/analysisdefinition/, read 2026-10-17).
- *Missing or noisy.* Neither docs page says what happens when a value cannot be retrieved. The only evidence read is the user report below, from the v1 path.
- *Baseline.* None on the pages read: objectives are compared with fixed thresholds or ranges written in the resource (https://keptn.sh/stable/docs/reference/crd-reference/analysisdefinition/, read 2026-10-17). Keptn v1's comparison with previous evaluation results was not read.
- *Partial success.* Weighted, with a gate. `weight` is "used to emphasize the importance of one objective over others". `totalScore` holds `passPercentage` and `warningPercentage`; the guide's example needs "an overall score of 90% to pass or 75% to get a warning". With `keyObjective: true`, "the entire analysis fails if this particular objective fails" whatever the score (https://keptn.sh/stable/docs/reference/crd-reference/analysisdefinition/ and https://keptn.sh/stable/docs/guides/slo/, both read 2026-10-17). Neither page says how many points a passed or warned objective earns.
- *User-reported failure mode.* With the Dynatrace SLI provider on Keptn v1, a query that returned no data failed the evaluation (the example payload shows `"result": "fail"`). The only message was "Metrics query result has no data", which "doesnt tell the user anything which metric query resulted in no data" and was "the only message that will be shown in the bridge indicating why the evaluation failed". The maintainer's fix names the metric in each indicator's message and sets the result to `warning`, not `fail`, when indicators fail only because they did not return a single value. It closed on 2022-03-08 through PR #733, and the repository was archived on 2024-01-12 (https://github.com/keptn-contrib/dynatrace-service/issues/728, read 2026-10-17).

**Takeaways for the family (inference, not a source claim).**
1. Keptn's form is the same as Kayenta's: weighted objectives scored against pass and warning thresholds, with a `keyObjective` hard gate that overrides the score. It supports D2's required-versus-weighted split, and a warning band is a third state between pass and fail.
2. Keptn v1 first treated no data as a failure, then changed it to a warning that names the query. That is a vendor reaching D1's `inconclusive` with a reason by itself, through a state not built for it.
3. The current Analysis has no relative comparison on the pages read; a baseline would have to live in the query. D4's frozen `at_start` baseline is not something Keptn's current spec shows.

**Takeaways for section 2 (inference, not a source claim).** The SRE workbook treats success over time as a budget: graded, spendable and read over a window, with a policy for when it is spent. That is the strongest source read so far for D5's reconcile-side reading of standing objectives. It suggests a budget or k-of-N over a window as well as, or instead of, a plain streak. Too-few-events is the noisy case in both the workbook and Pyrra, and Keptn's v1 history shows no data drifting from fail to warning. Both support an explicit `inconclusive` state with a reason. OpenSLO's spec fields (`timeWindow`, `budgetingMethod`) are still unread, so the case for rolling against calendar windows and occurrences against timeslices rests on the workbook alone.

## 3 CI quality gates

All sources below were read on 2026-10-17 through a fetch tool that summarises each page against a question. A later action should confirm the exact wording before quoting.

### Codecov status checks

Sources: [commit status](https://docs.codecov.com/docs/commit-status), [comparing commits](https://docs.codecov.com/docs/comparing-commits) and [codecov/feedback#861](https://github.com/codecov/feedback/issues/861), all read 2026-10-17. The separate codecov.yml reference page was not read; see Read and not read.
- *Criterion.* Two status checks are configured in `codecov.yml`. `codecov/project` "measures overall project coverage and compares it against the base of the pull request or parent commit". `codecov/patch` "only measures lines adjusted in the pull request", or in the single commit when there is no pull request. Each takes a `target` and a `threshold`, and can be scoped by `branches`, `flags` and `paths` (paths and flags are listed for patch on that page) (https://docs.codecov.com/docs/commit-status, read 2026-10-17).
- *Settled.* `target` is either `auto` (the default) or a number such as `75%`. `auto` "will use the coverage from the base commit (pull request base or parent commit)", so the check fails if coverage drops below the base. `threshold` lets coverage "drop by `<number>%`" and still post `success`. `if_ci_failed` is `error` by default, which sets success only if CI passed; `success` ignores the CI result. The page contradicts itself here: the text lists `success` and `error`, but a YAML comment lists `success, failure, error, ignore`. The `base` option is marked "Deprecated as of July 2020" with no replacement described (https://docs.codecov.com/docs/commit-status, read 2026-10-17).
- *Missing or noisy.* There are three separate cases:
  - **No report for the head commit.** `if_not_found` defaults to `success`, so a commit with no uploaded coverage passes the check unless the author sets `failure` (https://docs.codecov.com/docs/commit-status, read 2026-10-17).
  - **Flag coverage not uploaded on this commit.** `flag_coverage_not_uploaded_behavior` is `include` (process normally, which with carryforward means checking carried-forward numbers), `exclude` (send no check) or `pass` (pass automatically) (https://docs.codecov.com/docs/commit-status, read 2026-10-17).
  - **No report for the base commit.** "Codecov only chooses commits that have successful CI builds" as a base. When the git base has no coverage (a `[ci skip]` commit, a merge commit), Codecov "will seek a parent commit and offset the report to get an approximate base report", a pseudo-comparison. With `allow_pseudo_compare: False` it instead renders "an error in the UI" (https://docs.codecov.com/docs/comparing-commits, read 2026-10-17). Neither page says what the status check itself reports when no base can be found.
- *Baseline.* Relative by default: `target: auto` compares against the base commit's coverage, and `threshold` is the tolerance on the drop. The base is the pull request base, moved forward after a rebase, or the nearest ancestor with a successful build. It is not a frozen baseline: it moves with the base branch (https://docs.codecov.com/docs/comparing-commits, read 2026-10-17).
- *Partial success.* None. Each status is pass or fail, plus a separate "changes" status for coverage changes outside the diff. `informational: true` posts a status that "will pass no matter what the coverage is", the same shadow mode as Argo's `dryRun` and promptfoo's `weight: 0` (https://docs.codecov.com/docs/commit-status, read 2026-10-17).
- *User-reported failure mode.* After a 40-PR batch merged to main, an upload showed "Missing base commit" and a flag's coverage was not carried forward from the nearest covered ancestor. The reporter calls it intermittent and tied to "merge-heavy batches + partial/intermittent flag uploads"; another user said they were "currently experiencing this". The issue is open with no maintainer explanation (https://github.com/codecov/feedback/issues/861, read 2026-10-17). It does not say which status the checks posted.

**Takeaways for the family (inference, not a source claim).**
1. Codecov's defaults make "not measured" a pass: `if_not_found: success` for a missing head report and `pass` as an option for missing flag uploads. That is the opposite of SWE-bench (section 4), where a missing test is a failure. D4 should not copy these defaults.
2. A relative target needs a defined base. Codecov's base moves and is sometimes approximated (pseudo-comparison), and users hit "Missing base commit" in practice. This supports D4's frozen `baseline: at_start` and an `inconclusive` state when no baseline exists.
3. Gating coverage on the CI result (`if_ci_failed: error`) is the default: a measurement from a failed build is not trusted.

### GitHub required status checks

Sources: [about protected branches](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches) and [troubleshooting required status checks](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/collaborating-on-repositories-with-code-quality-features/troubleshooting-required-status-checks), both read 2026-10-17. The troubleshooting page returned 404 at its old path under `managing-protected-branches/` and was read at the path above.
- *Criterion.* A branch protection rule names required checks, each a check run or a commit status, by name. Optionally it names the app expected to set each one: if "the status is set by any other person or integration, merging won't be allowed" (https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches, read 2026-10-17).
- *Settled.* Required checks "must have a `successful`, `skipped`, or `neutral` status" before a merge. In strict mode ("Require branches to be up to date before merging") the branch must also be up to date with the base; in loose mode it need not be, and the page warns that checks "may fail after you merge" (https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches, read 2026-10-17). Checks must pass on the latest commit: "Checks from earlier commits don't satisfy the requirement". When a check and a commit status share a name, both must pass (https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/collaborating-on-repositories-with-code-quality-features/troubleshooting-required-status-checks, read 2026-10-17).
- *Missing or noisy.* A required check that never reports blocks the merge indefinitely, shown as "Waiting for status to be reported". The troubleshooting page gives these causes:
  - a workflow skipped by path filtering, branch filtering or commit message, whose checks "stay in a "Pending" state and block merging";
  - a run from an ineligible trigger: a `workflow_dispatch` run does not count even when it passes;
  - a merge queue without the `merge_group` trigger, where the check is never reported and the merge fails.

  A job skipped by a job-level `if` condition is the opposite case: "The job reports "Success"" and does not block. A job skipped because a job it depends on failed "may not block merging". The page's advice is "Avoid requiring workflows that can be skipped" and to use `always()` with `needs` for a required job (https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/collaborating-on-repositories-with-code-quality-features/troubleshooting-required-status-checks, read 2026-10-17). Neither page covers flaky checks or reruns.
- *Baseline.* None. A check is a verdict on one commit (or on the test merge commit when that commit has a status); any comparison with the base happens inside the check, as in Codecov above.
- *Partial success.* None. Every required check must be in a passing state; there are no weights.
- *User-reported failure mode.* Not read from an issue thread: a search of GitHub community discussions returned only pinned posts. The troubleshooting page itself describes the reported symptom: a pull request that touches only root files is blocked forever because a required workflow limited to `scripts/**` never runs. The same page shows the silent-pass twin: a required job whose `if` is false reports success without running anything.

**Takeaways for the family (inference, not a source claim).**
1. GitHub has two behaviours for "did not run", depending on how the skip happened: pending forever (blocks) or `skipped` counted as a pass. A verifier that did not run must have one state, `inconclusive: not_run` (D7), whatever the reason, and it must never count as met.
2. "Never reports" needs a bound. Waiting forever is how GitHub fails safe, but it leaves the pull request stuck; reconcile needs a timeout that turns a missing measurement into a reported `inconclusive` with a reason, not a silent wait.
3. Pinning the source of a result (the expected app) is GitHub's anti-spoofing rule; it matches D7's audit of which verifier produced each measurement and the rejection of third-party verifiers.
4. A result applies only to the commit it was measured at. That supports D7's rule that each measurement records its commit.

### SonarQube quality gates

Sources: [quality gates (fudge factor)](https://docs.sonarsource.com/sonarqube-server/latest/instance-administration/analysis-functions/quality-gates/), [introduction to quality gates](https://docs.sonarsource.com/sonarqube-server/quality-standards-administration/managing-quality-gates/introduction-to-quality-gates) and [community thread 70588](https://community.sonarsource.com/t/quality-gates-passes-without-showing-code-coverage-for-some-of-the-pull-requests/70588), all read 2026-10-17. The first page covers only the fudge factor.
- *Criterion.* A quality gate is a list of conditions. Each one "is a combination of" a metric, a comparison operator and an error value, for example Blocker issue `>` 0, "No blocker issues". Each condition applies "either to new code or overall code". The default gate, Sonar way, has four conditions, all on new code: no new issues, all new Security Hotspots reviewed, new-code coverage at least 80.0%, and new-code duplication at most 3.0% (https://docs.sonarsource.com/sonarqube-server/quality-standards-administration/managing-quality-gates/introduction-to-quality-gates, read 2026-10-17).
- *Settled.* The gate passes or fails: "If one of the conditions is met, the quality gate fails." The condition states the failure, not the target. On a branch both kinds of condition apply. On a pull request "Only conditions defined on *new code* are applied" (https://docs.sonarsource.com/sonarqube-server/quality-standards-administration/managing-quality-gates/introduction-to-quality-gates, read 2026-10-17). Neither page mentions a warning state.
- *Missing or noisy.* The fudge factor: conditions on duplication are ignored until there are at least 20 new lines, and conditions on coverage until there are at least 20 new lines to cover. It is on by default, can be turned off for the instance, and a project administrator can override it (https://docs.sonarsource.com/sonarqube-server/latest/instance-administration/analysis-functions/quality-gates/, read 2026-10-17). So a small change passes those conditions without being measured. Neither page says what happens with no analysis, an empty new code period, or a metric that was not computed.
- *Baseline.* Built in through "new code". On a pull request, new code is "whatever has changed relative to the target branch". On a branch it is set by the branch's new code definition, which lives on a separate page I did not read (https://docs.sonarsource.com/sonarqube-server/quality-standards-administration/managing-quality-gates/introduction-to-quality-gates, read 2026-10-17). Conditions on new code are SonarQube's form of a relative target: they judge only the change, not the whole codebase.
- *Partial success.* None. Every condition is a hard gate; there are no weights or scores.
- *User-reported failure mode.* On SonarQube Cloud, some pull requests showed coverage as "–" instead of a percentage while their quality gate passed. Others in the same TypeScript project on CircleCI showed coverage and gated as expected. The reporter asked whether Sonar was "not receiving the test reports for some of the Pull requests". The only staff reply asks for a screenshot, so the cause is not confirmed (https://community.sonarsource.com/t/quality-gates-passes-without-showing-code-coverage-for-some-of-the-pull-requests/70588, read 2026-10-17). The search listing that led to this thread also shows titles such as "Sonar scan passes with 0 coverage on new code with QG asking for >90" and "New Code always 0 for PRs", but I read only their titles and one-line summaries.

**Takeaways for the family (inference, not a source claim).**
1. Measuring only the change (new code) is a simple, widely deployed form of relative target with no stored baseline number. It fits a delivery criterion ("coverage of changed lines at least N%") as well as D4's frozen `at_start` baseline does, and it cannot drift.
2. The fudge factor is a minimum sample in all but name. SonarQube resolves too-few-lines as a pass, which is why a missing coverage report can look like success. D4's `min_samples` should yield `inconclusive: too_few_samples`, shown to the person as such.
3. A metric shown as "–" and a gate shown as passed in the same view is the failure D1's states exist to prevent: the criterion state must say `inconclusive` whenever the value is absent.

**Takeaways for section 3 (inference, not a source claim).** All three systems resolve some form of "not measured" as a pass by default: Codecov's `if_not_found: success`, GitHub's `if`-skipped job reported as success, and SonarQube's fudge factor. GitHub's path-skipped workflow fails the other way, blocking forever. None of them has a first-class inconclusive state, and the user reports above come from that gap. This is direct support for D1's `inconclusive` state with a reason, for D4's `min_samples` and frozen baseline, and for D7's rule that a verifier that did not run is never met.

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

Sections 2, 3, 5 and 6 held the marker when these decisions were written, so every decision below rests on sections 1, 4 and 7 only and is weaker for it: SLO error budgets, CI gate semantics (Codecov, SonarQube, GitHub required checks), agent frameworks' termination and acceptance-criteria practice were not read. Section 3 was written afterwards (2026-10-17); its takeaways support D1, D4 and D7, and the decisions have not been revised against it.

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

**Decision.**
- Targets, baselines, reducers and sample counts are fixed at create time and stored with the objective. The loop has no capability that writes them; a change goes through the API by a person, is audited, and resets any sustain streak.
- A verifier is a pack capability. A discovered MCP tool can never be a verifier (extends verify.go:88 and ADR 022 from "never met" to "rejected at create time").
- A `measured` or `verified` criterion is never handed to the model: when its verifier did not run, the state is `inconclusive: not_run`, not a judgement. This removes the verify.go:106 fallback for criteria that name a verifier.
- Every measurement is audited with its verifier, parameters, value, unit, source, trust and the commit it was taken at.
- Anti-gaming: when a pass changes the files a criterion's verifier reads its definition from (the test files behind `tests-pass`, the coverage config, the benchmark, the metric query), the criterion is `inconclusive: measure_changed` and needs a person, like SWE-bench running the original FAIL_TO_PASS/PASS_TO_PASS tests rather than the patch's. Packs declare those paths with the verifier.

**Rests on.** Section 4: SWE-bench's fixed test lists, Inspect's scorer kept apart from the solver. Section 7: Luo et al. F.12. Code reading fact 1 (reserved verifiers, the judge fallback). Weaker because section 3 (required checks that never report) was not read.

**Conflicts.** None; it reinforces ADR 022 and rule 9. Removing the judge fallback for verifier-named criteria changes `software.objective.delivery` behaviour on docs-only tasks (fact 4), which is wanted: the template should use the D8 deterministic booleans or not name the verifier.

### D8 Departures from the starting proposal

| Element | Kept, changed, dropped or added | Reason |
|---|---|---|
| Kinds JUDGED, VERIFIED, MEASURED | Kept | D1; sections 1 and 4 separate code-settled from model-settled outcomes. |
| "Not measured with a reason is neither met nor a failure" | Changed | Named `inconclusive` with a fixed reason list; it blocks completion but not the breaker (D1, D5). |
| `Met bool` | Changed | Replaced by a state; `met` stays derived in JSON (D1). |
| `Threshold any` | Dropped | Nothing reads it (fact 1); replaced by typed `op`/`target`/`unit`. |
| metric, op, target, unit | Kept | D4; unit must match the verifier's declaration (D3). |
| baseline (fixed or at_start) | Kept | `at_start` is frozen and audited (D4, D7). |
| change (relative) | Kept | Absolute or percentage delta to the baseline (D4). |
| aggregate (last, min, max, mean, all) | Changed | Renamed reducer; adds `median`, `p95`, `k_of_n`, `samples` and `min_samples`; "any passes" excluded (D4; section 7). |
| window | Changed | Moved out of the loop to reconcile (D5; rule 8). |
| sustain (N reconciles) | Kept, moved | Per-criterion streak in reconcile beside `ScoreStreak` (D5). |
| required versus weight | Changed | Required by default; weight is progress only; weight 0 is shadow (D2). |
| — | Added | Zero-required-criteria objectives rejected (D2; fact 1). |
| `--criterion 'id=..; ...'` | Kept, demoted | Secondary to the file (D6). |
| `--criteria-file criteria.yaml` | Kept | Primary form (D6). |
| Create-time validation, perhaps via OutputSchema | Changed | Validates against declared metrics beside OutputSchema, not inside it (D3, D6). |
| `krk verifier list` | Kept | D6. |
| `krk objective criteria <id>` | Kept | Shows state, reason, value against target, who decided (D6). |
| API criteria field | Added | The proposal named only the CLI; thin CLI needs the API (D6; ADR 001). |
| Template-plus-request merge, tighten only | Added | D6, D7. |
| `Measurements []{Name, Value, Unit, Source}` on ActionResult | Kept, extended | Adds trust (rule 9) and travels typed through MCP (D3). |
| Targets the loop can never edit | Kept | D7. |
| Every measurement audited | Kept | Adds commit and parameters (D7). |
| — | Added | Measured or verified criteria never fall back to the judge (D7). |
| — | Added | `measure_changed` anti-gaming state (D7). |
| — | Added | Discovered MCP tools rejected as verifiers at create time (D7). |
| Verifier: coverage per package | Kept | Needs a baseline (`at_start`) to be useful (D4). |
| Verifier: flaky-test count over N runs | Kept | Section 7, takeaway 3. |
| Verifier: p95 latency and error rate via Phase 32 adapters | Kept | `internal/platform/tools/observability/{prometheus,datadog}.go`; no-data must be inconclusive (D1). |
| Verifier: cost per pass | Kept, unverified | Cost appears in `internal/feature/report`; the source was not verified in time. |
| Verifier: escalation approval rate | Kept, unverified | No telemetry found by grep (fact 5); the next action must locate or drop it. |
| Verifier: web bundle size | Kept | Deterministic; sequencing is the next action's. |
| Verifier: CLI startup time | Kept | Noisy; needs `samples` and `median` (D4). |
| Deterministic booleans (stream PR opened or updated this pass, CI green at its head, only declared paths changed) | Kept, first | These address fact 4 directly and should ship before the numeric verifiers. |
| Statistical canary judge | Not adopted | Out of scope for Phase 41 (D4). |
| ADR | Added | ADR 029 is needed: core entity changes (D1, D3), breaker input (D5), judge fallback removal (D7). |

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

**Read first-hand on 2026-10-17** (Phase 41 research action 1, section 3), all through the summarising fetch tool:
- Codecov: https://docs.codecov.com/docs/commit-status, https://docs.codecov.com/docs/comparing-commits and https://github.com/codecov/feedback/issues/861
- GitHub: https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches and https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/collaborating-on-repositories-with-code-quality-features/troubleshooting-required-status-checks
- SonarQube: https://docs.sonarsource.com/sonarqube-server/latest/instance-administration/analysis-functions/quality-gates/, https://docs.sonarsource.com/sonarqube-server/quality-standards-administration/managing-quality-gates/introduction-to-quality-gates and https://community.sonarsource.com/t/quality-gates-passes-without-showing-code-coverage-for-some-of-the-pull-requests/70588

**Not read in section 3.**
- The Codecov codecov.yml reference page; the page budget went to the comparing-commits page, which documents missing-base behaviour. Neither page read says which status Codecov posts when no base is found.
- GitHub: the troubleshooting page at https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/troubleshooting-required-status-checks returned 404 (it was read at its current path instead). No issue or community thread with a user report was read: the community discussion search returned only pinned posts.
- SonarQube: the new code definition page (previous version, number of days, reference branch), and any statement of what the gate does with no analysis or a metric that was not computed. The other community threads in the search listing were read only as titles and one-line summaries.
