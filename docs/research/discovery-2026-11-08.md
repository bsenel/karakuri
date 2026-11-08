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
Part 3 extends the p3 sections of discovery-2026-11-01.md and -2026-10-25.md; read those first. This pass is mostly a record of what could not be read: every github.com releases page and every api.github.com request timed out or returned 503/504 on 2026-11-08, and `gh api` was not available to this headless run. Only one page came through. It was fetched with a tool that returns a model-written extract, so the items below are [E] and nothing is quoted. Vendor notes are claims, not measurements.

### 1. OpenAI Codex and the OpenAI Agents SDK

New: no. Nothing was read. https://github.com/openai/codex/releases?page=8, `?page=15` and https://github.com/openai/openai-agents-python/releases timed out twice each. The most-reacted-issue search on api.github.com returned 504. All of these are [N]. Sections (i) to (v) carry nothing new: see 2026-11-01.

### 2. Claude Code and the Claude Agent SDK

New: no. Nothing was read. https://github.com/anthropics/claude-code/releases?page=10, `?page=25` and https://github.com/anthropics/claude-agent-sdk-python/releases timed out twice each. https://api.github.com/repos/anthropics/claude-code/issues/27661 returned 503, and its `/comments` returned 504. All of these are [N]. Sections (i) to (v) carry nothing new.

### 3. LangGraph and LangSmith

New: yes. It fills part of the 2026-11-01 thin spot "LangSmith from April to August" from the other end: September to early November is now covered, and April to August is still not.

**(i) Shipped, part of the window read.** Observed [E] (https://docs.langchain.com/langsmith/changelog, reached by redirect from docs.smith.langchain.com/changelog; about 307,000 characters; the first ~40,000 were extracted and the next part was summarised by the tool; read 2026-11-08). The tool presented the first block as verbatim, but it came through a model extract, so it is graded [E]. All of these are vendor claims.
- Permissions: 2026-10-26 to 11-02, granular workspace administration permissions, delegated by action instead of `workspaces:manage`. 2026-10-05 to 10-12, finer organisation permissions. 2026-10-12 to 10-19, tracing-only API keys. 2026-09-28 to 10-05, a change to public share link permissions.
- Durable state and scheduling: 2026-10-26 to 11-02, a priority callback sets each thread's priority from -5 to 5 at its first run.
- MCP: 2026-10-26 to 11-02, human-in-the-loop prompts over MCP, so a client on MCP revision 2026-07-28 is asked for input when a graph pauses. 2026-09-28 to 10-05, an MCP `query_runs` tool.
- Telemetry: 2026-09-21 to 09-28, trajectory view for all users, guided tracing setup for coding tools, and Gemini cost reporting. 2026-09-28 to 10-05, cost coverage for more OpenAI and Vertex models. 2026-10-05 to 10-12, AI search over runs, traces and threads.
- Evaluation: 2026-10-19 to 10-26, `reference_outputs` on examples.
- Other: 2026-10-12 to 10-19, ScaleDown in the LLM Gateway and API-based GCP BYOC data planes. 2026-10-19 to 10-26, org-wide limits on creating bulk exports.
- LangGraph OSS: https://github.com/langchain-ai/langgraph/releases?page=6 and `?page=10` timed out twice each [N]. 2026-04-08 to 2026-06-29 is still unread, now for the fourth cycle.

**(ii) What it does that Karakuri does not.** Inferred from (i): when a run pauses for a person, the question reaches any MCP client, not only the vendor's UI. Thread priority is set per run. API keys can be scoped to tracing only.

**(iii) What Karakuri does that it does not.** Not checked against code this pass. No path:line was opened, so no claim is made.

**(iv) Complaints.** Not read. The api.github.com search returned 503 [N].

**(v) Inferred:** asking for human input over MCP is the vendor's answer to "where does a checkpoint reach a person". Karakuri's checkpoints (Phase 13.5, Phase 35) reach people through its own API and UI. That makes MCP elicitation a candidate surface for whoever proposes phases (part 8). This rests on one vendor claim; nothing was measured.

### Still thin (part 3)

1. Codex: 2026-05-01 to 2026-10-28 is still unread. Start from https://github.com/openai/codex/releases?page=8. If github.com HTML times out again, use `gh release list -R openai/codex` from a run that has `gh api` allowed.
2. OpenAI Agents SDK: April to October 2026 and its most-reacted open issue are unread. Start from https://github.com/openai/openai-agents-python/releases.
3. Claude Code: 2026-05-01 to 2026-10-19 is undated. The Agent SDK release notes are unread. Issue #27661 is unread (503). Start from https://github.com/anthropics/claude-code/releases?page=10 and https://github.com/anthropics/claude-agent-sdk-python/releases.
4. LangGraph: 2026-04-08 to 2026-06-29 is unread. Start from https://github.com/langchain-ai/langgraph/releases?page=6. LangSmith: April to mid-September 2026 is unread, about the last 167,000 characters of the changelog; continue from character offset ~140,000. The LangGraph most-reacted issue is unread.
5. Not done: the (iii) checks against Karakuri's code (path:line) for all three products.
<!-- p3:end -->

<!-- p4a:start -->
Part 4 extends the p4a sections of discovery-2026-11-01.md and -2026-10-25.md and does not restate them. Every page below came through a fetch tool that returns a model-written report, so all items are [E], including lines the tool called verbatim. Release pages show no year; 2026 is inferred from page order. Vendor notes are claims, not measurements. Karakuri cells come from Go files opened on this branch (path:line).

### 4. Langfuse

New: yes. It fills the 2026-11-01 thin spot "Langfuse: not reached" (licence and releases), in part.

**(i) Shipped, 2026-10-16 to 2026-11-05 only.** Observed [E] (https://github.com/langfuse/langfuse/blob/main/LICENSE; https://github.com/langfuse/langfuse/releases and `?page=2`; read 2026-11-08). Vendor claims:
- Licence: MIT (Expat) for the repository, except `ee/`, `web/src/ee/`, `worker/src/ee/` and `packages/shared/src/server/ee/`, which fall under `ee/LICENSE` (not read). v4.69.0 (10-23) moved `packages/shared/src/server/ee` under the enterprise licence.
- Audit: v4.62.0 (10-16) staged change events beside the Postgres audit log; v4.67.0 (10-21) dropped dataset item payloads from audit logs; v4.68.0 (10-22) restricted audit logs to enterprise plans; v4.73.0 and v4.74.0 (11-03) added a read-access gate and always-logged support-access reads; v4.76.0 (11-05) added read-access audit events for trace, session and score reads, and recorded exports as access events.
- MCP server: v4.68.0 `listLlmConnections` tool and MCP SDK 1.32.0; v4.69.0 skill versions created over MCP; v4.70.0 (10-24) `runBatchEvaluation` tool; v4.73.0 bundled skills exposed over MCP.
- OTel: v4.71.0 (10-27) model attribution fix; v4.72.0 (10-29) gen_ai usage double counting fixed, trace-level eval filters.
- Self-hosting: v4.71.0 a "self-hosted government plan"; v4.76.0 the AI gateway in self-hosted docker compose.
- 2026-04-25 to 2026-10-15 is still unread (pages 3 onward of 74).

**(ii) What it does that Karakuri does not.** Inferred from (i): it audits *reads* (who looked at a trace) and treats an export as an access event; it serves its evaluations as MCP tools to a coding agent.

**(iii) What Karakuri does that it does not.** Observed (code): Karakuri runs work on a cadence (`internal/core/objective/standing.go:43`, `type Cadence struct`), digests what ran unsupervised (`internal/core/digest/digest.go:195`, autonomy changes in the digest) and exports evidence over a time window (`internal/feature/audit/export.go:149`). Nothing read shows Langfuse acting rather than observing.

**(iv) Complaints.** Not read. No thread was opened this pass [N].

**(v) Inferred:** two signals, both from vendor notes. Audit logs moved behind the enterprise plan on 10-22, so buyers who need them pay; and read-access auditing became a feature in its own right. Karakuri's export (`export.go:149`) is Apache 2.0 with no plan gate; whether it records reads was not checked. Nothing was measured.

### 5. Temporal, with DBOS

New: yes. It fills the 2026-11-01 thin spot "Temporal's server history before 2026-09-11", for 04-10 to 06-10 and 07-08, and the stretch item DBOS.

**(i) Shipped.** Observed [E] (https://github.com/temporalio/temporal/releases?page=2, read 2026-11-08; page 1 was not fetched, so 07-11 to 09-11 is unread). Vendor claims: v1.31.1 (06-10) CLI 1.7.0, a search-attribute race fix and stale gRPC connection closing; v1.29.7 (06-12) and v1.30.5 (06-15) security and PostgreSQL visibility fixes; v1.30.6 (07-08) fixes CVE-2026-5724, with a possibly breaking change for authorization plus replication (`system.disableStreamingAuthorizer`). The v1.31.0 notes were cut off in the extract; https://github.com/temporalio/temporal/releases/tag/v1.31.0 was not read [N].
- Pause: https://github.com/temporalio/temporal/issues/3006, "Workflow Pause / Unpause", is **open** on 2026-11-08 [E]. Opened 2022-06-17, closed and reopened 2023-03-03. A Temporal contributor's 2025-02-20 comment called activity pause "soon" and workflow pause undesigned; the newest comment is a +1 of 2026-07-08. The issue names no version where either shipped. Observed: no release note read names pause. Whether activity pause shipped is still unknown.
- DBOS Transact (Python), https://github.com/dbos-inc/dbos-transact-py/releases, read 2026-11-08 [E]: 2.28.0 (07-21) to 3.4.0 (10-28), ten releases, 3.0.0 on 09-16. No note read mentions agents, LLMs or MCP; the nearest are streaming items (2.28.0, 2.31.0, 3.3.0 "read stream values in pages"). 05-08 to 07-20 is on page 2, unread.

**(ii) What they do that Karakuri does not.** Inferred: both are general durable-execution engines with years of server releases; Karakuri delegates to one rather than being one (see iii).

**(iii) What Karakuri does that they do not.** Observed (code): Karakuri already has a Restate executor with cancel (`internal/platform/executor/restate.go:139`), and resumes stored loops after a restart (`internal/feature/loop/service.go:394`, `ResumeStoredLoops`) and after a checkpoint decision (`internal/feature/loop/service.go:219`). Neither engine has checkpoints for people, cadences declared on an objective (`internal/core/objective/standing.go:43`) or authority bounds (`internal/core/agent/agent.go:37`) in anything read.

**(iv) Complaints.** #3006 was read as a whole page through the extract tool [E]; it is a long-open request, not a complaint thread, and the tool gave no comment or reaction counts, so none are reported.

**(v) Inferred:** "pause and keep state" is still not a shipped primitive in Temporal after more than four years open, as far as the pages read show. Karakuri's Phase 37 (Planned, `docs/roadmap.md:4030`) aims at the same gap; nobody among the engines read here has closed it. DBOS shows no move toward agents in its notes. Nothing was measured.

### 6. incident.io, dates settled

New: yes. It fills the 2026-11-01 thin spot "incident.io is still undated", though the answer moves nothing into the window.

**(i) Shipped.** Observed [E] (read 2026-11-08 from links on https://incident.io/blog; the slugs named in the brief returned 404). "Introducing Agentic CTO" (https://incident.io/blog/introducing-agentic-cto-executive-oversight-in-every-incident) is dated 2025-04-01 and the page itself says it is an April Fools' post, so it describes no product. "Avoiding the ironies of automation" (https://incident.io/building-with-ai/avoiding-the-ironies-of-automation) is dated 2025-03-26 and is an essay of intent ("we believe we'll ship in the next few months", per the extract). Neither falls in 2026-05-08 to 2026-11-08, and nothing shipped in the window was read.

**(ii) to (iv).** (ii) Nothing new. (iii) Karakuri reads PagerDuty alerts (`internal/platform/tools/observability/pagerduty.go:60`) and returns errors for logs and metrics it cannot see (`pagerduty.go:141`, `:146`). (iv) No complaint thread read [N].

**(v) Inferred:** the essay's stance, as the extract gives it, is that the agent proposes and people decide, and that a tired responder will rubber-stamp a "confirm" prompt. That is the same risk earned autonomy has to answer. It is a 2025 position paper, not evidence of shipped behaviour.

### Still thin (part 4)

1. Langfuse 2026-04-25 to 2026-10-15 (releases pages 3 onward), `ee/LICENSE`, and a complaint thread are unread.
2. Temporal 07-11 to 09-11 (releases page 1 below the fold) and the full v1.31.0 notes are unread; whether activity pause shipped is still unknown. DBOS 05-08 to 07-20 is unread; Restate and Inngest were not reached.
3. An AI SRE tool inside the window: incident.io's two posts date from 2025. Next cycle: Rootly's changelog, PagerDuty's release notes or Datadog Bits AI SRE.
4. Governance-only product (Credo AI): not reached.
5. Karakuri matrix cells marked "not checked": MCP client, MCP server, MCP OAuth, repository checks before push, eval sets, and inheritance of `AuthorityBounds` by delegated runs. Open `internal/feature/delegation/` and `internal/feature/eval/` first.
<!-- p4a:end -->

## Capability matrix

<!-- p4b:start -->
New: yes. It replaces the 2026-11-01 Karakuri column, which rested on roadmap headings (thin spot 5), with code lines opened on this branch, and adds DBOS. Key: **O** observed on a page read, **C** claimed by the vendor, **U** unknown; then yes, partial or no. "(11-01)" marks a cell carried over from the 2026-11-01 matrix. For Karakuri, "code:" names the file and line opened; "not checked" means no code line was found in this pass's search, which is not proof of absence. incident.io's column holds only carried-over cells, since its pages had not come back when this was written.

| Capability | Karakuri | Claude Code | Codex / Agents SDK | LangGraph / LangSmith | Langfuse | Temporal | DBOS | incident.io |
|---|---|---|---|---|---|---|---|---|
| Standing objectives on a cadence | O yes, code: `internal/core/objective/standing.go:43` | O yes, scheduled routines (11-01) | U | C partial, crons (11-01) | no, observes only (inferred) | U | U | U |
| Human checkpoints with earned autonomy | O yes, code: resume on decision `internal/feature/loop/service.go:219`; promotion in digest `internal/core/digest/digest.go:195` | C partial, permission modes, no earned rung (11-01) | O partial, approvals, no earned rung (11-01) | O partial, `interrupt`; C pauses reach MCP clients (part 3) | no | no (11-01) | U | U |
| Stop a run mid-pass and keep its state | no (Phase 37 Planned, `docs/roadmap.md:4030`); executor cancel exists, code: `internal/platform/executor/restate.go:139` | U | U | U | no | O no, pause #3006 open (this part) | U | U |
| Authority bounds inherited by delegated runs | partial: bounds type, code: `internal/core/agent/agent.go:37`; inheritance by delegated runs not checked | U | U | U | no | no | no | U |
| Repository checks before an agent pushes | not checked | U | U | U | no | no | no | no |
| Digests | O yes, code: `internal/core/digest/digest.go:65` | U | U | U | no (inferred) | no | no | U |
| Audit log and evidence export | O yes, code: `internal/feature/audit/export.go:149` | U | U | O partial, bulk-export limits (part 3) | C yes, read-access audit, enterprise plan only (this part) | C partial (11-01) | U | U |
| MCP client | not checked | C yes (11-01) | C yes (11-01) | U | U | U | U | U |
| MCP server | not checked | U | U | C yes (11-01); C MCP `query_runs` (part 3) | C yes, eval and skill tools (this part) | U | O no mention (this part) | C yes, beta (11-01) |
| MCP OAuth | not checked | U | U | U | U | U | U | U |
| OTel GenAI telemetry | O yes, code: `internal/core/telemetry/genai.go:7` | C yes (11-01) | O partial (11-01) | U | C yes, gen_ai usage fix (this part) | no seen (11-01) | U | U |
| Durable execution | O yes, code: `internal/feature/loop/service.go:394` (`ResumeStoredLoops`) | O partial (11-01) | O partial (11-01) | O yes, checkpointers (11-01) | no | C yes | C yes (product category) | U |
| Incident and paging | O partial, reads PagerDuty alerts only, code: `internal/platform/tools/observability/pagerduty.go:60` | no (11-01) | no (11-01) | no (11-01) | no | no | no | C yes (11-01) |
| Eval sets | not checked (an `internal/feature/eval` package exists; no line opened) | U | U | O yes, `reference_outputs` (part 3) | C yes, batch evaluation (this part) | no | no | U |
| Self-hosting and licence | O yes, Apache 2.0 LICENSE (11-01) | no (inferred, 11-01) | C partial, SDK open (11-01) | U | O MIT with `ee/` directories under a separate licence (this part) | O yes, MIT (11-01) | U | no, SaaS (inferred, 11-01) |
| A2A | O no: no Go file matches "a2a" (search this part) | O none seen (11-01) | O none seen (11-01) | C yes, LangSmith only (11-01) | no (11-01) | no (11-01) | U | U |

Inferred: on code, Karakuri's clear "yes" rows are cadence, digests, evidence export, OTel GenAI and durable resume. The row where Karakuri says no is stopping mid-pass, and Temporal, open since 2022, has not shipped it either. Langfuse is the only column with dated audit work this cycle, and it put that work behind a paid plan.
<!-- p4b:end -->

## Technical frontier

<!-- p5:start -->
Written by part 5 on 2026-11-08 (date read: 2026-11-08). Read first: the 'Technical frontier' and 'Still thin (part 5)' of 2026-11-01, and the Goal lines of Phases 28, 29, 33, 42, 43 and 46; nothing below restates them. Pages came through a fetch tool that returns a model-written report. Each report quoted the page text and said which parts were verbatim. [Q] marks words the tool gave as verbatim, and [E] marks the tool's paraphrase. On all three provider pages the tool's own summary of the page tail contradicted the verbatim text (wrong newest dates). This part used only the verbatim entries.

### C. Model and provider capabilities, 2026-10-21 to 2026-11-08

New: yes. Two of the primary URLs below appear in no earlier report, and this subsection fills 2026-11-01's thin spot "Item 3 ... not reached".

**C1. Anthropic: smaller requests for long agent loops; a cost analytics API.** Observed (https://platform.claude.com/docs/en/release-notes/api, newest entry November 5, 2026) [Q]: 2 November, "Long conversations and agent loops on the Messages API can now send smaller requests". 29 October, "We’ve launched the Usage and Cost Analytics API in beta with the analytics-usage-cost-2026-08-24 beta header" and "The Usage and Cost API is now a legacy API." 28 October, the maximum request size for Messages and Token Counting went "from 32 MB to 128 MB". 30 October, fast mode for Claude Opus 4.8 is deprecated, "with removal on November 30, 2026." 21 October, "We’ve released API version 2026-10-21." The mechanism behind "smaller requests" was not read: the linked page was not followed.
- Maturity: shipped by one vendor. The analytics API is a beta.
- What changes for an agent platform. Inferred: context management. If a loop can send a delta instead of the whole transcript each turn, an iteration's cost and latency stop growing with history. That matters for a reconcile pass that re-runs the loop many times. Cost controls: a provider-side usage and cost API is a second source for Phase 38's per-objective spend, so Karakuri's own token count could be reconciled against the bill. That is unbuilt, and the beta header is what an adapter would pin. Removal on a fixed date: a pinned model option that disappears on 30 November is the kind of drift Phase 43's shape test does not cover, because the option is a mode, not a tool-call shape.

**C2. OpenAI: an Agents API that schedules and edits tools mid-session.** Observed (https://developers.openai.com/api/docs/changelog, newest entry Nov 5) [Q]: 23 October, "Added scheduled automations to the Agents API public beta." 30 October, "You can now update the tools in an existing session in the Agents API public beta." and "Added automatic review for eligible Agents API projects." 26 October, "GPT-6.1 Terra, a fast, efficient model for visual reasoning, precise computer use, and clear, natural writing." 22 October, "The Decisions API is now generally available with gpt-6-luna." 21 October, "You can now choose which service tiers projects can use in Platform settings." The linked pages for "scheduled automations", "automatic review" and the Decisions API were not followed, so what each does beyond its one line is unknown.
- Maturity: public beta from one vendor, except the Decisions API (GA) and the tier setting.
- What changes. Inferred: long-running and background tasks. A vendor now schedules agent runs on the vendor's side. That is the standing-objective shape ADR 015 puts in `internal/feature/reconcile`, now hosted by a model provider. Karakuri's difference becomes authority (bounds, checkpoints, earned autonomy) and provenance, not the schedule itself. Tool use: changing tools mid-session means a run's capability set is no longer fixed at start. If Karakuri delegated to such a session, Phase 46's "one action, never a rung" would need the tool list pinned at the approval. Cost controls: a per-project service-tier allowlist is an operator spending lever outside Karakuri, with no Karakuri equivalent read.

**C3. Google: Computer Use GA; tools may return files; storage on by default.** Observed (https://aistudio.google.com/docs/changelog, reached by a 301 from https://ai.google.dev/gemini-api/docs/changelog; newest entry November 6, 2026) [Q]: 30 October, "Released the Computer Use tool to general availability (GA) across the Interactions and GenerateContent APIs." 5 November, "Tools can now return Office and OpenDocument files, audio, and video to the model." 26 October, Generate Content requests "now store requests by default (`store=true`)". 27 October, a Gemini API CLI in beta, "a stateless, zero-dependency command-line client for AI coding agents". 2 November, Gemini 2.5 Pro and 2.5 Flash are "no longer available to select in Google AI Studio". ADK release notes: not read (see Still thin).
- Maturity: Computer Use is GA from one vendor. The CLI is a beta.
- What changes. Inferred: computer use is now a GA vendor tool, and a computer-use observation is text and pixels from a third-party screen. Under AGENTS.md rule 9 it is `TrustThirdParty` by what it holds. Tool results that carry files mean an MCP or tool adapter's result is no longer only text, so the provenance flag has to travel with a blob too. That is relevant to ADR 002's content-addressed store. Memory and data handling: `store=true` by default means a request is retained by the provider unless the adapter says otherwise. A Gemini adapter (Phase 6's fallback chain) should set the flag explicitly. Whether Karakuri's Gemini adapter sends `store` was not checked in code.

### A. Protocols and standards

New: no. The draft changelog URL was read in earlier reports.

**A1. MCP draft: one schema fix since 2026-07-28.** Observed (https://modelcontextprotocol.io/specification/draft/changelog) [Q]: "Changes since the most recent release will accumulate here." The only entry restores "the optional `_meta` field on `CreateMessageRequestParams`, `ElicitRequestFormParams`, `ElicitRequestURLParams`, `ElicitResult`, and `ListRootsResult`", which "was unintentionally dropped in 2026-07-28" (#3468). The releases feed timed out twice and was not read. Maturity: a draft fix to a stable spec. Inferred: elicitation results can carry `_meta` again, so a client can attach provenance or a trace id to an elicitation answer. That is small, but it is the only place an MCP human answer could carry "who approved" metadata. The open SEPs, ext-auth, A2A, OTel agent spans past character 39,343 and IETF drafts were not reached this cycle.

### B. Research

Not reached (see Still thin). No paper is cited this cycle.

### Candidate frontier bets (part 5)

**Bet 1. Provider-side cost data reconciles Karakuri's own spend counter.** Primary evidence: C1, Anthropic's Usage and Cost Analytics API (beta, 29 October). The bet: Phase 38's per-objective spend is checked weekly against the provider's own usage figure, and a gap above a threshold raises a finding, because a cap that counts tokens wrong caps nothing. Proved wrong if the analytics API cannot be filtered finely enough (per key, per request tag) to attribute usage to one objective. First slice, one day: read the API's reference page and list its group-by dimensions. Kill it if none maps to a Karakuri objective or API key.

**Bet 2. An approval pins the tool list it approved.** Primary evidence: C2, mid-session tool updates in OpenAI's Agents API (30 October). The bet: a checkpoint resolution records the hash of the run's tool set, and an action after a tool-set change needs a fresh approval. That extends Phase 46 from third-party text to capability drift. Proved wrong if no runtime Karakuri delegates to can change tools after start, or if Phase 46's single-action rule already makes the approval void before any tool change can matter. First slice, half a day: list which delegated runtimes (Phase 6 CLIs, MCP servers via `tools/list_changed`) can change tools mid-run. Kill it if none can.

**Bet 3 (weak). File-bearing tool results keep their provenance in the blob store.** Primary evidence: C3, Gemini tools returning files (5 November). The bet: a stored artifact derived from a third-party tool result carries `TrustThirdParty` in its metadata, so a later run that reads the artifact inherits the flag. Proved wrong if `storage.StorageAdapter` already records trust per blob. First slice, two hours: grep the artifact store for a trust field. Kill it if present.

### Still thin (part 5)

- (B) Research: not reached. No paper was read, so this cycle cites no new paper. The three-paper requirement is unmet.
- (A) beyond the draft changelog: the MCP releases feed (two timeouts), the open SEPs, ext-auth, A2A past character 200,000 and an implementer's page, OTel agent spans past character 39,343, and IETF agent-identity drafts were all not read.
- Google ADK release notes: not read.
- Linked detail pages not followed: Anthropic's "smaller requests" mechanism and its Usage and Cost Analytics reference; OpenAI's scheduled automations, automatic review and Decisions API pages.
- Not checked in code: whether the Gemini adapter sets `store`, and whether the artifact store records trust (Bet 3).
<!-- p5:end -->

## Governance and buying criteria

<!-- p6a:start -->
Part 6 extends the p6a section of discovery-2026-11-01.md and does not restate it. All pages were read on 2026-11-08 through a fetch tool that returns a model-written report. [Q] marks fragments the tool gave as verbatim from the page (it capped each at 125 characters), [E] the tool's paraphrase. Karakuri cells name a file and line opened in this part; "carried" marks a line taken from part 4b and not reopened here.

### (1) EU AI Act, Regulation (EU) 2024/1689, and the AI Omnibus dates

New: no. Both URLs appear in discovery-2026-10-08.md; this is the first pass that read the articles themselves.

| Requirement | What it says (read 2026-11-08) | Karakuri can evidence today |
|---|---|---|
| Art. 12(1) record-keeping | [Q] "High-risk AI systems shall technically allow for the automatic recording of events (logs) over the lifetime of the system." | partial: tool events and checkpoints are exported over a window, `internal/feature/audit/export.go:149` (`Export`), `:219` (`checkpoints`), `:310` (`exportRow`). Karakuri records what its own runs did, not the lifetime of a customer's high-risk system. |
| Art. 12(2)(c) logs serve deployer monitoring | [Q] "monitoring the operation of high-risk AI systems referred to in Article 26(5)" | partial: same export; telemetry `internal/core/telemetry/genai.go:7` (carried). |
| Art. 14(1) oversight by people | [Q] "can be effectively overseen by natural persons during the period in which they are in use." | yes for actions: zero autonomous actions means draft and escalate, `internal/core/agent/agent.go:38-41`; the decide step applies the bounds, `internal/feature/loop/decide.go:81`, `:106`. |
| Art. 14(4)(d) override or reverse output | [E] decide not to use, override or reverse the output | partial: a checkpoint can be resolved with modify (`internal/feature/loop/decide.go:93`, comment); reverse: no (the undo gap of part 2). |
| Art. 14(4)(e) a "stop" button | [E] intervene or stop the system through a "stop" button or similar procedure | no: Phase 37 is Planned (`docs/roadmap.md:4030`, carried); executor cancel exists, `internal/platform/executor/restate.go:139` (carried). |
| Art. 26(5) deployer monitors | [Q] "Deployers shall monitor the operation of the high-risk AI system on the basis of the instructions for use" | partial: telemetry and digests (carried); Karakuri ships no "instructions for use" document. |
| Art. 26(6) keep logs ≥ 6 months | [Q] "shall keep the logs automatically generated by that high-risk AI system" … "of at least six months" | partial: the export states its retention floor and period, `internal/feature/audit/export.go:53-57` (`FloorDays`, `RetentionDays`, `WindowPrecedesRetention`). Whether the default retention reaches 180 days was not checked in `config/default.yaml`. |
| Art. 113 dates of application | [N] not read: the tool's reads stopped at Art. 112(4)(c). | n/a |
| Omnibus dates (Commission page) | [Q] Annex III high-risk uses "have been extended to 2 December 2027 as a result of the political agreement on the proposal to simplify the AI Act"; product-embedded systems "an extended transition period until 2 August 2028"; the AI Omnibus "entered into force on 27 July 2026". The page says "AI Omnibus", not "Digital Omnibus". | n/a |

Observed: the Commission's own page now gives 2 December 2027 and 2 August 2028 as the high-risk dates. Inferred: a regulated buyer's deadline for Art. 12/14/26 evidence moved about 16 months later than 2 August 2026, so the stop button (Art. 14(4)(e)) is the one "no" that a buyer could still ask for before the date. Art. 113 itself was not read, so the original dates are not confirmed from the regulation.

### (2) OWASP Top 10 for Agentic Applications for 2026

New: no. The URL appears in three earlier reports.

Observed [E]: the page title is "OWASP Top 10 for Agentic Applications for 2026", dated "December 9, 2025" on the page. The ten risks are not in the page text; they sit behind a download link (https://genai.owasp.org/download/52117/), not fetched [N]. No risk ID or name is reported, and no Karakuri cell is filled.

### (3) and (4) NIST AI 600-1 / ISO/IEC 42001 / a procurement questionnaire

New: no. Not reached in the time box [N].
<!-- p6a:end -->

## Feasibility

<!-- p6b:start -->
No web. Files opened on this branch on 2026-11-08; line numbers are from that checkout. The open questions of 2026-11-01 come first.

### (i) How bounds reach a run, and whether `CanDelegate` is enforced

New: yes. It answers open code question (i) of 2026-11-01.

Observed: `CanDelegate` is a field of `AuthorityBounds` (`internal/core/agent/agent.go:43`). The decide step reads `sc.agentDef.Authority` (`internal/feature/loop/decide.go:81`) and calls `authority.Decide(p.Confidence, threshold, planned, sc.evidence)` (`decide.go:116`), which caps the action count and escalates. The only non-test Go lines under `internal/` that name `CanDelegate` are the two writes in `internal/feature/reconcile/authority.go:32` and `:59`, which set it false at the sense level and for an unknown level. Inferred: nothing reads `CanDelegate`, so the loop does not refuse a delegation when it is false. A delegation goes through only because it is an action under the count cap or a capability in `RequiresApprovalFor`. This rests on a search that found no reader, plus the files opened; `AuthorityBounds.Decide` itself was not opened.
- Missing: a read of `CanDelegate` where a delegating capability is planned.
- First slice: `internal/core/agent/agent.go` (`Decide` escalates any planned delegating capability when `CanDelegate` is false), its test, and the capability IDs that delegate (`internal/platform/tools/cliagent/`).
- Not verified: which capability IDs count as delegation.

### (ii) Reconcile and `software.verify.review` sub-agents inherit bounds

New: no. Partly observed: reconcile renders the autonomy level into bounds from `def.Authority` (`internal/feature/reconcile/authority.go:21-22`), and the loop enforces those (`decide.go:81`). Not verified: whether the review sub-agents of `software.verify.review` run under the parent's bounds; the workflow and its runner were not opened.

### (iii) A pass that read `TrustThirdParty` text before a checkpoint

New: yes. It answers open code question (iii) of 2026-11-01, in part.

Observed: `sc.evidence` goes into `Decide` (`decide.go:116`), and when it escalates, the audit row records `"third_party_sources": sc.evidence.ThirdParty` (`decide.go:143`) with `BoundsViolation: true` (`decide.go:155`). Inferred: the loop does not end the pass; it hands the third-party evidence to the bounds check, which can escalate, and names the sources in the audit row. Whether `Decide` always escalates on third-party evidence was not verified (`Decide` not opened). Phase 46 is the phase that covers "one action per approval".

### (iv) `DelegateInput.Env` and an operator token

New: yes. It answers open code question (iv) of 2026-11-01.

Observed: `Env map[string]string` is "merged onto os.Environ()" (`internal/platform/tools/cliagent/adapter.go:24-25`). The subprocess sets `cmd.Env = procenv.ScrubNestedSession(procenv.Merge(in.Env))` (`internal/platform/tools/cliagent/subprocess.go:31`); the Copilot adapter sets `cmd.Env = procenv.Merge(in.Env)` with no scrub (`internal/platform/tools/cliagent/copilot.go:60`). An MCP bearer token travels in `MCPAttachment.Token`, "never ... on the command line" (`adapter.go:39`). Inferred: yes, `Env` can carry a token, and more: the delegated CLI inherits the server's whole environment, so any secret in the server's process environment (such as `ANTHROPIC_API_KEY`) reaches it unless `procenv` removes it. `procenv.Merge` and `ScrubNestedSession` were not opened, so what they remove is unknown. Phase 48 (added by #217, "keep agent-written code from server secrets") is the phase this belongs to; its Steps were not read.
- First slice: `internal/platform/procenv/` (an allowlist instead of a merge onto `os.Environ()`), `cliagent/subprocess.go:31`, `cliagent/copilot.go:60`, and a test that a server secret is absent from the child's environment.

### (v) Planned phases and today's candidates

New: no. Not reached in the time box. Candidates not assessed: rewind/undo (part 2), MCP elicitation for checkpoints (part 3), read-access audit (part 4), provider cost reconciliation, tool-set pinning, and blob provenance (part 5 Bets 1 to 3).

### Still thin (part 6)

1. EU AI Act Art. 113 not read (the tool stopped at Art. 112); try http://data.europa.eu/eli/reg/2024/1689/oj.
2. OWASP Agentic Top 10: the list is in https://genai.owasp.org/download/52117/, not fetched.
3. NIST AI 600-1, ISO/IEC 42001 summary, and a procurement questionnaire: not reached.
4. Code not opened: `AuthorityBounds.Decide`, `internal/platform/procenv/`, the `software.verify.review` runner, the Steps of Phases 37, 38 and 42 to 48, and the default retention in `config/default.yaml`.
5. Feasibility for the six candidates of parts 2 to 5: not done.
<!-- p6b:end -->

## Fit and bets

<!-- p7a:start -->
Written by part 7 on 2026-11-08. No web: everything below comes from parts 1 to 6 of today and keeps their grades. Session limit 600 seconds.

### Verdict: new evidence this cycle

New evidence: yes (13 subsections, 11 new primary sources)

- Observed, subsections marked 'New: yes': p1 (e), (f); p2 (1), (2); p3 §3; p4a §4, §5, §6; p4b matrix; p5 §C; p6b (i), (iii), (iv). That is 13. Marked 'New: no': p1 (a); p2 (3), (4); p3 §1, §2; p5 §A; p6a (1), (2), (3)/(4); p6b (ii), (v); p1w.
- Observed, [Q] or [E] primary URLs found by `grep -F` in no earlier `docs/research/discovery-2026-1*.md`: the gh-aw Fault Investigation post, the HMRC Ask Security record, the Replit databases post, crewAI #6043, gemini-cli #10223, litellm #25931, the DBOS Transact releases, both incident.io pages, Anthropic's release notes at platform.claude.com and Google's changelog at aistudio.google.com. That is 11. Not counted: the Agent Factory status page (its address is in 2026-11-01), search-result URLs used for navigation, and every [N].
- Inferred: of the 11, only the seven with [Q] text bear weight (gh-aw, HMRC, crewAI, gemini-cli, litellm, the two provider changelogs). The incident.io pages are from 2025 and one is satire, and DBOS says nothing about agents.

### Fit by segment

- **Platform and SRE teams: WEAK** (unchanged). Observed: no agent-in-incident postmortem again (p1 (a), table); incident.io's two pages are 2025 and one is satire (p4a §6). Reasons they would NOT want Karakuri: it cannot stop a run mid-pass (matrix row "Stop a run mid-pass", Karakuri "no", Phase 37 Planned); its paging is read-only PagerDuty (matrix row "Incident and paging", `pagerduty.go:60`). Inferred: until Phase 37 ships, the AI Act's "stop" (p6a Art. 14(4)(e), "no") and an SRE's first question get the same answer.
- **Self-hosters: WEAK** (unchanged; not attempted this cycle, p1 table). Fits: Apache 2.0 with no plan gate (matrix row "Self-hosting and licence"), while Langfuse put audit logs behind an enterprise plan (p4a §4 (i), [E], a vendor note). Reasons they would NOT want it: Temporal is MIT with years of server releases (matrix row "Durable execution", Temporal "C yes"); no second self-hoster source in five cycles.
- **Teams running unattended or standing coding agents: STRONG, one organisation.** Observed: GitHub Next runs schedules from every 30 minutes to weekly next to hundreds of on-demand entries, and reports 9 of 13 CI Doctor PRs merged (p1 (e), [Q], the team's own count). Karakuri cells: cadence "O yes" (`standing.go:43`), digests "O yes" (`digest.go:65`). Reasons they would NOT want it: OpenAI now schedules agent runs on its side (p5 C2, [Q], public beta), so the schedule alone is no longer a reason; Karakuri's "Authority bounds inherited by delegated runs" cell is "partial", and part 6 found nothing reads `CanDelegate` (p6b (i), Inferred) and the delegated CLI inherits the server's environment (p6b (iv), Inferred). Repository checks before push: "not checked" (Phase 45 Planned).
- **Regulated organisations: STRONG on mechanism, WEAK on proof** (fifth cycle). Observed: first first-hand public record, HMRC, but a chatbot that is "a processor not an initiator" (p1 (f), [Q]); the AI Act articles were read for the first time (p6a (1), [Q]) and Karakuri is partial on Art. 12 and 26, yes on Art. 14(1), no on Art. 14(4)(e) stop. The high-risk dates moved to 2 December 2027 and 2 August 2028 (p6a, Commission page [Q]). Reasons they would NOT want it: no stop and no reverse (matrix "Stop a run mid-pass" no; p6a Art. 14(4)(d) partial); Langfuse audits reads, Karakuri's read auditing is not checked (matrix row "Audit log", p4a §4 (v)). Inferred: the moved dates lower urgency for a buyer this year.
- **Product teams: IRRELEVANT** (unchanged, not attempted, p1 table). They embed an SDK; Karakuri ships none.
- **Enterprises: WEAK** (not attempted, p1 table). Fits: RBAC and evidence export (matrix "Audit log" O yes). Reasons they would NOT want it: LangSmith ships granular workspace permissions and tracing-only keys (p3 §3 (i), [E], vendor claims) and asks for human input over any MCP client (matrix "Human checkpoints", LangGraph "C pauses reach MCP clients"); Karakuri's MCP client, server and OAuth cells are all "not checked".

### Ranking of every candidate

Gates: DEMAND needs two independent organisations' primary sources; FRONTIER BET needs a primary technical source. Planned phases 37 to 40 and 42 to 48 are unbuilt and 47 gates the alpha (p1w), so a new phase must have a sized first slice and a gap no Planned phase covers. No discovery phase was declined (p1w). Every expected value below is Inferred.

| # | Candidate (part) | Kind | Sources | Planned phase covering it | Feasibility (p6) | Expected value | Result |
|---|---|---|---|---|---|---|---|
| 1 | `CanDelegate` is enforced: a planned delegation escalates when it is false (p6b (i)) | DEMAND | ZeroClaw #10968 and copilot-cli #3013 (2026-11-01 p2 §1, [Q]); gap Observed in code by p6b (only writes at `reconcile/authority.go:32`, `:59`, no reader found) | None: `grep CanDelegate docs/roadmap.md` finds nothing; Phase 39 routes tool calls through a gateway but its Goal does not name the field | **Sized**: `agent.go` `Decide` + test + delegating capability IDs (p6b (i)); not verified which IDs delegate | High: a bound the operator sets and nothing reads is a false promise in the evidence pack; small slice | **PASS** |
| 2 | Delegated CLI does not inherit server secrets; Copilot adapter skips the scrub (p6b (iv)) | DEMAND (owner requirement) | Code, p6b (iv): `subprocess.go:31` scrubs, `copilot.go:60` does not | **Phase 47 step 4** ("What an agent wrote runs without the server's secrets", `docs/roadmap.md:6539`). Part 6 named Phase 48; on main Phase 48 is "Independent Groups Run in Parallel" | Sized by p6b | High, already queued on the alpha path | FAIL as a new phase; **amendment** for the human: Phase 47 step 4 should name `cliagent/copilot.go:60` and the `procenv.Merge` onto `os.Environ()` |
| 3 | Rewind or undo an agent's changes (p2 (1)) | DEMAND | claude-code #52209, opencode #31338 (2026-11-01), gemini-cli #10223 [Q]: three organisations | None. Phase 37 says a run is "stopped, not undone" (`docs/roadmap.md:4259`); ADR 003 worktrees answer the half before push | **Not sized** (p6b (v) not reached) | Moderate: votes low (11 on #10223); one vendor shipped `/rewind`, a later user says it was not yet available ([E]) | FAIL this cycle: passes the gate but has no sized slice; heads next cycle's Feasibility |
| 4 | A setting honoured on every provider path (timeout is one case) (p2 (1)) | DEMAND | n8n #25360 (2026-11-01), litellm #25931 [Q], for the class; timeout itself one tracker | Phase 43 nearest (tool-call shape, not settings) | Not sized | Low to moderate | FAIL; possible amendment note to Phase 43, not made here |
| 5 | Text from one agent trusted by another without provenance (p2 (2)) | DEMAND | crewAI #6043 only [Q] | Phase 46 and rule 9 | n/a | Low as new work | FAIL (one tracker, covered) |
| 6 | State kept on cancel; stop mid-pass (p2 (3), p4a §5) | DEMAND | langgraph #5672, #3716 (one org); Temporal #3006 [E] | Phase 37 | n/a | Confirms Phase 37 | FAIL (covered) |
| 7 | MCP arguments limited to the declared schema (p2 (3)) | DEMAND | n8n #21716 | Phase 42 | n/a | — | FAIL (covered) |
| 8 | One digest across standing objectives (p2 (1), p1 (e)) | DEMAND | GitHub Next only; gh-aw search found nothing [E] | Phase 40 partly | n/a | Weak | FAIL (one org) |
| 9 | Usage limits hit without explanation (p2 (3)) | DEMAND | claude-code #16157 only | Phase 36 partly | n/a | A vendor quota Karakuri cannot fix | FAIL |
| 10 | Checkpoint questions reach any MCP client by elicitation (p3 §3 (v), p5 A1) | FRONTIER BET | LangSmith changelog [E] (vendor claim); MCP draft changelog [Q] (`_meta` restored on elicitation) | None named; Phase 33 (Completed) covers the MCP client | Not sized; Karakuri MCP cells "not checked" (p4b) | Moderate; one vendor claim | FAIL this cycle (unsized) |
| 11 | Provider cost data reconciles Karakuri's spend counter (p5 Bet 1) | FRONTIER BET | Anthropic release notes [Q], beta API | Per-objective spend is Phase 23 (`docs/roadmap.md:6768`), not Phase 38 as p5 said | Find-out slice only (p5), not sized by p6 | Low until the API's group-by dimensions are known | FAIL (beta, unsized) |
| 12 | An approval pins the tool list it approved (p5 Bet 2) | FRONTIER BET | OpenAI changelog [Q], one line, linked page not read | Phase 46 | Not sized | Moderate | FAIL as a new phase; **amendment** for the human: Phase 46 could record the tool-set hash at approval |
| 13 | File-bearing tool results keep provenance in the blob store (p5 Bet 3) | FRONTIER BET (weak) | Google changelog [Q] | Rule 9, ADR 002 | Two-hour grep, not done | Low | FAIL |
| 14 | Read-access audit events (p4a §4 (ii), (v)) | FRONTIER BET | Langfuse releases [E], vendor claim | Phase 31 (Completed) export | Not sized; whether Karakuri audits reads was not checked | Low to moderate for regulated buyers | FAIL (unsized, one vendor) |
| 15 | Gemini adapter sets `store` explicitly (p5 C3) | FRONTIER BET | Google changelog [Q] | None | Not checked in code | Low; a config fix, not a phase | FAIL as a phase; backlog item if the adapter omits it |

### Recommended

1. **`CanDelegate` is enforced where a delegating capability is planned (candidate 1). DEMAND.** Sources: ZeroClaw #10968 and copilot-cli #3013 (2026-11-01, [Q]); the gap is Inferred by p6b (i) from a search that found no reader and from `decide.go:81`, `:116`. Kill criterion: if `AuthorityBounds.Decide` (not opened by p6b) already escalates delegating capabilities, ship the test and stop. Inferred: a one-pull-request slice; it could fold into Phase 39 rather than stand alone, which the human should decide.

Only one. Rewind (3) clears the demand gate but has no sized slice, and Phase 37 says "stopped, not undone" on purpose; proposing it unsized would add an open-ended phase to a queue that already holds nine unbuilt ones before and after the alpha. Candidates 2 and 12 are amendments to Phases 47 and 46, not phases. Everything else rests on one tracker or one vendor's line.
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
All read or attempted on 2026-11-08.
- https://docs.langchain.com/langsmith/changelog: primary, [E] (model extract of ~40k characters plus a tool summary; ~167k characters not read).
- https://github.com/openai/codex/releases?page=8, ?page=15: primary, [N] (timeout).
- https://github.com/openai/openai-agents-python/releases: primary, [N] (timeout).
- https://api.github.com/search/issues?q=repo:openai/openai-agents-python+is:issue+is:open&sort=reactions&order=desc&per_page=1: primary, [N] (504).
- https://github.com/anthropics/claude-code/releases?page=10, ?page=25: primary, [N] (timeout).
- https://github.com/anthropics/claude-agent-sdk-python/releases: primary, [N] (timeout).
- https://api.github.com/repos/anthropics/claude-code/issues/27661 and /comments: primary, [N] (503, 504).
- https://github.com/langchain-ai/langgraph/releases?page=6, ?page=10: primary, [N] (timeout).
- https://api.github.com/search/issues?q=repo:langchain-ai/langgraph+is:issue+is:open&sort=reactions&order=desc&per_page=1: primary, [N] (503).
<!-- s3:end -->

### Sources, part 4

<!-- s4:start -->
All read or attempted on 2026-11-08, through a fetch tool that returns a model-written report, so every one read is [E].
- https://github.com/langfuse/langfuse/blob/main/LICENSE: primary, [E] (full page).
- https://github.com/langfuse/langfuse/releases and ?page=2: primary, [E] (pages 1 and 2 of 74).
- https://github.com/temporalio/temporal/releases?page=2: primary, [E] (v1.31.0 notes cut off).
- https://github.com/temporalio/temporal/releases/tag/v1.31.0: primary, [N] (not fetched).
- https://github.com/temporalio/temporal/issues/3006: primary, [E] (full page, no counts given).
- https://github.com/dbos-inc/dbos-transact-py/releases: primary, [E] (page 1 only).
- https://incident.io/blog/introducing-agentic-cto and https://incident.io/blog/avoiding-the-ironies-of-automation: primary, [N] (404).
- https://incident.io/blog: primary, [E] (index, used for links).
- https://incident.io/blog/introducing-agentic-cto-executive-oversight-in-every-incident: primary, [E] (full page; satire).
- https://incident.io/building-with-ai/avoiding-the-ironies-of-automation: primary, [E] (full page).
<!-- s4:end -->

### Sources, part 5

<!-- s5:start -->
All read 2026-11-08 through a fetch tool that returns a model-written report. P = primary.
- https://platform.claude.com/docs/en/release-notes/api (reached by a 301 from https://docs.claude.com/en/release-notes/api): P, the entries from 21 October to 5 November verbatim [Q]; the tool's summary of the tail was used for nothing
- https://developers.openai.com/api/docs/changelog (reached by a 301 from https://platform.openai.com/docs/changelog): P, the entries from 21 October to 5 November verbatim [Q]; the tail summary was used for nothing
- https://aistudio.google.com/docs/changelog (reached by a 301 from https://ai.google.dev/gemini-api/docs/changelog): P, the entries from 26 October to 6 November verbatim [Q]; the tail summary was used for nothing
- https://modelcontextprotocol.io/specification/draft/changelog: P, full text [Q]
- https://github.com/modelcontextprotocol/modelcontextprotocol/releases.atom: not read (timed out twice) [N]
<!-- s5:end -->

### Sources, part 6

<!-- s6:start -->
- [Q]/[E] https://eur-lex.europa.eu/legal-content/EN/TXT/HTML/?uri=OJ:L_202401689 (Arts. 12, 14, 26; Art. 113 not reached), read 2026-11-08
- [Q] https://digital-strategy.ec.europa.eu/en/policies/regulatory-framework-ai, read 2026-11-08
- [E] https://genai.owasp.org/resource/owasp-top-10-for-agentic-applications-for-2026/, read 2026-11-08
- [N] https://genai.owasp.org/download/52117/; http://data.europa.eu/eli/reg/2024/1689/oj
- Files opened: `internal/core/agent/agent.go`, `internal/feature/reconcile/authority.go`, `internal/feature/loop/decide.go`, `internal/feature/audit/export.go`, `internal/platform/tools/cliagent/adapter.go`; lines searched in `internal/platform/tools/cliagent/subprocess.go` and `copilot.go`.
<!-- s6:end -->

### Sources, part 7

<!-- s7:start -->
<!-- s7:end -->

### Sources, part 8

<!-- s8:start -->
<!-- s8:end -->
