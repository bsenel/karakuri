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

Part 5 status: truncated. Written: A1 to A5 (MCP, A2A, OTel GenAI, the five IETF agent-authorisation drafts read past the title, and CIMD `-03`), B (two papers of the four allowed), C (Anthropic, OpenAI, Google), the bets, the thin spots and the sources. Not reached: papers 3 and 4, the OTel span-status pages, and Google ADK. Part 4's status line is present (truncated). Nothing was built, run or tested.

Extends the p5 'Technical frontier' sections of discovery-2026-11-15, -11-08 and -11-01 without repeating them. The roadmap on origin/main has MCP 2026-07-28 built (Phase 33) and Phase 42 planned; no later revision is planned. Every page came through a fetch tool that returns a model-written extract, so every external line is [E] unless marked [Q]. Date read for every source: 2026-11-27.

### A. Protocols and standards

**A1. MCP: still no revision after 2026-07-28; the draft changelog is unchanged since 2026-11-15.** Observed [E] (https://modelcontextprotocol.io/specification/draft/changelog): the page holds only unreleased changes and two entries: the Client ID Metadata Document reference moves from `-00` to `-03`, which implementations that support it MUST follow, and the optional `_meta` field that 2026-07-28 dropped comes back on `CreateMessageRequestParams`, the elicitation params and result, and `ListRootsResult`. Maturity: spec draft. New: no, because 2026-11-15 A1 recorded both entries. Inferred: Phase 33's client needs nothing new. If Phase 42 reads `_meta` on sampling or elicitation, it should accept the restored field.

**A2. A2A: v1.0.1 is still the latest.** Observed [E] (https://github.com/a2aproject/A2A/releases): the latest release is v1.0.1 (changelog dated 2026-05-26), with three spec fixes: `application/a2a+json` preferred, transcoding error changes, and TaskStatus values. Maturity: 1.0 spec. New: no, because there has been no release in six months. Inferred: A2A is stable enough to target, but no implementer page has been read in two cycles, so how widely it is adopted is still unknown.

**A3. OpenTelemetry GenAI conventions: first standalone release.** Observed [E] (https://github.com/open-telemetry/semantic-conventions-genai/releases): v0.1.0, released 21 November 2026, is the repository's first release and depends on semantic-conventions v1.45.0. It renames the registry span type `gen_ai.execute_tool.internal` to `gen_ai.execute_tool`. The releases page does not say whether `invoke_agent` and `execute_tool` are development or stable. Maturity: a 0.x release, which is pre-stable by its version number (Inferred). New: yes. What it would let Karakuri do: Phase 29's agent and tool spans can pin to a versioned GenAI convention instead of tracking a moving doc page, so a dashboard built on Karakuri's spans does not break silently. Inferred: Karakuri should name the convention version it emits, `v0.1.0`, in its span attributes or its docs.

**A4. IETF agent-authorisation drafts, now read past the title** (this fills 2026-11-15's 'IETF drafts read by title only'). All are individual drafts, not adopted by a working group. Each datatracker page served about 40,000 characters as text and the rest only as a model summary. Where a line rests on that summary, it says so.
- `draft-niyikiza-oauth-attenuating-agent-tokens-02` (2026-10-26/27). Abstract read [E]: a signed credential format for task-scoped delegation in agent systems. Core read [E]: a holder derives a narrower child token offline. The child is signed by the key its parent names in `cnf.jwk` and carries a hash of its parent, so the chain verifies against the root without contacting the issuer. Section 4 (invariants I1 to I6) came only as a summary. New: yes (the body). What it would let Karakuri do: a delegated coding agent could carry a token derived from the run's own and strictly narrower, so `AuthorityBounds.CanDelegate` becomes something a resource server verifies, not something Karakuri asserts. Nobody ships this.
- `draft-araut-oauth-transaction-tokens-for-agents-03` (2026-10-31; replaces `draft-oauth-transaction-tokens-for-agents`). Abstract read [E]: an `agentic_ctx` JWT claim that carries chain-level metadata. Core read [E] (3.3.1): at each agent hop the Transaction Token Service issues a replacement token. `sub`, `txn`, `aud` and `act` stay fixed, `proximate_agent.id` becomes the new agent, `hop_count` goes up by one, and `min_assurance_level` is the minimum along the chain. Section 4.2 (monotonic attenuation) was not read. New: yes. Inferred: `txn` plus `hop_count` is the wire form of Karakuri's run id and delegation depth, and would let a downstream service see that a call came three hops after a human.
- `draft-chen-oauth-agent-revocation-01` (2026-10-30). Abstract read [E]: OAuth revocation lacks batch revocation, cascade and agent-level context. Core read [E]: a `POST /agent/revoke` endpoint takes `agent_id`, `reason`, `cascade_depth` (-1 for unlimited, 0 for this agent only) and `context`, plus optional `revoke_all_tokens` and `revoke_for_duration`. It answers with per-agent status and an `audit_reference`. New: yes. Inferred: this is the shape of a standing objective's kill switch. Revoking an objective should cascade to every delegated agent's credentials, and Karakuri today has no notion of revoking a credential it handed to a delegate (not checked in code this pass).
- `draft-ni-wimse-ai-agent-identity-04` (2026-10-27). Abstract read [E]: cryptographically binding an agent identity to an accountable user or organisation. Core read [E] (Section 4): a dual-identity credential binds agent and owner keys under three issuance models: agent-mediated (the owner countersigns), owner-mediated (the owner is a gateway) and server-mediated (the server challenges the owner). The server verifies both signatures. Security Considerations is still a TODO. New: yes. Maturity: early.
- `draft-chen-oauth-agent-authz-use-cases-05` (2026-10-26, Informational). Abstract read [E]: a systematic analysis of agent use cases. Section 3 read [E]: three gaps, which are the authorisation context, the delegation chain (a resource server cannot verify A to B to C back to the user) and mass revocation. Sections 4 and 5 (14 use cases; RAR, DPoP, CIMD, Transaction Tokens) came only as a summary. New: yes (the body).
- Inferred across A4: three drafts in six days (26 to 31 October) attack the same delegation-chain gap from three sides: attenuation, chain context and revocation. No draft is adopted and none has a second implementer that this pass found. Karakuri should not implement any of them now, but its own delegation record (who delegated to whom, with which bounds) should carry the same fields (chain id, hop count, parent, narrowed scope) so that it can be put on the wire later.

**A5. OAuth Client ID Metadata Document, `draft-ietf-oauth-client-id-metadata-document-03`** (2026-11-02, OAuth working-group document, Standards Track). This fills 2026-11-15's 'is CIMD used'. Sections 3 to 6 read [E]: the `client_id` is an https URL with a non-root path, and the authorisation server fetches the JSON document at that URL. The document's `client_id` must equal the URL by simple string comparison. The fetch must return 200 and must not follow redirects. The document may carry public keys but never a `client_secret`. Caching follows HTTP cache headers, and errors are not cached. Maturity: a working-group draft that the MCP draft spec requires at `-03` for implementations that support CIMD (A1). New: yes (the `-03` body). What it would let Karakuri do: an MCP client with no pre-registration at each server, so a twin can bind a new authenticated MCP server without an operator registering a client first. Not checked in code: whether Karakuri's MCP client (Phase 28/33) does OAuth at all. Inferred: if Phase 42 or a later phase adds MCP auth, CIMD `-03` is the registration path to build, and Karakuri would have to host its metadata document at a stable https URL. That is a deployment requirement for self-hosted installs.

### B. Research (two papers)

**B1. Confidence scores for gated automation** (arXiv 2611.33394v1, 24 Nov 2026, https://arxiv.org/html/2611.33394). Read: Abstract, Introduction, Related work and Results. Methods and Discussion came only as a summary. Maturity: preprint. New: yes.
- Observed, method [E]: a linear head trained on the final hidden state of a frozen gemma-2-2b-it predicts whether an answer is correct. Answers above a threshold are acted on automatically and the rest go to human review. It is compared with token log-probability, self-scoring and LLM-as-judge.
- Observed, result [E]: at a 95% precision target the head automates about 62% of the workload, against at most about 23% for the other methods (SQuAD, TruthfulQA, in domain). Out of domain, from the summary only, it is about 21% against under 1%.
- Inferred for Karakuri: earned autonomy needs a calibrated "act or ask" score, and self-scoring and LLM-as-judge calibrate poorly. The result needs hidden states, so it does not carry over to hosted APIs. What carries over is the method: set a precision target, measure coverage at that target per action kind from checkpoint outcomes, and widen bounds only where coverage holds out of domain.

**B2. MINE: indirect prompt injection defence by relevance selection** (arXiv 2611.34188v1, 25 Nov 2026, https://arxiv.org/html/2611.34188). Read: Abstract, Introduction, Related Work, Section 3 (method), Section 4 and the tables in 5.1 and 5.2. Sections 5.3 to 7 came only as a summary. Maturity: preprint. New: yes.
- Observed, method [E]: retrieved context is split into spans. A training-free optimiser selects the spans that maximise mutual information with the user query, with a penalty on embedding variance, and only those reach the model.
- Observed, result [E]: on Inj-SQuAD with Llama-3.1-70B, 91.93% accuracy and an attack success rate of 0.55%.
- Inferred for Karakuri: it complements CapLite (2026-11-15 B1). MINE filters what `TrustThirdParty` text reaches the model, and CapLite constrains what the action may carry. It is weaker as a guarantee, because it is statistical and evaluated on QA, not on agents that act.

### C. Model and provider capabilities, 2026-09-27 to 2026-11-27

Entries that 2026-11-15 C1 already recorded (self-hosted Managed Agents, Fable 5.2, message threads) are not repeated.
- **C1. Anthropic: inference hooks GA, and dynamic workflows** (https://platform.claude.com/docs/en/release-notes/api) [E]. On 2026-10-20, inference hooks, an allow or deny verdict before inference, came out of beta for Claude Enterprise, and Managed Agents `web_fetch` now follows the organisation's domain restrictions. On 2026-10-09, Managed Agents gained dynamic workflows (beta), in which the server runs many agents in phases. Maturity: shipped by one vendor. New: yes. Inferred: an organisation-level gate now sits below Karakuri. A run's action can be denied by the customer's hook, so Karakuri must surface a provider-side deny as its own outcome, not as a model error.
- **C2. OpenAI: model-run approval, scheduled automations, and background windows** (https://developers.openai.com/api/docs/changelog, redirected from platform.openai.com/docs/changelog) [E]. On 2026-10-30, the Agents API gained automatic review, in which a model approves or denies tool and permission requests. The same day it gained tool-list updates within a session and background compaction by default. On 2026-10-23, scheduled one-time or recurring tasks in sessions. On 2026-11-17, two- and eight-hour background completion windows at lower token rates (30% lower for two hours). Maturity: shipped by one vendor (Agents API in public beta since 2026-09-10). New: yes. Inferred: "a model approves tool calls" and "recurring tasks" are the vendor's versions of Karakuri's checkpoints and standing objectives. What Karakuri can still do that this does not: earned autonomy, meaning approval policy widened from recorded checkpoint outcomes (B1), and provider independence. The slow-window pricing suits non-urgent reconcile passes.
- **C3. Google: tool retrieval on demand, and Computer Use GA** (https://aistudio.google.com/docs/changelog, redirected from ai.google.dev/gemini-api/docs/changelog) [E]. On 2026-11-19, tool retrieval (preview) loads function schemas on demand, and remote MCP servers support `defer_loading`. On 2026-10-30, the Computer Use tool became GA. On 2026-11-02, `gemini-2.5-pro` requests route automatically to `gemini-3.8-flash` from 2026-12-01, billed at that model's rate. Maturity: shipped by one vendor. New: yes. Inferred: silent model rerouting breaks any evaluation baseline (Phase 30) pinned to a model id. Karakuri should record the model that answered, not the model it asked for. Deferred tool loading is now offered by two vendors.
- Inferred across C: in two months all three vendors shipped a managed agent runtime with approvals (Anthropic `auto` policy, 2026-09-10; OpenAI automatic review). Approval gates on their own no longer set Karakuri apart. What still does: approvals that earn wider bounds over time, provenance on every observation, and objectives that keep running across providers.

### Candidate frontier bets (part 5)

- **Record the model that answered.** Primary source: the Gemini changelog, 2026-11-02 (C3). Proved wrong if no provider reroutes a pinned model id within the next two cycles.

- **A delegation record shaped like a transaction token.** Primary sources: `draft-araut-oauth-transaction-tokens-for-agents-03` and `draft-niyikiza-oauth-attenuating-agent-tokens-02` (A4). The bet: every delegation Karakuri makes records the chain id, hop count, parent and narrowed bounds, so it can later be issued as a token a resource server verifies. Proved wrong if both drafts expire without working-group adoption by the next IETF meeting, or if the OAuth WG adopts a shape with different fields.
- **Revoking an objective cascades to its delegates.** Primary source: `draft-chen-oauth-agent-revocation-01` (A4). The bet: stopping a standing objective revokes every credential handed to its delegated agents, with an audit reference. Proved wrong if a code trace shows that delegated agents hold no credential that outlives the run.
- **CIMD as the MCP client registration path.** Primary sources: `draft-ietf-oauth-client-id-metadata-document-03` (A5) and the MCP draft changelog (A1). Proved wrong if the next MCP revision drops CIMD or keeps dynamic client registration as the recommended path.
- **Pin the OTel GenAI version.** Primary source: semantic-conventions-genai v0.1.0 (A3). Proved wrong if `invoke_agent` and `execute_tool` are renamed again before a 1.0 release, which would make pinning 0.x churn.

### Still thin (part 5)

- (B) Two papers, not four, and neither is on agents that act. A pass^k (repeated-run reliability) paper has still not been found.
- (C) Google ADK release notes are still unread, now three cycles. The C entries came from changelog lines only; no linked doc page was followed.
- (A) The OTel spec pages for the status (development or stable) of `invoke_agent` and `execute_tool` were not read. A4's later sections came only as a summary. There is still no A2A implementer page and no MCP SEP list.
- Not checked in code: whether the MCP client does OAuth at all, and whether delegated agents hold credentials that outlive a run.

### Earlier thin spots in this area

- IETF drafts read by title only (open since 2026-11-15, 1 cycle): **filled** for the abstracts and core sections; the later sections are still thin.
- Whether Karakuri's MCP client uses CIMD (open 1 cycle): **half filled**. The draft has been read; the code has not been checked.
- OTel span names at the new repository (open 1 cycle): **partly filled**. The release was read; the span status was not.
- MCP SEP list and ext-auth (open 1 cycle): still thin, now 2 cycles.
- A2A implementer page (open 2 cycles): still thin, now 3 cycles.
- OpenAI and Google AI Studio changelogs (open 1 cycle): **filled** (C2, C3). Google ADK release notes: still thin, now 3 cycles.
- pass^k reliability paper (open 1 cycle): still thin, now 2 cycles.

### Sources (part 5)

| URL | Date read | Primary or secondary | Grade |
|-----|-----------|----------------------|-------|
| https://modelcontextprotocol.io/specification/draft/changelog | 2026-11-27 | primary | [E] |
| https://github.com/a2aproject/A2A/releases | 2026-11-27 | primary | [E] |
| https://github.com/open-telemetry/semantic-conventions-genai/releases | 2026-11-27 | primary | [E] |
| https://datatracker.ietf.org/doc/draft-niyikiza-oauth-attenuating-agent-tokens/ | 2026-11-27 | primary | [E] first ~40k of 188k characters |
| https://datatracker.ietf.org/doc/draft-araut-oauth-transaction-tokens-for-agents/ | 2026-11-27 | primary | [E] first ~40k of 101k characters |
| https://datatracker.ietf.org/doc/draft-chen-oauth-agent-revocation/ | 2026-11-27 | primary | [E] |
| https://datatracker.ietf.org/doc/draft-ni-wimse-ai-agent-identity/ | 2026-11-27 | primary | [E] |
| https://datatracker.ietf.org/doc/draft-chen-oauth-agent-authz-use-cases/ | 2026-11-27 | primary | [E] first ~40k of 134k characters |
| https://datatracker.ietf.org/doc/draft-ietf-oauth-client-id-metadata-document/ | 2026-11-27 | primary | [E] first ~40k of 77k characters |
| https://arxiv.org/list/cs.AI/recent | 2026-11-27 | secondary (listing) | [E] |
| https://arxiv.org/html/2611.33394 | 2026-11-27 | primary | [E] |
| https://arxiv.org/html/2611.34188 | 2026-11-27 | primary | [E] |
| https://platform.claude.com/docs/en/release-notes/api | 2026-11-27 | primary | [E] |
| https://developers.openai.com/api/docs/changelog | 2026-11-27 | primary | [E] |
| https://aistudio.google.com/docs/changelog | 2026-11-27 | primary | [E] |
<!-- p5:end -->

<!-- p6:start -->
## Governance and buying criteria; Feasibility

Part 6 status: truncated. Written: (A) the AI Act amendment status from Commission pages, and what Karakuri can evidence; (B) seven feasibility rows. Article 26(6) was read; Article 113 was not (the EUR-Lex page was cut off at Article 112). Not reached: US state AI laws, NIST guidance on AI agents, UK AI procurement guidance and a published security questionnaire (no source read for any of them). Part 5's status line is present (truncated). Every web page was read through a fetch tool that returns a model-written extract, so every external line is [E] and nothing is quoted. Nothing was built, run or tested.

### A. Governance and buying criteria

**A1. EU AI Act, Article 113 and Article 26(6).** Observed: https://eur-lex.europa.eu/legal-content/EN/TXT/HTML/?uri=OJ:L_202401689 returned the text this time (about 525k characters), but the fetch stopped at Article 112(4)(c), so Article 113 was not reached [N]; the ELI fallback was not tried. Article 26(6) was read: deployers keep the logs the high-risk system generates automatically, to the extent they control them, [Q] "for a period appropriate to the intended purpose of the high-risk AI system, of at least six months", unless Union or national law (data protection in particular) provides otherwise [E]. A second subparagraph lets financial institutions meet this through their financial-services documentation [E]. The [Q] phrase was relayed by a fetch sub-agent that reported it as verbatim.

**A2. The Digital Omnibus on AI.** Observed [E]: the Commission proposal is COM(2025) 836 final, procedure 2025/0359(COD), dated 2025-11-19, amending Regulations (EU) 2024/1689 and (EU) 2018/1139 (https://eur-lex.europa.eu/legal-content/EN/TXT/?uri=CELEX:52025PC0836; https://digital-strategy.ec.europa.eu/en/library/digital-omnibus-ai-regulation-proposal). As proposed, the high-risk rules in Chapter III Sections 1 to 3 would apply after a Commission decision that support measures are available, with Annex III six months and Annex I twelve months after it, and fallback dates of 2027-12-02 (Annex III) and 2028-08-02 (Annex I). Only the start of the proposal text was read; the rest came as a summary. Observed [E]: a Commission news item, "AI Omnibus enters into force", gives 2026-07-27 as the entry into force, says Annex III high-risk rules apply from 2027-12-02 and Annex I from 2028-08-02, and links the text as OJ L_202601744 (https://digital-strategy.ec.europa.eu/en/news/ai-omnibus-enters-force, page updated 2026-07-31). Not read: the OJ text itself, so the final wording of Article 113 and whether the decision trigger survived are unverified. The Council press page returned HTTP 403 [N]; no Parliament or Council dates were read. Inferred: the high-risk dates have moved from 2026-08-02 to 2027-12-02 (Annex III) and 2028-08-02 (Annex I), on one Commission page. Nothing read says Article 26(6) was amended.

**A3. US state laws, NIST, UK procurement, security questionnaire.** Not reached (time) [N].

**A4. What Karakuri can already evidence (code opened unless marked).**
- Authority bounds: `internal/core/agent/agent.go:37-55` (`MaxAutonomousActions`, `RequiresApprovalFor`, `CanDelegate`, `ConfidenceThreshold`, `ActionLimits`, `RefuseOutsideLimits`); the zero value permits nothing (`agent.go:32-35`). Applied in `internal/feature/loop/decide.go:135`.
- The audit row records what produced the decision: `internal/platform/storage/adapter.go:124-132` (`Provider`, `Model`, `TemplateID`, `AutonomyRung`, Phase 31), with `Approver` and `BoundsViolation` at `adapter.go:122-123`.
- Checkpoints: a resolved or superseded checkpoint refuses a second decision, `internal/feature/checkpoint/service.go:185-197`; `Supersede` at `service.go:250-262`.
- Evidence export and audit API, located by grep and not opened this pass: `internal/feature/audit/export.go:177` (`Export` over a window), `:254` (checkpoints), `:356` (unmet obligations); `internal/api/handler/audit.go:52` (`ExportWindow`); retention at `internal/app/bootstrap.go:458`.
- Inferred: Karakuri can show an audit log, an export and a retention setting. Whether the default retention meets the six-month minimum in Article 26(6) (A1) was not checked.

### B. Feasibility

Read from origin/main: the phase list (`git show origin/main:docs/roadmap.md | grep -n '^## Phase'`), Phases 12 to 49. README.md was not read (time).

Candidates raised in parts 1 to 5 and still open from 2026-11-15: (1) re-validate authority at the moment of action; (2) refuse an answer to a superseded checkpoint; (3) per-run money budget; (4) timeout honoured end to end; (5) rewind; (6) record the model that answered; (7) subagents inherit the parent's permissions; (8) safe-by-default approval prompts; (9) delegation record shaped like a transaction token; (10) revocation cascades to delegates; (11) CIMD for MCP client registration; (12) pin the OTel GenAI version; (13) Windows sandbox (out of scope).

| # | Candidate | Exists (path:line) | Missing | First slice, files | Not verified |
|---|---|---|---|---|---|
| F1 | Re-validate authority at the moment of action (p2 item 1) | A restored loop whose checkpoint has been approved acts on `approved.observed, approved.plan`, the world state from checkpoint time, without observing again: `internal/feature/loop/runner.go:240-251`; `approvedPlan` at `service.go:200-215`. Phase 46 (planned) makes an approval after third-party text buy one action. | No re-check of bounds or world state between approval and action on this path. | Before acting on `takeApproved()`, observe again and re-run `DecideActions` against the current bounds; refuse with an audit row if the verdict narrows. Files: `internal/feature/loop/runner.go`, `decide.go`, tests. | The in-process (no restart) approve path was not read; it may already re-check. |
| F2 | Refuse an answer to a superseded checkpoint (p3 (v)) | **Already built**: `internal/feature/checkpoint/service.go:195-197` (Phase 40 step 5); resolved ones at `:190-192`. | Nothing for the stated case. | none | Whether the HTTP handler maps the error to a 409 was not read. |
| F3 | Per-run money budget (p2, 2026-11-15) | **Already built for objective runs**: `budgetedAgent` with per-run `spent`, `internal/feature/loop/budget.go:27-55`, `Budget.PerReconcile` (Phase 23); `RunBudgetSpent` outcome at `internal/core/loop/state.go:70`. | An ad-hoc run with no twin is not metered (`budget.go:65-69`). No warning before the cap was found. | A warning at a share of the ceiling in `budget.go`, plus a digest line. | The 2026-11-15 p4b 'no' for a per-run money budget looks wrong for objective runs. Whether the API exposes a ceiling per one-shot run was not checked. |
| F4 | Timeout honoured end to end (p2) | Delegation timeout in config: `config/default.yaml:402`, `:421`. No timeout found in `internal/platform/llm/*.go` (grep; only a comment at `claude.go:140`). | A whole-call deadline covering retries on the provider path. | Read the provider retry code first; then one deadline per model call in `internal/platform/llm`. | The retry code was not opened; a grep miss is not proof of absence. |
| F5 | Rewind | Worktree manager methods `internal/platform/git/gogitwt.go:56` (Create), `:157` (Remove), `:184` (Prune), listed only. | No method that reverts a delivered change appears in that list. | not assessed further | The Phase 44 salvage branch and the Phase 37 'stopped, not undone' text were not read. |
| F6 | Record the model that answered (p5) | **Already built** in the audit row: `internal/platform/storage/adapter.go:124-131` (Phase 31). | Unknown whether `Model` is the requested id or the id the provider returned. | If it is the requested one, store the response's model id in the LLM adapter (`internal/platform/llm/*`). | Which id is written was not traced. |
| F7 | Subagents inherit the parent's permissions (p2 item 2) | `CanDelegate` at `agent.go:43`; Phase 49 (planned), 'A Bound That Says No Delegation Is Kept'. | **Already a planned phase.** | Phase 49 | Phase 49's steps were not read. |

Not assessed (time): (8) safe-by-default approval prompts, (9) transaction-token delegation record, (10) revocation cascade, (11) CIMD, (12) OTel GenAI pin. (13) is out of scope.

### Still thin (part 6)

- The Article 113 text, the Omnibus OJ text (L_202601744), and the Council and Parliament dates [N].
- US state laws, NIST agent guidance, UK procurement guidance and a security questionnaire: none read.
- F1's in-process approve path, F4's retry code and F5's revert path were not opened.
- README.md was not read; `internal/feature/audit/export.go` was located but not opened.

### Earlier thin spots in this area

- AI Act Art. 113 and 26(6) text (open since 2026-11-08, 3 cycles): 26(6) **filled**; 113 still thin, now 4 cycles (EUR-Lex now returns text but the fetch is cut at Art. 112). The amendment status is **partly filled** from Commission pages.
- Feasibility of most rows (open 2 cycles): **partly filled**, 7 of 13 rows assessed.
- Rewind feasibility (open 3 cycles): still thin, now 4 cycles.
- A regulated organisation with an agent that holds its own authority (open since 2026-11-08): not touched, still thin.

### Sources (part 6)

| URL | Date read | Primary or secondary | Grade |
|---|---|---|---|
| https://eur-lex.europa.eu/legal-content/EN/TXT/?uri=CELEX:52025PC0836 | 2026-11-27 | primary | [E] start only, rest summary |
| https://digital-strategy.ec.europa.eu/en/library/digital-omnibus-ai-regulation-proposal | 2026-11-27 | primary | [E] |
| https://digital-strategy.ec.europa.eu/en/news/ai-omnibus-enters-force | 2026-11-27 | primary | [E] |
| https://www.consilium.europa.eu/en/press/press-releases/ | 2026-11-27 | primary | [N] HTTP 403 |
| https://eur-lex.europa.eu/legal-content/EN/TXT/HTML/?uri=OJ:L_202401689 | 2026-11-27 | primary | Art. 26(6) [Q]/[E]; Art. 113 [N] (cut at Art. 112) |
<!-- p6:end -->

<!-- p7:start -->
## Fit and bets; ranking; What changed since the last report

not started
<!-- p7:end -->

<!-- p8:start -->
## Proposed phases; Sources

not started
<!-- p8:end -->
