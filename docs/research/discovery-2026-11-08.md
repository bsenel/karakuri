# Discovery 2026-11-08: weekly cycle

Method. The cycle has eight parts and each writes only between its own markers. Sources are graded [Q] for verbatim words returned by the fetch, [E] for an extract or paraphrase (never quoted) and [N] for not read (cited for nothing). 'Observed:' and 'Inferred:' are kept apart. Nothing was built, run or tested. A vendor's or analyst's figure is a claim, not a measurement.

Baseline: docs/research/discovery-2026-11-01.md (which builds on 2026-10-25, 2026-10-08 and 2026-10-04)

## Who the users are and what they are trying to do

<!-- p1:start -->
Written by part 1 on 2026-11-08. The segments of 2026-10-04 to 2026-11-01 stand and are not restated. Every page came through a fetch tool that returns a model-written extract; quotation marks hold only words the extract marked as verbatim [Q]. Nobody was interviewed. Session limit 600 seconds.

### (e) GitHub Next's standing agentic workflows: schedules, failures, review

New: yes (the status page and the Fault Investigation post were not read in any earlier report; 2026-11-01 named this thin spot).

- **Agent Factory status page, GitHub Next** (https://github.github.com/gh-aw/agent-factory-status, reached from the github.io address, read 2026-11-08; the first ~40k of ~138k characters verbatim, the rest a model summary). Who speaks: the GitHub Next team about its own workflows. Observed [Q]: "These are experimental agentic workflows used by the GitHub Next team to learn, build, and use agentic workflows." Observed [Q], schedules in the verbatim part: Auto-Triage Issues `11 */6 * * *`; Avenger `23 * * * *`; Bot Detection `every 6h`; Daily Fact `daily around 14:00 on weekdays`; a workflow named "[aw] Failure Investigator (6h)" whose schedule cell reads `every 30m` (name and schedule disagree on the page itself). Observed [E], from the summarised part only: about 17 more scheduled workflows (Dependabot checker Mon/Wed/Fri, Duplicate Code Detector daily, a weekday Repository Quality Improvement Agent, a Monday Weekly Issue Summary), slash-command triggers (`/q`, `/scout`, `/smoke` and others), and roughly 200 entries with no schedule or trigger shown.
- What failed: Observed: the status cells are badge images with no readable text, so pass or fail could not be read. Not known.
- Who reviews or merges: Observed: no sentence on the page says.
- **"Meet the Workflows: Fault Investigation", GitHub Next, 13 January 2026** (https://github.github.com/gh-aw/blog/2026-01-13-meet-the-workflows-quality-hygiene/, read 2026-11-08, full page through an extract). Job: diagnose CI failures and schema or API breakage on the team's own repository. Observed [Q]: CI Doctor "Investigates failed workflows and opens diagnostic issues"; "9 merged PRs out of 13 proposed (69% merge rate)"; the Schema Consistency Checker created "55 analysis discussions"; "We learned that agents excel at the tedious investigation work that humans find draining." Observed: no schedule and no review or merge rule is stated on the page. The merge rate is the team's own count, not a measurement made here.
- What the agent may do unsupervised: Observed [E]: open issues, discussions and pull requests; merging is not described, and a merge rate below 100% implies a decision by someone other than the proposer, but the page does not say who.
- What went wrong: Observed: 4 of 13 CI Doctor pull requests were not merged; why is not stated.
- Inferred for Karakuri: the team publishes a *rate of accepted proposals per workflow* as its own success signal, the same shape as Phase 41's escalation approval rate (#211). The schedule mix (every 30 minutes to weekly, hundreds of on-demand entries) says one deployment runs standing and on-demand agents side by side, which Karakuri's reconcile cadence plus `krk` runs already cover. The unreadable badges and the name/schedule mismatch are a small case of a status surface nobody reads closely, which supports 2026-11-01's candidate need for one digest across many standing objectives.

### (f) A regulated organisation speaking first-hand

New: yes (a UK public body's own ATRS record, not cited before). It is a chatbot, not an agent, so the thin spot is only partly filled.

- **HMRC, "Ask Security", Algorithmic Transparency Recording Standard record** (https://www.gov.uk/algorithmic-transparency-records/hmrc-ask-security, published 9 September 2026, Private Beta, read 2026-11-08 through an extract; reached from https://www.gov.uk/government/collections/algorithmic-transparency-recording-standard-hub and https://www.gov.uk/algorithmic-transparency-records). Who speaks: HMRC, in the government's template; it names IBM, Accenture and Equal Experts as third parties. Job: let staff ask questions of HMRC security policy. Observed [Q]: "A generative AI chatbot for internal use to allow HMRC colleagues access to HMRC Security Policies, Standards and Guidance."
- What it may do unsupervised: Observed [Q]: it "is a processor not an initiator, so the blast radius is contained to GAILZ." Output is text only; the record plans "deeper integration (e.g., ServiceNow workflow automation)."
- Human review: Observed [Q]: "The colleague reviews the response and is invited to view the relevant policy or guidance documents." Observed [E]: a pop-up tells users not to enter personal data; prompt-injection protection, content filtering and a PII detection step are listed; traces are kept 30 days.
- What went wrong or is admitted: Observed [Q]: "users may act on advice provided by Ask Security without independently validating sources"; prompt injection is mitigated "without absolute guarantees"; "Formal statistical metrics (e.g. precision/recall) are less applicable to this use case." Observed [E]: the model is named inconsistently in two fields, several development-data fields are blank, appeals is "N/A"; weekly query counts since launch are given in the hundreds to low thousands.
- Inferred for Karakuri: the first first-hand regulated record in six cycles describes the boundary a regulator-facing team draws: *processor, not initiator*, and the step it plans next is exactly the one that crosses it (workflow automation). A public body moving from answer to action would need what Karakuri's `agent.AuthorityBounds`, checkpoints and evidence export record; the record's blank and inconsistent fields suggest the evidence pack should fill model identity and evaluation fields from the run itself, not by hand. This is one record, a chatbot, and not an agent deployment.
- Not read: https://www.lloydsbankinggroup.com/insights/ returned an error page ("Error 1007") [N]. The repository lists records closer to an agent (DWP Conversational Platform, Office of the Public Guardian Investigations Assistant); not opened.

### (a) A postmortem where an AI agent acted during an incident

New: no. Not filled.

- https://replit.com/blog/introducing-a-safer-way-to-vibe-code-with-replit-databases (Replit Team, 21 July 2025, read 2026-11-08 through an extract) is a product announcement, not a postmortem: it separates development and production databases. Observed [Q]: "Until now, Replit apps used a single database for both development and live customer data." It does not describe the incident that preceded it, so it is cited only for that sentence.
- https://www.replit.com/blog (index page 1 only) did not hold a July 2025 postmortem; https://github.com/google-gemini/gemini-cli/issues timed out twice [N].
- Inferred: a fix that separates the environment the agent writes to from the one customers use is the vendor's answer; it is weak evidence because the failure itself is not described first-hand here.

### Segment table

| Segment | Added this cycle | Still thin? |
|---|---|---|
| Teams running standing agents | GitHub Next status page and Fault Investigation post [Q]: schedules from 30 min to weekly; 9 of 13 CI Doctor PRs merged | Less thin; who merges is still unstated |
| Regulated organisations | HMRC Ask Security ATRS record [Q]: text-only "processor not an initiator", automation planned | Yes: a chatbot, not an agent; no bank |
| Platform and SRE teams | Nothing: no agent-in-incident postmortem found | Yes |
| Self-hosters | Not attempted | Yes |
| Product teams, enterprises | Not attempted | Yes |

### Still thin (part 1)

- (f) Still no agent in a regulated organisation. Next: the ATRS records for DWP Conversational Platform and the Office of the Public Guardian Investigations Assistant (https://www.gov.uk/algorithmic-transparency-records/ plus slug); a Barclays or Lloyds talk recording, not their insights index.
- (e) Who merges agent pull requests in gh-aw: try the gh-aw repository's closed PRs by its bots, or the "Operations & Release" Meet the Workflows post.
- (a) No postmortem: the Replit July 2025 post by its CEO was not located; try a direct search of replit.com/blog page 2 onward, or a Gemini CLI issue by number.
- (c) second self-hoster and (h) product or enterprise teams: not reached.
<!-- p1:end -->

## What they ask for and complain about

<!-- p2:start -->
<!-- p2:end -->

## Competitive teardown

<!-- p3:start -->
<!-- p3:end -->

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

## Roadmap phases proposed

<!-- p8:start -->
<!-- p8:end -->

## What changed since the last report

<!-- p1w:start -->
### How earlier proposals fared

New: no (repository state only, no web).

Observed from `git grep '^## Phase' origin/main -- docs/roadmap.md` and `gh pr list --label karakuri:discovery` on 2026-11-08:

| Phase proposed by discovery | Proposed in | Status on main |
|---|---|---|
| 33 MCP After the Handshake | 2026-10-04 (#148) | Completed |
| 35 An Unanswered Checkpoint Ends | 2026-10-08 (#158) | Completed |
| 36 A Provider's Refusal Is Not a Plan | 2026-10-08 (#158) | Completed |
| 37 Any Run Can Be Stopped at Any Time | 2026-10-08 (#158) | Planned |
| 38 What Was Required and Not Done | 2026-10-08 (#158) | Planned |
| 42 An MCP Call Carries Only What the Tool Declared | 2026-10-25 (#195) | Planned |
| 43 A Provider's Tool-Call Shape, Pinned by a Test | 2026-10-25 (#195) | Planned |
| 45 The Repository's Own Checks Run Before a Delegated Push | 2026-11-01 (#203) | Planned |
| 46 An Approval After Third-Party Text Buys One Action | 2026-11-01 (#203) | Planned |

- Declines: none. All four discovery pull requests (#148, #158, #195, #203) were merged; none was closed unmerged.
- The alpha path: Phase 47 (First Alpha Release, Planned) gates on Phases 37 to 41 plus 47 itself; of these, 41 is Completed and 37, 38, 39, 40 are Planned. So discovery's 37 and 38 are on the alpha path; 42, 43, 45 and 46 are not, and queue behind it alongside 44 and 48. A new phase proposed this cycle joins after 48.
<!-- p1w:end -->

<!-- p7b:start -->
<!-- p7b:end -->

## Sources

### Sources, part 1

<!-- s1:start -->
All read 2026-11-08 through a fetch tool that returns a model-written extract.

- https://github.github.com/gh-aw/agent-factory-status (via https://github.github.io/gh-aw/agent-factory-status) — primary; first ~40k characters verbatim, rest summarised; [Q] and [E].
- https://github.github.com/gh-aw/blog/2026-01-12-welcome-to-pelis-agent-factory/ — primary; full extract; used only for the series' links (already cited 2026-11-01).
- https://github.github.com/gh-aw/blog/2026-01-13-meet-the-workflows-quality-hygiene/ — primary; full extract; [Q].
- https://www.gov.uk/algorithmic-transparency-records/hmrc-ask-security — primary; full extract; [Q].
- https://www.gov.uk/government/collections/algorithmic-transparency-recording-standard-hub and https://www.gov.uk/algorithmic-transparency-records — primary index pages; used for navigation only.
- https://www.lloydsbankinggroup.com/insights/ — error page; [N], cited for nothing.
- https://replit.com/blog/introducing-a-safer-way-to-vibe-code-with-replit-databases — primary (vendor); extract; [Q] for one sentence.
- https://www.replit.com/blog — index page 1; navigation only.
- https://github.com/google-gemini/gemini-cli/issues — timed out; [N], cited for nothing.
<!-- s1:end -->

### Sources, part 2

<!-- s2:start -->
<!-- s2:end -->

### Sources, part 3

<!-- s3:start -->
<!-- s3:end -->

### Sources, part 4

<!-- s4:start -->
<!-- s4:end -->

### Sources, part 5

<!-- s5:start -->
<!-- s5:end -->

### Sources, part 6

<!-- s6:start -->
<!-- s6:end -->

### Sources, part 7

<!-- s7:start -->
<!-- s7:end -->

### Sources, part 8

<!-- s8:start -->
<!-- s8:end -->
