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
Written by part 2 on 2026-11-08. The rankings of 2026-10-25 and 2026-11-01 stand and are not restated. Direct `gh api` calls were refused in this session, so every GitHub API page came through a fetch tool that returns a model-written extract; quotation marks hold only words the extract marked as verbatim [Q], and counts are copied from the API objects as the extract reported them. Long comment pages were read verbatim only for their first ~40k characters. Session limit 600 seconds.

### (1) Second sources for the one-tracker candidates

New: yes (gemini-cli #10223 and litellm #25931 appear in no earlier report).

- **Rewind or undo of an agent's changes.** Observed: google-gemini/gemini-cli #10223, "Feature Request: Add Checkpointing/Rewind Capability" (lingzhong, NONE, 2025-09-30); 11 reactions (+1 10, -1 1), 3 comments; closed as completed 2026-01-22 by Adib234 (CONTRIBUTOR, not MEMBER or COLLABORATOR). Body [Q]: "Add an automatic checkpointing system with rewind functionality", whose stated goal is to "safely undo AI-generated code changes". Decision [Q]: "we have `/rewind` feature now on main branch". A later user (Morriz, 2026-01-28) [E] says the documented feature was not yet available. Inferred: with claude-code #52209 and opencode #31338 (2026-11-01), this is a third organisation (Google) asking for the same thing, and one that shipped it. The candidate passes the two-organisation gate.
- **A configured timeout honoured for every provider.** Observed: the reaction-sorted search of BerriAI/litellm for "timeout ignored" and of crewAIInc/crewAI for "timeout" returned no issue whose title is about a timeout. The nearest shape is litellm #25931 (7 reactions, all heart; 7 comments; closed 2026-06-29), a configured setting silently skipped for one provider path [Q]: "This code path never consults `drop_params` or `additional_drop_params`." Fixed in #26148 by a CONTRIBUTOR. Inferred: a second organisation shows the *class* (a setting honoured on one provider path, ignored on another), not the timeout itself. Timeout stays one tracker.
- **Durable agent state that survives the database; one digest across standing objectives; "nobody can read all these outputs".** Observed: the github/gh-aw search for "summary digest" returned only unrelated provider and MCP bugs (top: #54052, 2 reactions). No second tracker was found for either candidate this pass. Not searched: a complaint phrased as output overload.

### (2) Trackers never read: crewAI, AutoGen, Temporal forum

New: yes (crewAI #6043 and autogen #1700 appear in no earlier report).

- **crewAIInc/crewAI**: the most-reacted open issue is #6043, "[FEATURE] Memory write guards for multi-agent crews — prevent cross-agent memory poisoning" (2026-06-04), 2 reactions (+1), 31 comments per the issue object. Body [Q]: "Multi-agent crews amplify the risk because one poisoned memory entry propagates to ALL agents." Replies [Q]: "Memory provenance tracking is the right approach." (vgudur-dev); "the write-side guard is necessary but in a multi-agent crew you also need read-side quarantine." (norika1207-lab). Observed: 12 comments read verbatim, all author_association NONE; no maintainer decision seen. The rest of the page was not read verbatim.
- **microsoft/autogen**: the three most-reacted open issues are not agent behaviour: #1700 "Golang/Rust implementation" (32 reactions), #4079 an import error (6), #4006 the Studio roadmap (5). Their comments were not read. Observed: autogen's loudest open demand is low (32 reactions at the top) and is not behavioural.
- **Temporal community forum**: not read this pass [N].
- Inferred: crewAI's top open issue is a provenance and trust complaint (text from one agent must not be trusted by another without a label). That is the same ask as AGENTS.md rule 9 and ADR 021, from a user community, not a vendor. Its demand by votes is weak (2 reactions), its discussion is long (31 comments).

### (3) Tails of known threads

New: no (all four threads were cited before; this adds counts dated 2026-11-08).

- anthropics/claude-code #16157, "[BUG] Instantly hitting usage limits with Max subscription": open, 1,499 comments, 728 reactions (+1 697, rocket 17, eyes 8, confused 3, -1 2, hooray 1). Pinned reply by ThariqS (CONTRIBUTOR) [Q]: "we have not at all changed our rate limits since Opus 4.5 launched".
- langchain-ai/langgraph #5672, "Run Cancellation Causes Loss of Streamed State Not Yet Persisted as a Checkpoint": open, 52 comments, 9 reactions (+1). Comments 32 to the end were **not read** (the page was cut at ~40k of 191k characters). The one MEMBER reply seen, christian-bromann, 2026-04-15 [Q]: "we are actively looking into this as we are rethinking some of our streaming primitives." No decision.
- langgraph #3716 (checkpoint-postgres SSL "bad length"): open, 53 comments, 12 reactions (+1). Tail read only as an extract [E]: one user reports zero occurrences in six weeks after two fixes (2026-06-08); later comments are offers of help and services; no maintainer decision.
- n8n-io/n8n #21716 (MCP `toolCallId` breaks old workflows): closed as completed 2026-01-14, 77 comments, 0 reactions on the issue. dlavrenuek (CONTRIBUTOR, 2026-01-13) [Q]: "Even tho the validation is not correct in this case we still want to make the mcp tool calls work," Observed: no MEMBER or COLLABORATOR reply in the readable part.

### (4) Postmortems and unverified figures

New: no. Not reached in the time limit: a team's own postmortem of an agent that did damage, the "$4,200 in 63 Hours" author post, and the primary report behind "78% / under 15%", which stays **unverified**.

### Ranking (part 2)

| Pain point | Independent sources (old and new) | Reactions / duplicates seen | Strength | Phase that covers it |
|---|---|---|---|---|
| Usage limits hit without explanation (claude-code #16157) | 1 tracker (Anthropic) | 728 reactions, 1,499 comments | Strong in one tracker; vendor quota, not an agent behaviour | Partly Phase 31/36 cost caps and rate-limit handling; Karakuri cannot fix a vendor's quota. Fails the gate |
| Rewind or undo an agent's changes | 3 organisations: claude-code #52209, opencode #31338 (2026-11-01), gemini-cli #10223 (new) | #10223: 11 reactions, shipped as `/rewind` | Moderate; **passes** | gap: no Planned phase (37, 38, 39, 40, 42 to 48) names undo; Phase 37 stops a run, it does not reverse it |
| Text from one agent or tool trusted by another without provenance | crewAI #6043 (new); Karakuri's own ADR 021 is not a user source | 2 reactions, 31 comments | Weak by votes, one tracker | Phase 46 (an approval after third-party text buys one action) and rule 9; fails the gate |
| A configured setting honoured on every provider path (timeout is one case) | n8n #25360 (2026-11-01), litellm #25931 (new, not a timeout) | 8 and 7 reactions | Moderate for the class; timeout itself still one tracker | Phase 43 (a provider's tool-call shape pinned by a test) is the nearest; timeouts: gap |
| State lost on cancel or database fault (langgraph #5672, #3716) | 1 organisation | 9 + 12 reactions, 52 + 53 comments | Moderate, one tracker | Phase 37 (stop at any time) for cancellation; durable state: gap among Planned phases |
| MCP client sends only declared arguments (n8n #21716) | 1 | 0 on #21716, 77 comments; closed | Strong in one tracker, now fixed upstream | Phase 42 |
| One digest across standing objectives | 1 (2026-11-01) | none | Weak | Phase 40 (streams without a coordinator) only partly; digest itself was an earlier phase; fails the gate |

Inferred: only rewind/undo moves across the gate this pass, and it is a gap in the Planned phases.

### Still thin (part 2)

1. Temporal community forum not read; autogen comments not read; crewAI #6043 read only for 12 of 31 comments.
2. langgraph #5672 comments 32 to the end still not read; read the comments page with `page=2` or a smaller `per_page` so it fits.
3. Timeout honoured for every provider, durable state, one digest, and "nobody can read all these outputs": still one tracker or none.
4. No agent postmortem; "$4,200 in 63 Hours" author post not found; "78% / under 15%" still unverified.
5. All GitHub reads this pass were extracts; a part with `gh api` allowed should re-read counts verbatim.
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
All read 2026-11-08 through a fetch tool that returns a model-written extract; [Q] only where the extract marked words verbatim.

- https://api.github.com/search/issues?q=repo:crewAIInc/crewAI+is:issue+is:open&sort=reactions&order=desc&per_page=3, primary, extract
- https://api.github.com/repos/crewAIInc/crewAI/issues/6043/comments?per_page=100, primary, first ~40k characters verbatim, rest extract [Q]/[E]
- https://api.github.com/search/issues?q=repo:microsoft/autogen+is:issue+is:open&sort=reactions&order=desc&per_page=3, primary, extract (autogen #1700, #4079, #4006 not opened)
- https://api.github.com/search/issues?q=repo:google-gemini/gemini-cli+is:issue+undo+changes&sort=reactions&order=desc&per_page=3, primary, extract
- https://api.github.com/repos/google-gemini/gemini-cli/issues/10223 and /comments?per_page=100, primary, full text [Q]
- https://api.github.com/search/issues?q=repo:BerriAI/litellm+is:issue+timeout+ignored&sort=reactions&order=desc&per_page=3, primary, extract
- https://api.github.com/repos/BerriAI/litellm/issues/25931 and /comments?per_page=100, primary, full text [Q]
- https://api.github.com/search/issues?q=repo:crewAIInc/crewAI+is:issue+timeout&sort=reactions&order=desc&per_page=3, primary, extract
- https://api.github.com/search/issues?q=repo:github/gh-aw+is:issue+summary+digest&sort=reactions&order=desc&per_page=3, primary, extract
- https://api.github.com/repos/anthropics/claude-code/issues/16157, primary, full issue object [Q]
- https://api.github.com/repos/langchain-ai/langgraph/issues/5672 and /comments?per_page=100, primary, first ~40k of 191k characters verbatim; comments 32 onward not read
- https://api.github.com/repos/langchain-ai/langgraph/issues/3716 and /comments?per_page=100, primary, first ~40k characters verbatim, tail extract [E]
- https://api.github.com/repos/n8n-io/n8n/issues/21716 and /comments?per_page=100, primary, first ~40k characters verbatim, last ~19k not read
- https://community.temporal.io, not read [N]
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
