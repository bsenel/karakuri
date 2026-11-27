# Discovery 2026-11-22: weekly cycle

> **Progress note (part 1).** The 2026-11-22 pass stopped at the skeleton (commit 4588d32). This report was written from 2026-11-27 in eight parts, each by a separate session. Each part's state is the 'Part N status:' line that opens its section; a section without that line was not written. Part 8 adds the summary.

## Method

This report is written in eight parts, each writing only between its own markers. Sources are graded [Q] verbatim words returned by the fetch, [E] extract or paraphrase, never quoted, and [N] not read, cited for nothing. 'Observed:' and 'Inferred:' are kept apart. Nothing was built, run or tested. A vendor's or analyst's figure is a claim, not a measurement.

## Status

See the progress note above: each part's state is the 'Part N status:' line that opens its section.

<!-- p1:start -->
## Earlier reports and proposals; Who the users are and what they are trying to do

Part 1 status: truncated, not reached: users and jobs segments (1) to (4); no web source was read

### State of earlier reports and proposals

Observed from `gh pr list --state all --label karakuri:discovery`, the seven `docs/research/discovery-*.md` files and `docs/roadmap.md` headings on origin/main (all read 2026-11-27). New: yes (fates as of 2026-11-27).

| Report | PR, state, merged | Proposed | Fate in `docs/roadmap.md` |
|---|---|---|---|
| 2026-10-04 | #148, merged 2026-10-05 | Phase 33 (MCP After the Handshake) | Completed |
| 2026-10-08 | #158, merged 2026-10-09 | Phases 35 to 38 | all four Completed |
| 2026-10-25 | #195, merged 2026-10-25 | Phases 42, 43 | both Planned |
| 2026-11-01 | #203, merged 2026-11-01 | Phases 45, 46 | both Planned |
| 2026-11-08 | #223, merged 2026-11-08 | Phase 49 | Planned |
| 2026-11-15 | #249, merged 2026-11-15 | no phase (part 7 passed no candidate) | nothing written |
| 2026-11-22 | #285, open (this draft) | none yet | n/a |

- Against the planner's hints: every PR number, title and state matches, and no PR was closed unmerged, so no `gh pr view` was needed. Differences: #148 merged 2026-10-05 and #158 on 2026-10-09; the hint's dates are the report dates. Phase 44 came from PR #200, not a discovery report (discovery-2026-11-01.md, line 333).
- Phases declined by a human: **none declined.** Every proposed phase is a heading in `docs/roadmap.md`, and a grep for declined, rejected or withdrawn finds no proposal marked so. Never written as phases (by the reports themselves, not by a human): 2026-10-08's rank 5 (MCP inventory in the evidence record); 2026-11-08's rewind and its Phase 46/47 amendments; 2026-11-15's carried list (rewind, whole-call timeout, request-sourced authority arguments, cost reconciliation, MCP OAuth, per-run money budget).
- Inferred: of ten phases proposed, five are Completed (33, 35 to 38) and five Planned (42, 43, 45, 46, 49); every proposal since 2026-10-25 waits behind the alpha gate (Phase 47), queued rather than refused.
- Open from 2026-11-15 part 1's 'Still thin': SRE postmortem, self-hosters, enterprises, who merges gh-aw PRs, an ATRS record whose tool acts without review.

### Who the users are and what they are trying to do

Not reached this cycle. The session spent its time on the merge, the PR description and the subsection above; a `gh search issues` on github/gh-aw for segment (3) was refused by the session's permissions (2026-11-27), and no web page was fetched. Nothing below adds a user or a job.

| Segment | Added this cycle | Still thin? |
|---|---|---|
| (1) Team that ran an AI agent in an incident or on call | Nothing | Yes |
| (2) Self-hosters of an agent platform | Nothing | Yes |
| (3) Who reviews and merges gh-aw or repo-resident agent PRs | Nothing (search refused) | Yes |
| (4) Regulated organisation, agent on its own authority | Nothing | Yes |

### Still thin (part 1)

- All four segments, in the order above; the 2026-11-15 'Still thin (part 1)' list still holds the starting URLs (gh-aw closed bot PRs, LangGraph discussions, community.n8n.io, ATRS index).

### Earlier thin spots in this area

- SRE postmortem by the team that ran an agent: named in 2026-10-25, 11-01, 11-08, 11-15 and this report; still thin, open six cycles.
- Self-hosters in their own words: named 2026-11-08 and 11-15; still thin, open three cycles.
- Who merges gh-aw PRs: named 2026-11-08 and 11-15; still thin, open three cycles.
- Regulated organisation whose agent acts on its own authority: 2026-11-15 found only reviewed assistants (DWP, OPG, [E]); still thin, open two cycles.
- Enterprises (permissions and audit): named 2026-11-15; still thin, open two cycles.
- Inferred: cycle counts come from the 'Still thin' headings of each report, read by grep on 2026-11-27, not from a full reading; a thin spot named earlier under another wording may be older.

### Sources (part 1)

| Source | Read | Primary or secondary | Grade |
|---|---|---|---|
| `gh pr list --state all --label karakuri:discovery` | 2026-11-27 | Primary (repository) | [Q] |
| docs/research/discovery-2026-10-04.md to -11-15.md (headings, 'Still thin', phase lines) | 2026-11-27 | Primary (repository) | [E] |
| docs/roadmap.md phase headings on origin/main | 2026-11-27 | Primary (repository) | [Q] |
| README.md | not read | n/a | [N] |
<!-- p1:end -->

<!-- p2:start -->
## What they ask for and complain about

Part 2 status: truncated. Items 1 to 4 are written. Not reached: item 5 (second sources for the cancelled-run state and for output overload). Pages that could not be read: claude-code #21460, codex #62943, codex #57101 and the claude-code `permission` search page, each HTTP 504 twice. Part 1's status line is present (truncated).

Method: all pages were read on 2026-11-27 through a web-fetch tool, which returns a model-written report of each page. The `gh` CLI was refused in this session, so no issue was read through the API. Grading: [Q] means the tool marked the words as verbatim page text; [E] means an extract or a paraphrase; [N] means not read. On most pages the tool did not show reaction counts: where a count is missing below, it was not seen, which does not mean it is zero. Nothing was built, run or tested.

**Item 1. Re-validating authority at the moment of action, after a long human wait.** New: yes (a second-source search; the requirement itself is from 2026-11-15).
- Observed: the only source is still Temporal forum t/19610, titled "Dealing with Authorization/Execution Drift (TOCTOU) in long-running workflows: Recommended patterns?" and opened 2026-06-05 [Q] https://community.temporal.io/t/19610 (read 2026-11-27).
- Observed, a partial match from a different organisation: langgraph #8102, "RFC: Pre-execution tool call interception hooks for policy enforcement". It was opened 2026-06-16 and closed as not planned on 2026-10-16. The body asks for a policy hook that runs before execution; resume after a wait comes up only in the replies. rpelevin (2026-06-27): [Q] "resume can consume the prior decision only when the proposed envelope is still identical." armorer-labs (2026-06-29): [Q] "execution must re-check that the envelope did not mutate before the tool actually runs." The tool read the first 39,938 of 77,782 characters verbatim and summarised the rest. Several repliers promote their own products. https://github.com/langchain-ai/langgraph/issues/8102 (read 2026-11-27).
- Observed, a weaker match: openai-agents-python #5413, "Approval decision transfers across runs when call ID is reused", opened 2026-10-24, still open, last comment 2026-11-27. [Q] "a stale approval from a previous interaction can authorize a new execution." The proposed fix: [Q] "Bind approval records to the run that issued them." The issue is about an approval replayed in a later run, not about a wait inside one run. https://github.com/openai/openai-agents-python/issues/5413 (read 2026-11-27).
- Searched with no match: langgraph `approval expire` (8 results), openai-agents-python `approval stale` (10 results) and modelcontextprotocol `authorization revalidate` (3 results, none relevant).
- Inferred: there is no clean second source asking for the check after a wait. Two further organisations (LangChain, OpenAI) describe the neighbouring requirement: an approval binds to the exact action, and to the run, that it was given for. Karakuri's Phase 46 rule, under which one approval buys one action, covers the neighbour, not the wait.

**Item 2. Do subagents inherit hooks and permission rules? claude-code #21460, #18950, #27533.** New: yes.
- #21460: [N]. HTTP 504 twice; its state is unknown. https://github.com/anthropics/claude-code/issues/21460
- #18950, "Skills/subagents do not inherit user-level permissions from settings.json": open, 72 👍. Its labels include area:permissions, area:security and has repro. In 2026-01 a bot flagged #10906, #11934 and #18172 as possible duplicates; the author disputes this, and the issue was never auto-closed. Linked from the thread: #58652, #64162, #12796, #10337 and #22576. The fetched page hid 22 timeline items, so the comment total is unknown. Last comment, kreare-priv, 2026-06-18: [Q] "Nope, i'm using exactly that and is followed only by the main agent, subagents doesn't follow the auth rules in that file". The latest timeline event is a maintainer adding area:agents, area:permissions and area:skills on 2026-11-20. https://github.com/anthropics/claude-code/issues/18950 (read 2026-11-27).
- #27533, "[FEATURE] `--freeze-context` / `--context-lock` for subagent and agent team configuration inheritance": a bot closed it as not planned for inactivity on 2026-03-22 and locked it on 2026-03-30. It has 1 👍 and 4 comments, three of them from the bot. The author says it is not a duplicate of #26479, #27194 or #27038. The last comment is the lock notice: [Q] "This issue has been automatically locked since it was closed and has not had any activity for 7 days." https://github.com/anthropics/claude-code/issues/27533 (read 2026-11-27).
- Inferred: not fixed. The main thread is still open, the last user report says subagents ignore the rules, and a maintainer re-triaged it a week ago. Closing the feature request recorded inactivity, not a shipped feature.

**Item 3. New recurrences since 2026-11-15.** New: yes.
- A configured timeout is honoured end to end. claude-code #110211, created 2026-11-15, open, 2 comments: when a PostToolBatch SDK callback [Q] "exceeds its configured timeout ends the session with `error_during_execution`", even though `onFailure: continue` is set. https://github.com/anthropics/claude-code/issues/110211. claude-code #112366, created 2026-11-26, 1 comment, apparently open: on macOS, timeouts count time the machine was asleep, so [Q] "the timeout fires as soon as it wakes". That covers MCP idle, hook and Bash limits. https://github.com/anthropics/claude-code/issues/112366 (both read 2026-11-27).
- A money budget per run. No per-run thread was found. The two nearest are account-level. claude-code #112086, created 2026-11-25, labelled `invalid` by a bot: [Q] "The spend limit did not stop usage at $200." The reporter puts the charge at $222.69; that figure is a claim. https://github.com/anthropics/claude-code/issues/112086. claude-code #110655, created 2026-11-17, open: [Q] "zero notifications at any threshold (50%, 75%, 90%)". https://github.com/anthropics/claude-code/issues/110655 (both read 2026-11-27).
- Inferred: both new timeout threads come from the same organisation as each other. They add no new organisation, but they show that "configured" and "enforced" still drift apart in two more places, the hook path and the clock source. The budget threads want a cap that holds and an alert before it is reached. They are evidence for spend control in general, not for a per-run budget.

**Item 4. Tracker sweep: open issues from the last 60 days (created on or after 2026-09-28), sorted by reactions.** New: yes. Only the first page of each search was read.
- anthropics/claude-code. Source: https://github.com/anthropics/claude-code/issues?q=is%3Aissue+is%3Aopen+created%3A%3E%3D2026-09-28+sort%3Areactions-%2B1-desc. The narrower `permission` search returned 504 [N].
  - #97854, "Auto mode: server-side safety classifier intermittently returns no verdict, blocking Bash and ScheduleWakeup entirely": 39 👍, 4 👀, 31 comments. A maintainer folded #97766, #97812, #97855, #97874 and #98073 into it on 2026-11-20. [Q] "All Bash-based work (including SSH to remote hosts, git operations, running tests) is blocked". https://github.com/anthropics/claude-code/issues/97854
  - #103832, "Desktop: start_session asks 'Allow Claude to start the session?' even in Bypass mode": 22 👍, 17 comments, duplicate #104101. [Q] "every mcp__ccd_session__start_session call raises an app dialog". https://github.com/anthropics/claude-code/issues/103832
  - #102136, "Permission prompt defaults to 'Always allow' instead of 'Allow once' - dangerous UX regression": 22 👍, 14 comments. [Q] "Muscle-memory click on the big button silently saves a permanent allow rule". https://github.com/anthropics/claude-code/issues/102136
  - #98159, "claude.ai: let users set a default permission mode, incl. 'Skip all approvals'": 29 👍, 12 comments. [Q] "authorise routine tool use once and let the task finish without supervision". https://github.com/anthropics/claude-code/issues/98159
- openai/codex. Sources: https://github.com/openai/codex/issues?q=is%3Aissue+is%3Aopen+created%3A%3E%3D2026-09-28+sort%3Areactions-%2B1-desc and the same search with `approval`.
  - #51601, "Windows app 26.1002.51308: sandbox setup fails with sharing violation when validating its own active runtime": 31 👍; the comment count is from a summary, about 17. [Q] "Sandbox setup should complete while Codex's own runtime processes are running." https://github.com/openai/codex/issues/51601
  - #51634, "Windows sandbox provisioning fails with os error 32 when any runtime file is in use (0.162.0-alpha.2 regression)": 16 👍. A bot flagged 5 possible duplicates. Users ask which release carries PR #51822 [E]. https://github.com/openai/codex/issues/51634
  - #62943 and #57101 were picked but not read [N], so they are not cited.
  - The tool reported no issue in the sorted lists that was clearly about hooks, budget, durability, resume, audit or delegation.
- langchain-ai/langgraph. Sources: https://github.com/langchain-ai/langgraph/issues?q=is%3Aissue+is%3Aopen+created%3A%3E%3D2026-09-28+sort%3Areactions-%2B1-desc and the same search with `interrupt`. Reactions were not shown on any page.
  - #9706, "Strict resume accepts a bare resume on a thread that was never started or has already finished": created 2026-11-14, 3 comments. [Q] "A bare `Command(resume=value)` is still accepted when nothing is pending". https://github.com/langchain-ai/langgraph/issues/9706
  - #9491, "Time travel to an interrupt reapplies the Command(update=...) of the old branch": 4 comments, labelled bug. [Q] "`Command(update=...)` is stored as null-task pending writes". https://github.com/langchain-ai/langgraph/issues/9491
  - #9480, "pydantic RootModel state loses its type on checkpoint round trip and breaks interrupt/resume": 4 comments. [Q] "does not survive a checkpoint round trip." https://github.com/langchain-ai/langgraph/issues/9480
  - #9277, "interrupt_after='node_one' pauses on a node named 'no'": 3 comments. [Q] "`interrupt_after` should pause after the node named `node_one`." https://github.com/langchain-ai/langgraph/issues/9277
- Not swept: openai/openai-agents-python (only #5413, read for item 1), google/adk-python, the temporalio SDKs, modelcontextprotocol/modelcontextprotocol (one search for item 1 only) and BerriAI/litellm.
- Inferred: the claude-code issues with the most reactions are about approval UX, not approval semantics. Users want fewer and safer prompts: a default that does not save a permanent rule, a bypass that really bypasses, and a classifier that does not block all work when it fails. The langgraph issues are about the correctness of resume: a resume with nothing pending, a stale update reapplied, state that does not survive the checkpoint. Codex's most-reacted issues are Windows sandbox setup failures.

**Ranking (part 2).** "Orgs" counts independent organisations seen this pass or carried from 2026-11-15. Counts are as the pages showed them.

| Requirement | Independent orgs | Sources | Counts | Strength | New |
|---|---|---|---|---|---|
| Approval prompts that are safe by default and few (allow once, bypass honoured, a classifier failure does not block work) | 1 (Anthropic users); codex permission-selector issue unread | claude-code #97854, #103832, #102136, #98159 | 39/22/22/29 👍; 31/17/14/12 comments; 6 duplicates folded | high | yes |
| Subagents inherit the parent's permission rules and hooks | 1 | claude-code #18950 (+#27533 closed) | 72 👍; maintainer re-triaged 2026-11-20 | high | no (state new) |
| Resume and interrupt are correct (refuse a bare resume, no stale replay, state survives the checkpoint) | 2 (LangChain; OpenAI via #5413) | langgraph #9706, #9491, #9480, #9277; agents-python #5413 | 3 to 4 comments each; reactions not seen | medium | yes |
| A configured timeout is honoured end to end | 2 (2026-11-15) + 2 new threads at Anthropic | claude-code #110211, #112366; earlier litellm #49751 | 2 and 1 comments | medium | no (recurrence new) |
| Spending cap that holds, alerts before it is reached | 1 | claude-code #112086, #110655 | 2 comments each; one labelled invalid | low-medium | yes (account-level, not per-run) |
| Approval binds to the exact action and run; re-check authority at execution after a wait | 3 partial (Temporal user, LangChain thread, OpenAI) | t/19610; langgraph #8102; agents-python #5413 | 439 views (2026-11-15); #8102 closed not planned | low-medium | no (2nd source partial) |
| Sandbox setup that works on Windows | 1 | codex #51601, #51634 | 31 and 16 👍 | medium | yes (out of Karakuri's scope) |

**Still thin (part 2).**
- claude-code #21460 unread (504); next: the same URL, or the API through `gh` in a session that allows it.
- No clean second source for re-checking authority after a long wait; next: langgraph #8026 (ApprovalNode, referenced in #8102) and the temporalio SDK trackers.
- No per-run money budget asked for by users. The threads found are account caps; next: https://github.com/openai/codex/issues/62694 (unread).
- Reaction counts on langgraph and on the new claude-code threads were not shown; next: the issues API.
- Codex #62943 (permission selectors lost) and #57101 unread (504).
- Item 5 not reached: the cancelled-run state and output overload still lack a second-source search.

**Earlier thin spots in this area.** Cycles are counted with this one included.
- 2026-11-15, re-validate authority after a wait (one source): still thin; two partial neighbours found. Open 2 cycles.
- 2026-11-15, durable state across a cancelled run (second organisation): still thin, not searched. Open 2 cycles.
- 2026-11-15, "nobody can read all these outputs": still thin, not searched. Open 2 cycles.
- 2026-11-15, subagent inheritance #21460, #18950, #27533 (from the p3 next steps): partly filled; #18950 open and #27533 closed not planned; #21460 unread. Open 2 cycles.
- 2026-11-15, the tracker sweep: partly filled for three repositories; five repositories not swept. Open 2 cycles.
- 2026-11-15, opencode undo (#5474), crewAI #6043 comments, the two unverified figures: still thin, not attempted. Open 2 cycles.

**Sources (part 2).** All primary unless marked; all read 2026-11-27.
- https://community.temporal.io/t/19610: primary, title only [Q]
- https://github.com/langchain-ai/langgraph/issues/8102: primary, first half verbatim, rest extract [Q]/[E]
- https://github.com/openai/openai-agents-python/issues/5413: primary [Q]
- Searches: langgraph `approval expire`, openai-agents-python `approval stale`, modelcontextprotocol `authorization revalidate`: primary list pages, used only to choose issues [E]
- https://github.com/anthropics/claude-code/issues/21460: [N] (504)
- https://github.com/anthropics/claude-code/issues/18950: primary [Q]
- https://github.com/anthropics/claude-code/issues/27533: primary [Q]
- https://github.com/anthropics/claude-code/issues/110211, /112366, /112086, /110655: primary [Q]
- Search pages for timeout and budget in claude-code and codex (created on or after 2026-11-15): list pages for choosing issues [E]
- https://github.com/anthropics/claude-code/issues/97854, /103832, /102136, /98159: primary [Q]
- The claude-code `permission` search: [N] (504)
- https://github.com/openai/codex/issues/51601: primary, later comments extract only [Q]/[E]
- https://github.com/openai/codex/issues/51634: primary, comments extract only [Q]/[E]
- https://github.com/openai/codex/issues/62943, /57101: [N] (504)
- https://github.com/langchain-ai/langgraph/issues/9706, /9491, /9480, /9277: primary [Q]
- The three sorted search pages (claude-code, codex, langgraph): list pages for choosing issues [E]
<!-- p2:end -->

<!-- p3:start -->
## Competitive teardown, products 1 to 3

Part 3 status: truncated. Written: A, B and C, each from one or two release pages and one issue or forum listing. Not reached: Codex release pages 2, 4, 5 and 7 onward (so Codex May to October is still unread), the Agents SDK, LangSmith April to August, LangGraph April to June (page 8 timed out), Temporal August and September, and any full complaint thread with its reaction counts. Part 2's status line is present (truncated). Extends the p3 and p4 sections of discovery-2026-11-15 without repeating them; the -11-08, -11-01 and -10-25 sections were not re-read this pass. Every page was read through a fetch tool that returns a model-written extract, so every external line below is [E] and nothing is quoted. Nothing was built, run or tested.

### A. OpenAI Codex CLI and the OpenAI Agents SDK

New: yes, for 2026-11-05 to 11-18. Still unread: 2026-05-01 to 10-28. Releases page 3 covers 11-16 to 11-18 and page 6 covers 11-05 to 11-11, so the window's first five months lie on pages past 6, which were not reached.

- **(i) Shipped. Months read: November 2026 only (11-05 to 11-11 and 11-16 to 11-18).** [E] https://github.com/openai/codex/releases?page=6 (read 2026-11-27): 0.167.0, 2026-11-06, adds an agent command center (`codex agents --no-daemon`), inline patch review while an approval prompt is open, *managed command checkpoints that require fresh human approval*, managed network requirements under `permissions.network`, a context budget for its Guardian reviewer, and resume of chats across a change of model provider. [E] https://github.com/openai/codex/releases?page=3 (read 2026-11-27): 0.170.0, 2026-11-18, fixes permission updates and child threads that lost their sandbox restrictions, and documents sandbox permission profiles. 0.169.1 (11-17) carries chores only. Agents SDK: not read this pass.
- **(ii) What it does that Karakuri does not.** Inferred from (i): approval is asked at the level of a single shell command against an administrator-managed list, and a reviewer model (Guardian) inspects recorded commands. Karakuri's approval gate is per capability (`RequiresApprovalFor`) and per count of actions (internal/core/agent/agent.go:37-48, opened), with no second model reviewing what ran.
- **(iii) What Karakuri does that it does not.** Bounds nobody filled in permit nothing; opting out takes the explicit `UnlimitedActions = -1` (internal/core/agent/agent.go:32-35, opened). Third-party text in evidence escalates the plan whatever autonomy the agent earned (internal/core/environment/environment.go:55-59, opened). A checkpoint superseded by a newer plan can no longer be approved (internal/feature/checkpoint/service.go:193-196, opened). Inferred: the 0.170.0 fix, child threads losing sandbox restrictions, is the failure Karakuri avoids by carrying authority in the request rather than in a session (internal/feature/reconcile/authority.go:10-19, opened); this is a design claim, not a test.
- **(iv) Complaints.** [E] https://github.com/openai/codex/issues?q=is%3Aissue+sort%3Areactions-%2B1-desc (read 2026-11-27): the two issues GitHub ranks first are #11023 "Codex desktop app for Linux" (closed 2026-08-11) and #10450 "Remote Development in Codex Desktop App" (closed 2026-05-14). The listing does not show +1 counts and neither thread was opened, so no reaction count is reported [N for the threads]. Observed: both are about where the tool runs, not about agent authority.
- **(v) Inferred.** Codex moved in November toward administrator-managed approval per command and a reviewer model; the overlap with Karakuri's checkpoint layer grows. These are vendor release notes, not measurements.

### B. LangGraph and LangSmith

New: yes, for LangGraph 2026-06-30 to 07-30. Still unread: LangGraph 2026-04-08 to 06-29 (page 8 returned HTTP 504 twice) and LangSmith April to August (not reached).

- **(i) Shipped. Months read: LangGraph late June and July 2026 (06-30 to 07-30); August seen only by title.** [E] https://github.com/langchain-ai/langgraph/releases?page=6 (read 2026-11-27): langgraph 1.2.7 (06-30), 1.2.8 (07-06) and 1.2.9 (07-10) fix delta-channel snapshots and `updateState` on delta channels; 1.2.10 (07-28) adds a per-node `trace_policy` and typed v3 `stream_events`; checkpoint-sqlite and checkpoint-postgres 3.1.1 (07-30) scope namespace matching and add an opt-in `omit_expired` read; langgraph-cli 0.4.31 (07-10) adds prebuilt images for `langgraph deploy`. Observed: no release in this range is about interrupts, human approval or cancellation; the work is checkpoint correctness.
- **(ii) What it does that Karakuri does not.** Inferred from (i): per-node control of what is traced, and checkpoint rows that expire and can be skipped on read. Karakuri has no per-step trace policy that was found this pass (not searched).
- **(iii) What Karakuri does that it does not.** Karakuri's autonomy is rendered into the same bounds an operator writes, with no second gate (internal/feature/reconcile/authority.go:10-19, opened); provenance on evidence decides escalation (internal/core/environment/environment.go:55-59, opened). Inferred: nothing in the LangGraph notes read here ties escalation to the source of a payload.
- **(iv) Complaints.** [E] https://github.com/langchain-ai/langgraph/issues?q=is%3Aissue+sort%3Areactions-%2B1-desc (read 2026-11-27): the listing's first entries are #5891 (a `Send` object not msgpack-serialisable when a tool uses `BaseStore`, closed 2025-10-08, 7 on the listing badge) and #5023 (feedback on moving `config.configurable` to `context`, closed 2025-07-25, 14 on the badge). The badges may be comment counts, and neither thread was opened, so these are not reaction counts. Observed: both are from 2025, outside the window; the listing gave no in-window complaint.
- **(v) Inferred.** Mid-2026 LangGraph releases were maintenance of checkpoint serialisation, which suggests durable state is still where its users hit bugs. One page; weak.

### C. Temporal as a durable-execution engine for agents

New: yes, for April to early July 2026 and for three forum threads on LLM use. Extends 2026-11-15 p4 item 5 (which read 09-11 to 11-02).

- **(i) Shipped. Months read: April, May, June and early July 2026 (04-10 to 07-08); August not reached; September onward was read on 2026-11-15.** [E] https://github.com/temporalio/temporal/releases?page=2 (read 2026-11-27): v1.31.0 (04-29, before the window) brings Serverless Workers on AWS Lambda in pre-release, principal attribution, IAM auth for SQL, Worker Versioning GA and a Standalone Activities preview. In the window: v1.29.6.1 (05-06), v1.31.1 (06-10, a search-attribute race fix and stale gRPC connection cleanup), v1.29.7 (06-12) and v1.30.5 (06-15, security and Postgres fixes), v1.30.6 (07-08, CVE-2026-5724 and a possibly breaking change for replication with authorization). Observed: no AI-agent feature appears in the server releases read.
- **(ii) What it does that Karakuri does not.** Inferred: principal attribution on server calls, and running workers serverless. Karakuri runs as one server process (ADR 001).
- **(iii) What Karakuri does that it does not.** Temporal stores and replays whatever the workflow does; deciding whether an agent may act is left to the application. Karakuri decides it in the domain: bounds that fail toward asking (internal/core/agent/agent.go:32-35, opened), and escalation on third-party evidence (internal/core/environment/environment.go:55-59, opened).
- **(iv) Complaints.** [E] https://github.com/temporalio/temporal/issues/680 (read 2026-11-27): wait for an external workflow to complete, 77 thumbs-up, open. [E] https://github.com/temporalio/temporal/issues/2318 (read 2026-11-27): MongoDB persistence, 48 thumbs-up, open; a comment says an upstream PR was declined and a fork exists. Forum, [E] https://community.temporal.io/latest (read 2026-11-27): "A2A integration for Temporal" (t/19678, last active 2026-11-18, 4 replies, 378 views); "The Lord of the Loop" on the best abstraction for LLM-driven Temporal apps (t/18631, 2026-11-08, 6 replies, 1,251 views); "Letting Temporal own LLM retries" (t/19849, 2026-11-04, 2 replies, 77 views). The threads themselves were not opened [N].
- **(v) Inferred.** The most-viewed agent thread asks what abstraction to put on top of Temporal for an LLM loop: users bring the loop themselves. That is the layer Karakuri occupies; Temporal is a possible substrate, not a direct rival. Based on thread titles only.

### Still thin (part 3)

- Codex 2026-05-01 to 10-28: releases pages 7 onward of https://github.com/openai/codex/releases.
- Agents SDK May to September; LangSmith April to August (https://docs.langchain.com/langsmith/changelog past the first 40,000 characters).
- LangGraph 2026-04-08 to 06-29: https://github.com/langchain-ai/langgraph/releases?page=7 and 8 (8 timed out).
- Temporal August 2026; any full complaint thread with reaction counts for Codex and LangGraph; the three Temporal forum threads in full.

### Earlier thin spots in this area

- Codex 05-01 to 10-28: still thin (third cycle: 11-08, 11-15, 11-22).
- LangGraph 04-08 to 06-29: still thin; 06-30 to 07-30 now read (sixth cycle, per 11-15's "fifth-cycle").
- LangSmith April to August: still thin (third cycle).
- Codex and LangGraph complaints: still thin, only listings read (second cycle).
- Temporal complaints: partly filled, two open issues with thumbs-up counts (second cycle).

### Sources (part 3)

| URL | Read | Primary/secondary | Grade |
|---|---|---|---|
| https://github.com/openai/codex/releases?page=3 | 2026-11-27 | primary | [E] |
| https://github.com/openai/codex/releases?page=6 | 2026-11-27 | primary | [E] |
| https://github.com/openai/codex/issues?q=is%3Aissue+sort%3Areactions-%2B1-desc | 2026-11-27 | primary | [E] |
| https://github.com/langchain-ai/langgraph/releases?page=6 | 2026-11-27 | primary | [E] |
| https://github.com/langchain-ai/langgraph/releases?page=8 | 2026-11-27 | primary | [N] HTTP 504 twice |
| https://github.com/langchain-ai/langgraph/issues?q=is%3Aissue+sort%3Areactions-%2B1-desc | 2026-11-27 | primary | [E] |
| https://github.com/temporalio/temporal/releases?page=2 | 2026-11-27 | primary | [E] |
| https://github.com/temporalio/temporal/issues/680 | 2026-11-27 | primary | [E] |
| https://github.com/temporalio/temporal/issues/2318 | 2026-11-27 | primary | [E] |
| https://community.temporal.io/latest | 2026-11-27 | primary | [E] |
| https://community.temporal.io/search?q=agent%20order%3Alatest | 2026-11-27 | primary | [N] empty placeholder |
<!-- p3:end -->

<!-- p4:start -->
## Competitive teardown, products 4 onward, and the capability matrix

Part 4 status: truncated. Written: D (Claude Code), E (incident.io), F (Arize Phoenix). Not reached: Claude Code month-by-month (the changelog carries no dates, so no version could be placed in a month), any incident.io entry with a date, and any full complaint thread for D, E or F. Part 3's status line is present (truncated). Every page was read through a fetch tool that returns a model-written extract, so every external line below is [E] and nothing is quoted. Nothing was built, run or tested. The capability matrix is part 7's.

Not torn down this cycle: Google ADK, Microsoft Agent Framework, GitHub Agentic Workflows (github/gh-aw).

### D. Claude Code and the Claude Agent SDK

New: yes, for versions 2.1.331 and 2.1.332. Still unread: every earlier version in the window; the window could not be mapped to versions.

- **(i) Shipped. Months read: none could be dated.** [E] https://raw.githubusercontent.com/anthropics/claude-code/main/CHANGELOG.md (read 2026-11-27): the file has no release dates; only the top ~40,000 of ~1.42M characters were read (2.1.332 and most of 2.1.331). Observed in those two versions: `/permissions` refuses a rule that would be skipped at next start and says why; a `PreSkillShellCommand` hook event can refuse a skill's inline shell commands at load; a `PermissionDenied` hook's `retry: true` changes meaning; fixes to rewind (a rewind deleting files after a failed restore; a freeze opening a rewind point); fixes to background agents and background commands in `claude -p` and the SDK; "routine" renamed to "cloud scheduled task" in `/schedule` and `/loop`; `gen_ai.*` usage attributes on `claude_code.llm_request` trace spans. One entry mentions a change "on or after November 30, 2026", which only suggests these versions are from late November. The release notes were asked for month by month; they could not be given.
- **(ii) What it does that Karakuri does not.** Matrix cells, from (i): permission rules evaluated per tool call with hook events that can refuse before a command runs (including at skill load); file-level rewind of a session's edits; recurring prompts in a local `/loop` and in cloud scheduled tasks; OTel `gen_ai.*` attributes on LLM spans. Karakuri's gate is per capability and per action count (internal/core/agent/agent.go:37-48, opened), with no per-shell-command hook and no file rewind (not searched beyond this file).
- **(iii) What Karakuri does that it does not.** Matrix cells: bounds left empty permit nothing, and opting out needs `UnlimitedActions = -1` (internal/core/agent/agent.go:32-35, opened); per-parameter value limits on a planned action that can refuse rather than escalate (internal/core/agent/agent.go:46-54, opened); third-party text in evidence escalates the plan (internal/core/environment/environment.go:55-59, opened); an audit export with obligations, retention and oversight records (internal/feature/audit/export.go:34-98, opened); a standing objective owned by a reconciler rather than a recurring prompt (AGENTS.md rule 8). Inferred: Claude Code's `/loop` re-runs a prompt; nothing read here says it keeps a declared objective or its authority across runs.
- **(iv) Complaints.** Not read this pass [N]; the 2026-11-15 cycle read them.
- **(v) Inferred.** Both late-November versions harden the per-call permission layer (rules that would be silently skipped, hooks at skill load). Claude Code is converging on per-command policy, as Codex did (part 3 A). One version pair; weak.
<!-- p4:D:end -->

### E. incident.io (AI SRE)

Why this one: Rootly and PagerDuty were read on 2026-11-15 (p4a), so incident.io is the one of the four not yet torn down, and it publishes a changelog page. New: yes.

- **(i) Shipped. Months read: none could be dated.** [E] https://incident.io/changelog (read 2026-11-27): the newest entries show titles without dates, and only ~40,000 of ~67,000 characters were read. Undated AI entries near the top (page order suggests newest first; not confirmed): "Plug your skills and tools into the agent" (says a change can be approved before it runs), "Plug your internal AI stack into Nexus", "The incident.io plugin: our skills, in your agent", "Investigations are 2x faster" (a vendor claim, not a measurement), "Investigations now available, powered by Nexus" (posts a root-cause hypothesis with evidence), "Access the agent from everywhere", "Introducing incident.io MCP". Observed: none of these can be placed inside 2026-05-27 to 11-27.
- **(ii) What it does that Karakuri does not.** Matrix cells, from (i): an investigation agent that posts a root-cause hypothesis with its evidence into the incident; customer skills and tools plugged into the vendor agent with approval before a change runs; an MCP server and a plugin that put the vendor's skills into the customer's own agent. Karakuri has no incident-domain agent or paging integration (no such package under internal/feature, listed this pass).
- **(iii) What Karakuri does that it does not.** Matrix cells: escalation decided by the provenance of evidence, so a third-party payload (an alert body or chat message) forces a human (internal/core/environment/environment.go:55-59, opened); self-hosted, with observability exporters for OTLP, Prometheus, Loki, Datadog and others (internal/platform/observability/, listed); authority written into the request with no second gate (internal/core/agent/agent.go:37-54, opened). Inferred: nothing read says incident.io's approval depends on where the evidence came from.
- **(iv) Complaints.** Not read [N]; no thread opened.
- **(v) Inferred.** incident.io is moving both ways: its agent takes the customer's tools, and its skills go into the customer's agent over MCP. Approval-before-change is now a selling line in SRE tools too. Titles only; weak.

### F. Arize Phoenix (agent observability and evaluation)

Why this one: Langfuse was read on 2026-11-15; Phoenix publishes dated release notes. New: yes.

- **(i) Shipped. Months read: May (05-27 only), June, July, August, September, October and November to 11-23.** [E] https://arize.com/docs/phoenix/release-notes (read 2026-11-27); the page's own summary named 05-13 as the latest entry, which contradicts the dated entries read, so it was not used. June: PXI agent in beta (06-02), evals as pytest and Vitest tests (06-26), trace-level annotations (06-24). July: OAuth2 authorization server and remote MCP in beta (07-17), user-friction evaluator (07-22), toxicity evaluator (07-28). August: hallucination evaluator (08-04), persistent agent sessions in 20.0.0 (08-11), OAuth2 workload identity (08-12), read-only analytics SQL over MCP (08-17). September: PII-detection evaluator and prompt versions over REST (09-01), MCP plugins for coding agents (09-14), trace-tree cost metrics (09-29). October: DECISION spans and trace-tree evals (10-01), Docker sandboxes for code evaluators (10-13), Claude Managed Agents tracing (10-16), decision-model evaluators (10-27). November: authentication on by default, a breaking change, and DECISION and AGENT annotator kinds (11-02, 21.0.0); prompt version timeline (11-18); decision span costs and removal of `cumulative_*` filters, breaking (11-23, 22.0.x). Observed: no in-window entry mentions guardrails or blocking an action.
- **(ii) What it does that Karakuri does not.** Matrix cells: a library of LLM-judged evaluators (hallucination, toxicity, PII, retrieval relevance, user friction); evals runnable as unit tests in CI; human annotation of spans and traces with annotator kinds; DECISION spans that label an agent's choice points; per-span and per-trace cost; tracing for third-party agent frameworks. Karakuri has an eval service with a judge and calibration against golden cases (internal/feature/eval/eval.go:60-92, opened), but no evaluator library or annotation UI was found (not searched further).
- **(iii) What Karakuri does that it does not.** Matrix cells: Phoenix records and scores decisions after the fact; Karakuri gates them before they run, with bounds that fail toward asking (internal/core/agent/agent.go:32-35, opened) and value limits that can refuse (internal/core/agent/agent.go:46-54, opened); escalation on third-party evidence (internal/core/environment/environment.go:55-59, opened); an audit export with retention and oversight records (internal/feature/audit/export.go:34-98, opened). Inferred: Phoenix's DECISION spans are where Karakuri's decision records could be exported to, via the OTLP exporter (internal/platform/observability/otlp.go, listed, not opened).
- **(iv) Complaints.** Not read [N]; no thread opened.
- **(v) Inferred.** Phoenix added "decision" as a first-class span kind in October and November; observability vendors are naming the same object Karakuri gates. Shipping auth-on-by-default as breaking in November suggests self-hosted instances ran open. From release notes; vendor text.

### Still thin (part 4)

- Claude Code by month: the changelog has no dates; GitHub release tags (https://github.com/anthropics/claude-code/releases) were not read, and versions before 2.1.331 were not reached.
- incident.io: no dated entry; the rest of https://incident.io/changelog past 40,000 characters.
- Complaint threads for D, E and F: none opened.
- Google ADK, Microsoft Agent Framework, gh-aw: not torn down this cycle.

### Earlier thin spots in this area

- Claude Code changelog over a six-month window: still thin; dates not on the page (first cycle this is recorded; complaints were read 11-15).
- Changelogs under six months (11-08 "Still thin"): partly filled for Phoenix, which now has six months read; still thin for Claude Code and incident.io (at least fourth cycle: 10-08, 10-25, 11-15, 11-22).
- SRE agent in an incident: still thin; incident.io's changelog shows the vendor's view, not a responder's (seventh cycle per 11-15's "sixth cycle").
- Langfuse releases 04-25 to 10-15 (11-15 p4a): not revisited; Phoenix was read instead (second cycle).

### Sources (part 4)

| URL | Read | Primary/secondary | Grade |
|---|---|---|---|
| https://raw.githubusercontent.com/anthropics/claude-code/main/CHANGELOG.md | 2026-11-27 | primary | [E] top ~40,000 characters only |
| https://incident.io/changelog | 2026-11-27 | primary | [E] ~40,000 of ~67,000 characters; undated |
| https://arize.com/docs/phoenix/release-notes | 2026-11-27 | primary | [E] |
| https://github.com/anthropics/claude-code/releases | not read | primary | [N] |
<!-- p4:end -->

<!-- p5:start -->
## Technical frontier

not started
<!-- p5:end -->

<!-- p6:start -->
## Governance and buying criteria; Feasibility

not started
<!-- p6:end -->

<!-- p7:start -->
## Fit and bets; ranking; What changed since the last report

not started
<!-- p7:end -->

<!-- p8:start -->
## Proposed phases; Sources

not started
<!-- p8:end -->
