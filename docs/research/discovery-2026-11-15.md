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
Part 2: started; status line written in p2, nothing else yet.
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
