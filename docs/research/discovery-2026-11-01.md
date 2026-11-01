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

### A regulated organisation speaking first-hand (thin spot f): not filled

Two searches found no bank, insurer, health provider or public body writing about its own agent deployment on its own engineering blog. The nearest candidate was a write-up of a Lloyds Banking Group talk at Infosecurity Europe (June 2026); the page could not be fetched ("Socket is closed", twice) (https://www.pulse.bot/cybersecurity/news/infosecurity-europe-practical-lessons-from-lloyds-agentic-ai-security-playbook-95addba0-8826-4c52-9fa6-514224cffa45/, read 2026-11-01) [N], and it would have been secondary anyway. Nothing from it is claimed here. The search results (index only, cited for nothing) also pointed to a Barclays podcast and a Bank of England news report; neither was opened.

### Segments

| Segment | Added this cycle | Still thin? |
|---|---|---|
| Platform and SRE teams | Nothing new. Rests on SREcon26 (2026-10-25), Brex and HyperProbe (2026-10-08). | Yes: no postmortem where an agent acted; Redick's slides unread. |
| Self-hosters | Nothing new. Rests on ZeroClaw #10968 (2026-10-25). | Yes: one tracker only. |
| Teams running unattended or standing coding agents | GitHub Next, Peli's Agent Factory [Q]: over 100 workflows on one repository, PR-proposing, output volume as the stated pain. | Less thin: Temporal, Stripe and now GitHub Next are three organisations; but none of the three is a full account of a failure. |
| Regulated organisations | Nothing; one [N]. | Yes, for four cycles; part 6 holds the governance reading. |
| Product teams embedding agents | Nothing. | Yes: not attempted. |
| Enterprises | Inferred only: GitHub Next's workflows are applied in GitHub internal repositories, per the same post [E]. | Yes. |

Inferred: the one segment this cycle strengthened is the one closest to Karakuri's own standing streams, and its first-hand evidence says the bottleneck moves from doing work to reading what was done.

### Still thin (part 1)

- (f) Regulated organisation: not filled; one [N]. Next: a bank's own talk with a recording (Lloyds at Infosecurity Europe 2026, Barclays' Andy McMahon), or a public body's published algorithmic-transparency record (UK ATRS) for an agent.
- (e) Standing agents: one introductory post. The schedule and failures are in the series' later posts and the status page, not read; next cycle should open https://github.github.io/gh-aw/agent-factory-status and one "Meet the Workflows" post.
- Product teams and enterprises: not reached.
- Stretch (b) InfoQ transcript, (a) agent-in-incident postmortem, (d) Ona white paper, (g) Anthropic report PDF, (c) second self-hoster: **not reached**.
- **Candidate needs for parts 2 and 6 to test:** (1) one digest across many standing objectives that surfaces only what needs a person; (2) an objective that watches the health of other standing objectives (stuck, noisy, costly); (3) whether "nobody can read all these outputs" shows up as a complaint in any tracker.

<!-- p1:end -->

## What they ask for and complain about

<!-- p2:start -->
Written by part 2 on 2026-11-01. It extends the 2026-10-25 section (items A to D there) and the 2026-10-08 section (items 1 to 6), and does not restate them. GitHub threads were read through the api.github.com form with a fetch tool that returns a model-written extract, so quotations are [Q] only where the extract gave them as verbatim; the rest is [E]. Counts are as the API showed them on 2026-11-01. Session limit: 600 seconds.

### 1. Candidate (A): a delegated or background agent carries the bounds of the run that started it. Second tracker found

Last cycle this rested on one tracker (ZeroClaw #10968). This pass found two more organisations' trackers; one asks for exactly the escalation direction, the other for the opposite direction.

- Observed: github/copilot-cli #3013, "Hooks don't fire for background (task) agents" (logar16, 2026-04-28, version 1.0.37; labels area:permissions, area:agents, area:plugins). Body: "Any `preToolUse` hook that enforces policy (file permissions, lock checks, security scanning) is bypassed entirely by sub-agents." Expected: "A preToolUse hook that returns {"permissionDecision":"deny"} should block the sub-agent's tool call, not just the parent's." (https://api.github.com/repos/github/copilot-cli/issues/3013, read 2026-11-01) [Q]
- Observed: 4 comments, 0 reactions on the issue, no duplicates or linked issues named. Two comments promote a third-party product; one, 2026-10-22, asks whether it still reproduces. The assignee (szabta89, site admin) closed it as completed on 2026-08-05: "The hook bypass affecting tools run by background and sub-agents has been fixed in current releases: preToolUse and postToolUse hooks now apply to sub-agent tool calls. Session lifecycle hooks remain scoped to the parent session". No release version is named. (https://api.github.com/repos/github/copilot-cli/issues/3013/comments?per_page=100&page=1, read 2026-11-01) [Q]
- Observed: anthropics/claude-code #37442, "Subagents don't inherit bypassPermissions mode from parent session" (sorfanit, 2026-03-22; labels bug, has repro, area:agents, area:permissions, stale), 11 comments, 1 reaction (eyes). Expected: "Subagents should inherit the parent session's permission mode, including `bypassPermissions` and all `allow` patterns." No MEMBER or COLLABORATOR reply; the bot proposed duplicates #26083, #26479 and #36225 and later closed it as not_planned for inactivity (2026-05-28). A user lists #29110, #32402, #37730, #40242 as "Related and likely the same underlying bug" and #45888 as fixed for Read/Edit in v2.1.97 with Bash still denied. ThatDragonOverThere (2026-04-22): "The parent window's permission state should be inherited by Task-spawned subagents, or at minimum there should be a flag to propagate it explicitly." (https://api.github.com/repos/anthropics/claude-code/issues/37442 and /comments?per_page=100&page=1, 11 of 11 comments read 2026-11-01) [Q]
- Observed, not read in full (search-result titles only, cited for nothing): anthropics/claude-code #27661 (hooks and allow/deny rules not inherited by Task subagents), opencode PRs #13974 and #26845 (parent denies reaching subagents, once too little and once too much). [N]
- Inferred: candidate (A) now passes a two-tracker gate in the escalation direction: ZeroClaw #10968 (scheduled turns skip approval) and copilot-cli #3013 (a deny hook does not reach sub-agents), different organisations. Both were treated by their maintainers as real bugs (ZeroClaw p1/risk:high; GitHub fixed it). Demand by count is weak: 0 reactions on #3013. claude-code #37442 and its five related issues show the same mechanism failing the other way (the child is stricter than the parent, so unattended runs stall on prompts); it is louder (at least nine issues in one tracker) but it is one tracker and asks for propagation of permissiveness, not of limits. What users want in both directions is one rule: the child's bounds are derived from the parent's, explicitly and testably.
- Roadmap: see the Ranking below.

### 2. Candidate (B): the repository's own checks run before an agent pushes. Third organisation, read in full

- Observed: Spotify Engineering, "Background Coding Agents: Predictable Results Through Strong Feedback Loops (Honk, Part 3)", Max Charas and Marc Bruggmann, 2025-12-09; whole page read (about 14,700 characters; the embedded images were not readable). "Without these feedback loops, the agents often produce code that simply doesn't work." "These verifiers that invoke formatting, building, and testing make sure our agent produces syntactically correct code that builds and passes tests." "If one of the verifiers fails, the PR isn't opened and the user is presented with an error message." (https://engineering.atspotify.com/2025/12/feedback-loops-background-coding-agents-part-3, read 2026-11-01) [Q]
- Observed, same page [E]: verifiers are deterministic and selected by what the repository holds (a Maven verifier activates when it finds a `pom.xml` at the root); the agent does not know what they do; they are exposed as an MCP tool for mid-task feedback and all relevant ones run before a PR is attempted, through Claude Code's stop hook; failures are parsed down to the relevant error lines to save context. An LLM judge runs after them on the diff and the prompt and vetoes about a quarter of sessions, after which the agent corrects course about half the time; the judge has no evals yet. Verifiers run on Linux x86 only.
- Note: the 2026-10-08 report cited this page for the judge's veto rate, not for candidate (B). It is a third organisation for (B), after Temporal and Stripe.
- Inferred: (B) now has three independent primary sources, and Spotify's is the most specific: checks are chosen from the repository's own build files, run deterministically, block the PR on failure, and come back to the agent as short error text. For Karakuri that sets the bar as "the delivery stream discovers and runs the repository's checks and refuses to push on failure", not "the prompt asks the model to run tests".

### 3. The most-reacted openai/codex issue is a platform request, not an agent pain (partly read)

- Observed: the search sorted by reactions returns openai/codex #11023, "Codex desktop app for Linux" (Suhaibinator, 2026-02-07), closed as completed 2026-08-11, 209 comments, 1,464 reactions (954 +1). On comment page 1 only 20 comment objects came back verbatim (the page was truncated at about 39 KB of 190 KB); no MEMBER or COLLABORATOR in them; the most-reacted are platform votes: skypher (348) "Arch and Ubuntu", kosumic (34) "NixOS". Page 2 and the rest of page 1 were not read. (https://api.github.com/search/issues?q=repo:openai/codex+is:issue&sort=reactions&order=desc&per_page=5 and https://api.github.com/repos/openai/codex/issues/11023/comments?per_page=100&page=1, read 2026-11-01) [Q for title and short bodies, E for the rest]
- Inferred: the loudest Codex demand is where the client runs, which Karakuri's thin-CLI, server-side design (ADR 001) sidesteps rather than answers. It adds no agent-behaviour pain point; it is recorded so the next cycle does not repeat the search.

### Ranking (part 2)

| Pain point | Independent sources (this cycle and earlier) | Total reactions/duplicates seen | Strength | Phase that covers it |
|---|---|---|---|---|
| Repository's own checks run before an agent pushes (B) | 3: Temporal blog, Stripe session page (2026-10-25), Spotify Honk part 3 (this cycle) | none (blogs) | Moderate: three first-hand operators, no user votes | Partly Phase 41 (Goal: success stated "as numbers and checks that code settles", at objective level; not a pre-push gate). Gap: no phase states "discover and run the repo's checks, refuse to push on failure" |
| A delegated, scheduled or sub-agent run carries the parent's bounds (A) | 2 escalation-direction: ZeroClaw #10968 (2026-10-25), github/copilot-cli #3013 (this cycle); plus 1 opposite-direction tracker, claude-code #37442 with 3 bot-named duplicates and 5 user-named related issues | #3013: 0 reactions, 4 comments; #37442: 1 reaction, 11 comments, about 9 issues | Moderate-weak: passes two trackers, low votes, maintainers treat it as a bug (fixed at GitHub) | Partly Phase 40 (Goal: streams governed by Karakuri; its steps declare which tools a delegated coding action may hold) and Phase 34 (credential scoped to one delegation); rule 8 of AGENTS.md puts bounds in `agent.AuthorityBounds`. Whether reconcile passes and sub-agents inherit them: not checked |
| MCP client sends only declared arguments (D) | 1 (n8n #21500, #21716) | 77 + 27 comments; 12 on one reply | Strong in one tracker | Phase 42 (Goal: "an MCP tool call sends a server only the keys its discovered input schema declares, or is refused") |
| A provider's tool-call shape change does not break the run (E) | 1 (n8n #22181, #9862) | 6 and 7 reactions | Moderate, one tracker | Phase 43 (Goal: a test fails when the parsed shape stops matching the provider's) |
| A configured timeout is honoured for every provider | 1 (n8n #25360 and two earlier) | 8 reactions, 10 comments | Moderate, one tracker | not checked this pass |
| Durable state survives the database and stays small | 1 (langgraph #3716, #5672) | 53 comments, 12 reactions | Moderate, one tracker | not checked this pass |
| Native Linux client (openai/codex #11023) | 1 | 1,464 reactions, 209 comments | Strong but off-scope | none; ADR 001 thin CLI |

Inferred: this pass moves (A) and (B) across the two-source gate; (C) to (E) still rest on one tracker each.

### Still thin (part 2)

1. openai/codex #11023: page 2 and most of page 1 not read; counts are from the issue object.
2. claude-code #27661, #37730 and opencode PRs #13974, #26845, #30288 not opened; they could add a third tracker for (A), and opencode is a separate organisation.
3. Not reached: langgraph #5672 comments 32 to the end; second sources for (C), (D), (E); claude-code #16157 reaction count; the "$4,200 in 63 Hours" author post; the "78% / under 15%" primary report (still unverified); crewAI, autogen and the Temporal forum.
4. Whether Karakuri's reconcile path and `software.verify.review` sub-agents inherit `agent.AuthorityBounds` was not read in code; the next cycle's part 6 should test it first.
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
- https://www.pulse.bot/cybersecurity/news/infosecurity-europe-practical-lessons-from-lloyds-agentic-ai-security-playbook-95addba0-8826-4c52-9fa6-514224cffa45/ — fetch failed 2026-11-01 ("Socket is closed"); secondary; [N], cited for nothing.
- Local files, read 2026-11-01: `README.md`, `docs/roadmap.md` (phase headings), `docs/research/discovery-2026-10-25.md`; `gh pr list` output. Primary for this repository.
<!-- s1:end -->

<!-- s2:start -->
### Sources, part 2

- https://api.github.com/repos/github/copilot-cli/issues/3013 and /comments?per_page=100&page=1: read 2026-11-01, primary, full thread (body and 4 of 4 comments) as extracted, [Q]
- https://api.github.com/repos/anthropics/claude-code/issues/37442 and /comments?per_page=100&page=1: read 2026-11-01, primary, full thread (body and 11 of 11 comments) as extracted, [Q]
- anthropics/claude-code #27661, #37730; anomalyco/opencode PRs #13974, #26845, #30288: search-result titles only, not opened, [N]
- https://engineering.atspotify.com/2025/12/feedback-loops-background-coding-agents-part-3: read 2026-11-01, primary, full text (images unreadable), [Q]
- https://api.github.com/search/issues?q=repo:openai/codex+is:issue&sort=reactions&order=desc&per_page=5: read 2026-11-01, primary (tracker metadata), full, [Q]
- https://api.github.com/repos/openai/codex/issues/11023/comments?per_page=100&page=1: read 2026-11-01, primary, extract truncated at about 39 KB of 190 KB (20 comments verbatim), [E]
- docs/roadmap.md (Phase 34, 40, 41, 42, 43 Goal paragraphs): opened file, read 2026-11-01
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
