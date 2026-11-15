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
Part 2: items 1 to 4 written in p2 (langgraph #5672, crewAI #6043, autogen, Temporal forum) with ranking table; items 5 to 7 not yet reached.
<!-- w2:end -->

### Part 3

<!-- w3:start -->
<!-- w3:end -->

### Part 4

<!-- w4:start -->
<!-- w4:end -->

### Part 5

<!-- w5:start -->
<!-- w5:end -->

### Part 6

<!-- w6:start -->
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
Part 2 status: in progress

Read before writing: p1w above. The 2026-11-08 and 2026-11-01 sections of this heading are not restated; the items below only add to them. All reads 2026-11-15 through a fetch agent; a quote marked [Q] is the fetch agent's verbatim copy, not re-checked by hand.

**Item 1. langgraph #5672, comments 21 to 52.** New: yes (the unread pages). Observed: the issue is open, 52 comments, 9 reactions (all +1), labels bug, pending, external. Pages 2 and 3 hold no comment with association MEMBER, COLLABORATOR or OWNER, so there is still no maintainer decision; commenters report a bot auto-closed a contributor PR (#8634), which is a bot action, not a decision. [Q] 1320800521 (NONE, 2026-09-01): "durability=sync changes write timing for completed graph-state changes; it does not turn unfinished node-local streamed progress into a state update." [Q] safal207 (NONE, 2026-08-15): "streamed visibility, durable graph state, and safe resume are different facts". Page 3 (12 comments) was read verbatim; the tail of page 2 came back partly as an extract [E]. https://api.github.com/repos/langchain-ai/langgraph/issues/5672/comments?per_page=20&page=2 and &page=3. Inferred: users separate "what was streamed" from "what is durable"; the documented `durability` option does not close the gap they describe.

**Item 2. crewAI #6043, comments 13 to 31.** New: yes. Observed: open, 31 comments, 2 reactions on the body, every comment 0 reactions, label feature-request; no comment by a MEMBER, COLLABORATOR or OWNER anywhere in the thread, so no maintainer decision. Comments 13 to 20 came back as an extract [E] and are mostly links, promotion of outside projects or off-topic posts; 21 to 31 were read verbatim. [Q] tonydzi (NONE, 2026-09-03): "the failure a write guard can't see is revocation that doesn't propagate." [Q] 1picassoai (NONE, 2026-09-06): "An ingest gatekeeper validates writes BEFORE they become memory". https://api.github.com/repos/crewAIInc/crewAI/issues/6043/comments?per_page=20&page=2. Inferred: the thread's volume is inflated by promotion; strength is low (2 reactions) and should not be counted as demand.

**Item 3. microsoft/autogen #1700, #4079, #4006.** New: yes, but none is about agent behaviour. Observed [E]: #1700 is a Go/Rust port request (4 comments, 34 reactions), #4079 a `ModuleNotFoundError` install error (27 comments, 6 reactions), #4006 the AutoGen Studio roadmap (0 comments, 5 reactions). #1700 has no MEMBER/COLLABORATOR/OWNER comment; [Q] ekzhu (CONTRIBUTOR, 2024-02-17): "You are welcome to start a new language implementation." The most-reacted open issue about agent behaviour in the github.com reaction-sorted list is #6995, MCP tool JSON serialised without `ensure_ascii=False` (5 reactions, 0 comments, 2025-09-06), [Q] "JSON serialization does not set `ensure_ascii=False`." https://github.com/microsoft/autogen/issues/6995. The API `sort=reactions-+1` parameter was ignored [N for ranking]. Inferred: autogen's open tracker carries little agent-behaviour demand by reactions; it is a weak source for this question.

**Item 4. Temporal community forum.** New: yes (never read). Observed: the two 2026 AI-agent threads found by Discourse search both have zero staff replies. (a) "Pattern Sharing: Durable Multi-Agent AI Workflows with HITL via CrewAI", 2026-07-08, 1097 views, 1 reply; a pattern share, not a complaint: [Q] "fault-tolerant, long-running multi-agent AI orchestrations with Temporal and CrewAI, including explicit Human-in-the-Loop (HITL) approval gates." https://community.temporal.io/t/19715. (b) "Dealing with Authorization/Execution Drift (TOCTOU) in long-running workflows", 2026-06-05, 439 views, 3 replies (one hidden by a flag, not read): [Q] "how do you recommend ensuring that the execution step re-validates the initial authorization context?" https://community.temporal.io/t/19610. Inferred: (b) is a new requirement shape, re-checking authority at the moment of action after a long human wait, with one source so far.

**Ranking (part 2), updated after item 4.** Counts as the source shows them; "phase" is matched by phase title and Goal only.

| Requirement | Independent sources | Counts | Strength | Phase on main |
|---|---|---|---|---|
| Durable state across a cancelled run (streamed work not lost) | langgraph #5672 (1 org this pass) | 52 comments, 9 reactions, no maintainer reply | medium | Phase 37 (Completed, stop any run); Phase 44 step 2 salvage branch covers coding edits only |
| Re-validate authority at execution after a long wait | Temporal forum t/19610 (1) | 439 views, 3 replies | low | partly Phase 46 (approval buys one action); else gap |
| Memory write guards in multi-agent crews | crewAI #6043 (1) | 31 comments, 2 reactions | low | gap |
| MCP tool output encoding degrades non-English text | autogen #6995 (1) | 5 reactions, 0 comments | low | gap (Phase 42 is about call arguments, not encoding) |

**Still thin (part 2).**
- Temporal forum post 4 of t/19610 hidden; next: https://community.temporal.io/t/19610 read by a signed-in human.
<!-- p2:end -->

## Competitive teardown

### Products 1 to 3 (part 3)

<!-- p3:start -->
<!-- p3:end -->

### Products 4 to 6 (part 4)

<!-- p4a:start -->
<!-- p4a:end -->

## Capability matrix

<!-- p4b:start -->
<!-- p4b:end -->

## Technical frontier

<!-- p5:start -->
<!-- p5:end -->

## Governance and buying criteria

<!-- p6a:start -->
<!-- p6a:end -->

## Feasibility

<!-- p6b:start -->
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
<!-- s2:end -->

### Part 3

<!-- s3:start -->
<!-- s3:end -->

### Part 4

<!-- s4:start -->
<!-- s4:end -->

### Part 5

<!-- s5:start -->
<!-- s5:end -->

### Part 6

<!-- s6:start -->
<!-- s6:end -->

### Part 7

<!-- s7:start -->
<!-- s7:end -->

### Part 8

<!-- s8:start -->
<!-- s8:end -->
