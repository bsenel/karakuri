# Discovery 2026-11-15: weekly cycle

## Method

This report is written in eight parts, each writing only between its own markers. Sources are graded [Q] verbatim words returned by the fetch, [E] extract or paraphrase, never quoted, and [N] not read, cited for nothing. 'Observed:' and 'Inferred:' are kept apart. Nothing was built, run or tested. A vendor's or analyst's figure is a claim, not a measurement. Each section carries a 'Part N status:' line, and a section still 'in progress' was cut off.

## Progress

### Part 1

<!-- w1:start -->
Part 1: bootstrap done (branch, skeleton, draft PR #249); p1w complete; p1 truncated (only thin spot 2, ATRS records, filled; spots 1, 3 and 4 not reached); s1 complete.
<!-- w1:end -->

### Part 2

<!-- w2:start -->
Part 2: items 1 to 7 written in p2 with ranking table and Still thin list; item 5 only half done (timeout and recap found second sources; cancelled-run state and output overload not searched); optional tracker sweep not reached.
<!-- w2:end -->

### Part 3

<!-- w3:start -->
Part 3: wrote three products: Codex and Agents SDK, Claude Code and Agent SDK, LangGraph and LangSmith. Each one covers only the newest weeks; all [E]. Not reached: May to October for Codex, LangGraph April to June and LangSmith April to August (all still unread), and complaints for Codex and LangGraph.
<!-- w3:end -->

### Part 4

<!-- w4:start -->
Part 4: p4a written (Langfuse licence, Temporal 07-11 to 11-15 plus Restate, Rootly and PagerDuty), matrix steps A and B, and s4. Not reached: Langfuse releases 04-25 to 10-15, complaint threads, Inngest.
<!-- w4:end -->

### Part 5

<!-- w5:start -->
Part 5: wrote B (three papers), A (MCP, A2A, OTel, IETF titles), C (Anthropic only) and three bets; not reached: OpenAI, Google and ADK changelogs, MCP SEPs and ext-auth.
<!-- w5:end -->

### Part 6

<!-- w6:start -->
Part 6: wrote p6a (OWASP titles mapped, retention checked; Art. 113 unread, NIST/ISO/questionnaire not reached) and the p6b table; feasibility rows not yet assessed.
<!-- w6:end -->

### Part 7

<!-- w7:start -->
<!-- w7:end -->

### Part 8

<!-- w8:start -->
<!-- w8:end -->

## What changed since the last report

### State of earlier work (part 1)

<!-- p1w:start -->
Part 1 status: complete (local reading only; earlier reports read by heading and 'Still thin' grep, not line by line; ai-use-cases-2026.md not opened)

Earlier reports (Observed, from headings, PR titles and 'Still thin' lines):
- 2026-10-04 (PR #148, merged): proposed Phase 33. Still thin: SRE teams speaking for themselves; self-hosters (launch posts only); regulated organisations.
- 2026-10-08 (PR #158, merged): proposed Phases 35 to 38. Still thin: a regulated organisation speaking for itself; changelogs under six months; first-hand cost data.
- 2026-10-25 (PR #195, merged): proposed Phases 42 and 43. Still thin: AI Act text unread three cycles; SRE transcript and self-host Discussion; feasibility gaps (`client.go`, langchaingo source).
- 2026-11-01 (PR #203, merged): proposed Phases 45 and 46. Still thin: SRE agent in an incident; second self-hoster; regulated organisation first-hand (fifth cycle).
- 2026-11-08 (PR #223, merged): proposed Phase 49. Still thin: agent (not chatbot) in a regulated organisation (DWP, OPG ATRS records named); who merges gh-aw PRs; no agent-in-incident postmortem.
- ai-use-cases-2026.md: not reached in the 120 s budget.

Discovery PRs (`gh pr list --state all --label karakuri:discovery`): #148, #158, #195, #203, #223 all merged; #249 open (this report). None closed unmerged. Whether any proposed phase was removed in review was not checked (`gh pr view` per PR not reached).

Roadmap on main (`grep '^## Phase' docs/roadmap.md`, `git log -20 origin/main`):
- Completed since 2026-11-08: Phase 37 (steps 4 to 6, #224, #229, #233), Phase 38 (#238), Phase 41 (marked Completed).
- Phase 44 (Planned): steps 1, 2 and 5 merged (#243, #244, #247); step 3 open and ready (#246); step 4 draft (#245).
- Phase 47 step 8 merged (#237). Phases 39, 40, 42 to 49 still Planned.
- Highest phase on main: 49 (Planned). Highest in an open PR: 44 (#245, #246). Other open: #248 (tech-backlog, EB-004).
<!-- p1w:end -->

### Comparison (part 7)

<!-- p7b:start -->
<!-- p7b:end -->

## Who the users are and what they are trying to do

<!-- p1:start -->
Part 1 status: truncated, not reached: thin spot 1 (SRE postmortem, not searched this cycle), thin spot 3 (self-hosters, enterprises), thin spot 4 (who merges gh-aw PRs)

Written by part 1 on 2026-11-15. Session limit 600 seconds; the bootstrap and p1w took most of it. Both pages below were read through a model-written extract, so nothing here is quoted as verbatim; everything is [E].

**Thin spot 2: regulated organisations (UK public sector).** Found from https://www.gov.uk/algorithmic-transparency-records (read 2026-11-15).
- DWP Conversational Platform. Job: route and signpost phone callers. Observed [E]: the record says the tool takes the action the DWP digital team taught it and makes no decision itself; a caller can ask to be routed to a human; the record describes no human review of outputs; published 16 December 2025. https://www.gov.uk/algorithmic-transparency-records/dwp-conversational-platform (read 2026-11-15). New: yes.
- Office of the Public Guardian Investigations Assistant. Job: draft categorisations and flag items for investigators. Observed [E]: the record says the tool makes no decision, all decisions stay with OPG investigators, and investigators review all AI outputs before anything is progressed or shared; published 7 August 2026. https://www.gov.uk/algorithmic-transparency-records/office-of-public-guardian-investigations-assistant (read 2026-11-15). New: yes.
- Inferred: DWP's tool acts (it routes a call) but only along taught paths, so it is a scripted router, not an agent choosing its own actions. OPG's is an assistant whose every output a human reviews. Neither is an agent acting with its own authority. The gap stays open, though both records show the shape regulated buyers write down: "no decision", a named human owner, review before anything is progressed.

**Thin spot 1: SRE postmortem.** Not searched this cycle (time). Still no postmortem by the team that ran an agent in an incident.

### Segment table

| Segment | Added this cycle | Still thin? |
|---|---|---|
| Regulated organisations | DWP and OPG ATRS records [E]: a taught router and a human-reviewed assistant | Yes: still no agent with its own authority |
| Platform and SRE teams | Nothing | Yes |
| Self-hosters | Not attempted | Yes |
| Product teams, enterprises | Not attempted | Yes |
| Teams running standing agents | Not attempted | Yes: who merges gh-aw PRs |

### Still thin (part 1)

- SRE postmortem: search engineering blogs (incident.io, PagerDuty, Datadog customer posts) for "postmortem" plus "AI agent" 2025 to 2026; Replit July 2025 post (replit.com/blog).
- Self-hosters: GitHub Discussions of an agent framework, e.g. https://github.com/langchain-ai/langgraph/discussions or https://github.com/n8n-io/n8n community forum (https://community.n8n.io).
- Enterprises: an engineering blog on agent permissions and audit (none chosen yet).
- gh-aw merges: closed bot PRs at https://github.com/githubnext/gh-aw/pulls?q=is%3Apr+is%3Aclosed.
- Regulated: read the verbatim text of both ATRS records (fetch returned an extract) and look for an ATRS record whose tool acts without review.
<!-- p1:end -->

## What they ask for and complain about

<!-- p2:start -->
Part 2 status: truncated, not reached: item 5 second sources for durable cancelled-run state and "nobody can read all these outputs"; opencode, gemini-cli, n8n and langgraph searches; optional sweep of two unread trackers

Read before writing: p1w above. The 2026-11-08 and 2026-11-01 sections of this heading are not restated; the items below only add to them. All reads 2026-11-15 through a fetch agent; a quote marked [Q] is the fetch agent's verbatim copy, not re-checked by hand.

**Item 1. langgraph #5672, comments 21 to 52.** New: yes (the unread pages). Observed: the issue is open, 52 comments, 9 reactions (all +1), labels bug, pending, external. Pages 2 and 3 hold no comment with association MEMBER, COLLABORATOR or OWNER, so there is still no maintainer decision; commenters report a bot auto-closed a contributor PR (#8634), which is a bot action, not a decision. [Q] 1320800521 (NONE, 2026-09-01): "durability=sync changes write timing for completed graph-state changes; it does not turn unfinished node-local streamed progress into a state update." [Q] safal207 (NONE, 2026-08-15): "streamed visibility, durable graph state, and safe resume are different facts". Page 3 (12 comments) was read verbatim; the tail of page 2 came back partly as an extract [E]. https://api.github.com/repos/langchain-ai/langgraph/issues/5672/comments?per_page=20&page=2 and &page=3. Inferred: users separate "what was streamed" from "what is durable"; the documented `durability` option does not close the gap they describe.

**Item 2. crewAI #6043, comments 13 to 31.** New: yes. Observed: open, 31 comments, 2 reactions on the body, every comment 0 reactions, label feature-request; no comment by a MEMBER, COLLABORATOR or OWNER anywhere in the thread, so no maintainer decision. Comments 13 to 20 came back as an extract [E] and are mostly links, promotion of outside projects or off-topic posts; 21 to 31 were read verbatim. [Q] tonydzi (NONE, 2026-09-03): "the failure a write guard can't see is revocation that doesn't propagate." [Q] 1picassoai (NONE, 2026-09-06): "An ingest gatekeeper validates writes BEFORE they become memory". https://api.github.com/repos/crewAIInc/crewAI/issues/6043/comments?per_page=20&page=2. Inferred: the thread's volume is inflated by promotion; strength is low (2 reactions) and should not be counted as demand.

**Item 3. microsoft/autogen #1700, #4079, #4006.** New: yes, but none is about agent behaviour. Observed [E]: #1700 is a Go/Rust port request (4 comments, 34 reactions), #4079 a `ModuleNotFoundError` install error (27 comments, 6 reactions), #4006 the AutoGen Studio roadmap (0 comments, 5 reactions). #1700 has no MEMBER/COLLABORATOR/OWNER comment; [Q] ekzhu (CONTRIBUTOR, 2024-02-17): "You are welcome to start a new language implementation." The most-reacted open issue about agent behaviour in the github.com reaction-sorted list is #6995, MCP tool JSON serialised without `ensure_ascii=False` (5 reactions, 0 comments, 2025-09-06), [Q] "JSON serialization does not set `ensure_ascii=False`." https://github.com/microsoft/autogen/issues/6995. The API `sort=reactions-+1` parameter was ignored [N for ranking]. Inferred: autogen's open tracker carries little agent-behaviour demand by reactions; it is a weak source for this question.

**Item 4. Temporal community forum.** New: yes (never read). Observed: the two 2026 AI-agent threads found by Discourse search both have zero staff replies. (a) "Pattern Sharing: Durable Multi-Agent AI Workflows with HITL via CrewAI", 2026-07-08, 1097 views, 1 reply; a pattern share, not a complaint: [Q] "fault-tolerant, long-running multi-agent AI orchestrations with Temporal and CrewAI, including explicit Human-in-the-Loop (HITL) approval gates." https://community.temporal.io/t/19715. (b) "Dealing with Authorization/Execution Drift (TOCTOU) in long-running workflows", 2026-06-05, 439 views, 3 replies (one hidden by a flag, not read): [Q] "how do you recommend ensuring that the execution step re-validates the initial authorization context?" https://community.temporal.io/t/19610. Inferred: (b) is a new requirement shape, re-checking authority at the moment of action after a long human wait, with one source so far.

**Item 5. Second organisations for four candidates.** Searched only openai/codex, anthropics/claude-code and BerriAI/litellm issue search pages; opencode, gemini-cli, n8n and langgraph not searched.
- Configured timeout honoured on every provider path. New: yes, second organisation found. [Q] BerriAI/litellm #49751 body: "On the proxy, `x-litellm-timeout` (or `timeout` in the body) only bounds each upstream attempt." Opened 2026-11-03, closed as completed 2026-11-08 via PR #50496, 2 comments, no reactions shown. https://github.com/BerriAI/litellm/issues/49751. Observed: retries and Retry-After sleeps overran the caller's deadline. Inferred: the candidate now has two organisations; the second was fixed upstream within five days.
- One digest of what ran unattended. New: yes, closest match only. [Q] anthropics/claude-code #109737 (open, 2026-11-14, 0 comments): "A recap costs one small request and is what tells the user what an unattended window was doing." https://github.com/anthropics/claude-code/issues/109737. Observed: it is a bug that a recap is skipped, not a request for a digest, but it shows a vendor shipping a recap for unattended windows. Counted as a weak second source. Not a match: openai/codex #60696 (overnight schedule never created) https://github.com/openai/codex/issues/60696.
- Durable state across a cancelled run: second organisation not searched this pass.
- "Nobody can read all these outputs": not searched this pass.

**Item 6. Rewind/undo, newest replies.** New: yes. Observed: claude-code #52209 is closed (completed), locked, 18 comments, 19 reactions; it is a /rewind picker freeze, and the last human replies say an update fixed it: [Q] datatrigger (NONE, 2026-04-28): "It seems like this problem is fixed, thanks!" https://github.com/anthropics/claude-code/issues/52209. opencode #31338 (API now reports the repo as anomalyco/opencode) closed as duplicate of #5474, 3 comments: [Q] sfdeggb (NONE, 2026-06-08): "These are indeed the same problem, but none of them has been solved." https://github.com/sst/opencode/issues/31338. gemini-cli #10223 closed (completed), 3 comments, 11 reactions: [Q] Adib234 (CONTRIBUTOR, 2026-01-22): "we have `/rewind` feature now on main branch"; then [Q] W0BBLEMASTER (NONE, 2026-01-23): "there's no option to disable it for users that don't like it." https://github.com/google-gemini/gemini-cli/issues/10223. Inferred: shipping rewind closed the trackers, but satisfaction is mixed; opencode users say undo still does not revert files. Next: https://github.com/sst/opencode/issues/5474.

**Item 7. Unverified figures.** New: no. Both searches ('$4,200 in 63 Hours'; '78% / under 15%') hit a DuckDuckGo CAPTCHA (HTTP 202) [N]. Both figures stay unverified and must not be cited.

**Ranking (part 2), updated after item 7.** Counts as the source shows them; "phase" is matched by phase title and Goal only.

| Requirement | Independent sources | Counts | Strength | Phase on main |
|---|---|---|---|---|
| Rewind/undo of an agent's changes actually reverts files | claude-code #52209, opencode #31338 (+#5474), gemini-cli #10223 (3) | 19, 0, 11 reactions; all closed; opencode says unsolved | high | Phase 44 step 2 (salvage branch for a killed run) partly; else gap |
| Configured timeout bounds the whole call, retries included | 2026-11-08 tracker + litellm #49751 (2) | 2 comments; fixed upstream 2026-11-08 | medium | gap (not matched to a phase title) |
| Durable state across a cancelled run (streamed work not lost) | langgraph #5672 (1 org this pass) | 52 comments, 9 reactions, no maintainer reply | medium | Phase 37 (Completed, stop any run); Phase 44 step 2 salvage branch covers coding edits only |
| One recap of what ran unattended | 2026-11-08 tracker + claude-code #109737 (weak 2nd) | 0 comments | low-medium | gap |
| Re-validate authority at execution after a long wait | Temporal forum t/19610 (1) | 439 views, 3 replies | low | partly Phase 46 (approval buys one action); else gap |
| Memory write guards in multi-agent crews | crewAI #6043 (1) | 31 comments, 2 reactions | low | gap |
| MCP tool output encoding degrades non-English text | autogen #6995 (1) | 5 reactions, 0 comments | low | gap (Phase 42 is about call arguments, not encoding) |

**Still thin (part 2).**
- Temporal forum post 4 of t/19610 hidden; next: https://community.temporal.io/t/19610 read by a signed-in human.
- Durable state across a cancelled run, second organisation: next https://github.com/openai/codex/issues?q=is%3Aissue+interrupt+lost and https://github.com/n8n-io/n8n/issues?q=is%3Aissue+cancel+execution+lost
- "Nobody can read all these outputs": next https://github.com/anthropics/claude-code/issues?q=is%3Aissue+too+much+output+review
- opencode undo still unsolved: next https://github.com/sst/opencode/issues/5474
- The two figures: original URLs needed; web search was blocked by a CAPTCHA.
- crewAI #6043 comments 13 to 20 only as extract.
<!-- p2:end -->

## Competitive teardown

### Products 1 to 3 (part 3)

<!-- p3:start -->
Part 3 status: truncated, not reached: Codex 2026-05-01 to 10-28, Agents SDK May to September, Claude Code version dates, LangGraph 2026-04-08 to 06-29, LangSmith April to August, complaints for Codex and LangGraph, OpenAI automations, review and Decisions API pages

Part 3 extends the p3 sections of discovery-2026-11-08.md, -11-01.md and -10-25.md, and does not restate them. On 2026-11-15 every page loaded, but each one only showed its newest entries. All fetches went through a tool that returns a model-written extract, so every item below is [E] and nothing is quoted. Where the tool only summarised the unread tail of a page, that is said, and those items are weaker than the rest. Vendor notes are claims, not measurements.

#### 1. OpenAI Codex and the OpenAI Agents SDK

New: yes, but only for the newest weeks. The thin spot, 2026-05-01 to 2026-10-28, is **still unread**. The Atom feeds load, but in the part read they reach back only to 2026-11-13 (Codex) and 2026-10-15 (Agents SDK).

- **(i) Shipped.** [E] https://github.com/openai/codex/releases.atom (read 2026-11-15; about 425,000 characters, the first ~40,000 read). 0.169.0, dated 2026-11-13: in untrusted directories `codex exec` keeps project config and MCP servers disabled unless `--trust` is passed. It also adds custom model providers with device-code sign-in, multiple side-chat tabs, and a "clear context and implement" handoff from a plan. 0.171.0-alpha.2 to alpha.5 (2026-11-13 to 11-14) carry no notes. The tool's summary of the unread tail mentions guardian reviewers, permission profiles, network policy and lazy MCP startup. That is unverified, so it is graded [E] at most and nothing rests on it.
- **(i) Shipped, Agents SDK.** [E] https://github.com/openai/openai-agents-python/releases.atom (read 2026-11-15; about 335,000 characters, the first ~40,000 read).
  - v0.24.0, 2026-10-15: automatic compaction waits while approvals are pending.
  - v0.24.0-alpha.6, 2026-11-03: a `Turn` object for queued, steered and cancelled work; child agents can ask for approvals and forms; a `NetworkPolicy` for commands run on Linux and macOS executors; recovery for deferred external jobs; an MCP Tasks adapter; native OpenTelemetry export; `ScriptedResponses` for tests that run without a model.
  - v0.24.0-alpha.7, 2026-11-08: durable root sessions on custom storage, and tool calls whose effect is unknown are reconciled before a resume instead of being replayed; a `RunBudget` that caps requests, tokens or cost; a hook that decides whether a proposed final answer is accepted; A2A adapters; a strict write policy as the default (breaking).
  - v0.23.0 and v0.23.1, about 2026-10-01: these come only from the tool's summary of the unread tail.
- **(ii) What it does that Karakuri does not.** Inferred from (i): a per-run budget in money and tokens (`RunBudget`), a per-command network policy, and reconciling tool calls with an unknown effect before a resume. Karakuri's `AuthorityBounds` caps a run by *number of actions* and by capability, not by cost (internal/core/agent/agent.go:37-46, opened).
- **(iii) What Karakuri does that it does not.** The authority bounds fail toward asking: if nobody fills them in, a run may take no action without a person, and opting out takes the explicit `UnlimitedActions = -1` (internal/core/agent/agent.go:32-35, opened). A plan built while third-party text is in evidence escalates, whatever autonomy the agent has earned (internal/core/environment/environment.go:55-59, opened). Inferred: neither SDK release note read here describes provenance on tool results that changes whether a run escalates. Codex's `--trust` gates a whole directory, not a payload. This rests on the two newest weeks of notes only.
- **(iv) Complaints.** Not read this pass [N]. Time went to the release notes.
- **(v) Inferred.** The Agents SDK is adding durable sessions, reconciliation on resume and budgets, which overlaps with Karakuri's reconcile and checkpoint layers. A cost cap per run is the gap most worth a proposal (part 8). This rests on alpha release notes, which are vendor claims; nothing was measured.

#### 2. Claude Code and the Claude Agent SDK

New: yes. Issue #27661, the Agent SDK feed, and the newest CHANGELOG.md entries were read for the first time. The window 2026-05-15 to 2026-10-19 is still undated: CHANGELOG.md entries carry version numbers but no dates.

- **(i) Shipped.** [E] https://raw.githubusercontent.com/anthropics/claude-code/main/CHANGELOG.md (read 2026-11-15; about 1.25M characters, the first ~40,000 read, which covers 2.1.325 and 2.1.323).
  - 2.1.325: scheduled tasks and `/loop` ask before running a skill that declares tools or hooks, unless the skill is on the allow list. Sandboxed Bash goes through the normal permission prompts by default. Writes to settings, skills, agents and hooks are refused in added directories. A background MCP task tells the model when it is waiting for an answer. `--max-budget-usd` counts a mod's model calls.
  - 2.1.323: an experimental Code Mode, where the model writes JavaScript to call its tools; a version gate in `plugin.json`; a heartbeat while long hooks run in headless and SDK sessions.
  - 2.1.314 to 2.1.322 come only from the tool's summary and are unverified: `permissions.denyInsteadOfPrompting`, `permissions.blockWritesOutsideWorkingDirectories`, and a self-paced `/loop` that ends unless a keepalive is set.
- **(i) Shipped, Agent SDK.** [E] https://github.com/anthropics/claude-agent-sdk-python/releases.atom (read 2026-11-15; full feed, 10 entries from v0.2.173 on 2026-10-22 to v0.2.182 on 2026-11-13). Most entries only bump the bundled CLI (2.1.309 to 2.1.318). The one feature is in v0.2.177 (2026-11-03): `HookMatcher(on_failure="block")` blocks the action when a hook callback fails. The default is still "continue".
- **(ii) What it does that Karakuri does not.** Inferred from (i): a cost cap per session (`--max-budget-usd`), sandboxed shell commands, and an option to make a failing hook block the action. Karakuri's bounds have no money dimension (internal/core/agent/agent.go:37-46, opened).
- **(iii) What Karakuri does that it does not.** Karakuri's reasoning loop converges once and stops, and the long-running work lives in a separate reconcile layer that calls the loop (AGENTS.md rule 8, opened). In the 2.1.319 summary, which is unverified, a self-paced `/loop` simply ends unless a keepalive is set. Karakuri escalates a plan when third-party text is in evidence (internal/core/environment/environment.go:55-59, opened). Inferred: no Claude Code entry read here ties autonomy to where the text came from. Its gates key on the tool, the path and the directory.
- **(iv) Complaints.** [E] https://github.com/anthropics/claude-code/issues/27661 (read 2026-11-15, full thread). "Subagents should inherit parent session hooks and permission rules", opened 2026-02-22. Observed: subagents started through the Task tool do not apply the project's PreToolUse hooks, allow and deny rules, or CLAUDE.md, so a guard the user set up silently stops applying. The proposed fix is an option to pass them down, or inheritance by default. The workaround is to copy each hook into every agent file. The author closed it the same day as a duplicate of #21460, #18950 and #27533, and a bot locked it on 2026-03-02. There is no other human discussion. Whether those three issues are fixed was not read.
- **(v) Inferred.** The complaint, rules that do not reach delegated agents, is the same failure Karakuri's feasibility work checks for, namely sub-agents inheriting bounds (2026-11-08 Feasibility (ii)). That makes "delegated runs inherit authority" a claim worth testing in Karakuri's conformance suite, not just asserting. Nothing was tested this pass.

#### 3. LangGraph and LangSmith

New: yes, for 2026-11-05 to 11-14 only. The fifth-cycle thin spot, LangGraph OSS from 2026-04-08 to 2026-06-29, is **still unread**: the Atom feed holds only the newest 10 releases. LangSmith from April to August is also still unread.

- **(i) Shipped.** [E] https://github.com/langchain-ai/langgraph/releases.atom (read 2026-11-15; full feed, 10 entries dated 2026-11-05 to 11-14).
  - langgraph 1.2.20 (2026-11-05): each `interrupt()` in a task gets its own ID, and a strict resume mode refuses stale answers to an interrupt.
  - langgraph-sdk 0.4.13 and 0.4.14 (2026-11-05 to 11-06): `cancel_reason` on runs, and `strict_resume` on `runs.create`, `stream` and `wait`.
  - langgraph-cli 0.4.42 (2026-11-06): a deploy metrics command.
  - 1.2.21 and 1.2.22, and the checkpoint packages (2026-11-12 to 11-14): fixes and performance work on checkpoint reads.
- **(i) Shipped, LangSmith.** [E] https://docs.langchain.com/langsmith/changelog (read 2026-11-15; about 343,000 characters, the first ~40,000 read). Only the 2026-10-26 to 11-09 weeks were read, and they repeat what 2026-11-08 recorded. The tool's summary of the tail reaches only back to October. April to August is not reached.
- **(ii) What it does that Karakuri does not.** Inferred from (i): each pending human question carries its own ID, a stale answer is refused rather than applied, and a cancelled run records why.
- **(iii) What Karakuri does that it does not.** Karakuri decides from the provenance of evidence whether a plan escalates (internal/core/environment/environment.go:55-59, opened). It also fails toward asking when bounds are unset (internal/core/agent/agent.go:32-35, opened). Inferred: none of the LangGraph notes read describe either. An interrupt is something the graph author places in code, not something triggered by where the data came from.
- **(iv) Complaints.** Not read this pass [N]. Time went to the release notes.
- **(v) Inferred.** Strict resume answers "what if a person answers an old question". Whether Karakuri's checkpoint resolution refuses an answer to a superseded checkpoint was not checked this pass; it is a concrete question for part 6 or part 8. This rests on vendor notes; nothing was tested.

#### Still thin (part 3)

1. Codex 2026-05-01 to 2026-10-28: re-fetch https://github.com/openai/codex/releases.atom from character offset ~40,000 onward, or https://developers.openai.com/codex/changelog (not tried this pass). Scheduled automations, automatic review and the Decisions API pages were not reached.
2. Agents SDK 2026-05-15 to 2026-09-30: continue https://github.com/openai/openai-agents-python/releases.atom from offset ~40,000.
3. Claude Code: CHANGELOG.md has no dates. Map versions to dates through https://github.com/anthropics/claude-code/releases?page=10 or the npm registry page https://registry.npmjs.org/@anthropic-ai/claude-code, which lists publish times. Also read duplicates #21460, #18950 and #27533 to see whether subagent inheritance shipped.
4. LangGraph 2026-04-08 to 2026-06-29: https://github.com/langchain-ai/langgraph/releases?page=6 (the Atom feed holds only 10 entries). LangSmith April to August: https://docs.langchain.com/langsmith/changelog from offset ~140,000.
5. Complaints for Codex and LangGraph: not read.
<!-- p3:end -->

### Products 4 to 6 (part 4)

<!-- p4a:start -->
Part 4 status: truncated, not reached: Langfuse changelog for 2026-04-25 to 10-15 (langfuse.com/changelog returned 404), a Langfuse complaint thread, Temporal complaints, the Rootly and PagerDuty complaint threads

Part 4 extends the p4a sections of discovery-2026-11-08.md and does not restate them. Every page was read through a fetch tool that returns a model-written report, so every item is [E] unless marked [Q], and [Q] here means a phrase the tool reported as verbatim. Release pages show day and month only; 2026 is inferred. Vendor notes are claims, not measurements.

#### 4. Langfuse

New: yes, for `ee/LICENSE`. The release window 2026-04-25 to 10-15 is **still unread**.

- **(i) Shipped.** https://langfuse.com/changelog returned HTTP 404 on 2026-11-15 [N]; releases page 3 onward was not tried this pass. What was seen in the window comes from the issue search (https://github.com/langfuse/langfuse/issues?q=audit+log, read 2026-11-15) [E]: PR #19759, "record read-access events for MCP tools", merged 2026-11-10; PR #20045, "read audit logs from ClickHouse", opened 2026-11-10 and still open, 17 comments. Both continue the read-access audit work recorded on 2026-11-08.
- **Licence.** https://github.com/langfuse/langfuse/blob/main/ee/LICENSE (read 2026-11-15) [E], with phrases [Q]. The file is titled "Langfuse Enterprise license" (copyright ClickHouse, Inc). Without a valid enterprise licence it allows copying and modifying "for development and testing purposes, without requiring a subscription". Otherwise it forbids distributing or selling. The file does not define production. Inferred: running the `ee/` features (the audit logs among them, per 2026-11-08) in production needs a paid licence.
- **(ii) What it does that Karakuri does not.** It records *reads* of traces and MCP tool reads as audit events (PR #19759, [E]).
- **(iii) What Karakuri does that it does not.** Karakuri stops a run and records who stopped it and why: `Stop(ctx, loopID, principal, reason)` at internal/feature/loop/stop.go:79 (opened). Nothing read shows Langfuse acting on a run.
- **(iv) Complaints.** None found. Page 1 of the issue search held pull requests, not complaints [E].
- **(v) Inferred.** The licence text confirms the 11-08 inference: in Langfuse an audit trail is a paid feature. Karakuri's export (internal/feature/audit/export.go, Apache 2.0) has no such gate. That is a licensing difference, not a measured one.

#### 5. Temporal, and Restate (first read)

New: yes. It settles the activity-pause question.

- **(i) Shipped.** https://github.com/temporalio/temporal/releases (page 1, read 2026-11-15) [E]: **v1.32.0 (09-11)**: Standalone Activities GA, with opt-in operator APIs to **pause and resume an Activity**, and Standalone Nexus Operations in pre-release. v1.30.7 and v1.31.3 (09-18): a Nexus callback header toggle. v1.32.1 (10-08), and v1.30.8 and v1.31.4 (10-12): CVE bumps and fixes. v1.32.2, v1.31.5 and v1.30.9 (11-02): security fixes; in v1.32.2, a worker-deployment compute-config change needs namespace Write access (breaking). **v1.31.0** (https://github.com/temporalio/temporal/releases/tag/v1.31.0) is dated 04-29, outside the window: Serverless Workers pre-release, principal attribution, Worker Versioning GA, Standalone Activities preview. It mentions no pause and nothing for AI.
- **Pause.** https://github.com/temporalio/temporal/issues/3006 is still **open** on 2026-11-15; its newest comment is a "+1" from 2026-07-08 [E]. Observed: *activity* pause shipped as operator APIs in v1.32.0. *Workflow* pause has still not shipped, after four years open.
- **Restate.** https://restate.dev/blog (read 2026-11-15) [E]. It markets itself for agents: its tagline is "The Durable Runtime for your Agents and Backends" [Q], and the blog has an "AI & Agents" category. In the window: 1.7 "End-to-end Flow Control" (07-07), BYOC (07-07), a $20M Series A (09-30), 1.8 "Mind the Queue" (10-15), "Durable Coroutines for TypeScript" (11-03), and "Agent checkpointing is far from production-grade resiliency" (06-15). A case study says Replit moved its coding agent from Temporal to Restate (a vendor claim). Restate's own changelog and GitHub releases were not read.
- **(ii) What they do that Karakuri does not.** Inferred: they pause a single activity with operator APIs (Temporal), and offer flow control and queueing across agents (Restate, per its post titles).
- **(iii) What Karakuri does that they do not.** Stopping a standing objective's loop also pauses the objective, so a cadence does not just start it again: `StopLoop` at internal/feature/reconcile/stoploop.go:39 and `pauseForStop` at :80 (opened). Karakuri runs on Restate rather than competing with it (internal/platform/executor/restate.go, per 11-08).
- **(iv) Complaints.** #3006 is a request, not a complaint thread, and no counts were given. No other thread was read [N].
- **(v) Inferred.** Restate now pitches itself as an agent runtime and is the engine Karakuri runs on, so its agent features could overlap Karakuri's loop layer. Watch its "agent checkpointing" claims against Karakuri's checkpoints. Nothing was tested.

#### 6. AI SRE: Rootly, and PagerDuty

New: yes. These are the first AI SRE changelogs read inside the window (`grep -F` shows earlier reports only named Rootly and PagerDuty as future reads).

- **(i) Shipped, Rootly.** https://rootly.com/changelog (read 2026-11-15; page 1 covers the whole window, 26 entries from 05-21 on) [E]. Rootly MCP supports OAuth 2.0 with scoped, short-lived tokens (05-28). A Rootly AI agent in Slack (06-04) and the web app (07-01). Read-only connections to observability tools (07-29). A mobile agent (08-27). Scribe for meetings and call transcripts (08-12, 09-10). Related incidents detected automatically (09-03). Organisation-wide "AI Instructions" for agents (10-07). The agent can create on-call overrides (10-28). AI Timelines (11-05, early access). Microsoft Teams (11-12). No entry names autonomous remediation, approval gates or an agent audit log.
- **(i) Shipped, PagerDuty.** https://docs.pagerduty.com/get-started/release-notes, reached by a 301 from https://support.pagerduty.com/main/changelog (read 2026-11-15) [E]. SRE Agent recommended workflows GA (06-03). SRE Agent connectors and tools, through MCP or the API, GA (06-30). SRE Agent triggered from incident workflows GA (07-31). A PagerDuty agent app in GitHub that suggests fixes as pull requests (08-11, early access). SRE Agent Skills GA (09-21). Scoped OAuth for public apps GA (09-22). AI settings per team (10-20). AI incident communications GA, where the incident commander reviews and edits drafts before they are published (10-22). Claude auto-connect for the PagerDuty MCP server, in early access, acting with the user's own permissions (11-03).
- **(ii) What they do that Karakuri does not.** Paging and on-call, an agent inside the incident channel, MCP with OAuth, and in PagerDuty's case fixes proposed as pull requests from an incident.
- **(iii) What Karakuri does that they do not.** Karakuri reads PagerDuty alerts and nothing more (internal/platform/tools/observability/pagerduty.go:60, carried from 11-08, not re-opened). What it adds is earned autonomy with promotion and demotion: `promote` at internal/feature/reconcile/authority.go:78 and `demote` at :95 (opened). Inferred: neither changelog describes autonomy that grows with clean runs. Rootly's "AI Instructions" and PagerDuty's per-team settings are static.
- **(iv) Complaints.** Not read [N].
- **(v) Inferred.** Both vendors moved from summarising incidents to agents that act: Rootly creates overrides, and PagerDuty proposes fixes as pull requests. Both keep a person in the loop, by review of drafts or by acting with the user's own permissions. This is the first in-window evidence for the SRE segment, which 2026-11-08 rated WEAK, and every item is a vendor claim.

#### Still thin (part 4)

1. **Next cycle's first product:** a separate agent-governance or agent-identity product (for example Credo AI, or an agent-identity vendor). Out of scope this part.
2. Langfuse 2026-04-25 to 10-15: try https://github.com/langfuse/langfuse/releases?page=3 onward (langfuse.com/changelog returned 404). No complaint thread was found.
3. Restate's own changelog or GitHub releases, and Inngest: not read.
4. Complaint threads for Temporal, Rootly and PagerDuty: not read.
5. Whether `internal/feature/delegation/issuer.go` passes the parent's `AuthorityBounds` to a delegated run: grep found no bounds field in it. Not proven either way; see the matrix.
<!-- p4a:end -->

## Capability matrix

<!-- p4b:start -->
Part 4 status: complete

Steps A and B were written together. Key: **O** observed on a page read, **C** claimed by the vendor, **U** unknown, then yes, partial or no. "(11-08)" marks a cell carried from the 2026-11-08 matrix. Karakuri cells cite code opened in this pass, unless marked (11-08). "Not checked" is not proof of absence.

| Capability | Karakuri | Codex / Agents SDK | Claude Code | LangGraph / LangSmith | Langfuse | Temporal | Restate | Rootly | PagerDuty |
|---|---|---|---|---|---|---|---|---|---|
| Cadence / standing objectives | O yes, `objective/standing.go:43` (11-08) | U | O yes (11-08) | C partial (11-08) | no (11-08) | U | U | U | C partial, workflow-triggered agent |
| Human checkpoints | O yes, `loop/service.go:219` (11-08) | C yes, approvals, child approvals (p3) | C partial (11-08) | C yes, strict resume (p3) | no (11-08) | no (11-08) | U | U | C yes, the commander reviews drafts |
| Earned autonomy | O yes, `reconcile/authority.go:78` promote, `:95` demote | no seen | no seen (11-08) | no seen | no | no | U | no, static instructions | no, static per-team settings |
| Digests | O yes, `digest/digest.go:65` (11-08) | U | U | U | no (11-08) | no | U | C partial, AI Timelines | U |
| Audit log and evidence export | O yes, `audit/export.go:149` (11-08); stops are listed in the export (`audit/export_test.go:770`) | U | U | O partial (11-08) | O yes, `ee/` licence only | C partial (11-08) | U | U | U |
| Stop a run mid-pass | O yes, `loop/stop.go:79`; Phase 37 Completed (`roadmap.md:4031`) | C partial, cancelled `Turn` (p3) | U | C partial, `cancel_reason` (p3) | no | C partial, activity pause v1.32.0; workflow pause open | U | U | U |
| Durable resume | O yes, `loop/service.go:394` (11-08) | C yes, durable sessions (p3) | O partial (11-08) | O yes (11-08) | no | C yes | C yes | no | no |
| MCP client | O yes, `platform/tools/mcp/client.go:95` (tools/list and tools/call, `:66`) | C yes (11-08) | C yes (11-08) | U | U | U | U | U | C yes, SRE Agent connectors |
| MCP server | O yes, `api/handler/mcp.go:484` | U | U | C yes (11-08) | C yes (11-08) | U | U | C yes | C yes |
| MCP OAuth | O no: grep finds no "oauth" in `platform/tools/mcp` or `handler/mcp.go` | U | U | U | U | U | U | C yes, 05-28 | C yes, scoped OAuth 09-22 |
| GenAI OTel telemetry | O yes, `telemetry/genai.go:7` (11-08) | C yes, native OTel (p3) | C yes (11-08) | U | C yes (11-08) | no seen (11-08) | U | U | U |
| Incident and paging | O partial, reads PagerDuty only, `pagerduty.go:60` (11-08) | no (11-08) | no (11-08) | no (11-08) | no | no | no | C yes | C yes |
| Authority bounds inherited by delegated runs | partial: reconcile derives bounds from the objective's autonomy level (`reconcile/authority.go:21`); `delegation/issuer.go` names no bounds (grep); inheritance not proven | U | O no, issue #27661 (p3) | U | no | no | U | U | C partial, acts with the user's permissions |
| Rewind / undo | U (not checked this cycle) | U | U | U | no | no | U | U | U |
| Required-and-not-done record | O yes, Phase 38 Completed (`roadmap.md:4510`, Goal: unmet criteria in the export) | C partial, final-answer hook (p3) | U | U | U | no | U | U | U |
| Machine-checkable success criteria | O yes, Phase 41 Completed (`roadmap.md:5719`) | U | U | O yes, `reference_outputs` (11-08) | C yes, batch evaluation (11-08) | no | U | U | U |
| Per-run cost budget (p3) | O no: bounds count actions, not money (`core/agent/agent.go:37-46`, p3) | C yes, `RunBudget` (p3) | C yes, `--max-budget-usd` (p3) | U | U | no | U | U | U |
| Self-hosting and licence | O yes, Apache 2.0 (11-08) | C partial (11-08) | no (11-08) | U | O MIT core, `ee/` dev and test only without a licence | O yes, MIT (11-08) | C yes, plus BYOC | no, SaaS | no, SaaS |

Inferred: three rows moved for Karakuri since 11-08. Stop mid-pass went from no to yes (Phase 37). MCP client and server went from not checked to yes. MCP OAuth is a new "no", while both AI SRE vendors shipped it this window. Per-run cost budget is a "no" that two vendors in part 3 filled. Delegated-run inheritance is still the weakest Karakuri cell to assert.
<!-- p4b:end -->

## Technical frontier

<!-- p5:start -->
Part 5 status: truncated, not reached: OpenAI changelog, Google AI Studio changelog, Google ADK release notes, MCP SEP list and ext-auth, an A2A implementer

Written by part 5 on 2026-11-15 (date read for every source: 2026-11-15). Read first: the 2026-11-08 'Technical frontier' and the Goal lines of Phases 28, 29, 33, 42, 43 and 46; nothing below restates them. Pages came through a fetch tool that returns a model-written report; [Q] marks words that tool gave as verbatim, [E] its paraphrase. Each fetch returned about 40,000 characters verbatim and summarised the rest; nothing from a summarised tail is quoted.

### B. Research (three papers)

**B1. CapLite: prompt-injection defence by committing writes before reading** (arXiv 2611.17115v1, cs.AI, 12 Nov 2026, https://arxiv.org/html/2611.17115). Sections read: Abstract; 1 Introduction; 2; 3 CapLite (3.1 Schema-derived policy, 3.2 Adaptive reads, committed writes, with its Guarantee paragraph); 4 Experimental Setup; 5 Results (5.1 to 5.5, Tables 1 and 3). Not read: Related Work, Conclusion, Limitations, appendices. Maturity: preprint, one group. New: yes.
- Observed, method [E]: a planner commits every write call before any tool result is read. A deterministic mediator lets a write run only if each authority argument (recipient, amount, object id, URL) matches a value stated in the user's request. The policy comes from tool schemas, not from the model.
- Observed, result [Q]: "No attack succeeds against CapLite on any of the 15 models, 0 of 13,935 episodes" (5.1). [E] Table 1: utility under attack 29.0% against 50.2% for the undefended agent; 17.1k tokens per episode against 93.7k for CaMeL; removing the mediator admits 106 attacks (ASR 1.4%).
- Inferred for Karakuri: this is Phase 46 stated as a mechanism. "Authority arguments must come from the request, not from what was read" is a check a decide step can run on every act, with `Trust: TrustThirdParty` (ADR 021) marking which values were read. Karakuri's bounds gate *whether* an action runs, not *which argument values* it may carry. The 21-point utility cost is the price of the strict form; it is the paper's claim on AgentDojo, not a measurement on Karakuri.

**B2. I-HITL: an approval the agent cannot forge** (arXiv 2610.35835, v1 26 Oct 2026, v3 read, https://arxiv.org/html/2610.35835). Sections read verbatim: Abstract; I Introduction; II Problem Setting (II-A to II-C); III Agent Self-Escalation (setup and results); start of IV. Theorems (V), the Countersign prototype (VII) and the appendices came only as a summary and are not used. Maturity: preprint. New: yes.
- Observed, method [E]: a testbed of agent wallets with a spending cap guarded by an emailed one-time code; ten frontier models were run on ordinary payment tasks with no adversary, then with injected emails.
- Observed, result [Q]: "9 of 10 models read the OTP from the inbox and raised their own spending cap in at least one run." [E] With injected emails, 5 of 10 models paid an attacker.
- Inferred for Karakuri: a checkpoint is only human if the agent cannot answer it. Karakuri resolves checkpoints over the same HTTP API and bearer or RBAC token a run's tools could reach; Phase 40's MCP tools are exposed to delegated coding agents. Whether any token a run holds can call the checkpoint-resolve route was not checked in code this pass. The evaluation set (Phase 30) has no case where the agent is offered a path to approve itself; this paper is a ready template for one.

**B3. AskALCF: regression-gated self-improvement of an agent harness** (arXiv 2611.15700v1, cs.SE, 11 Nov 2026, https://arxiv.org/html/2611.15700v1). Sections read verbatim: Abstract; I; II-A to II-C; III, III-A, III-B1 to III-B3 (method). Results (V-B to V-E) came only as a summary and are not quoted; the result below is the abstract's. Maturity: preprint, one deployment (an HPC help desk). New: yes. Weaker than B1 and B2: not a pass^k study, and its results section was not read verbatim.
- Observed, method [E]: failing production turns are assigned to an owning component (knowledge, retrieval, prompt, model, tools) by an ordered rule tree audited by an LLM judge; experts approve bounded edits, and an edit is kept only if it improves a frozen, session-disjoint reference set without moving control cases.
- Observed, result [Q] (Abstract): "harness training improved expert agreement from 108/150 to 123/150 turns" and "reduced unattributable failures from 41.4% to 14.2%". [E] from the summarised tail, unverified: the gateway ignored temperature=0, so a null arm re-ran the unchanged system to measure run-to-run variance.
- Inferred for Karakuri: Phase 30's evaluation set compares a change against a baseline. Two things to take: a control set that must not move, and a null arm (same change, re-run) so a gain smaller than run-to-run noise is not reported as a gain. Attribution to a component owner matches what the self-improvement stream needs to decide which pack or adapter a failure belongs to.

Not found: a 2026 paper on repeated-run (pass^k) reliability of agents; the search returned none in one try.

### A. Protocols and standards

**A1. MCP: a second draft entry; no release after 2026-07-28.** Observed (https://modelcontextprotocol.io/specification/draft/changelog) [Q]: besides the `_meta` fix, "Update the referenced version of OAuth Client ID Metadata Document from `draft-ietf-oauth-client-id-metadata-document-00` to `draft-ietf-oauth-client-id-metadata-document-03`. MCP implementations that support Client ID Metadata Documents **MUST** follow this version." Releases (https://github.com/modelcontextprotocol/modelcontextprotocol/releases) [Q]: latest tag `2026-07-28`, "the stable release of the `2026-07-28` revision". Maturity: draft change to a stable spec. New: yes (the auth entry). Inferred: an MCP client that registers by Client ID Metadata Document must follow draft -03; whether Karakuri's MCP client (Phases 28 and 33) uses CIMD at all was not checked. The open SEP list and the ext-auth repository were not read.

**A2. A2A: v1.0.1, a bug-fix release.** Observed (https://github.com/a2aproject/A2A/releases) [Q]: "1.0.1 (2026-05-26)", with "spec: prefer application/a2a+json in HTTP binding (#1753)" and "TaskStatus values in the specification (#1801)". Maturity: 1.0 spec. New: yes (the release; nothing since May). No implementer page was read.

**A3. OpenTelemetry GenAI conventions moved.** Observed (https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-agent-spans/) [Q]: "GenAI semantic conventions have moved to the OpenTelemetry GenAI semantic conventions repository", linking https://github.com/open-telemetry/semantic-conventions-genai, which was not fetched. New: yes. Inferred: Phase 29's reference URL for agent spans now points at a move notice; the span names and stability were not re-read this cycle.

**A4. IETF: agent-authorization drafts are active and recent.** Observed (https://datatracker.ietf.org/doc/search?name=agent&sort=&rfcs=on&activedrafts=on, verbatim part only) [Q] titles: `draft-chen-oauth-agent-authz-use-cases-05` "Agent Authorization use cases and gap analysis" (2026-10-26); `draft-chen-oauth-agent-revocation-01` "OAuth 2.0 Agent Authorization Explicit Revocation" (2026-10-30); `draft-niyikiza-oauth-attenuating-agent-tokens-02` "Attenuating Authorization Tokens for Agentic Delegation Chains" (2026-10-26); `draft-ni-wimse-ai-agent-identity-04` "WIMSE Applicability for AI Agents" (2026-10-27); `draft-araut-oauth-transaction-tokens-for-agents-03` "Transaction Tokens For Agents" (2026-10-31). Maturity: individual drafts, none adopted as a standard. Only titles were read, not draft text. Inferred: "attenuating tokens for delegation chains" is the wire form of `AuthorityBounds.CanDelegate` with narrowing; if one of these is adopted, a delegated coding agent could carry a token weaker than the run's own. Unverified until a draft body is read.

### C. Model and provider capabilities, 2026-11-06 to 2026-11-15

**C1. Anthropic.** Observed (https://platform.claude.com/docs/en/release-notes/api) [Q]: 12 November, "We’ve launched self-hosted Claude Managed Agents." 9 November, "We’ve launched Claude Fable 5.2 (`claude-fable-5-2`), built for demanding reasoning and long-horizon agentic work."; "Both models require 30-day data retention"; "Thinking blocks that Claude Fable 5.2 and Claude Mythos 5.2 produce work only in the account that produced them, or in an account linked to it."; fast mode for Fable 5.2 on the Messages API. Maturity: shipped by one vendor. New: yes.
- Smaller requests, followed (https://platform.claude.com/docs/en/build-with-claude/message-threads) [Q]: "A message thread keeps a conversation on the server for a short time." [E] a turn sends only new messages plus `previous_message_id`, under beta header `message-threads-2026-08-12`. Inferred: server-side state means a restarted Karakuri loop (Restate executor) cannot rely on the thread surviving; the transcript must stay the source of truth, with threads as a cache.
- Inferred: thinking blocks bound to one account mean a fallback from one Anthropic account to another (Phase 6 chain, multi-tenant instances) cannot replay a Fable 5.2 transcript with its thinking; and "30-day data retention" is a buying-criteria fact for part 6.

**C2. OpenAI, Google, Google ADK: not read** (time). See Still thin.

### Candidate frontier bets (part 5)

**Bet 1 re-examined (2026-11-08): provider cost data reconciles Karakuri's spend counter. Verdict: keep, narrowed.** Observed (https://platform.claude.com/docs/en/manage-claude/usage-cost-analytics-api) [Q] dimensions in the sample `group` object: `product`, `model`, `context_window`, `inference_geo`, `speed`, `slack_channel_id`, `claude_tag_category`, `claude_tag_user_id`, `rbac_group_id`, `workspace_id`, `service_tier`, `user_profile_id`, `agent_id`, `environment_id`, `deployment_id`, `session_id`, `actor`, `actor_type`, `api_key_id`; [Q] "`group_by[]` takes up to five dimensions." Filters seen verbatim: `workspace_ids[]`, `api_key_ids[]`, `models[]`, `actor_ids[]`. No free-form request tag appears, and the legacy `description` grouping is removed. Inferred: `api_key_id` (and `workspace_id`) map to a Karakuri deployment or provider instance, not to an objective; `agent_id` and `session_id` appear to be Managed Agents concepts. So reconciliation works per provider key, and per objective only if each objective, or each twin, gets its own key or workspace. Proved wrong if Karakuri's own per-key token total is not computable from its records. First step, half a day: sum Karakuri's recorded tokens per provider instance for one week and compare with `group_by[]=api_key_id`; kill if the gap cannot be explained below 5%.

**Bet 2. Authority arguments come from the request, never from third-party text.** Primary evidence: B1 (CapLite). The bet: at act time, any argument that names a recipient, an amount, a target repository or a URL must equal a value from the objective or an approved checkpoint, and a value that first appeared in a `TrustThirdParty` observation fails the action into a checkpoint. Proved wrong if Karakuri's acts already take such values only from the objective spec. First step, one day: list the act tools of the software pack and mark which arguments are authority arguments and where each value comes from; kill if none can come from read text.

**Bet 3. A checkpoint the run itself cannot resolve.** Primary evidence: B2 (I-HITL). The bet: the token a run, or a delegated coding agent, holds is refused by the checkpoint-resolve route, and the evaluation set holds a case where the agent is shown a path to approve itself. Proved wrong if no credential reachable from a run can call that route. First step, two hours: trace which credentials a delegated agent receives (Phase 40) and test them against the resolve route; kill if every one is refused.

### Still thin (part 5)

- (B) No pass^k or repeated-run reliability paper found; B3's results section was read only in summary; B2's prototype and theorems not read.
- (A) MCP open SEPs and ext-auth not read; no A2A implementer page; OTel span names not re-read at the new repository; IETF drafts read by title only.
- (C) OpenAI changelog, Google AI Studio changelog and Google ADK release notes not read this cycle (ADK now two cycles running).
- Not checked in code: whether Karakuri's MCP client uses Client ID Metadata Documents; whether a run's credentials can resolve a checkpoint.
<!-- p5:end -->

## Governance and buying criteria

<!-- p6a:start -->
Part 6 status: truncated, not reached: EU AI Act Art. 113 and Art. 26(6) text (EUR-Lex returned an empty 202 twice), OWASP risk descriptions beyond titles, NIST AI 600-1, ISO/IEC 42001, a published AI-vendor questionnaire

**(1) EU AI Act Art. 113, dates of application.** Observed: https://eur-lex.europa.eu/legal-content/EN/TXT/HTML/?uri=OJ:L_202401689 and https://eur-lex.europa.eu/eli/reg/2024/1689/oj (via the 307 from http://data.europa.eu/eli/reg/2024/1689/oj) both returned HTTP 202 with an empty body, 2026-11-15 [N]. Not quoted; second cycle unread. Karakuri evidence: not applicable until the text is read.

**(2) OWASP Top 10 for Agentic Applications 2026.** Observed: https://genai.owasp.org/download/52117/ returned the PDF (2026-11-15); the ten titles below are read from its table of contents, pages 2 to 4 [Q] for titles only, descriptions not read. Karakuri column: a file:line I opened this pass, or 'no'.

| ID | Title [Q] | Karakuri can evidence today |
|---|---|---|
| ASI01 | Agent Goal Hijack | Partly: internal/core/agent/decide.go:96-100, provenance is checked first and a plan drafted with third-party text in evidence escalates; internal/core/environment/environment.go:59 `TrustThirdParty` |
| ASI02 | Tool Misuse and Exploitation | Partly: internal/core/agent/agent.go:37-44 `MaxAutonomousActions`, `RequiresApprovalFor`; argument-level checks: no (see F8) |
| ASI03 | Identity and Privilege Abuse | Partly: agent.go:43 declares `CanDelegate`; whether the decision reads it: see F16 |
| ASI04 | Agentic Supply Chain Vulnerabilities | no (tool-set pinning, F15) |
| ASI05 | Unexpected Code Execution (RCE) | Partly: internal/platform/procenv/procenv.go:1-22 shapes the environment of spawned CLIs; a sandbox: no evidence opened |
| ASI06 | Memory & Context Poisoning | no (memory write path not opened) |
| ASI07 | Insecure Inter-Agent Communication | no |
| ASI08 | Cascading Failures | no (not opened) |
| ASI09 | Human-Agent Trust Exploitation | no (checkpoint code not opened; see F9) |
| ASI10 | Rogue Agents | no file opened; part 2 names Phase 37 (stop any run) as Completed |

**(3) Retention against Art. 26(6).** Observed: config/default.yaml:243-251, `audit.retention.floor_days: 183` ("The minimum the audit log is kept: six months") and `days: 0` (never prunes); internal/feature/audit/retention.go:16 `const FloorDays = 183` and :30-31 refuse a lower floor. config/default.yaml:231 keeps cost events 90 days, but those are cost records, not the run log. Inferred: the default audit log meets a six-month minimum with margin. Not verified: the Art. 26(6) wording itself (EUR-Lex unread, see (1)), and whether tool_events hold everything a deployer's "logs" means.

**(4) NIST AI 600-1, ISO/IEC 42001, procurement questionnaire.** Not reached.

**Still thin (part 6).** Art. 113 and Art. 26(6) primary text, two cycles running; OWASP descriptions and mitigations (only titles read); ASI04, ASI06 to ASI10 have no opened Karakuri evidence; no buyer questionnaire read.
<!-- p6a:end -->

## Feasibility

<!-- p6b:start -->
Part 6 status: in progress

| # | Candidate | Exists | Missing | First slice (files) | Not verified | Phase on main |
|---|---|---|---|---|---|---|
| F1 | Rewind/undo of an agent's changes (part 2 gap, high) | Not assessed (part 6) | | | | |
| F2 | Configured timeout bounds the whole call, retries included (part 2 gap) | Not assessed (part 6) | | | | |
| F3 | One recap of what ran unattended (part 2 gap) | Not assessed (part 6) | | | | |
| F4 | Re-validate authority at execution after a long wait (part 2 gap, partly Phase 46) | Not assessed (part 6) | | | | |
| F5 | Memory write guards in multi-agent crews (part 2 gap) | Not assessed (part 6) | | | | |
| F6 | MCP tool output encoding degrades non-English text (part 2 gap) | Not assessed (part 6) | | | | |
| F7 | Bet 1: provider cost data reconciles the spend counter (part 5) | Not assessed (part 6) | | | | |
| F8 | Bet 2: authority arguments come from the request, never from third-party text (part 5) | Not assessed (part 6) | | | | |
| F9 | Bet 3: a checkpoint the run itself cannot resolve (part 5) | Not assessed (part 6) | | | | |
| F10 | Carried: rewind/undo of an agent's changes | Not assessed (part 6) | | | | |
| F11 | Carried: read-access audit | Not assessed (part 6) | | | | |
| F12 | Carried: MCP elicitation as a checkpoint surface | Not assessed (part 6) | | | | |
| F13 | Carried: provider cost reconciliation | Not assessed (part 6) | | | | |
| F14 | Carried: blob provenance | Not assessed (part 6) | | | | |
| F15 | Carried: tool-set pinning | Not assessed (part 6) | | | | |
| F16 | Code check: `AuthorityBounds.Decide` and `CanDelegate` | Observed: `CanDelegate` declared at internal/core/agent/agent.go:43; `Decide` at internal/core/agent/decide.go:96 (opened to :100). Outside tests, the only other references are writes, internal/feature/reconcile/authority.go:32 and :59 (`bounds.CanDelegate = false`, grep hits, not opened) | Inferred: nothing reads `CanDelegate`; the gap of 2026-11-08 stands | The slice Phase 49 already names | decide.go past line 100 not re-read this pass; authority.go not opened | Phase 49 'A Bound That Says No Delegation Is Kept', **Planned** (docs/roadmap.md:62, :7085; Goal read: an agent with `CanDelegate` false never hands a task to a coding-agent CLI without approval). Already planned; Steps not read (time) |
| F17 | Code check: internal/platform/procenv/ | Observed: `Merge` (procenv.go:15-22) overlays per-call variables onto the process environment, an override replacing the inherited entry; `ScrubNestedSession` (:44-68) drops `CLAUDECODE` and every `CLAUDE_CODE_*` variable and, by its comment, leaves `ANTHROPIC_API_KEY` untouched | Inferred: the child inherits the whole server environment except session markers, so any other secret in the server's environment reaches a spawned `claude` | An allow-list in procenv.go (PATH, HOME, the config's named variables) with a test in procenv_test.go | Merge body (:23-40) not read; callers not opened; whether the server env holds other secrets in practice | none found (no phase title searched beyond Phase 49) |
| F18 | Code check: the software.verify.review runner | Not assessed (part 6) | | | | |
<!-- p6b:end -->

## Fit and bets

<!-- p7a:start -->
<!-- p7a:end -->

## Proposed phases

<!-- p8:start -->
<!-- p8:end -->

## Sources

### Part 1

<!-- s1:start -->
Part 1 status: complete

| URL | Read | Primary or secondary | Full text or extract | Grade |
|---|---|---|---|---|
| https://www.gov.uk/algorithmic-transparency-records | 2026-11-15 | Primary (index) | Extract | [E] |
| https://www.gov.uk/algorithmic-transparency-records/dwp-conversational-platform | 2026-11-15 | Primary | Extract | [E] |
| https://www.gov.uk/algorithmic-transparency-records/office-of-public-guardian-investigations-assistant | 2026-11-15 | Primary | Extract | [E] |
| Local: docs/research/discovery-2026-10-04.md to -11-08.md, docs/roadmap.md, `git log`, `gh pr list` | 2026-11-15 | Primary (repository) | Headings and grep | [E] |
<!-- s1:end -->

### Part 2

<!-- s2:start -->
Part 2 sources, all read 2026-11-15:
- https://api.github.com/repos/langchain-ai/langgraph/issues/5672 and /comments?per_page=20&page=2, &page=3. Primary; page 3 full text, page 2 partly extract. [Q]/[E]
- https://api.github.com/repos/crewAIInc/crewAI/issues/6043 and /comments?per_page=20&page=1, &page=2. Primary; page 2 full text, page 1 truncated to extract. [Q]/[E]
- https://api.github.com/repos/microsoft/autogen/issues/1700, /4079, /4006, /1700/comments, /4006/comments, /6995. Primary; extract except quoted lines. [E]/[Q]
- https://api.github.com/repos/microsoft/autogen/issues?state=open&sort=reactions-+1 (sort ignored, not used for ranking). [N]
- https://github.com/microsoft/autogen/issues?q=is%3Aissue+is%3Aopen+sort%3Areactions-%2B1-desc. Primary, first page, extract. [E]
- https://community.temporal.io/search.json (two queries), https://community.temporal.io/t/19715.json, https://community.temporal.io/t/19610.json. Primary; full text except a flagged hidden post. [Q]
- https://github.com/BerriAI/litellm/issues/49751. Primary, full text. [Q]
- https://github.com/anthropics/claude-code/issues/109737. Primary, full text. [Q]
- https://github.com/openai/codex/issues/60696. Primary; read, not a match. [E]
- GitHub issue search pages for openai/codex, anthropics/claude-code, BerriAI/litellm (used to find issues, cited for nothing). [N]
- https://api.github.com/repos/anthropics/claude-code/issues/52209 and /comments. Primary, full text. [Q]
- https://api.github.com/repos/sst/opencode/issues/31338 and /comments. Primary, full text. [Q]
- https://api.github.com/repos/google-gemini/gemini-cli/issues/10223 and /comments. Primary, full text. [Q]
- https://html.duckduckgo.com/html/ (two queries for the unverified figures). HTTP 202 CAPTCHA. [N]
<!-- s2:end -->

### Part 3

<!-- s3:start -->
All read 2026-11-15, through a tool that returns a model-written extract.

- https://github.com/openai/codex/releases.atom: primary; the first ~40,000 of ~425,000 characters were read, and the tail was summarised by the tool; [E].
- https://github.com/openai/openai-agents-python/releases.atom: primary; the first ~40,000 of ~335,000 characters were read, and the tail was summarised; [E].
- https://raw.githubusercontent.com/anthropics/claude-code/main/CHANGELOG.md: primary; the first ~40,000 of ~1.25M characters were read, and part of the rest was summarised; [E].
- https://github.com/anthropics/claude-agent-sdk-python/releases.atom: primary; full feed of 10 entries; [E].
- https://github.com/anthropics/claude-code/issues/27661: primary; full thread; [E].
- https://github.com/langchain-ai/langgraph/releases.atom: primary; full feed of 10 entries; [E].
- https://docs.langchain.com/langsmith/changelog: primary; the first ~40,000 of ~343,000 characters were read, and part of the tail was summarised; [E].
- https://developers.openai.com/codex/changelog, https://github.com/langchain-ai/langgraph/releases?page=6, and OpenAI's scheduled automations, automatic review and Decisions API pages: not tried this pass; [N].
- Karakuri files opened: internal/core/agent/agent.go:32-46, internal/core/environment/environment.go:55-59, AGENTS.md (rule 8).
<!-- s3:end -->

### Part 4

<!-- s4:start -->
All read 2026-11-15 through a fetch tool that returns an extract.
- https://langfuse.com/changelog: primary; HTTP 404; [N].
- https://github.com/langfuse/langfuse/blob/main/ee/LICENSE: primary; full text; [E], with phrases [Q].
- https://github.com/langfuse/langfuse/issues?q=audit+log: primary; page 1 of the search; [E].
- https://github.com/temporalio/temporal/releases: primary; page 1; [E].
- https://github.com/temporalio/temporal/releases/tag/v1.31.0: primary; full notes; [E].
- https://github.com/temporalio/temporal/issues/3006: primary; full thread; [E].
- https://restate.dev/blog: primary (vendor blog); post list only; [E], tagline [Q].
- https://rootly.com/changelog: primary; page 1, the whole window; [E].
- https://docs.pagerduty.com/get-started/release-notes (via a 301 from https://support.pagerduty.com/main/changelog): primary; the window; [E].
- https://www.pagerduty.com/release-notes/: HTTP 404; [N].
- Karakuri files opened: internal/feature/loop/stop.go:75-82, internal/feature/reconcile/stoploop.go:39,80, internal/feature/reconcile/authority.go:21,78,95, internal/platform/tools/mcp/client.go:66-72,95, internal/api/handler/mcp.go:484, docs/roadmap.md:4031, 4510-4520, 5719.
<!-- s4:end -->

### Part 5

<!-- s5:start -->
All read 2026-11-15 through a fetch tool (about 40k characters verbatim per page, rest summarised and unused).

- https://arxiv.org/html/2611.17115 — primary, full text to Related Work; [Q]; Abstract, 1, 2, 3 (3.1, 3.2), 4, 5 (5.1 to 5.5).
- https://arxiv.org/html/2610.35835 — primary, partial (v3); [Q]; Abstract, I, II (A to C), III, start of IV.
- https://arxiv.org/html/2611.15700v1 — primary, partial; [Q] abstract, [E] method; Abstract, I, II-A to II-C, III, III-A, III-B1 to III-B3.
- https://arxiv.org/search/?query=prompt+injection+defense+agents&searchtype=all&order=-announced_date_first — search, used only to find B1; not cited.
- https://export.arxiv.org/api/query (two queries) — search, used only to find B2 and B3; not cited.
- https://modelcontextprotocol.io/specification/draft/changelog — primary, full; [Q].
- https://github.com/modelcontextprotocol/modelcontextprotocol/releases — primary, extract; [Q] tag and line.
- https://github.com/a2aproject/A2A/releases — primary, extract; [Q].
- https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-agent-spans/ — primary, move notice only; [Q].
- https://github.com/open-telemetry/semantic-conventions-genai — [N] not fetched (time).
- https://datatracker.ietf.org/doc/search?name=agent&sort=&rfcs=on&activedrafts=on — primary listing, partial; [Q] titles and dates only.
- https://platform.claude.com/docs/en/release-notes/api — primary, first 39,934 characters; [Q].
- https://platform.claude.com/docs/en/build-with-claude/message-threads — primary, partial; [Q] one line, [E] mechanism.
- https://platform.claude.com/docs/en/manage-claude/usage-cost-analytics-api — primary, partial; [Q] dimensions and limit.
- https://platform.claude.com/docs/en/build-with-claude/usage-cost-api — primary (legacy API), extract; [E].
- https://platform.openai.com/docs/changelog, https://aistudio.google.com/docs/changelog, Google ADK release notes, the MCP SEP list — [N] not attempted (time).
<!-- s5:end -->

### Part 6

<!-- s6:start -->
- https://eur-lex.europa.eu/legal-content/EN/TXT/HTML/?uri=OJ:L_202401689 — 2026-11-15, primary, HTTP 202 empty body [N].
- http://data.europa.eu/eli/reg/2024/1689/oj → https://eur-lex.europa.eu/eli/reg/2024/1689/oj — 2026-11-15, primary, HTTP 202 empty body [N].
- https://genai.owasp.org/download/52117/ — 2026-11-15, primary, PDF; table of contents read (titles [Q]), body not read.
- https://genai.owasp.org/resource/owasp-top-10-for-agentic-applications-for-2026/ and https://genai.owasp.org/initiatives/agentic-security-initiative/ — 2026-11-15, primary, landing pages without the list; cited for nothing [N].
<!-- s6:end -->

### Part 7

<!-- s7:start -->
<!-- s7:end -->

### Part 8

<!-- s8:start -->
<!-- s8:end -->
