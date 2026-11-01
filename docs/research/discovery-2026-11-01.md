# Discovery 2026-11-01: weekly cycle

**Method.** This report is written by eight parts, each writing only between its own markers. Sources are graded [Q] (verbatim words returned by the fetch), [E] (extract or paraphrase, never quoted) and [N] (not read, cited for nothing). 'Observed:' (a fetched page or opened file says it) and 'Inferred:' (the writer's conclusion) are kept apart. Nothing was built, run or tested for this report. A vendor's or analyst's figure is a claim, not a measurement.

**Baseline:** docs/research/discovery-2026-10-25.md (which builds on 2026-10-08 and 2026-10-04).

## Who the users are and what they are trying to do

<!-- p1:start -->
Written by part 1 on 2026-11-01. The segments and sources of 2026-10-04, 2026-10-08 and 2026-10-25 stand and are not restated; this section adds to the thin spots 2026-10-25 named ('Still thin (part 1)' and 'Not attempted'). Every page was read through a model-written extract; quotations are only what the extract gave as verbatim. Nobody was interviewed. Session limit 600 seconds.

### A team running standing agents on its own repository (thin spot e)

- **GitHub Next and Microsoft Research, "Welcome to Peli's Agent Factory", Don Syme, Peli de Halleux and Mara Kiefer, 12 January 2026.** Who speaks: the team that built GitHub Agentic Workflows, about the workflows it runs on its own repository (`github/gh-aw`), some also applied in GitHub internal repositories. Job: keep a repository maintained by automation (issue triage, CI failure diagnosis, docs upkeep, test coverage, security compliance, workflow efficiency) without a person starting each run. Observed: "We have built and operated over 100 automated agentic workflows within the github/gh-aw repository."; "Others proactively propose changes through pull requests."; "Strict constraints actually make it easier to experiment safely."; "No one can read all these outputs (except, of course, another workflow)."; "Longer analyses aren’t always better." (https://github.github.com/gh-aw/blog/2026-01-12-welcome-to-pelis-agent-factory/, read 2026-11-01) [Q] Observed [E]: the post names read-only analysts and meta-agents that watch the health of other workflows; workflows compile to GitHub Actions with scoped permissions; it says agents "fail in instructive ways" but names no failure, and leaves failures and security lessons to later posts in a series, which were not read. What runs on which schedule is not on this page; the search pointed to a status page (https://github.github.io/gh-aw/agent-factory-status) listing daily workflows, not opened [N].
- What the agent may do unsupervised: Observed: proposes changes by pull request and writes reports; the page does not say whether a person reviews or merges. Not known here.
- What went wrong: Observed: volume, stated by the team itself: nobody can read every output. Nothing more specific on this page.
- Inferred: this is the first first-hand team in four cycles that runs many *standing* agents, not one migration loop (Temporal) or an on-demand fleet (Stripe). Its stated pain is the reader, not the writer: output outgrows the humans, and the team's answer is more agents reading agents. Inferred for Karakuri: digests (Phase 21) and the audit log are the same answer done by a deployment; a candidate need for parts 2 and 6 to test is **one digest across many standing objectives that says what needs a person and suppresses the rest**, and a **meta-objective that watches the health of other standing objectives**. Also Inferred: "strict constraints make experimenting safer" is the case for `agent.AuthorityBounds` from a team that has no reason to sell Karakuri's version of it.
- Strength: one primary source, an introductory post; thin.

<!-- p1:end -->

## What they ask for and complain about

<!-- p2:start -->
_Pending: part 2._
<!-- p2:end -->

## Competitive teardown

<!-- p3:start -->
_Pending: part 3._
<!-- p3:end -->

<!-- p4a:start -->
_Pending: part 4._
<!-- p4a:end -->

## Capability matrix

<!-- p4b:start -->
_Pending: part 4._
<!-- p4b:end -->

## Technical frontier

<!-- p5:start -->
_Pending: part 5._
<!-- p5:end -->

## Governance and buying criteria

<!-- p6a:start -->
_Pending: part 6._
<!-- p6a:end -->

## Feasibility

<!-- p6b:start -->
_Pending: part 6._
<!-- p6b:end -->

## Fit and bets

<!-- p7a:start -->
_Pending: part 7._
<!-- p7a:end -->

<!-- p8:start -->
_Pending: part 8._
<!-- p8:end -->

## What changed since the last report

<!-- p1w:start -->
### How earlier proposals fared

Written by part 1 on 2026-11-01 from `README.md`, `grep -n '^## Phase' docs/roadmap.md` on `origin/main` (a83e32a), `gh pr list --state all --label karakuri:discovery`, `gh pr list --state open` and title searches for each phase number. No web.

- **Phase 33** (from #148, 2026-10-04), "MCP After the Handshake": Observed: **Completed** on main.
- **Phases 35 and 36** (from #158, 2026-10-08), "An Unanswered Checkpoint Ends" and "A Provider's Refusal Is Not a Plan": Observed: **Completed** on main.
- **Phase 37** (from #158), "Any Run Can Be Stopped at Any Time": Observed: **Planned** on main; #171 (merged) restated it as the owner's requirement. No open pull request names it.
- **Phase 38** (from #158), "What Was Required and Not Done": Observed: **Planned** on main; no open pull request names it. Phase 41's text cross-refers to it (its settlement values), and #187 (open, "phase 41 steps 1 to 9") works next to it, not on it.
- **Phases 42 and 43** (from #195, 2026-10-25), "An MCP Call Carries Only What the Tool Declared" and "A Provider's Tool-Call Shape, Pinned by a Test": Observed: **Planned** on main; no pull request names either. Their find-out first slices have not been started.
- **Declined:** none. Observed: #148, #158 and #195 were all merged; no discovery pull request was closed unmerged. Rank 5 of 2026-10-08 (MCP server and tool inventory in the evidence record) is still cited by no phase; it was not declined, only not taken up.
- Not from discovery, for context: Observed: #200 (open, not labelled `karakuri:discovery`) proposes Phase 44; #187 and #202 deliver Phase 41.
- Inferred: delivery has not yet reached any discovery phase proposed after 2026-10-08 except the two that were Completed; four discovery phases (37, 38, 42, 43) are queued behind Phase 41. A new proposal this cycle adds to a queue, so it should clear a higher bar than the last one.

<!-- p1w:end -->

<!-- p7b:start -->
_Pending: part 7._
<!-- p7b:end -->

## Sources

<!-- s1:start -->
### Sources, part 1

- https://github.github.com/gh-aw/blog/2026-01-12-welcome-to-pelis-agent-factory/ — read 2026-11-01; primary (the team's own blog); full text of a short post, no truncation reported; [Q] for the five quoted sentences, [E] for the rest.
- https://github.github.io/gh-aw/agent-factory-status — not opened; [N], cited for nothing.
- Local files, read 2026-11-01: `README.md`, `docs/roadmap.md` (phase headings), `docs/research/discovery-2026-10-25.md`; `gh pr list` output. Primary for this repository.
<!-- s1:end -->

<!-- s2:start -->
_Pending: part 2._
<!-- s2:end -->

<!-- s3:start -->
_Pending: part 3._
<!-- s3:end -->

<!-- s4:start -->
_Pending: part 4._
<!-- s4:end -->

<!-- s5:start -->
_Pending: part 5._
<!-- s5:end -->

<!-- s6:start -->
_Pending: part 6._
<!-- s6:end -->

<!-- s7:start -->
_Pending: part 7._
<!-- s7:end -->

<!-- s8:start -->
_Pending: part 8._
<!-- s8:end -->
