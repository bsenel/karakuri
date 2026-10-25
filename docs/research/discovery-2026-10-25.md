# Discovery 2026-10-25: weekly cycle

**Method.** This report is written in eight parts by separate sessions that are meant to run one after another, share no memory, and are not guaranteed to run in order; each part therefore guards itself and writes only between its own marker comments. It builds on `docs/research/discovery-2026-10-04.md` and `docs/research/discovery-2026-10-08.md`, refers to them rather than repeating them, and is meant to go deeper. Every source is graded: **[Q]** the fetch returned the words verbatim, and only then are they shown in quotation marks; **[E]** extract or paraphrase, never quoted; **[N]** not read (blocked, paywalled, 404, empty or rate-limited) and cited for nothing. "Observed:" marks what a fetched page says; "Inferred:" marks a conclusion drawn here. Nothing was built, run or tested. A vendor's or analyst's figure is theirs, not a measurement.

**Why this cycle runs.** The 2026-10-08 report's list 'Where this cycle is still thin', item 3, names sources that were not read and can be read now (a conference transcript of an SRE team, a GitHub Discussion of a self-hosted agent project, Anthropic's report PDF). The MCP specification's release feed shows nothing after 2026-07-28 (https://github.com/modelcontextprotocol/modelcontextprotocol/releases.atom, read 2026-10-25) [E], so the cycle runs on the thin spots, not on a new release.

## Who the users are and what they are trying to do

<!-- p1:start -->
Part 1 run: started, then finished on 2026-10-25 by a second session (see Run log)

Written by part 1 on 2026-10-25. The segments of the 2026-10-04 report and the first-hand sources of the 2026-10-08 report ('Who the users are', Brex, Zalando, Spotify, four Hacker News threads) stand and are not restated. This section adds sources for the thin spots that report named, in its order. Quotations are fragments as the fetch tool returned them; the tool splits long sentences, so a fragment is not always a whole sentence. Nobody was interviewed. The regulated-organisation segment has moved to part 6, which reads it together with governance.

### Platform and SRE teams: SREcon26 Americas

- **The conference program.** SREcon26 Americas lists five talks on AI agents or AI acting during incidents (program page, read to character 100,000 of about 157,000). Observed: Vladyslav Budichenko (Vocaly AI), "AI Agents for Incident Investigation: The Good, The Bad, and The Ugly", whose abstract covers "LLM-based agents and coding engines like Claude Code during real on-call work."; it is the same speaker the 2026-10-08 report found at Conf42, where his talk described no incident he handled. Paul Huang (Meta), "Autonomous Policy Validation": "This talk details how we built AI agents that use log analysis and artifact reasoning to automate and augment investigations." Rohan R. Arora and Bhavya (IBM Research), on 50 failure scenarios: "Current AI models resolve only 13.8% of scenarios—a reality check for anyone evaluating these tools." That is IBM's figure on IBM's benchmark, not a measurement made here. J. Paul Reed (Chime), "The Ironies of AI²", on automation and AI in "high consequence, high tempo situations (aka incidents)." (https://www.usenix.org/conference/srecon26americas/program, read 2026-10-25) [Q]
- **An agent that acted during a P1: Eddie Redick (CTC Ops), "Human Factors in the Age of AI Ops: Re-Engineering Trust between Humans and Machines", 25 March 2026.** Who speaks: an incident-management leader with, per his bio, "over 15 years of experience managing large-scale distributed systems and high-severity outages,". Job: command a P1 in which automation and AI act alongside people. Observed: "When everything fails at once... cascading service degradation, overlapping automations, and an over-eager AI auto-remediator"; "What happens when human intuition collides with machine logic in the middle of a P1?"; "I have witnessed countless times where precious outage minutes are wasted chasing false positives."; "AI is garbage-in/out, or, as I always say, "Only as smart as you feed it.""; "Non-structured data is tricky." (https://www.usenix.org/conference/srecon26americas/presentation/redick, read 2026-10-25) [Q] The page links slides and shows a video section with no video URL; no transcript is linked. The slides were fetched but their text could not be decoded (https://www.usenix.org/system/files/sre26amer_slides_redick.pdf, read 2026-10-25) [N], so what the auto-remediator did, and what the speaker drew from it, is not known here.
- Inferred: this is the first source in three cycles in which an AI **acts** during an incident, and it is presented as a hazard ("over-eager", "overlapping automations"), not as the goal. The abstract names the failure as one of coordination between several actors, people and automations, during a P1, not as one wrong answer. Inferred for Karakuri: an agent that may remediate needs to know what other automation is acting on the same system and to yield to the incident commander. Phase 37 (a stop that reaches the running pass) is part of that; knowing about other actors is not on the roadmap. The Brex and HyperProbe sources of 2026-10-08 already said read-only first; nothing read this cycle contradicts that.
- **What would make this segment NOT want Karakuri.** Inferred, from the Redick abstract: one more automation that can act during a P1 is part of the problem it describes; a team that has lived through an over-eager remediator will want Karakuri read-only during incidents and will ask how it is stopped mid-action. Observed, from IBM's abstract: the vendors' own benchmark puts resolution at "13.8% of scenarios", which gives a team no reason to let any agent act.

### Self-hosters: an agent runtime whose scheduled turns skip approval

- **ZeroClaw issue #10968, "[Bug]: Unattended agent turns (cron, heartbeat, headless SOP, spawn_subagent) run with no ApprovalManager".** Who speaks: JordanTheJet, who opened it on 19 September 2026, and three maintainers or triagers who replied (Audacity88 twice, vyahhi). The page shows 4 comments and no hidden ones; the issue is open, labelled `priority:p1`, `risk:high` and `status:accepted`, and was moved to milestone v0.8.7 on 19 October 2026. ZeroClaw is, per the page, an open-source Rust agent runtime that runs agents from cron jobs, a daemon heartbeat, SOPs and sub-agents, with risk profiles that govern tool approval. Job: an operator schedules an agent and expects the approval rules they configured to hold when nobody is there. Observed: "`agent::run` builds an `ApprovalManager` only when its `interactive` argument is `true`:"; "An operator who configures a restrictive profile and then schedules that agent on cron has a control that does not apply."; the three options the author lists are "Auto-deny prompt-required tools on unattended paths.", "Let a risk profile declare what is permitted when no operator is present." and "Reuse `approval_route` so an unattended turn asks a configured approver channel."; from the triage reply, "Do not treat `approval: None` as proof that approval is unnecessary."; and vyahhi links #11335 because both "concern the same objective: preserve risk-profile approval semantics when no operator can answer." On 26 September Audacity88 reports the headless SOP path fixed; cron jobs, the heartbeat and unscoped sub-agents remain. (https://github.com/zeroclaw-labs/zeroclaw/issues/10968, read 2026-10-25) [Q] The author's note that fail-closed would break existing schedules is [E].
- **No GitHub Discussion was read.** The Discussions page of OpenClaw, the self-hosted agent the search pointed to, returned 404 (https://github.com/openclaw/openclaw/discussions?discussions_q=sort%3Atop, read 2026-10-25) [N]. One Hacker News search for self-hosted agent stories with more than 20 comments since April 2026 returned five hits, none about running an agent unattended, and no thread was opened from it (index, cited for nothing). The second independent thread asked for is missing.
- Inferred: the self-hoster of the 2026-10-08 report wanted scheduled work waiting in the morning; this thread shows what that costs a runtime. The approval model was built for a person at a terminal, and the scheduled path quietly treated "nobody can answer" as "no approval needed". The maintainers' three options are the same three decisions Karakuri has already made in other words: authority written into the run (rule 8 of `AGENTS.md`, `agent.AuthorityBounds`), a checkpoint that waits for a person, and since Phase 35 a checkpoint that ends as a rejection when nobody answers. Inferred, and not checked in code by this part: whether every unattended path in Karakuri (reconcile passes, delegated coding agents, sub-agents) carries the same bounds as an attended one is exactly the question this issue asks, and the feasibility part could test it.
- **What would make this segment NOT want Karakuri.** Observed: ZeroClaw is a single Rust runtime that already has cron, heartbeat, risk profiles and approval routing, and its maintainers accepted the gap as a P1 and are fixing it; a self-hoster on it has no reason to move. Inferred: the objections of 2026-10-08 (framework dependencies, "a directory of ticket files may be enough") still stand, and a Go server with a database is heavier than a single binary.

### Unattended coding agents, outside Spotify and after late 2025

- **Temporal, "Using coding agents on a migration: Three practices that mattered", Paul Oh and Chandler Ortman, 2 September 2026.** Who speaks: the two engineers who ran the migration, on their own company's blog. Job: let agents carry a migration and keep CI and review moving without a person at each step. Observed: "We ran unattended loops to handle continuous integration failures and review comments,"; "most of what they produced was hygiene that our own local tooling should have caught before the push."; "The automation was thinner than it sounds, too." (https://temporal.io/blog/using-coding-agents-on-a-migration-three-practices-that-mattered, read 2026-10-25) [Q] The fetch returned only these fragments about the unattended loops, and nothing on how much reviewing the loops cost, so that is not known here.
- **Stripe, Alistair Gray, Background Agents Summit session, recorded 6 May 2026.** Who speaks: a Stripe engineer on the team that built Minions. The page has a summary and a timestamped outline and **no transcript**; the words below are the page's, not the speaker's. Observed: "Thousands of agent-written PRs a week, with humans still reviewing the code."; "They are unattended coding agents that can start from Slack, Jira, or a web UI"; "Human developer infrastructure becomes the agent runtime."; "Deterministic steps handle linting, generated code, tests, and pull request setup." (https://background-agents.com/summit/sessions/alistair-gray/, read 2026-10-25) [Q, page summary only] The weekly volume is Stripe's figure as the page states it, not a measurement.
- Strength: two independent primary sources (Temporal, Stripe), one of them a summary page rather than the talk itself. Thin.
- Inferred: both put the weight outside the model. Temporal says the unattended loop mostly cleaned up what local tooling should have caught; Stripe's outline makes the developer environment the runtime and hands lint, tests and PR setup to deterministic steps. Inferred for Karakuri: a delivery stream gains more from running the repository's own checks before it pushes than from a better prompt, and a person still reviews every pull request even at Stripe's volume.
- **What would make this segment NOT want Karakuri.** Inferred: a company with Stripe's internal tooling builds the runtime on its own developer environment and needs no outside platform; a small team with one migration (Temporal) needs a loop that runs CI, not a server with standing objectives.

### A conference talk with a transcript (thin spot b): not filled

No talk with a transcript was read. The SREcon26 talk above links slides and no transcript; the Stripe session page has an outline and no transcript; an InfoQ search returned only previews of QCon San Francisco (16 to 20 November 2026) and QCon AI New York (15 to 16 December 2026), whose talks have not happened yet (search index, cited for nothing).

### Still thin (part 1)

- (a) An SRE agent acting during an incident: one abstract (Redick); its slides could not be decoded and no recording was read. No postmortem in which an agent acted was found.
- (b) A talk with a transcript about agents in production: none read. Next: the InfoQ presentation pages for QCon AI Boston 2026 (June), which may have transcripts by now, and QCon SF 2026 after 20 November.
- (c) Self-hosters: one issue thread (ZeroClaw #10968) and one 404 (OpenClaw Discussions). A second independent self-hoster is missing.
- (d) Unattended coding agents: Temporal (fragments only) and Stripe (page summary, no transcript). The Ona white paper (https://ona.com/files/background_agents_white_paper.pdf) was not opened [N].

**Not attempted this cycle; first for the next:**

- (e) Anyone running a STANDING (recurring, scheduled) coding or maintenance agent, speaking first-hand. ZeroClaw #10968 shows that runtimes schedule agents, not what a team ran on a schedule.
- (f) A regulated organisation (bank, insurer, health provider, public body) speaking first-hand about deploying agents.
- (g) Anthropic's report PDF, which the 2026-10-08 report never opened.
- (h) Product teams and large enterprises.
<!-- p1:end -->

## What they ask for and complain about

<!-- p2:start -->
Written by part 2 on 2026-10-25. It extends the 2026-10-08 section (items 1 to 6 there) and does not restate it. Every GitHub thread below was read through the api.github.com form with the WebFetch tool, which returns an extract written by a small model, not the raw JSON; so quotations are [Q] only where the extract gave them as quotations, and the rest are [E]. Counts are as returned on 2026-10-25. Session limit: 600 seconds, so thin spots (4) to (8) were not reached; see "Still thin (part 2)".

### A. The fixed five-minute ceiling is a client-library default, not an Ollama quirk (extends 2026-10-08 item 6)

- Observed: n8n #25360 has 10 comments and no reply from a maintainer account; the project's only answers are two bot comments, one of which threatens automatic closure after 14 days. (https://api.github.com/repos/n8n-io/n8n/issues/25360/comments?per_page=100, read 2026-10-25) [E]
- Observed: the ceiling hits other providers. cotsuka, 2026-03-21: "This seems to be the exactly same issue with the OpenAI node as well." cthorner, 2026-05-13 (2 reactions), on the Anthropic node: "Verified still present in 2.20.6 (latest stable)", with the cause that the proxy helper returns no agent unless a proxy is set, so the 300 s default applies; their workaround was lowering `thinkingBudget` to 8000. damscot, 2026-03-31, with `N8N_AI_TIMEOUT_MAX` and two Ollama settings set: "does not fix the issue, still killed after 5m". (same URL) [Q for quoted text, E for the cause]
- Observed: the open fix, pull request #32081 (cthorner, 2026-06-10), covers the Anthropic node only; the comment says Groq and OpenAI still need the same change. (same URL) [E]
- Inferred: this contradicts the 2026-10-08 framing of item 6 as a local-model problem. It is a timeout that a configuration variable claims to control and does not, and long thinking budgets on hosted models reach it too.

### B. A provider's tool-calling contract: "fixed" twice, still reported (extends 2026-10-08 item 5)

- Observed: n8n #22181 (Gemini 3 `thought_signature`) has 10 comments, none from a maintainer account. A user claims it works in 1.123.1 thanks to PR #22500; then ascendedent, 2025-12-17: "This is still happening for me even with 1.123.10", and javiercornejo, 2026-04-23: "this is still happening in newest release." (https://api.github.com/repos/n8n-io/n8n/issues/22181/comments?per_page=100, read 2026-10-25) [Q]
- Observed: n8n #9862 (models list on a non-OpenAI base URL) has 3 comments: maintainer Joffcom, 2024-06-26: "We should probably update the node to remove the filter if using a different base url which would solve the problem."; the same maintainer closed it on 2025-03-17 "To keep our backlog focused on current priorities"; a user, 2025-10-14: "This appears to be supported now (v1.113.3)." (https://api.github.com/repos/n8n-io/n8n/issues/9862/comments?per_page=100, read 2026-10-25) [Q]
- Inferred: this contradicts the 2026-10-08 conclusion that the hosted-provider breakage "was fixed in two weeks" and the self-hoster's request was dropped: the Gemini issue was closed as completed but is reported broken five months later, and the stale-closed one appears to have shipped quietly. A closed issue says little in either direction.

### C. The MCP argument leak: the full thread is much larger, and the fix is a schema filter (fills 2026-10-08 thin spot 3)

- Observed: n8n #21716, "MCP tool calls are failing due to new toolCallId, previous n8n worfklows won't run on update to most recent version!" (bsk-amboss, 2025-11-11), closed as completed on 2026-01-14, **77 comments**, against 27 on #21500 which the 2026-10-08 report ranked by. (https://api.github.com/repos/n8n-io/n8n/issues/21716, read 2026-10-25) [Q for the title]
- Observed, first 100,000 of about 158,700 characters of the comments: skware (12 reactions): "this makes n8n useless if you can't run mcp tools."; liaz-af (7 +1): "I think this should be considered a security issue by n8n.io team."; garyellow (6 +1): "The issue originates from how n8n merges parent node (AI Agent) parameters with tool call arguments"; agaldemas (5 +1) proposed sending only schema-defined parameters rather than filtering by a blocklist. A contributor-tagged account: "The fix was released with 2.3.1."; the bot: "Fix got released with n8n@1.123.11". After that, tobi-ahx: "I just tested it on v2.3.1: But i still get the same error". (https://api.github.com/repos/n8n-io/n8n/issues/21716/comments?per_page=100, read 2026-10-25) [Q for quoted text, E for the rest]
- Observed: pull request #23167 (TomTom101), "fix(McpClientTool Node): Sanitize MCP tool arguments based on schema", merged 2026-01-06. It keeps only arguments in the tool's input schema, passes all through when the schema allows additional properties, and sends none for a tool without parameters. (https://api.github.com/repos/n8n-io/n8n/pulls/23167, read 2026-10-25) [Q for the title, E for the body]
- Inferred: confirms and strengthens 2026-10-08 item 4 (about three times the comments once the duplicate is counted). Users framed it as a security issue, not only a breakage: a client that merges its own parameters into tool arguments sends data to a third-party server that the server never asked for.

### D. The checkpoint store's "SSL error" is idle connections and oversized state (fills 2026-10-08 thin spot 1, langgraph #3716)

- Observed, first 100,000 of about 125,600 characters: no MEMBER or COLLABORATOR reply is visible; vbarda (CONTRIBUTOR), 2025-03-06: "any chance you might be running out of disk space?". Causes users found: a cloud database dropping idle connections that psycopg reuses (vignesh-deltaforge, 2025-07-01; workaround a ping every 5 minutes); large base64 images or a 100,000-character prompt in state (kashyap-aditya, piersreynaud; fixed by storing a link instead); pool settings (jamesheavey, 2025-07-10, 3 +1: "we havent had any of the same errors since"). cloudlessdreams, 2025-06-04: "The SSL connection failure appears to be a symptom rather than the root cause." The most-reacted comment has 3 reactions; two of the top eight are "any update?" (https://api.github.com/repos/langchain-ai/langgraph/issues/3716/comments?per_page=100, read 2026-10-25) [Q for quoted text, E for the rest]
- Inferred: refines 2026-10-08 item 2. The pain is that a checkpointer which holds whole messages and images in a database row fails in the network layer, and the error names neither cause.
- langgraph #5672, comments 32 to 52: **not read**; the fetch stopped at comment 31 again. [N]

### Ranking (part 2, items read this pass plus 2026-10-08)

| Requirement or pain | Independent sources (count, list) | Strongest quote with URL | Counts shown | Karakuri addresses it |
|---|---|---|---|---|
| MCP client sends only the arguments the tool declared | 1 (n8n tracker; #21500 and #21716) | "this makes n8n useless if you can't run mcp tools." (n8n #21716 comments) | 77 + 27 comments; 12 reactions on one reply; 11 on #21500 | not checked |
| A configured timeout is honoured, for every provider | 1 (n8n tracker; #25360 plus two earlier issues cited 2026-10-08) | "does not fix the issue, still killed after 5m" (n8n #25360 comments) | 8 reactions, 10 comments | not checked |
| Durable state survives the database and stays small | 1 (langgraph tracker; #3716, #5672) | "The SSL connection failure appears to be a symptom rather than the root cause." (langgraph #3716 comments) | 53 comments, 12 reactions; top reply 3 | not checked |
| A provider's tool-calling change does not break the run | 1 (n8n tracker; #22181, #9862) | "this is still happening in newest release." (n8n #22181 comments) | 6 and 7 reactions | not checked |

All four rest on one vendor's tracker each, and three share the n8n population; no new independent population was added this pass. Confirms 2026-10-08 items 2 and 4; contradicts its item 5 ("fixed in two weeks") and narrows item 6 (not local models only). Karakuri's roadmap was not read this pass, hence "not checked".

### Still thin (part 2)

1. langgraph #5672 comments 32 to 52: still unread (fetch truncated twice).
2. The claude-code reaction count (#16157, "1499" unconfirmed in 2026-10-08): not attempted.
3. No production postmortem written by the team that ran the agent: not attempted.
4. Temporal community forum, microsoft/autogen, crewAIInc/crewAI, openai/codex: not attempted.
5. "The Agent That Burned $4,200 in 63 Hours": not attempted; no first-hand cost numbers this pass.
6. "78% of enterprises with pilots, under 15% at production scale": not attempted; it remains unverified.
7. The last 58,700 characters of n8n #21716 comments and the last 25,600 of langgraph #3716 comments were not read.
8. docs/roadmap.md was not read this pass; no "Karakuri addresses it" cell is filled.
<!-- p2:end -->

## Competitive teardown

<!-- p3:start -->
Every fetch in this part went through a tool that returns a model-written extract of the page, so nothing below is quoted verbatim and every external claim is graded [E]. Each long page was read for its first 100,000 characters only; the rest is not read. The window asked for (2026-04-25 to 2026-10-25) was not covered for any product; the part actually read is stated per product. Vendor release notes are claims, not measurements.

### 1. Claude Code

**(i) Shipped, part of window read.** Observed [E] in the CHANGELOG (https://raw.githubusercontent.com/anthropics/claude-code/main/CHANGELOG.md, read 2026-10-25): the first 100,000 of about 1,084,700 characters, versions 2.1.296 to 2.1.310. The file carries no dates, so these versions cannot be placed inside the window; April to September 2026 is still unread, as in 2026-10-08. Changed since 2026-10-08 (which read 2.1.285 to 2.1.294):
- Human approval: `/auto-mode-setup` and `decider: "person"` on `tool.check` (2.1.304); `CLAUDE_CODE_AUTO_MODE_MODEL` (2.1.302); auto-mode `PermissionRequest` hooks can no longer approve shell commands or subagents in Cowork (2.1.300); permission prompts name the plugin or skill behind a hook's `ask` (2.1.301).
- Durability and resume: a run of resume fixes (compaction summary lost on headless resume, 2.1.300; subagents forgetting recent work on resume, 2.1.304; `/rewind` attribution, 2.1.309).
- Budgets: `/usage` warnings (2.1.299), rate-limit end times in `/usage` (2.1.304), unattended retry controls `CLAUDE_CODE_RETRY_WATCHDOG_MAX_USAGE_LIMIT_WAIT_MS` (2.1.301) and a recheck interval (2.1.305); a plugin no longer keeps headless runs past `--max-turns` (2.1.304).
- Observability: GenAI semantic-convention attributes and a cache-write token split (2.1.300); `classifier_verdict`, `permission_mode` on `tool_decision` (2.1.301); a `claude_code.safety_event` for permission, auto-mode, hook and sandbox stops (2.1.307).
- MCP: protocol revision 2026-07-28 as default (2.1.300); allow rules scoped to the named server (2.1.300). A2A: nothing in the part read.
- Security and sandboxing: shell startup files no longer run outside the sandbox (2.1.309); `sandbox.allowNestedUserNamespaces` (2.1.305).
- Multi-agent: fork subagents on by default (2.1.302); teammates inherit the leader's deny rules (2.1.309).
- Scheduling: routine status and repeat-run fixes (2.1.302), scheduled-wakeup and DST fixes (2.1.299), `/loop` wakeup fixes (2.1.304).

**(ii) What it does that Karakuri does not.** Inferred from the list above: a model-backed classifier deciding tool calls in auto mode, with its verdict exported as telemetry; an OS-level sandbox for shell commands; in-session rewind of file changes.

**(iii) What Karakuri does that it does not.** Karakuri's README names an `act_with_notice` autonomy level whose autonomous actions all appear in the next digest, and one digest per twin per day ([ADR 016](../adr/016-earned-autonomy-and-digests.md), README "Standing Objectives" and "Digests"). Inferred: nothing in the Claude Code part read reports unsupervised work back as a digest, and its permission modes are chosen by a person, not widened by a track record. docs/roadmap.md was not read this pass, so no phase status is claimed.

**(iv) Complaints.** The two most-reacted open issues, from the search API (https://api.github.com/search/issues?q=repo:anthropics/claude-code+is:issue+is:open&sort=reactions&order=desc&per_page=2, read 2026-10-25, extract of bodies, comments not read) [E]: #45596 "Bring Back Buddy", opened 2026-04-09, 2,116 reactions (1,195 +1), 282 comments, asking to restore the `/buddy` companion removed in 2.1.97; #18435, multiple accounts in the Desktop app, opened 2026-01-15, 1,186 reactions (951 +1), 217 comments. Inferred: neither touches agent autonomy or durability; the top complaints are about the product's surface, so they say little for Karakuri.

**Rows.** Scheduled or standing agents: observed [E] (routines, scheduled wakeups, `/loop`). Earned autonomy: unknown; the part read shows modes set by a person and a classifier, no widening by record. A2A: unknown (not in the part read).

### 2. OpenAI Codex and the OpenAI Agents SDK

**(i) Shipped, part of window read.** Codex: not read. The releases list at per_page=100 exceeded the fetch tool's 10 MB limit; at per_page=15 (https://api.github.com/repos/openai/codex/releases?per_page=15, read 2026-10-25) the first 100,000 of about 8.16 million characters held only rust-v0.164.0 (2026-10-23) and its asset list, no notes [E]. Nothing is claimed about what Codex shipped.

Agents SDK, observed [E] (https://api.github.com/repos/openai/openai-agents-python/releases?per_page=100, read 2026-10-25, first 100,000 of about 529,600 characters): v0.22.1 (2026-09-08) to v0.24.0 (2026-10-15); 2026-04-25 to early September was not read.
- Human approval: deferred approval history persists across resumes and compaction waits for pending approvals (v0.24.0); approvals scoped to the owning agent (v0.23.0); conditional approvals checked against validated arguments (v0.22.3).
- Durability: Redis compaction history replaced atomically (v0.24.0); encrypted session history requires encrypted envelopes (v0.23.0).
- Budgets: bounded streaming callback backlogs, MCP listing page limits, a compaction rollback item budget (v0.23.0). No spend budget seen.
- Observability: default trace exports omit reasoning, rate-limited exports retried (v0.23.0); approved resumed tools attach to the resumed agent span (v0.24.0). No OpenTelemetry item seen.
- MCP: server-wide guardrails for MCP tools (v0.22.1); resumed MCP calls bound to their original recipients (v0.23.0); MCP annotations exposed (v0.24.0). A2A: none seen.
- Sandbox: configurable Unix-local environment isolation (v0.22.1); opt-in Docker removal protection (v0.23.0).
- Multi-agent: handoff and nested agent-tool state fixes (v0.22.1 to v0.24.0). Scheduling: none seen.

**(ii) What it does that Karakuri does not.** Inferred: approval state that survives a resume and compaction inside a library a developer embeds, and guardrails attached server-wide to MCP tools.

**(iii) What Karakuri does that it does not.** Inferred, from the SDK part read: no scheduler, no standing objective and no digest; it is a library, so cadence and reporting are left to the caller. Karakuri names standing objectives on a cadence and digests in its README.

**(iv) Complaints (Codex tracker).** https://api.github.com/search/issues?q=repo:openai/codex+is:issue+is:open&sort=reactions&order=desc&per_page=2, read 2026-10-25 [E], bodies summarised, comments not read: #8745 LSP integration, opened 2026-01-05, 610 reactions (505 +1), 72 comments; #9203 "Please make /undo back", opened 2026-01-14, 540 reactions (489 +1), 96 comments, because Codex has deleted untracked files or changed uncommitted work. Inferred: #9203 is the same need as Claude Code's `/rewind`, a way back from an agent's file changes. The Agents SDK tracker was not read.

**Rows.** Scheduled or standing agents: unknown (Codex not read; none in the SDK part read). Earned autonomy: unknown. A2A: unknown; none in the SDK part read.

### 3. LangGraph and LangSmith

**(i) Shipped, part of window read.** LangGraph, observed [E] (https://api.github.com/repos/langchain-ai/langgraph/releases?per_page=100, read 2026-10-25, first 100,000 of about 274,800 characters): 1.2.7 (2026-06-30) to 1.2.17 (2026-10-24). The 8 to 30 April gap left by 2026-10-08, and 2026-04-25 to 2026-06-29, are still not read.
- Human approval: `response_schema` on `interrupt()` (1.2.12, 2026-09-21); answered interrupts no longer reported in `get_state` (1.2.13, 2026-10-05).
- Durability: `prune` on PostgresSaver (checkpoint-postgres 3.2.0, 2026-10-14); subgraphs no longer rerun finished tasks on resume (1.2.16, 2026-10-16); no checkpoint saved past a failed DeltaChannel write (1.2.15).
- Budgets: only a recursion-limit fix (1.2.17). Observability: `trace_policy` on `add_node` (1.2.10, 2026-07-28). Scheduling: a cron's `end_time` can be cleared (sdk 0.4.3, 2026-08-19). MCP, A2A, sandbox: none in the part read.

LangSmith, observed [E] (https://docs.langchain.com/langsmith/changelog, read 2026-10-25, first 100,000 of about 235,900 characters): weekly entries from 2026-08-31 to 2026-10-19 only; April to August 2026 still unread, so the April to May gap is not filled.
- A2A: `SendMessage` honours `returnImmediately` so a caller can poll with `GetTask` (week of 2026-10-12).
- MCP: `query_runs` in the LangSmith MCP server (week of 2026-09-28); managed-tools gateways up to 10,000 tools (week of 2026-10-12).
- Sandboxes: memory-preserving resume (week of 2026-09-28), HTTP forward proxy and optional command history (week of 2026-10-05), customer-managed KMS keys and short-lived workload identity (week of 2026-10-12).
- Cost and access: per-call prompt-cache breakdown (2026-10-12 week); granular organisation permissions (2026-10-05 week); tracing-only API keys (2026-10-12 week).
- Scheduling: only cron-fired runs carry a `cron_id` (2026-10-12 week). Approvals: none seen.

**(ii) What it does that Karakuri does not.** Inferred: a hosted sandbox product with its own cost reporting, an A2A endpoint on the agent server, and a typed `response_schema` on a human interrupt.

**(iii) What Karakuri does that it does not.** Inferred: neither part read shows autonomy that widens with a track record or a daily digest of unsupervised actions; LangGraph's crons fire runs, but nothing read ties them to a standing objective that reconciles. Karakuri names both in its README.

**(iv) Complaints.** https://api.github.com/search/issues?q=repo:langchain-ai/langgraph+is:issue+is:open&sort=reactions&order=desc&per_page=2, read 2026-10-25 [E]: #3716, PostgresSaver "SSL error: bad length", 12 reactions, 53 comments; #5672, a cancelled run loses streamed state not yet checkpointed, 9 reactions, 52 comments. Both were read in part 2 of this report (section D). Inferred: the counts are an order of magnitude below the coding agents', so "most reacted" here means little. LangSmith has no public tracker that was read.

**Rows.** Scheduled or standing agents: observed [E], crons on the agent server (scheduled, not standing). Earned autonomy: unknown. A2A: claimed by the vendor [E] (LangSmith changelog, week of 2026-10-12).

### Still thin (part 3)

1. Claude Code: 984,700 characters of CHANGELOG unread, and the file is undated; release dates need the GitHub releases API or tags. The Agent SDK release notes were not read.
2. Codex: nothing read. The releases API needs a smaller page or the `gh` CLI, which was not available to this session.
3. Agents SDK: 2026-04-25 to early September not read; its own tracker not read.
4. LangGraph: 2026-04-08 to 2026-06-29 not read; LangSmith April to August not read.
5. No issue's comments were read; every complaint is from the opening post.
6. docs/roadmap.md not read; (iii) rests on README only.

Products 4 to 6 and the capability matrix: written by part 4, below.
<!-- p3:end -->

<!-- p4a:start -->
Every page in this part came through a fetch tool that returns a model-written extract, so nothing below is quoted verbatim and every external claim is [E]. All pages were read on 2026-10-25. Vendor notes are claims, not measurements. Part 4 ran out of time before reading any complaint thread in full, so every (iv) below is an opening-post extract, not the two full threads the brief asked for.

### 4. Temporal (durable execution)

**(i) Shipped, part of window read.** Server releases, observed [E] (https://api.github.com/repos/temporalio/temporal/releases?per_page=30, first 100,000 of about 432,500 characters): six releases, v1.32.0 (2026-09-11) to v1.30.8 and v1.31.4 (2026-10-12). 2026-04-25 to 2026-09-10 is not in the part read. v1.32.0: Standalone Nexus Operations in pre-release, Standalone Activities GA, and proactive activity cancellation when a workflow closes; later releases are Nexus callback fixes. No release in the part read names AI agents or OpenTelemetry. Temporal Cloud changelog, observed [E] (https://temporal.io/change-log/product-area/cloud, the page as loaded, which has a "Load More" button): 2026-04-30 principal attribution for workflow executions (pre-release); 2026-05-04 Worker Controller GA; 2026-05-05 task queue priority and fairness GA; 2026-05-14 External Storage (public preview); 2026-06-25 custom roles (pre-release); 2026-08-03 Serverless Workers on AWS Lambda (public preview); 2026-09-15 pay-as-you-go plan with a $0 minimum; 2026-09-18 Serverless Workers for AWS Bedrock AgentCore (pre-release); 2026-10-05 Serverless Workers on Cloud Run (public preview). The SDK repositories' releases were not read. Licence, observed [E] (https://raw.githubusercontent.com/temporalio/temporal/main/LICENSE): MIT, copyright Temporal Technologies 2025 and Uber 2020. DBOS, Inngest and Restate: not read.

**(ii) What it does that Karakuri does not.** Inferred: a general durable-execution engine with replay, multi-language SDKs, priority and fairness on task queues, and a managed cloud with serverless workers; the only agent-specific item in the part read is AgentCore as a worker host.

**(iii) What Karakuri does that it does not.** From README and Completed phases (docs/roadmap.md headings Phases 20, 21, 23, 27, 35): standing objectives that reconcile, digests, per-objective spend ceilings, provenance on observations, checkpoints that end when unanswered. Inferred: Temporal provides none of these as product features; it is the layer such features would be built on. Karakuri's Phase 11 "Distributed & Durable Execution" is its own resume path, not Temporal.

**(iv) Complaints.** https://api.github.com/search/issues?q=repo:temporalio/temporal+is:issue+is:open&sort=reactions&order=desc&per_page=2 [E], opening posts only: #680, waiting from workflow code on an external workflow's completion, opened 2020-08-18, 76 reactions, 15 comments; #3006, workflow pause and unpause, opened 2022-06-17, 69 reactions, 21 comments. Inferred: #3006 is the same need as Karakuri's Phase 37 ("Any Run Can Be Stopped at Any Time", Planned).

**Rows.** Durable resume: claimed by the vendor, yes. Earned autonomy: no (not a concept in the part read). A2A: unknown.

### 5. incident.io (AI SRE)

**(i) Shipped, part of window read.** https://incident.io/changelog [E]: the page carries no dates on its entries (slugs show 2021 to January 2025), so nothing can be placed in the window. Undated entries near the top: Investigations GA "powered by Nexus" (its agent, not Temporal's Nexus), posting root-cause hypotheses after an incident is declared; a claim that investigations became twice as fast (median 6.7 to 3 minutes to an accurate channel update, a vendor claim); "Extensions" to plug internal tools into the agent; an incident.io plugin bringing its skills into a coding harness; a hosted MCP server in public beta. Rootly, PagerDuty and Datadog Bits AI SRE: not read.

**(ii) What it does that Karakuri does not.** Inferred: an incident product with paging and a Slack-native channel, where the agent's investigation runs from declaration; a hosted MCP server for the incident record.

**(iii) What Karakuri does that it does not.** Inferred, against README and Phase 32 "The SRE Path, Actually Wired" (Completed, heading only): a self-hosted open-source runtime, earned autonomy on standing objectives, evidence export (Phase 31). Nothing in the part read describes autonomy that widens with a record.

**(iv) Complaints.** No public tracker or forum was found or read. Not filled.

**Rows.** SRE path: claimed by the vendor, yes. MCP server: claimed, yes (beta). Earned autonomy, A2A: unknown.

### 6. Langfuse (observability); governance product not read

**(i) Shipped, part of window read.** https://api.github.com/repos/langfuse/langfuse/releases?per_page=40 [E], first 100,000 of about 219,600 characters: v4.57.0 (2026-10-12) to v4.70.0 (2026-10-24) only; 2026-04-25 to 2026-10-11 is still unread, so the "whole window" was not covered. https://langfuse.com/changelog returned 404 [N]. In the part read: batch evaluation runnable through an MCP tool (v4.70.0); MCP tool listing LLM connections and managed skills loaded from Langfuse (v4.68.0); agent-session spans named after the agent (v4.59.0); OTel input media extraction (v4.57.0, v4.58.0); a billable-units ledger and prompt-cache hit rate (v4.60.0); score configs in annotation queues (v4.63.0).

Governance-only product: Zenity's guessed changelog URL (https://www.zenity.io/changelog) returned 404 [N]; Credo AI was not tried. Not filled.

**(ii) What it does that Karakuri does not.** Inferred: a trace store and UI that ingests OpenTelemetry from any agent, with human annotation queues and evaluators, and an MCP server over that data.

**(iii) What Karakuri does that it does not.** Inferred: Langfuse observes runs; it does not run them. Karakuri's checkpoints, spend ceilings (Phase 23) and evidence pack (Phase 31) act on the run. Karakuri emits GenAI telemetry (Phase 29) that a store such as Langfuse could ingest; whether it does was not tested.

**(iv) Complaints.** https://api.github.com/search/issues?q=repo:langfuse/langfuse+is:issue+is:open&sort=reactions&order=desc&per_page=2 [E], opening posts only: #5030, inline media preview in traces, opened 2025-01-14, 25 reactions, 9 comments; #11874, numeric score filters miss traces, opened 2026-02-04, 15 reactions, 5 comments. Inferred: low counts; neither touches agents.

**Rows.** OTel GenAI: claimed by the vendor, yes (ingest). MCP server: claimed, yes. Self-hosted licence: unknown (not read this pass).
<!-- p4a:end -->

## Capability matrix

<!-- p4b:start -->
Key: O = observed (in a page or file read), C = claimed by the vendor, U = unknown; then yes / partial / no. KARAKURI cells rest on docs/roadmap.md phase status (headings and status lines, not the full phase text, which part 4 had no time to read) except where a Go file is named. Only one Go file was read in full enough to cite: `internal/feature/reconcile/authority.go` (`effectiveAuthority`). `internal/core/environment/environment.go` was seen only through a search for `Trust`, which is not a reading. The brief asked for four; this falls short.

| Capability | KARAKURI | Claude Code | Codex / Agents SDK | LangGraph / LangSmith | Temporal | incident.io | Langfuse |
|---|---|---|---|---|---|---|---|
| Standing / scheduled objectives | O yes (Phase 20 Completed; reconcile package) | C yes, scheduled (routines) | U | C partial, crons | U | U | no (observes only, inferred) |
| Checkpoint enforced by runtime | O yes (authority.go writes bounds the loop enforces; Phase 13.5) | C yes (permission modes) | C yes (SDK approvals) | C yes (`interrupt`) | U | U | no |
| Earned autonomy | O partial: authority.go maps levels sense/propose/act_with_notice/act to bounds and never widens past the agent's own bounds | U | U | U | no | U | no |
| Per-run spend ceiling | O-heading yes (Phase 23) | C partial (usage warnings, `--max-turns`) | U | U | no | U | partial (cost ledger, not a ceiling) |
| Durable resume after restart | O-heading yes (Phase 11) | C partial (session resume) | C partial (session resume) | C yes (checkpointers) | C yes | U | no |
| Stop any run at any time | O-heading no (Phase 37 Planned) | U | U | U | C partial (cancel; pause is open #3006) | U | no |
| Audit log and evidence export | O-heading yes (Phases 19, 31) | U | U | U | C partial (principal attribution, pre-release) | U | U |
| MCP client | O-heading yes (Phases 28, 33) | C yes | C yes | U | U | U | U |
| MCP server | O-heading yes (Phase 34) | U | U | C yes | U | C yes (beta) | C yes |
| Agent-to-agent protocol | U (no Completed phase heading names A2A) | U | U | C yes (LangSmith `SendMessage`/`GetTask`) | no | U | no |
| OTel GenAI telemetry | O-heading yes (Phase 29) | C yes | no seen | U | no seen | U | C yes (ingest) |
| SRE / incident path | O-heading yes (Phase 32) | no | no | no | no | C yes | no |
| Self-hosted OSS licence | U this pass (licence file not read) | no (inferred) | C partial (SDK open source; not read) | U | O yes, MIT | no (inferred, SaaS) | U |
| Provenance labelling of third-party text | O partial-read yes (Phase 27; `TrustThirdParty` seen by search) | U | U | U | no | U | no |

### Still thin (part 4)

1. Every (iv) is an opening-post extract; no thread was read in full, and incident.io has no tracker read.
2. Temporal: server releases before 2026-09-11 and every SDK's releases unread; DBOS, Inngest, Restate not opened.
3. incident.io's changelog is undated; nothing is placed in the window. Rootly, PagerDuty, Datadog not read.
4. Langfuse: 2026-04-25 to 2026-10-11 unread; its licence unread. No governance-only product read (Zenity 404, Credo AI not tried).
5. KARAKURI column: one Go file read; the rest rests on phase headings, the same weakness as 2026-10-08. docs/research/discovery-2026-10-08.md's teardown and matrix were not read this part.
6. A2A is unknown for Karakuri itself; only LangSmith has a vendor claim.
<!-- p4a:end -->

## Capability matrix

<!-- p4b:start -->
_Pending: part 4._
<!-- p4b:end -->

## Technical frontier

<!-- p5:start -->
Part 5 in progress.
<!-- p5:end -->

## Governance and buying criteria

<!-- p6a:start -->
_Pending: part 6._
<!-- p6a:end -->

## Feasibility

<!-- p6b:start -->
_Pending: part 6._
<!-- p6b:end -->

<!-- p7f:start -->
_Pending: part 7._
<!-- p7f:end -->

## Fit and bets

<!-- p7a:start -->
_Pending: part 7._
<!-- p7a:end -->

<!-- p8:start -->
_Pending: part 8._
<!-- p8:end -->

## What changed since the last report

### How earlier proposals fared

<!-- p1h:start -->
Written by part 1 on 2026-10-25 from `gh pr list --state all --label karakuri:discovery --limit 50`, `git ls-remote --heads origin 'karakuri/discovery-*'` (no branch returned) and `docs/roadmap.md`.

- Pull request #148, "Discovery 2026-10-04: users, pain points, trends; proposes Phase 33 (MCP revision 2026-07-28)", head `karakuri/discovery-2026-10-04`: **merged** 2026-10-05. Produced Phase 33, "MCP After the Handshake", now **Completed**.
- Pull request #158, "Discovery 2026-10-08: deep cycle; proposes Phases 35 to 38", head `karakuri/discovery-2026-10-08`: **merged** 2026-10-09. Produced Phase 35, "An Unanswered Checkpoint Ends" (its rank 1, **Completed**); Phase 36, "A Provider's Refusal Is Not a Plan" (rank 2, **Completed**); Phase 37, "Any Run Can Be Stopped at Any Time" (rank 3, **Planned**; the roadmap now records it as the owner's requirement of 2026-10-10, no longer a bet); Phase 38, "What Was Required and Not Done" (rank 4, **Planned**). Its rank 5, an MCP server and tool inventory in the evidence record, is cited by no phase: a search of `docs/roadmap.md` for the report's name finds ranks 1 to 4 only.
- Phases 39 to 41 (Planned) do not cite a discovery report in their opening paragraphs; Phase 41 opens with the owner's request of 2026-10-14.
- **No phase was declined.** No discovery pull request was closed unmerged, and none was open when this part started.
- Pull request #195, this cycle's, head `karakuri/discovery-2026-10-25`: open. Its title, "docs(roadmap): discovery report in progress (discovery 2026-10-25)", was not imperative and named the wrong scope; part 1 repaired it on 2026-10-25 to "docs(research): add discovery report in progress (discovery 2026-10-25)". No pass ran between 2026-10-09 and this cycle: no report is dated in between.
- `docs/research/success-criteria-2026-10-14.md` is on `main` but is not a discovery report; it is not counted here.
<!-- p1h:end -->

<!-- p7b:start -->
_Pending: part 7._
<!-- p7b:end -->

## Sources

### Sources, part 1

<!-- s1:start -->
Every URL that part 1 read or tried, all on 2026-10-25. P = primary, S = secondary; X = extract as returned by the fetch tool.

- https://github.com/modelcontextprotocol/modelcontextprotocol/releases.atom: P, X [E]
- https://www.usenix.org/conference/srecon26americas/program: P, X (to character 100,000 of about 157,000) [Q]
- https://www.usenix.org/conference/srecon26americas/presentation/redick: P, X [Q]
- https://www.usenix.org/system/files/sre26amer_slides_redick.pdf: P, text not decodable [N]
- https://github.com/zeroclaw-labs/zeroclaw/issues/10968: P, whole thread as fetched (4 comments), X [Q]
- https://github.com/openclaw/openclaw/discussions?discussions_q=sort%3Atop: 404 [N]
- https://temporal.io/blog/using-coding-agents-on-a-migration-three-practices-that-mattered: P, X [Q]
- https://background-agents.com/summit/sessions/alistair-gray/: P (event page with summary and outline, no transcript), X [Q]
- https://ona.com/files/background_agents_white_paper.pdf: not opened [N]
- Hacker News and InfoQ searches: indexes only, cited for nothing.
<!-- s1:end -->

### Sources, part 2

<!-- s2:start -->
All read on 2026-10-25 with WebFetch; P = primary; X = extract written by the fetch tool's model, not raw text.

- https://api.github.com/repos/n8n-io/n8n/issues/25360/comments?per_page=100: P, X, all 10 comments [Q]
- https://api.github.com/repos/n8n-io/n8n/issues/22181/comments?per_page=100: P, X, all 10 comments [Q]
- https://api.github.com/repos/n8n-io/n8n/issues/9862/comments?per_page=100: P, X, all 3 comments [Q]
- https://api.github.com/repos/n8n-io/n8n/issues/21716: P, X [Q for the title]
- https://api.github.com/repos/n8n-io/n8n/issues/21716/comments?per_page=100: P, X, first 100,000 of about 158,700 characters [Q]
- https://api.github.com/repos/n8n-io/n8n/pulls/23167: P, X [E]
- https://api.github.com/repos/langchain-ai/langgraph/issues/3716/comments?per_page=100: P, X, first 100,000 of about 125,600 characters [Q]
- https://api.github.com/repos/langchain-ai/langgraph/issues/5672/comments?per_page=100: P, X, stopped at comment 31; comments 32 to 52 not read [N for those]
<!-- s2:end -->

### Sources, part 3

<!-- s3:start -->
All read 2026-10-25 by a fetch tool that returns extracts. P = primary, X = extract.
- https://raw.githubusercontent.com/anthropics/claude-code/main/CHANGELOG.md: P, X, first 100,000 of about 1,084,700 characters [E]
- https://api.github.com/repos/openai/codex/releases?per_page=100: not read, over the tool's 10 MB limit [N]
- https://api.github.com/repos/openai/codex/releases?per_page=15: P, X, first 100,000 characters, asset metadata only [E for the tag and date; N for contents]
- https://api.github.com/repos/openai/openai-agents-python/releases?per_page=100: P, X, first 100,000 of about 529,600 characters [E]
- https://api.github.com/repos/langchain-ai/langgraph/releases?per_page=100: P, X, first 100,000 of about 274,800 characters [E]
- https://docs.smith.langchain.com/changelog: redirect only, not read [N]
- https://docs.langchain.com/langsmith/changelog: P, X, first 100,000 of about 235,900 characters [E]
- https://api.github.com/search/issues?q=repo:anthropics/claude-code+is:issue+is:open&sort=reactions&order=desc&per_page=2: P, X [E]
- https://api.github.com/search/issues?q=repo:openai/codex+is:issue+is:open&sort=reactions&order=desc&per_page=2: P, X [E]
- https://api.github.com/search/issues?q=repo:langchain-ai/langgraph+is:issue+is:open&sort=reactions&order=desc&per_page=2: P, X [E]

Repository files read: this report's headings and the parts 1 and 2 markers, README.md (a search for autonomy and digest terms only). docs/research/discovery-2026-10-08.md was located but its teardown was not read in full; docs/roadmap.md was not read.
<!-- s3:end -->

### Sources, part 4

<!-- s4:start -->
All read 2026-10-25 by a fetch tool that returns extracts. P = primary.
- https://api.github.com/repos/temporalio/temporal/releases?per_page=30: P, first 100,000 of about 432,500 characters [E]
- https://raw.githubusercontent.com/temporalio/temporal/main/LICENSE: P [E]
- https://docs.temporal.io/cloud/release-notes: redirect only [N]
- https://temporal.io/change-log/product-area/cloud: P, entries as first loaded [E]
- https://api.github.com/search/issues?q=repo:temporalio/temporal+is:issue+is:open&sort=reactions&order=desc&per_page=2: P [E]
- https://incident.io/changelog: P, undated entries [E]
- https://langfuse.com/changelog: 404 [N]
- https://api.github.com/repos/langfuse/langfuse/releases?per_page=40: P, first 100,000 of about 219,600 characters [E]
- https://api.github.com/search/issues?q=repo:langfuse/langfuse+is:issue+is:open&sort=reactions&order=desc&per_page=2: P [E]
- https://www.zenity.io/changelog: 404 [N]

Repository files read: this report's part 3; docs/roadmap.md phase headings and statuses; internal/feature/reconcile/authority.go lines 1 to 60; a search of internal/core/environment/environment.go and internal/core/agent/agent.go. README.md and docs/research/discovery-2026-10-08.md were not read in full this part.
<!-- s4:end -->

### Sources, part 5

<!-- s5:start -->
_Pending: part 5._
<!-- s5:end -->

### Sources, part 6

<!-- s6:start -->
_Pending: part 6._
<!-- s6:end -->

### Sources, part 7

<!-- s7:start -->
_Pending: part 7._
<!-- s7:end -->

## Run log

<!-- rl1:start -->
- Part 1: started 2026-10-25 by a first session, which was cut off after the self-hoster segment; resumed at 17:03 UTC and finished on 2026-10-25 by a second session.
<!-- rl1:end -->
- Part 2: ran 2026-10-25 from 17:04 UTC in one 600-second session; filled thin spots 1 (except #5672) and 3; spots 2 and 4 to 8 not reached.
- Part 3: ran 2026-10-25 from 17:07 UTC in one 600-second session; `gh api` and `curl` were not permitted, so every page came through an extract tool; wrote products 1 to 3 in one commit, not one per product; the 2026-10-08 teardown was not read in full.
- Part 4: ran 2026-10-25 from 17:11 UTC in one 600-second session; extract tool only; wrote products 4 to 6 and the matrix in one commit after the in-progress marker; no complaint thread read in full, no governance product read, one Go file read.
