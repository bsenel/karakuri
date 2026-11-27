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

not started
<!-- p2:end -->

<!-- p3:start -->
## Competitive teardown, products 1 to 3

not started
<!-- p3:end -->

<!-- p4:start -->
## Competitive teardown, products 4 onward, and the capability matrix

not started
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
