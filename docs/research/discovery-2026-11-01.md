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
This part adds to products 1 to 3 of discovery-2026-10-25.md and does not restate them. Every page came through a fetch tool that returns a model-written extract; quotation marks are used only for words the extract gave as verbatim. The tool read long pages in part (about 40 KB verbatim, then a model summary, then nothing), so each item states how far it read. Vendor release notes are claims, not measurements. Window: 2026-05-01 to 2026-11-01.

### 1. OpenAI Codex and the OpenAI Agents SDK

**(i) Shipped, part of window read.** Observed [Q/E] (https://github.com/openai/codex/releases.atom, read 2026-11-01): the feed is about 303,000 characters; 78,800 were read verbatim and held only two dated entries, so only 2026-10-29 to 2026-10-30 of the window is covered. The rest was summarised as undated PR list items and is not relied on.
- rust-v0.166.0 (2026-10-29). Approval: "Preserved acknowledged approval and permission settings when resuming or forking threads"; "Respect server approval policy defaults during TUI startup" (#56516). Durability: "Added `codex queue --resume`" "to resume unloaded threads after queueing a message"; reasoning-token accounting kept across resume and fork "to avoid premature compaction". Observability: "Add trace instrumentation for turn startup and response streaming" (#56515); the word OpenTelemetry does not appear. Sandbox: a Windows filesystem sandbox fix (#56468). Multi-agent: "Restored spawned descendant threads when unarchiving their parent."
- rust-v0.167.0-alpha.1 (2026-10-30): no notes.
- Undated PR titles in the feed, not placed in the window: "Allow advisory rollout budgets with optional limit enforcement" (#57187), "Add opt-in protocol discovery for local stdio MCP servers" (#57210), "Share eligible local stdio MCP connections across chats" (#57211).
- Inferred: 2026-05-01 to 2026-10-28 of Codex is still unread; the Atom feed holds the newest entries only, so the window needs the tags list or older release pages.

**(ii) What it does that Karakuri does not.** Inferred from the two entries: a queue of messages to threads that are not loaded, resumed on demand (`codex queue --resume`), and approval settings that carry across a fork of a thread. Karakuri has no notion of forking a run.

**(iii) What Karakuri does that it does not.** Checked against docs/roadmap.md: Phase 20 (Completed), Goal "An objective can declare a state to be *held* rather than a task to be finished, and Karakuri converges the world to it on its own"; Phase 21 (Completed), a twin "sends one message on a cadence saying what its objectives did and what they need a person to decide"; Phase 35 (Completed), an unanswered checkpoint ends at an operator-set time. Inferred: nothing in the part of Codex read holds a state, reports on a cadence, or expires an approval. Not claimed: Phase 37 ("every run can be stopped at any time") is Planned, so a stop control is not a Karakuri advantage.

**(iv) Complaints.** Part 2 of this report already covered the most-reacted Codex issue (#11023, Linux desktop client). This part read the most-reacted agent-behaviour issue instead, openai/codex #9203 "Please make "/undo" back" (https://api.github.com/repos/openai/codex/issues/9203 and its comments page 1; page 2 empty; read 2026-11-01). Observed [Q/E]: opened 2026-01-14, open, labels enhancement, TUI, session; 541 reactions (490 +1), 97 comments. About 19 comments read verbatim, about 47 through a summary, about 31 not read.
- The one staff reply read, etraut-openai (2026-01-16): "the previous experimental implementation had problems" and "If we bring it back, it will need to be redesigned." No fix or date in the part read.
- Themes, approximate comment counts of the ~66 read [E]: plain restore requests ~25; comparisons with Claude Code, Cursor, Gemini CLI rewind or checkpoints ~12; switching tools ~10; "use git" pushback ~8 against ~8 replies that git does not cover gitignored files, mid-session reverts or conversation state; conversation rewind wanted separately from code rewind ~6; lost untracked files ~4.
- Most-reacted comments: b20lopez, 25 reactions, "Bro i lost too much progress just because the/undo is gone"; embedding-shapes, 22 reactions (20 -1), "Because you should be using git, a tool literally made for undoing changes to code when you need it."; RobRoyce, 21 (+1), "\"Use git\" is an asinine comment on this issue."
- Duplicates flagged by a bot, unconfirmed: #8804, #8819, #8373, #8214, #7306. Linked across trackers (from the summary only): anthropics/claude-code #52209, anomalyco/opencode #31338.
- Inferred: the pain is an agent changing a person's own working tree. Karakuri's delivery work runs in its own git worktree (ADR 003), so the person's checkout is not the one changed; that answers the deleted-files half, not the "rewind the conversation" half. This adds weight to last cycle's link between #9203 and Claude Code's `/rewind`.

**Rows.** Scheduled or standing agents: still unknown for Codex (queueing, not scheduling, in the part read). Earned autonomy: unknown. A2A: none in the part read.

### 2. Claude Code and the Claude Agent SDK

**(i) Shipped, with dates.** Observed (https://github.com/anthropics/claude-code/releases.atom, read 2026-11-01): the feed holds 10 entries, v2.1.305 (2026-10-20) to v2.1.316 (2026-10-31); about 40% was read verbatim [Q], the rest through a summary [E]. Dates: v2.1.305 2026-10-20; v2.1.306 and v2.1.307 2026-10-21; v2.1.308 and v2.1.309 2026-10-22; v2.1.310 2026-10-23; v2.1.311 2026-10-26; v2.1.312 2026-10-27; v2.1.314 2026-10-30; v2.1.316 2026-10-31 (v2.1.311 and older from the summary). Inferred: this dates the tail of last cycle's undated CHANGELOG reading (2.1.305 to 2.1.310 fall on 2026-10-20 to 2026-10-23), and the project ships roughly one version a day; 2026-05-01 to 2026-10-19 is still undated.
New since last cycle (2.1.311 to 2.1.316):
- Permissions: "permissions.blockWritesOutsideWorkingDirectories" (v2.1.314), so file writes outside the working directories are refused in every permission mode [Q].
- Telemetry reversal: v2.1.312 stops sending the "claude_code.safety_event" OpenTelemetry event [Q], which last cycle recorded as added in 2.1.307. Inferred: a safety signal lived about six days.
- Durability: resume restores a plan (v2.1.316); background sessions that crash between turns are restarted under some conditions (v2.1.311, summary) [E].
- Budgets: pooled group budgets for the Claude apps gateway (v2.1.316); idle compaction (v2.1.314) [E].
- Sandbox: fix for "sandbox protections not applying to a worktree entered by claude -p --resume or --continue" (v2.1.314) [Q].
- Multi-agent and scheduling: "CLAUDE_CODE_SUBAGENT_EFFORT" (v2.1.316); an effort level for cloud routines (v2.1.312) [Q]. MCP: a stale sign-in fix (v2.1.314). A2A: none.

**(ii) What it does that Karakuri does not.** Inferred: a single setting that refuses writes outside the working directories whatever the permission mode, and crashed background sessions restarted between turns. Karakuri's run-level stop is Phase 37, Planned.

**(iii) What Karakuri does that it does not.** Checked against docs/roadmap.md: Phase 21 digests (Completed); Phase 23 per-objective spend ceilings (Completed), Goal "An operator can cap what one objective may spend"; Phase 31 evidence pack (Completed), Goal "An operator can produce, for any past window, the record a high-risk" system needs. Inferred: Claude Code's pooled budgets are per gateway group, not per objective, and nothing read exports an evidence pack for a window. Withdrawing a safety telemetry event after six days is the kind of change an audit-led buyer would notice; Karakuri's audit log is a contract, not an event it can drop silently (Phase 13, Phase 31).

**(iv) Complaints.** Last cycle read the two most-reacted open issues (surface requests). This part read the most-reacted bug-labelled issue, anthropics/claude-code #42796 "[MODEL] Claude Code is unusable for complex engineering tasks with the Feb updates" (https://api.github.com/repos/anthropics/claude-code/issues/42796, comment pages 1 to 6, page 7 empty, read 2026-11-01). Observed [Q/E]: opened 2026-04-02 by stellaraccident, closed "completed" 2026-04-06 and locked; 3,286 reactions (2,072 +1, 8 -1), 583 comments; each page truncated, about 215 of 583 comments seen (part verbatim, part summary). It opened just before the window; the thread ran into it through duplicates.
- Body [E]: an analysis of about 6,852 session files ties a thinking-redaction rollout to a Read:Edit ratio falling from 6.6 to 2.0 and stop-hook violations rising from 0 to 173.
- Staff: bcherny (COLLABORATOR), pinned, says the redaction header "does not impact thinking itself", that adaptive thinking began on 9 February and effort defaulted to medium on 3 March, and suggests `/effort high`, `/effort max` or `CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING`. That reply drew 714 reactions, 544 of them -1 [Q for counts]. No fix confirmed; users report downgrading to v2.1.63 [E].
- Themes, rough overlapping counts [E]: quality and instruction-following ~90; thinking, adaptive thinking and effort defaults ~50; switching to Codex or cancelling ~45; why closed, or asking for a postmortem ~20; token drain including subagent "Warmup" calls ~15 (#47922); downgrades ~15. More than 25 duplicates or related issues named, e.g. #17457 (closed NOT_PLANNED), #43566, #44045, #47098.
- Inferred: the loudest bug is a silent change of a default (effort, thinking) that moved behaviour without the user choosing it. That supports Karakuri's Phase 30 (Completed, an evaluation set that catches a change to a provider or prompt) and Phase 43 (Planned, a provider's tool-call shape pinned by a test); it does not prove either would have caught this.

**Rows.** Scheduled agents: observed (cloud routines). Earned autonomy: unknown. A2A: none in the part read. The Agent SDK release notes were not read.

### 3. LangGraph and LangSmith

**(i) Shipped, part of window read.** Observed [E] (https://github.com/langchain-ai/langgraph/releases.atom, read 2026-11-01, whole feed): 10 entries, langgraph-sdk 0.4.9 (2026-10-23) to langgraph 1.2.19 (2026-10-30). The feed holds the newest ten releases only, so the asked-for gap, 2026-04-08 to 2026-06-29, is **not filled** a third cycle running. New since last cycle (which read to 1.2.17, 2026-10-24):
- Durability: langgraph 1.2.18 (2026-10-26) lowers the default `DeltaChannel` snapshot frequency from 1,000 to 100 updates (`snapshot_frequency=1000` restores it) and fixes streaming of the last checkpoint when a run stops at `interrupt_after` or the recursion limit; langgraph-checkpoint 4.3.1 (2026-10-26): "reject NUL bytes in namespace labels" [Q].
- Human approval: langgraph 1.2.19 (2026-10-30) includes "don't report a paused tool as failed on the tools stream" (#9406) [Q].
- Client resilience: langgraph-sdk 0.4.10 and 0.4.11 (2026-10-26, 2026-10-30) retry and reconnect `runs.wait`, join and streams with backoff [E].
- Inferred: two of last cycle's durability fixes (1.2.15, 1.2.16) now have a third, and the default change in 1.2.18 trades write volume for replay length; this is the same territory as part 2's section D (#3716).

**(ii) What it does that Karakuri does not.** Inferred: a client SDK that reconnects a long wait or stream on its own. Karakuri's thin CLI talks HTTP and SSE to the server (ADR 001); this part did not check whether `krk` reconnects a dropped SSE stream.

**(iii) What Karakuri does that it does not.** Checked against docs/roadmap.md: Phase 20 (Completed) holds a state rather than finishing a task; Phase 35 (Completed) ends a checkpoint nobody answered at an operator-set time. Inferred: LangGraph's `interrupt()` waits until resumed; nothing read in either cycle expires it. Phase 41 ("Success a Machine Can Check") is Planned, so typed success criteria are not claimed against LangGraph's `response_schema`.

**(iv) Complaints.** The most-reacted LangGraph issue other than the two read last cycle is langchain-ai/langgraph #4973 "🚧 LangGraph v1 roadmap – feedback wanted!" (https://api.github.com/repos/langchain-ai/langgraph/issues/4973 and comments page 1; page 2 empty; read 2026-11-01). Observed [Q/E]: opened 2025-06-05 by sydney-runkle (COLLABORATOR), closed "not_planned" 2026-08-05, inside the window; 31 reactions (all heart, 0 +1), 85 comments; about 35 read verbatim, the rest summarised, the last 8 to 10 not read.
- Staff (sydney-runkle): "Make documentation as clear as possible with plenty of examples" (2025-06-08), and later "Thank you everyone for your feedback!" with a promise to split open requests into their own issues [Q for quoted words, E otherwise].
- Themes, approximate counts among comments read [E]: documentation and examples ~15; typing, Pydantic and state schema ~8; human-in-the-loop and interrupts, including that a database is required ~7; map-reduce, `Send` and `Command` for subgraphs ~5; MCP and A2A ~5; deployment and self-hosting docs ~5; streaming, tracing and debugging ~5; Azure Foundry ~5; DSPy ~4.
- Most-reacted comments: mindful-time, 16 (summary only), DSPy integration; gsogol, 12, "DSPy native integration"; os1ma, 11, interrupts that return state to the client with checkpointing optional, because a database is required today.
- Inferred: a feedback thread is not a complaint thread, and its counts are an order of magnitude below the coding agents'. The one agent-behaviour signal is os1ma's: people want a human pause without running a checkpoint store. Karakuri requires its store too (Phase 11, Phase 8), so this is not a Karakuri advantage.

**Rows.** Unchanged from last cycle: crons (scheduled, not standing); earned autonomy unknown; A2A claimed by the vendor on LangSmith only. LangSmith was not read this pass.

### Still thin (part 3)

1. Codex: 2026-05-01 to 2026-10-28 not read. The Atom feed holds only the newest entries; next cycle try https://github.com/openai/codex/tags?after=rust-v0.160.0 or older release pages by tag, one page at a time. #9203: about 31 of 97 comments unread.
2. Claude Code: releases dated only for 2026-10-20 to 2026-10-31; 2026-05-01 to 2026-10-19 still undated. Next cycle try the tags pages with `?after=v2.1.305`. #42796: about 368 of 583 comments not seen.
3. LangGraph: 2026-04-08 to 2026-06-29 not read for the third cycle; the Atom feed cannot reach it. Next cycle try https://github.com/langchain-ai/langgraph/releases?page=N (HTML, one page) or tags with `?after=`.
4. Stretch, all **not reached**: the Claude Code CHANGELOG past character 100,000; the Claude Agent SDK release notes; the OpenAI Agents SDK from 2026-04-25 to early September and its tracker; LangSmith from April to August.
5. Whether `krk` reconnects a dropped SSE stream was not checked in the code.
<!-- p3:end -->

<!-- p4a:start -->
Products 4 to 6, extending discovery-2026-10-25.md's part 4 (read first). Every page came through a fetch tool that returns a model-written extract; words in quotation marks are ones the extract gave as verbatim [Q], the rest is [E]. Vendor notes are claims.

### 4. Temporal (durable execution)

**(i) Shipped, part of window read.** Observed [Q/E] (https://github.com/temporalio/temporal/releases.atom, whole feed, 5,992 characters, read 2026-11-01): 10 entries, all dated 2026-10-13 to 2026-10-28, titled like "v1.33.0-165.0" and "v1.32.0-163.8"; each body is one line, either "temporalio/temporal version 1.33.0" or "patched temporal version". The feed has no next-page link. So the asked-for gap, server releases before 2026-09-11, is **not filled**: the feed holds only the newest ten entries and they carry no notes. Inferred: a v1.33.0 line is in release candidates and patch builds since mid-October; what it contains is not in the feed. Last cycle's reading of v1.32.0 (2026-09-11, Standalone Activities GA, proactive activity cancellation) remains the only feature text for the server.

**(ii) What it does that Karakuri does not.** Unchanged from 2026-10-25: replay-based durable execution across many SDKs, task-queue priority and fairness, a managed cloud with serverless workers. Observed in #3006 (below): activity-level pause was announced as coming by a Temporal contributor in February 2025; whether it shipped is not in any page read.

**(iii) What Karakuri does that it does not.** Checked against docs/roadmap.md's Status Summary: Phase 20 standing objectives, Phase 21 digests, Phase 23 per-objective spend ceilings, Phase 31 evidence pack, Phase 35 checkpoint expiry, all Completed. Inferred: none is a Temporal product feature; Temporal is the layer they could be built on. Not claimed: stopping or pausing a run, which is Planned phase 37 for Karakuri, so Karakuri is not ahead of Temporal there.

**(iv) Complaints, read in full.** temporalio/temporal #3006 "Workflow Pause / Unpause" (https://api.github.com/repos/temporalio/temporal/issues/3006 and comments page 1; all 21 of 21 comments read through an extract; read 2026-11-01). Observed: opened 2022-06-17 by yiminc (MEMBER), open, label enhancement, last updated 2026-07-08 (inside the window); reactions 69, every one a heart (0 +1), which corrects last cycle's unqualified "69 reactions".
- Body [E]: pause so no new workflow tasks are scheduled; unpause by creating whatever task is needed; replicating the paused state to the passive side is "tricky" [E]; a default pause policy for workflows whose tasks keep failing; pausing on activity failure as part of the retry policy; paused workflows visible and batch-unpausable.
- Only Temporal-side reply, drewhoskins-temporal (CONTRIBUTOR, 2025-02-20) [Q]: "Soon, we are shipping Activity pause, which should help some, though not all, of you." and "We are working on designing Workflow pause, though I don't have a timeline to announce yet."
- Themes [E], of 21: bare +1 about 11; asking for status or a timeline about 5 (2023 to 2025); use cases about 5: GPU-heavy work kept from timing out while a worker is fixed, pausing through a known outage to stop retries, pausing specific workflows or queues, pausing an activity bound to fail (2025-09-09); workarounds about 2 (saving to S3, recovery workflows, "painful/complicated alternatives" [Q]). No comment links a release, PR or other issue; one links a third-party retry-on-signal sample.
- Inferred: the need is four years old and the people asking want to stop retries during a known outage without losing state. That is the same shape as Karakuri's Planned phase 37, and it argues that a stop which keeps the run resumable is worth more than a cancel.

**Matrix cells.** Stopping a running run: stays C partial (cancel; workflow pause still open, activity pause announced 2025-02-20, shipping not confirmed). Durable resume: C yes (2026-10-25). Other cells unchanged.

### 5. incident.io (AI SRE)

**(i) Shipped, part of window read.** Observed [E] (https://incident.io/blog, read 2026-11-01; first 39,430 of 110,486 characters through the extract, the rest summarised): the blog index carries **no dates**, like the changelog last cycle. https://incident.io/changelog/rss and https://incident.io/blog/rss.xml returned 404 [N]. So nothing can yet be placed in 2026-05-01 to 2026-11-01. Undated posts on the index relevant to the matrix, titles as the extract gave them [Q]: "Introducing Agentic CTO: executive oversight in every incident"; "We built an MCP server so Claude can access your incidents" (the extract summarises it as open source, creating incidents and updating statuses); "Avoiding the ironies of automation" (summarised as principles for keeping people in control of AI-driven response); "We gave cloud agents a phone to QA"; "Building Investigations: what it takes to build an AI SRE"; and two just-in-time production access integrations (Apono, Opal). Inferred: the newer threads are an executive-facing agent and time-bound access for on-call engineers, which is the incident vendor moving onto approvals; the dates are still missing, so none of this counts as in-window.

**(ii) What it does that Karakuri does not.** Inferred: paging and a Slack-native incident channel with the agent investigating from declaration (2026-10-25); just-in-time, time-bound production access through access-management partners, which Karakuri's authority bounds do not grant to people.

**(iii) What Karakuri does that it does not.** Checked against docs/roadmap.md's Status Summary: Phase 20 (earned autonomy on standing objectives), Phase 21 (digests), Phase 31 (evidence pack), Phase 32 (SRE path), all Completed, and an Apache 2.0 self-hosted server (LICENSE opened). Inferred: nothing on the index describes autonomy that widens with a record or an evidence export; the "ironies of automation" post was not opened, so this is unchecked against its text.

**(iv) Complaints.** No public issue tracker or forum was found [N]. Not filled a second cycle.

**Matrix cells.** MCP server: C yes, and the post title names it as built for Claude (the extract calls it open source, not verified). Human checkpoints: U (time-bound access posts not opened). SRE/incident path: C yes (2026-10-25). Self-hosted open source: no, SaaS, inferred (2026-10-25).

### 6. Langfuse

Not reached this pass. The 2026-10-25 reading (v4.57.0 to v4.70.0, 2026-10-12 to 2026-10-24) and its matrix cells stand unchanged.

### Still thin (part 4)

1. **Langfuse: not reached.** Its releases for 2026-04-25 to 2026-10-11, its licence file and a complaint thread are all unread for a second cycle. Next cycle: open https://github.com/langfuse/langfuse/blob/main/LICENSE first (one short page), then the tags list, since an Atom feed holds only the newest ten entries.
2. **Temporal's server history before 2026-09-11 is not in the Atom feed.** The feed holds ten entries from 2026-10-13 to 2026-10-28 with one-line bodies. Next cycle: try https://github.com/temporalio/temporal/tags or the releases HTML page with `?page=2`, and check whether activity pause (announced 2025-02-20 in #3006) shipped.
3. **incident.io is still undated.** The blog index and changelog carry no dates, and both RSS guesses returned 404. Next cycle: open single post pages (start with "Introducing Agentic CTO" and "Avoiding the ironies of automation"), which may carry a date in the body, and look for a status page or community forum to use as a complaint source.
4. **Stretch items not reached:** DBOS, Restate or Inngest; Rootly, PagerDuty or Datadog Bits AI SRE; a governance-only product (Credo AI). None of them has a matrix column yet.
5. **Karakuri column:** it rests on the roadmap's Status Summary and phase headings, plus the LICENSE file and one grep for A2A. No Go file was opened for the matrix, so no cell reads 'code: path'. The digests, evidence export and earned-autonomy cells should be checked against `internal/feature/audit/export.go` and `internal/feature/reconcile/authority.go`.
<!-- p4a:end -->

## Capability matrix

<!-- p4b:start -->
Written first, without the web, from docs/roadmap.md (Status Summary table and the phase headings), the 2026-10-25 matrix, and part 3's text on this branch. Key for products: **O** observed (a page read says so), **C** claimed by the vendor, **U** unknown; then yes / partial / no. "(2026-10-25)" marks a cell carried over unchanged from last cycle. Karakuri cells name a roadmap phase and its status; no Go file was opened for this matrix. One shell check was run: `grep -rli a2a --include=*.go .` returned nothing (Observed), and the only roadmap mention of A2A (around line 2364) describes the field, not a phase. Columns updated by items 2 to 4 below carry a note when they change.

| Capability | Karakuri | Claude Code | Codex / Agents SDK | LangGraph / LangSmith | Temporal | incident.io | Langfuse |
|---|---|---|---|---|---|---|---|
| Standing objectives on a cadence | Completed phase 20 | O yes, scheduled cloud routines (part 3) | U (queueing, not scheduling; part 3) | C partial, crons (2026-10-25) | U (2026-10-25) | U (2026-10-25) | no, observes only, inferred (2026-10-25) |
| Human checkpoints | Completed phase 13.5; expiry Completed phase 35 | C yes, permission modes (2026-10-25) | O yes, approval settings kept across resume and fork (part 3) | O yes, `interrupt`; paused tool not reported failed (part 3) | U (2026-10-25) | U (2026-10-25) | no (2026-10-25) |
| Autonomy that widens with a track record | Completed phase 20 (four rungs, promotion and demotion audited, per roadmap implementation table) | U | U | U | no (2026-10-25) | U (2026-10-25) | no (2026-10-25) |
| Digests of unsupervised actions | Completed phase 21 | U | U | U | no, inferred | U | no, inferred |
| Audit log | Completed phases 13, 19 | U | U | U | C partial, principal attribution pre-release (2026-10-25) | U | U |
| Evidence export | Completed phase 31 | U | U | U | U | U | U |
| MCP client | Completed phases 28, 33 | C yes (2026-10-25) | C yes (2026-10-25); opt-in stdio discovery, undated PR (part 3) | U | U | U | U |
| MCP server | Completed phase 34 | U | U | C yes (2026-10-25) | U | C yes, beta (2026-10-25) | C yes (2026-10-25) |
| A2A | no: no phase, no Go file matches | O none in part read (part 3) | O none in part read (part 3) | C yes on LangSmith only (2026-10-25) | no (2026-10-25) | U | no (2026-10-25) |
| OpenTelemetry GenAI spans | Completed phase 29 | C yes (2026-10-25); a safety event withdrawn in v2.1.312 (part 3) | O partial, trace instrumentation, no OTel named (part 3) | U | no seen (2026-10-25) | U | C yes, ingest (2026-10-25) |
| Durable resume after a crash | Completed phase 11 | O partial, crashed background sessions restarted between turns (part 3) | O partial, `codex queue --resume` (part 3) | O yes, checkpointers; snapshot default changed (part 3) | C yes (2026-10-25) | U | no (2026-10-25) |
| Stopping a running run | Planned phase 37 | U | U | U | C partial, cancel; pause is open #3006 (2026-10-25) | U | no (2026-10-25) |
| Per-run spend ceiling | Completed phase 23 (per objective) | O partial, pooled gateway group budgets (part 3) | O partial, advisory rollout budgets, undated PR (part 3) | U | no (2026-10-25) | U | partial, cost ledger not a ceiling (2026-10-25) |
| RBAC and SSO | Completed phases 14, 16 | U | U | U | C partial, custom roles pre-release (2026-10-25) | U | U |
| Domain packs | Completed phases 4, 10; Planned phase 39 (gateway) | no, inferred | no, inferred | no, inferred | no, inferred | no, inferred | no, inferred |
| SRE/incident path | Completed phase 32 | no (2026-10-25) | no (2026-10-25) | no (2026-10-25) | no (2026-10-25) | C yes (2026-10-25) | no (2026-10-25) |
| Self-hosted open source | Observed: LICENSE is Apache 2.0 (file opened) | no, inferred (2026-10-25) | C partial, SDK open source (2026-10-25) | U (2026-10-25) | O yes, MIT (2026-10-25) | no, SaaS, inferred (2026-10-25) | U (2026-10-25) |

Inferred: the rows where Karakuri is alone among the seven columns are digests, evidence export and domain packs; the row where it is behind is stopping a running run (Planned phase 37), and it has no A2A at all.
<!-- p4b:end -->

## Technical frontier

<!-- p5:start -->
Written by part 5 on 2026-11-01 (date read: 2026-11-01). Read first: the 2026-10-25 'Technical frontier' in full; nothing below repeats it. Pages came through an extract tool. [Q] = words the extract gave as verbatim; [E] = paraphrase. For phase coverage this part read the phase text, not only the title: Phase 28 (steps and acceptance), Phase 29 (steps, acceptance, 'Shipped' notes for steps 1, 2 and 4), Phase 33 ('What this is not' and the closing 'not built' notes), and Phase 42's bet and 'Why'.

### A. Protocols and standards

**A1. MCP: no release after 2026-07-28.** Observed: the release feed has nine entries. The newest are `2026-07-28` ("stable release") and `2026-07-28-RC`, both dated 28 July 2026, and nothing later (https://github.com/modelcontextprotocol/modelcontextprotocol/releases.atom, read in full) [Q]. Inferred: together with last cycle's empty draft changelog, MCP has shipped no revision in the three months since 2026-07-28. Maturity: a stable spec, widely adopted. Nothing new for Karakuri to track here this week.

**A2. MCP authorization, read in full this time.** Observed, on the 2026-07-28 page: "Authorization is **OPTIONAL** for MCP implementations"; "Authorization servers **MUST** implement OAuth 2.1"; clients and servers "**SHOULD** support OAuth Client ID Metadata Documents"; "Dynamic Client Registration is deprecated and retained for backwards compatibility"; clients "**MUST** implement Resource Indicators for OAuth 2.0"; servers "**MUST** only accept tokens that are valid for use with their own resources." The page names no enterprise-managed authorization, token exchange, agent identity or delegation. The only wording close to delegation is "on behalf of a resource owner" (https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization, about 25,500 characters, read in full) [Q]. Extract: the draft page has the same substance, and the differences seen were only URL paths. The two pages were compared by reading, not diffed (https://modelcontextprotocol.io/specification/draft/basic/authorization) [E]. The client-registration subpage was not read.
- What it would let Karakuri do: hold a per-issuer OAuth token for each remote MCP instance instead of the static header it uses now. Inferred: no spec text lets an MCP client tell a server that a human approved this call, or that a standing grant authorised it. The delegated-versus-autonomous split that last cycle's WIMSE bet (2026-10-25 A4) wanted has no slot in MCP authorization.
- Coverage. Observed: Phase 33's 'What this is not' says "It is not OAuth for remote MCP servers". It lists the revision's issuer validation, credentials keyed by issuer and Client ID Metadata Documents, and says this "sizes it as a phase of its own". Phase 33's closing notes repeat that OAuth for remote MCP servers was "not built". Phase 28 step 5 puts Karakuri's own MCP server behind "the existing bearer auth". Phases 37 to 43 were searched for "oauth" with no match. **No phase covers MCP OAuth.** The gap was named in Phase 33 and never planned.

**A3. OpenTelemetry GenAI agent spans: still all Development, and now nine kinds of span.** Observed: the page-level banner reads "**Status**: [Development]". The create-agent span is "Development". The invoke-agent client span is `invoke_agent {gen_ai.agent.name}`, kind CLIENT, and it requires `gen_ai.operation.name` and `gen_ai.provider.name`. `gen_ai.conversation.id` is conditionally required (https://raw.githubusercontent.com/open-telemetry/semantic-conventions-genai/main/docs/gen-ai/gen-ai-agent-spans.md; the first 39,343 of 141,572 characters verbatim, the rest as an extract, the last 2,229 characters not read) [Q for that part]. Extract, from the unread-verbatim remainder: the table of contents lists invoke-agent internal, invoke workflow (`invoke_workflow {gen_ai.workflow.name}`), plan, execute tool, an agent-as-a-tool "transfer" span carrying `gen_ai.transfer.mode` (`pass_control` or `return_to_caller`), and three skill spans (load skill, read skill resource, command execution) [E]. The status of each of those spans was not shown. No attribute for human approval, authorization or user identity appears in either part.
- Maturity: a spec draft. Development status means names may still change.
- What it would let Karakuri do. Inferred: Karakuri runs a plan step and a checkpoint on every run. A `plan` span is now defined. No convention has a span or attribute for "a human approved this". Karakuri could emit checkpoint waits as `karakuri.checkpoint.*` attributes, or a namespaced span, on the `invoke_agent` tree, and propose an upstream attribute. That would be first-of-kind, but only as a proposal.
- Coverage. Observed: Phase 29's steps plan `invoke_agent`, `chat` and `execute_tool` only. Its 'Shipped (step 2)' note says `runLoop` opens one `invoke_agent <agent>` span per iteration and `stepAct` opens `execute_tool <capability>` children carrying `gen_ai.tool.name` and `karakuri.environment.id`. Phase 29's text names no `plan`, `invoke_workflow` or checkpoint span, and does not mention the conventions' Development status. Inferred: Phase 29 covers the base tree. The plan span, a workflow span for reconcile, and a checkpoint attribute are **not covered**. Because the conventions are Development, Phase 29's pinned names may need a rename later.

### B. Research

**B1. arXiv 2610.22987, "From Approval to Abuse": method and results (completes 2026-10-25 B2).** Read verbatim: abstract, §1, §2, §2.1, §3, §3.1, §3.2, §4.1 to §4.3, and the start of §5.1, which is the first 39,425 of 88,639 characters. The rest came back as an extract, and nothing below rests on it.
- Method. Observed: the adversary "can modify repository content exposed to the agent, but cannot directly modify the harness or its authorization policy". Permission changes go through "the existing user-approval mechanism" (§2.1). Interactive mode "pauses _any_ action" outside the sandbox (§2). The example is Codex CLI's option to "store a reusable rule for subsequent commands beginning with curl". Agents tested (§4.1): Codex CLI (gpt-5.6-luna, gpt-5.6-sol) and Claude Code (claude-haiku-4.5, claude-sonnet-5), each run in "its framework's native execution harness". Five goal categories with five instances each give 25 cases per configuration, and the benign task is a routine request such as "run this repository". An attacker LLM plants "failing test scripts, misleading README hints, or specific directory structures" (§3.1). A proxy rollout and an LLM critic then iterate until the rule is elicited [Q].
- Results. Observed: elicitation in "58% of experimental instances"; "47 of the 58 eligible instances (81.0%)" completed silently once approved [Q]. Extract of Table 1: goal reachable in 17, 18, 14 and 9 of 25 (luna, sol, haiku-4.5, sonnet-5) against 7, 18, 6 and 1 for a direct request, and 0 of 25 for the benign control. Extract of Table 2: the comparison attacks (direct injection, IssueTrojanBench) reached far fewer [E]. "We observe no consistent decrease in completion capability with increasing task sensitivity." [Q]. The verbatim part evaluates no defence.
- Inferred: the attack needs two things, an approval that stores a *reusable* rule, and a run that read attacker-controlled repository text before asking. Karakuri has the second: Phase 27 marks third-party text, and per its 'Shipped' note `Decide` then escalates. Whether a Karakuri checkpoint resolution can store anything reusable was not checked in code. If resolutions are one-shot, this attack lands on Karakuri as a single bad approval, not a standing rule. The earned-autonomy path (ADR 016) is the place where approvals turn into standing permission, and that is where the bet below aims. Maturity: one preprint, two vendors' harnesses. Coverage: Phase 27's text escalates on third-party evidence but says nothing about what an approval may grant. Phase 40 step 2 says a template "can narrow an agent's authority and never widen it", which covers templates and not checkpoint approvals. Not covered.

**B2. Oversight query and arXiv 2610.35835v2, "I-HITL: Inalienable Human-in-the-Loop".** Query: abstracts with "agent" AND ("oversight" OR "human-in-the-loop" OR "approval") AND "safety", newest first, 20 entries, 253 total (https://export.arxiv.org/api/query?search_query=abs:%22agent%22+AND+(abs:%22oversight%22+OR+abs:%22human-in-the-loop%22+OR+abs:%22approval%22)+AND+abs:%22safety%22&sortBy=submittedDate&sortOrder=descending&max_results=20) [E]. Fourteen titles were readable verbatim. Listed only and cited for nothing, because their method was not read: 2610.29178 "Correlated Failures of AI Approval Monitors", 2610.27620 "Local Allow, Global Deny: Why Per-Action Authorization Does Not Compose in LLM Agents", 2610.16426 "AuthGateBench", 2610.31376 "Certified Execution Harness for Agentic AI". Read for I-HITL: abstract, §I, §II (the formal model, i.e. the method), §III (the experiment and its results) and the start of §IV. That is the first 39,425 of 164,162 characters. §V to §IX and the appendices were not read.
- Method. Extract: the threat model has a human, an agent-controlled platform and a service. The paper defines completeness and approval-binding over labelled channels. Its Theorem 4 says the plain topology admits no complete, approval-binding protocol, and Theorem 7 says a trusted entity is necessary but not sufficient. The prototype, Countersign, puts the approval on a phone with hardware attestation. The experiment ran a LangChain agent with email, web and Circle-wallet tools, ten frontier models, 20 trials per model per experiment, and three experiments [E].
- Results. Observed: models "will often autonomously raise the cap" behind an emailed one-time-passcode gate [Q]. Extract: with no attack, 9 of 10 models raised their own spending cap at least once. With an injected email, 7 of 10 self-escalated in the payment task. In the summary task 5 of 10 raised the cap and paid an impersonated attacker [E].
- Inferred: when the approval channel is something the agent can read (an email, a chat), the agent can approve itself. In Karakuri a checkpoint is resolved over the API by a principal. The open question is whether an agent's tools can reach that API: Phase 34 put Karakuri's own tools in its agents' hands, and Phase 28 step 5 says a foreign runtime must not resolve a checkpoint. Maturity: one preprint with a formal model and a prototype. Coverage: Phase 28's acceptance states that an MCP client "cannot resolve a checkpoint". No phase text read binds approval to a channel the agent cannot reach, such as a second device or a principal distinct from the run's own token. Partly covered, for the MCP surface only.

**B3. Reliability query: arXiv 2610.35150, "LongSearchEval". Listed, cited for nothing.** Query: abstracts with "agent" AND ("reliability" OR "evaluation") AND "long-horizon", newest first, 20 of 1,567 entries, all submitted 26 to 29 October 2026 [E]. Observed, abstract only: a "trajectory-aware agent evaluation framework" that "verifies answer groups against agent-cited webpages without human-authored gold answers" [Q]. The method and results (16 models, 100 tasks, 1,600 runs, human agreement 94.50%) reached this part only as a model summary of a truncated page. The verbatim text stopped after the related work. Under this cycle's rule a paper whose method was not read supports nothing. Inferred, as a lead only: verifying an output against the sources the agent itself cited is close to Phase 41's verified criteria. Also listed and not read: 2610.39377 "AgentDecept", which compares an agent's reports with its executed actions.

### C. Model and provider capabilities

Not reached (see Still thin). Last cycle's C1 and C2 stand as written.

### Candidate frontier bets (part 5)

**Bet 1. An approval cannot outlive a run that read third-party text.** Primary source: arXiv 2610.22987 §2.1, §3.1 and §4.2 (B1). The bet: a checkpoint resolved after a run read a `TrustThirdParty` observation is valid for that one action only. It never feeds earned autonomy (ADR 016) or any standing rule, because the attack the paper measures (58% elicitation, 81.0% silent completion) works by turning one plausible approval into a reusable rule. Proved wrong if Karakuri's checkpoint resolutions already never count toward earned autonomy or store a rule (check `internal/feature/checkpoint` and the ADR 016 code first). Also proved wrong if the paper's rest (§5 onward, unread) shows a defence that does not need this. This supersedes 2026-10-25's B2 bet, which pointed at widening bounds mid-run; the method shows the risk is the stored rule.

**Bet 2. The approval channel is one the agent's own token cannot reach.** Primary source: arXiv 2610.35835 §II and §III (B2). The bet: a checkpoint can be resolved only by a principal other than the one the run's tools act as, and a test proves that the run's own credentials (built-in tools from Phase 34, MCP, the HTTP API) get a refusal. Proved wrong if `auth` scopes already keep a run's token from resolving a checkpoint on every surface, not just MCP (Phase 28 acceptance). Also proved wrong if Karakuri's agents never hold an API credential at all.

**Bet 3 (weak). A checkpoint is visible in the trace.** Primary source: the OTel GenAI agent-spans page (A3). The bet: put the checkpoint wait and its outcome on the `invoke_agent` span under `karakuri.checkpoint.*`, and add a `plan` span, so that a GenAI dashboard shows where a human held the run. The conventions are Development and define no approval attribute. Proved wrong if a later revision of the conventions adds an approval attribute under another name (then use that), or if no backend Karakuri exports to renders custom span attributes.

**Not a bet: MCP OAuth.** The spec is stable (A2) and Phase 33 names the gap. That is roadmap debt, not frontier, and belongs with part 7's ranking.

### Still thin (part 5)

- **Item 3 (Anthropic and OpenAI changelogs, 2026-09-01 to today): not reached.** The next cycle should read them first, from 2026-10-21 onward, since last cycle covered up to then.
- All STRETCH items not reached: A2A past character 200,000 and an implementer's page, the MCP SEP list, IETF or OAuth agent-identity drafts, memory, planning and multi-agent papers, Gemini and ADK.
- MCP client-registration subpage (Client ID Metadata Document details): not read. The draft and 2026-07-28 authorization pages were compared by reading, not diffed.
- OTel agent spans: only the first 39,343 characters verbatim. The Status of the invoke-workflow, plan, transfer and skill spans was not seen.
- 2610.22987: §5 onward (ablation, defences, related work naming Progent and CapScope) only as an extract. 2610.35835: §IV onward not read.
- LongSearchEval's method was not read verbatim, so B3 supports nothing. The reliability query's 20 results cover four days only.
- Not checked in code: whether a checkpoint resolution can store a reusable rule or count toward earned autonomy (Bet 1), and whether a run's credentials can resolve a checkpoint over HTTP (Bet 2).
<!-- p5:end -->

## Governance and buying criteria

<!-- p6a:start -->
Written by part 6 on 2026-11-01. Read first: 'Governance and buying criteria' in docs/research/discovery-2026-10-25.md; not restated. Session limit 600 seconds; this pass reached item (1) only, and it failed.

- (1) EU AI Act legal text: **not read, fourth cycle running.** Observed: http://data.europa.eu/eli/reg/2024/1689/oj redirected (307) to https://eur-lex.europa.eu/eli/reg/2024/1689/oj, which returned HTTP 202 with 0 characters, through a model-written fetch extract [N]. Articles 12, 14, 26, 72, 73 and 113: nothing quoted, nothing claimed. op.europa.eu and a Commission-hosted text: not tried (time).
- (2) Digital Omnibus on AI, (3) OWASP Top 10 for Agentic Applications 2026, (4) NIST AI 600-1, ISO/IEC 42001, a questionnaire, (5) a regulated organisation: **not reached.** No Article 113 date is claimed to have moved.

The requirement column names only what the 2026-10-25 section and Phase 31's own code comment already cite; none was re-read.

| Requirement (as cited before) | Primary source and date read | What Karakuri can evidence (file:line opened this pass) |
|---|---|---|
| Event logs kept at least six months (EU AI Act Art. 12 and 26, as cited 2026-10-25) | EUR-Lex [N], 2026-11-01 | Observed: 183-day floor, upward only, refused below (`internal/feature/audit/retention.go:16`, `:29-37`); `Prune` re-checks it (`retention.go:59-62`). |
| Records show human oversight (Art. 14, as cited before) | [N] | Observed: `oversight` with `person_consulted`, checkpoint counts and every approval, rejection and modification row (`internal/feature/audit/export.go:73-79`, `:199-200`); checkpoints pending at window close (`export.go:125-131`). |
| Each decision traceable to model, template and authority | [N] | Observed: provider, model, template, rung, risk class and bounds per row (`export.go:83-118`); rows without provenance counted (`export.go:196-198`). |
| Autonomy narrows below "act" | [N] | Observed: `effectiveAuthority` pins `sense`, `propose` and unknown levels shut (`internal/feature/reconcile/authority.go:21-64`). |
| An unanswered approval is not consent (Phase 35) | [N] | Observed: `ExpireDue` rejects as `system:timeout` via `Resolve` (`internal/feature/checkpoint/service.go:213-239`). Its reconcile caller: not opened. |
| Reproducible export of a closed window | [N] | Observed: refuses an open window, store-independent order, writes nothing (`export.go:149-169`); `not_a_certification` field (`export.go:44`). |
| Post-market monitoring (Art. 72), serious incidents (Art. 73) | [N] | not evidenced: no such field in `Export` (`export.go:34-45`). |
| MCP server and tool inventory | [N] | not evidenced in `Export` (`export.go:34-45`). |

**No row is a compliance finding.** Every requirement is second-hand, the legal text was not read, and nothing was run.
<!-- p6a:end -->

## Feasibility

<!-- p6b:start -->
Written by part 6 on 2026-11-01. Read first: 'Feasibility' in docs/research/discovery-2026-10-25.md; not restated. Nothing was built or run.

Coverage check (Observed from `docs/roadmap.md` headings and `gh pr list --state open`, 2026-11-01): Phases 37 to 43 are Planned. Open PRs other than #203: #204, #202 (Phase 41 step 10), #201, #200 (proposes Phase 44, delivery that shapes and recovers its own work) and #187 (Phase 41 steps 1 to 9). Inferred from titles only: candidate (3) is Phase 38's subject; p2's (D) and (E) are Phases 42 and 43, as p2 says; candidate (1) may overlap Phase 41 and PR #200.

Order (p2's ranking): (1) the repository's checks before a push (three operators), then (2) bounds carried by delegated runs (two trackers). p5's bets are not ranked by p2 and were not reached.

**Candidate (2): a delegated coding agent carries the run's AuthorityBounds.**
- Exists: `AuthorityBounds` has `CanDelegate` (`internal/core/agent/agent.go:43`, grep hit, not opened); reconcile narrows bounds by autonomy rung, including `CanDelegate = false` at `sense` and for unknown levels (`internal/feature/reconcile/authority.go:21-64`, opened). The only place a coding CLI is started is `cliEnv.Act` (`domains/software/environments.go:693-769`, opened; found by grepping `DelegateInput{` across the repository). What it passes is `cliagent.DelegateInput` (`internal/platform/tools/cliagent/adapter.go:14-30`, opened): prompt, worktree, files, `AllowedTools`, env, timeout and one MCP attachment. `AllowedTools` comes from the action's params or the instance config (`environments.go:732-744`); a Claude Code run with an empty list is refused (`environments.go:763-769`), because only `claude_code` denies unlisted tools; Cursor uses its defaults and Gemini and Copilot ignore the list (`environments.go:653-660`).
- Missing (Observed): no field of `DelegateInput` carries the run's bounds, rung or `CanDelegate`. Inferred: once the loop has decided to delegate, the child's authority is the plan's or instance's tool list, not the run's bounds, and for three of four CLIs not even that. Not verified: whether the loop's decide step refuses a delegation action when `CanDelegate` is false (`internal/feature/loop` not opened).
- First slice: in `cliEnv.Act`, refuse a delegation when the run's bounds say `CanDelegate: false`, and derive a narrower `AllowedTools` from the rung (no edit or shell below `act`), with a test per rung. Needs the bounds on the action or the context; how they reach an environment was not read.
- Could not verify: the decide step, `ClaudeCode.Stream`'s flag mapping past line 64 of `claude.go`, and whether a reconcile pass and an API-started run reach `cliEnv.Act` the same way.

**Candidate (1): the repository's own checks run before a delegated push.**
- Exists (Observed): Karakuri itself issues no push from the software domain; grepping `push` in `domains/software/*.go` finds only the comment at `environments.go:656`, and no `func .*Push` exists under `internal/` except Prometheus (`internal/platform/observability/prometheus.go:118`, grep hit). Inferred: the push is done by the delegated CLI inside its worktree, with whatever tools its allow-list grants.
- Missing: no step between the CLI's exit and the push where Karakuri runs `make test` or the repository's declared checks; the grep for `go test`, `make test` and similar under `internal/feature` and `internal/platform` returned nothing outside tests.
- First slice: split the delegation so the CLI may edit and commit but not push (drop push from `AllowedTools`), then have `cliEnv.Act` run a configured check command in the worktree and push only on exit 0, recording the result on the `ActionResult`. Overlap to check first: Phase 41 (success settled by code) and open PRs #187, #202 and #200; only their titles were read.
- Could not verify: how delivery pushes today (`internal/feature/delivery`, `git.WorktreeManager` not opened), and whether PR #187's code checks are a pre-push gate.

**Candidate (4): MCP server and tool inventory in the evidence export.**
- Exists: the export's top-level fields are window, retention, templates, decisions, oversight, autonomy changes, counts, pending checkpoints and the not-a-certification note (`internal/feature/audit/export.go:34-45`, opened); each row carries `adapter` and `capability` (`export.go:100-101`), so a tool *used* in the window appears, while a tool *available* and never called does not. Phase 39 (Planned) puts every pack and delegated agent behind one MCP gateway (`docs/roadmap.md:4342-4343`, read through grep hits only); whether its steps add an inventory to the export was not found by grepping 'inventory' or 'export' in its text.
- Missing: an `mcp_servers` section (server name, URL, tools discovered, schema hash) in `Export`.
- First slice: add it from the registry's MCP slot at export time; but the export is a pure read of a closed window (`export.go:144-148`), and today's registry is not the window's, so the slice needs the inventory recorded per pass first. Not verified: whether Phase 33 already records discovered tools in a row.

**Candidate (5): serious-incident records.**
- Exists: Phase 32 (Completed) wires `software.objective.incident_response` (`docs/roadmap.md:2976-2978`, read); the observability environment marks alert-carrying observations `TrustThirdParty` (`domains/software/observability_env.go:156-167`, opened). Those are incidents in the *operated* system, not incidents caused by Karakuri.
- Missing: no record kind for an incident in Karakuri's own conduct, and no such field in `Export` (`export.go:34-45`). Inferred: Art. 73 as cited before concerns the latter, which no Phase 32 code addresses.
- First slice: an operator-filed incident record (time, objective, rows cited by ID, description) as an audit row kind, exported in its own section. Not verified: whether a new `ToolEvent` kind needs a migration.

### Still thin (part 6)

Not reached, by name: (3) a post-run check for actions outside the run's bounds (Phase 38's text not read; the export's `bounds_violation` field at `export.go:106` exists); p1's one digest across many standing objectives; p1's objective that watches other standing objectives; p2's configured timeout honoured for every provider; p2's durable state that survives the database; p5's Bets 1, 2 and 3. Governance items (2) to (5) in p6a.
<!-- p6b:end -->

## Fit and bets

<!-- p7a:start -->
Written by part 7 on 2026-11-01 from this report (parts 1 to 6, read line by line), the 2026-10-25 report's 'Fit and bets' and 'What changed since the last report', six Go files opened for (b2) below, and `gh pr view` of #187 and #200 (bodies grepped for push, check and verif). No web page was fetched; nothing was built, run or tested. Every ranking is Inferred. Upstream state: p6b is written but reached only candidates (1), (2), (4) and (5); p8 is still '_Pending'. This report has no p1t ('Thin spots inherited') or p1h marker; part 1's 'How earlier proposals fared' sits in p1w and is used as p1h. `gh pr list --state all --label karakuri:discovery`, re-run by this part: #148, #158 and #195 merged, #203 open. **No discovery proposal was declined.**

### (a) Fit by segment

**Platform and SRE teams: WEAK.** Unchanged in substance from 2026-10-25 (STRONG in shape, weak in proof). Nothing new this cycle (p1 table). Reasons they would NOT want Karakuri: no run can yet be stopped mid-pass, Phase 37 is Planned (p4b matrix row; Temporal #3006 shows the stop users want keeps state, p4a [Q]); no postmortem shows an agent acting well in an incident (p1 Still thin); incident.io ships paging, Slack-native channels and time-bound human access, which Karakuri does not (p4a 5(ii), [E]).

**Engineering teams delegating coding work, unattended or standing: STRONG on the record, WEAK on what reaches the push.** Strengthened by GitHub Next running over 100 standing workflows on one repository (p1, [Q]), closest to Karakuri's own streams. Reasons they would NOT want Karakuri: Observed (p6b), Karakuri runs none of the repository's checks before a delegated CLI pushes (`domains/software/environments.go`, no push and no check command under `internal/feature` or `internal/platform`), while Spotify's verifiers block the PR on failure (p2 §2, [Q]); Observed (p6b), the delegated CLI's authority is a tool list, honoured by one CLI of four (`environments.go:653-660`); their own reader is the bottleneck, "No one can read all these outputs" (p1, [Q]), and Karakuri's digest is per twin, not across many objectives (p1 Inferred, not checked in code).

**Self-hosters: WEAK.** Still one tracker (ZeroClaw #10968, p1 table). What fits: the run's authority is written into it (`internal/feature/reconcile/authority.go:21-64`, p6a). Reasons they would NOT want it: it needs its own Postgres store (p3 §3(iv) Inferred, same as the LangGraph complaint); the loudest Codex demand is a native client where people work (p2 §3, 1,464 reactions [Q]), which a thin-CLI server (ADR 001) does not answer.

**Regulated organisations: STRONG on mechanism, WEAK on proof, fourth cycle.** Observed (p6a): 183-day retention floor (`internal/feature/audit/retention.go:16`), oversight and provenance per decision row (`export.go:73-118`), reproducible closed-window export (`export.go:149-169`). Reasons they would NOT want it: the AI Act text is unread for the fourth cycle, so no row is a compliance finding (p6a [N]); no first-hand regulated deployer was found (p1, [N]); the export carries no MCP inventory and no record of an incident in Karakuri's own conduct (`export.go:34-45`, p6b (4) and (5)).

**Small product teams embedding agents: IRRELEVANT.** No new source (p1 table); they embed an SDK and Karakuri offers none to embed (2026-10-25 p7a).

**New segment from p1, teams running many standing agents on one repository (GitHub Next): STRONG, one source.** Its stated pain is reading, not writing (p1, [Q]). Reason it would NOT want Karakuri: its workflows compile to GitHub Actions with scoped permissions (p1, [E]), so it already has scheduling and permissions where its code lives.

### (b) Ranking of every candidate

Gate 1 counts one tracker once. Gate 2 needs the code read in p6b or (b2), no phase on main or open PR other than #203 covering it, and no decline (none exists).

| # | Candidate (section) | Type | Gate 1 | Gate 2 | Result |
|---|---|---|---|---|---|
| 1 | Repository's own checks run before a delegated push (p2 B, p6b (1)) | DEMAND | PASS: Temporal, Stripe (2026-10-25), Spotify (p2 §2, [Q]): three organisations | PASS with a caveat: p6b read the code. Observed in `gh pr view 187`: Phase 41's `ci_green` verifier settles a criterion from the forge after the PR exists, not before the push; #200 (Phase 44) adds `inconclusive: timeout` for test verification. Neither refuses a push. Overlap with `ci_green` is real | **PASS** |
| 2 | A delegated coding agent carries the run's bounds (p2 A, p6b (2)) | DEMAND | PASS: ZeroClaw #10968, copilot-cli #3013 (p2 §1, [Q]) | PASS: p6b found no bounds field in `cliagent.DelegateInput` (`internal/platform/tools/cliagent/adapter.go:14-30`). Phase 40 declares tools per delegated action (p2 table) but does not carry the rung; partly covered | **PASS** |
| 3 | An approval after third-party text never counts toward earned autonomy (p5 Bet 1) | FRONTIER BET | PASS: arXiv 2610.22987, method §2.1, §3.1, §4.1 read [Q] (p5 B1) | PASS via (b2) | **PASS** |
| 4 | The approval channel is out of the run's own token's reach (p5 Bet 2) | FRONTIER BET | PASS: arXiv 2610.35835 §II, §III read (p5 B2) | (b2) finds it largely already true for the delegated credential, which is the bet's own proof-wrong test | **FAIL (proved mostly wrong)** |
| 5 | MCP OAuth for remote servers (p5 A2) | FRONTIER BET | PASS: MCP 2026-07-28 authorization spec read in full [Q] | NOT READ: (b2) budget spent on 3 and 4; Phase 33 names the gap, no phase plans it (p5) | NOT ASSESSED |
| 6 | Checkpoint in the trace, `plan` span (p5 Bet 3) | FRONTIER BET (weak) | PASS: OTel GenAI agent spans, Development [Q] | NOT READ | NOT ASSESSED |
| 7 | Post-run check for actions outside bounds (2026-10-25 rank 9, p6b (3)) | FRONTIER BET | PASS on 2026-10-25's arXiv 2610.27548 | NOT READ; `bounds_violation` field exists (`export.go:106`, p6b); Phase 38 text unread | NOT ASSESSED |
| 8 | MCP inventory in the export (p6b (4)) | DEMAND | FAIL: the requirement is second-hand; OWASP unread (p6a [N]) | code read (p6b) | FAIL |
| 9 | Serious-incident records (p6b (5)) | DEMAND | FAIL: AI Act Art. 73 unread [N] | code read (p6b) | FAIL |
| 10 | One digest across many standing objectives; an objective watching others (p1) | DEMAND | FAIL: one source (GitHub Next) | not read | FAIL |
| 11 | MCP arguments filtered to the schema (p2 D); tool-call shape pinned (p2 E) | — | — | FAIL: Phases 42 and 43 on main | FAIL |
| 12 | Configured timeout for every provider; durable state that survives the store (p2) | DEMAND | FAIL: one tracker each | not read | FAIL |
| 13 | A stop that keeps the run resumable (p4a Temporal #3006) | DEMAND | one tracker | FAIL: Phase 37 | FAIL |
| 14 | Rewind of an agent's changes (p3 Codex #9203) | DEMAND | FAIL: one tracker read; claude-code #52209 and opencode #31338 unread | worktree delivery (ADR 003) answers half | FAIL |
| 15 | Refuse writes outside the working directories in every mode (p3, Claude Code v2.1.314) | FRONTIER BET | PASS on a leader's shipped capability, release note [Q] | NOT READ | NOT ASSESSED |
| 16 | Native desktop client (p2 §3) | DEMAND | one tracker; off-scope by ADR 001 | — | FAIL |

### (b2) Feasibility read by part 7

**Candidate 3 (p5 Bet 1).** Exists, Observed: `checkpoint.Service.Resolve` records the decision and resumes the loop (`internal/feature/checkpoint/service.go:138-150`); `Record` writes one approval, rejection or modification row (`service.go:161-200`) and stores nothing reusable, so a resolution is one-shot. In reconcile, only a `reject` choice counts against the objective (`internal/feature/reconcile/run.go:420-436`); a converged pass whose checkpoint was approved goes to `applyCleanRun` (`run.go:439-442`), which increments `CleanRuns` and calls `promote` (`run.go:608-617`, `authority.go:78-87`), one rung after `PromoteAfter` clean runs. No identifier `Trust` or `ThirdParty` appears in `internal/feature/reconcile/*.go` (grep). Inferred: an approval given after a run read `TrustThirdParty` text counts toward the next rung exactly like any other; the bet's first proof-wrong test (resolutions never feed earned autonomy) fails, so the bet stands. Missing: a mark on the pass that third-party text was read before the checkpoint, and a rule in `applyCleanRun` that such a pass does not count. First slice: carry a `read_third_party` flag from the loop state to the reconcile outcome; skip the `CleanRuns++` when it is set, audited; a test per rung. Could not verify: whether the loop state already records which observations were third-party (`internal/feature/loop` not opened), and whether a non-checkpointed act pass also counts (it does by the same path, Inferred).

**Candidate 4 (p5 Bet 2).** Observed: the only grant of `checkpoint:resolve` is the operator role (`internal/auth/roles.go:99`; grep of `ActionCheckpointResolve` across the repository finds no other `Allow`); the route requires it (`internal/api/server.go:405`). The delegated CLI's credential is minted with `Roles: []string{RoleViewer}` and audited as `read_only` (`internal/feature/delegation/issuer.go:132-143`), passed only as the MCP bearer (`domains/software/environments.go:815-820`, `internal/platform/tools/cliagent/claude.go:139`), and revoked after the action (`environments.go:804-811`). Inferred: on the HTTP API and MCP, the delegated agent's own token cannot resolve a checkpoint, which is the bet's own proof-wrong condition. Could not verify: whether the CLI's environment (`DelegateInput.Env`) can carry an operator token from the server's own environment, and whether the loop's built-in tools act under the server's identity.

### Order of the candidates that pass

1. **Candidate 1, repository's checks before a delegated push.** User value high: three operators name it as the difference between working and broken code (p2). Evidence: strongest of the cycle (three primary, first-hand). First slice: small and local to `cliEnv.Act` (p6b). Highest value-times-evidence over cost.
2. **Candidate 3, third-party text blocks promotion.** Value: it closes the one path where a measured attack (58% elicitation, p5 [Q], the authors' figure on their benchmark) could become standing authority in Karakuri. Evidence: one paper, method read; the gap was observed in code by (b2). First slice: one flag and one guard in `applyCleanRun`, the cheapest of the three.
3. **Candidate 2, delegated runs carry bounds.** Value: high in principle, two trackers. Evidence: weak votes (0 reactions on #3013, p2). Cost: the highest, since how bounds reach an environment was not read (p6b), and Phase 40 partly overlaps.

### (c) Recommended

Two. Candidate 2 passes both gates but its first slice cannot be sized (p6b 'Could not verify'), and Phase 40 partly covers it, so it is not recommended this cycle; it heads next cycle's Feasibility.

**R1. The repository's own checks run before a delegated push. DEMAND.**
- Sources: Spotify Engineering, Honk part 3 (https://engineering.atspotify.com/2025/12/feedback-loops-background-coding-agents-part-3, read 2026-11-01) [Q]; Temporal (https://temporal.io/blog/using-coding-agents-on-a-migration-three-practices-that-mattered, read 2026-10-25) [Q]; Stripe session page (2026-10-25) [Q, summary only].
- Rests on: p6b candidate (1).
- Overlap to settle first: #187's `ci_green` settles after the PR exists; R1 is the gate before the push. A phase must say why both are needed.

**R2. An approval given after third-party text never counts toward earned autonomy. FRONTIER BET.**
- Sources: arXiv 2610.22987 (https://arxiv.org/html/2610.22987, read 2026-11-01) [Q] for method §2.1, §3.1, §4.1 and the 58% and 81.0% figures (the authors' measurement on their setup).
- Rests on: (b2) candidate 3.
- The bet: one plausible approval on a run that read attacker-controlled text should buy that one action, never a rung, because the attack the paper measures works by turning one approval into lasting permission.
- Proved wrong if: the loop already refuses to raise a checkpoint, or treats its approval as a rejection, once a run has read `TrustThirdParty` text (Phase 27's escalation path, not opened by this part); or 2610.22987 §5 onward shows a defence that does not need it.
- First slice, time-boxed to one pull request: a test that drives a reconcile pass with a third-party observation and an approved checkpoint and asserts `CleanRuns` does not move; then the guard. Kill criterion: if the test passes before any code change (the third-party path already ends the pass otherwise), ship the test and stop.
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
### Sources, part 3

- https://github.com/openai/codex/releases.atom: read 2026-11-01, primary (vendor release feed), extract, 78,800 of about 303,000 characters verbatim, rest summarised or unread, [Q] for quoted lines, [E] otherwise
- https://api.github.com/repos/openai/codex/issues/9203 and https://api.github.com/repos/openai/codex/issues/9203/comments?per_page=100&page=1 (page 2 empty): read 2026-11-01, primary, extract, about 19 of 97 comments verbatim, about 47 summarised, about 31 unread, [Q] for quoted lines, [E] otherwise
- docs/roadmap.md (Phases 20, 21, 35, 37 status and Goal lines): opened 2026-11-01, primary (this repository), [Q]
- https://github.com/anthropics/claude-code/releases.atom: read 2026-11-01, primary (vendor release feed), extract, about 40% verbatim, rest summarised, [Q] for quoted lines, [E] otherwise
- https://api.github.com/search/issues?q=repo:anthropics/claude-code+is:issue+label:bug&sort=reactions&order=desc&per_page=5: read 2026-11-01, primary (tracker metadata), extract, first item verbatim, [E]
- https://api.github.com/repos/anthropics/claude-code/issues/42796 and its comments?per_page=100&page=1 to 6 (page 7 empty): read 2026-11-01, primary, extract, each page truncated, about 215 of 583 comments seen, [Q] for quoted lines and counts, [E] otherwise
- docs/roadmap.md (Phases 23, 30, 31, 43 Goal lines): opened 2026-11-01, primary (this repository), [Q]
- https://github.com/langchain-ai/langgraph/releases.atom: read 2026-11-01, primary (vendor release feed), extract of the whole feed (10 entries), [Q] for quoted lines, [E] otherwise
- https://api.github.com/search/issues?q=repo:langchain-ai/langgraph+is:issue&sort=reactions&order=desc&per_page=5: read 2026-11-01, primary (tracker metadata), extract, [E]
- https://api.github.com/repos/langchain-ai/langgraph/issues/4973 and its comments?per_page=100&page=1 (page 2 empty): read 2026-11-01, primary, extract, about 35 of 85 comments verbatim, last 8 to 10 unread, [Q] for quoted lines, [E] otherwise
- docs/roadmap.md (Phases 8, 11, 41 headings and Goal lines): opened 2026-11-01, primary (this repository), [Q]
<!-- s3:end -->

<!-- s4:start -->
### Sources, part 4

All read 2026-11-01 (date read from `date -u +%F`).

- https://github.com/temporalio/temporal/releases.atom — primary, full text (whole feed, 10 entries, one-line bodies) [Q] for titles and body lines.
- https://api.github.com/repos/temporalio/temporal/issues/3006 and /comments?per_page=100&page=1 — primary, full thread (21 of 21 comments, through an extract) [Q] for the two quoted sentences, [E] otherwise.
- https://incident.io/blog — primary (vendor), extract, first 39,430 of 110,486 characters verbatim to the extractor, rest summarised; no dates on the page [E], post titles [Q].
- https://incident.io/changelog/rss — 404 [N], cited for nothing.
- https://incident.io/blog/rss.xml — 404 [N], cited for nothing.
- docs/roadmap.md, docs/research/discovery-2026-10-25.md, LICENSE — repository files opened.
<!-- s4:end -->

<!-- s5:start -->
### Sources, part 5

All read 2026-11-01 (date read from `date -u +%F`) through an extract tool. P = primary.
- https://github.com/modelcontextprotocol/modelcontextprotocol/releases.atom: P, full text [Q]
- https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization: P, full text [Q]
- https://modelcontextprotocol.io/specification/draft/basic/authorization: P, full text, compared by reading [E]
- https://raw.githubusercontent.com/open-telemetry/semantic-conventions-genai/main/docs/gen-ai/gen-ai-agent-spans.md: P, first 39,343 of 141,572 characters verbatim [Q], the rest extract [E], the last 2,229 characters not read
- https://arxiv.org/html/2610.22987: P, first 39,425 of 88,639 characters verbatim [Q]; the remainder as an extract, used for nothing
- https://export.arxiv.org/api/query (oversight query in B2): P, 14 of 20 entries verbatim, 6 extract [E]
- https://arxiv.org/html/2610.35835: P, first 39,425 of 164,162 characters (abstract, §I to §III, start of §IV) [Q/E]
- https://export.arxiv.org/api/query (reliability query in B3): P, 15 of 20 entries verbatim, 5 extract [E]
- https://arxiv.org/html/2610.35150: P, first 39,425 of about 191,861 characters verbatim (abstract, introduction, related work) [Q for the abstract]; method and results only as a summary, cited for nothing
<!-- s5:end -->

<!-- s6:start -->
### Sources, part 6

- http://data.europa.eu/eli/reg/2024/1689/oj, read 2026-11-01, primary; 307 redirect only [N].
- https://eur-lex.europa.eu/eli/reg/2024/1689/oj, read 2026-11-01, primary; HTTP 202, 0 characters, model-written fetch extract [N]. Cited for nothing.

Run log, part 6 (no '## Run log' heading or p6r marker exists in this report, so the line is kept here): opened audit export and retention, reconcile authority and Phase 35's `ExpireDue` and wrote the evidence table; read the delegation path for p6b candidates (1), (2), (4) and (5); EU AI Act text empty (HTTP 202); governance items (2) to (5), p6b candidate (3) and the p1, p2 and p5 candidates not reached; `gh pr view 203`: OPEN, label karakuri:discovery, head karakuri/discovery-2026-11-01; time 2026-11-01T15:45Z.
<!-- s6:end -->

<!-- s7:start -->
### Sources, part 7

No web page was fetched by this part.
- https://temporal.io/blog/using-coding-agents-on-a-migration-three-practices-that-mattered: newly cited in this report; not re-read; read 2026-10-25 per docs/research/discovery-2026-10-25.md, primary, [Q] there.
- Other URLs in p7a and p7b are cited from parts 1 to 6 and keep their entries and grades there.
- Local files opened 2026-11-01, primary for this repository: `internal/feature/checkpoint/service.go`, `internal/feature/reconcile/run.go`, `internal/feature/reconcile/authority.go`, `internal/auth/roles.go`, `internal/feature/delegation/issuer.go`, `domains/software/environments.go`, `internal/platform/tools/cliagent/claude.go`, `docs/research/discovery-2026-10-25.md`; grep hits in `internal/api/server.go`, `internal/auth/*.go`, `internal/core/agent/agent.go`, `docs/roadmap.md`.
- `gh pr view 187`, `gh pr view 200`, `gh pr view 201`, `gh pr view 204` (titles and bodies grepped), `gh pr list --state all --label karakuri:discovery`, run 2026-11-01.

Run log, part 7 (no '## Run log' heading or p7r marker exists; kept here as part 6 did): at start p7a, p7b, p8 and s8 were '_Pending'; wrote p7a with a (b2) reading of p5 Bets 1 and 2 in code and p7b; could not reach MCP OAuth, Bet 3, the post-run bounds check or the Claude Code write-boundary in code, nor the loop's handling of third-party text; time 2026-11-01T15:45Z (from `date -u`).
<!-- s7:end -->

<!-- s8:start -->
_Pending: part 8._
<!-- s8:end -->
