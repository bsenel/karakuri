# Market Discovery — 2026-10-04

Date read: 2026-10-04 (UTC).

**Method.** This report is written in four parts by separate sessions that share no memory; each part writes its own sections and leaves the rest marked as not yet written. Sources are public web pages fetched on the date above; every external claim carries its URL and the date read, and a page that was not fetched in the session is not cited. "Observed:" marks what a fetched page says; "Inferred:" marks a conclusion drawn here. No users were interviewed, nothing was built or tested, and vendor or analyst figures are reported as theirs, not as measurements. This is the first discovery report in `docs/research/`; the older `ai-use-cases-2026.md` is separate research and is not repeated here.

## Who the users are and what they are trying to do

**Limits of this section.** Four pages were fetched for it: one vendor survey, one vendor blog summarising another vendor's report, one vendor engineering post, and one arXiv abstract. Pages were read through a fetch tool that returns a model-written extract, so the figures below are as that extract reported them and were not checked against the raw page. Two of the four segments named in the brief (platform/SRE teams, and solo developers or open-source self-hosters) have **no direct evidence** here; they are listed so the gap is visible, not because anything was found.

### Segment 1 — Teams at small technology companies building agents into products

- Observed: LangChain's "State of Agent Engineering" survey (LangChain's own figures; 1,340 responses collected 18 November to 2 December 2025) reports respondents as 63% technology, 10% financial services, 6% healthcare, and 49% from companies under 100 employees; 57.3% say they have agents in production and a further 30.4% are developing with plans to deploy. (https://www.langchain.com/state-of-agent-engineering, read 2026-10-04)
- Observed: the same survey gives the leading use cases as customer service (26.5%), research and data analysis (24.4%) and internal workflow automation (18%). (same URL, read 2026-10-04)
- Job to be done: ship an agent that answers customers or does analysis reliably enough to leave running.
- Inferred: this is the best-evidenced segment only because it is the population a framework vendor's survey reaches. The sample is self-selected from one vendor's audience and says nothing about how common these users are in the market at large.

### Segment 2 — Large enterprises automating internal work, with governance as a condition

- Observed: in the LangChain survey, 67% of respondents at companies over 10,000 employees report agents in production against 50% at companies under 100; for large enterprises internal productivity is the top use case (26.8%), and security is a top concern at 24.9%. (https://www.langchain.com/state-of-agent-engineering, read 2026-10-04)
- Observed: an arXiv preprint, "Beyond Autonomy: A Dynamic Tiered AgentRunner Framework for Governable and Resilient Enterprise AI Execution" (Kai Pan, Rong Hou, submitted 11 May 2026), states that "Current large language model agent frameworks prioritize autonomy but lack the governability mechanisms required for enterprise deployment. High-risk write operations proceed without independent review, complex tasks lack acceptance verification". (https://arxiv.org/abs/2605.10223, read 2026-10-04) Only the abstract was read; it is a preprint by authors describing their own platform, not an independent study.
- Job to be done: let agents act on internal systems while a person can still review the risky writes and someone can later show what happened.
- Inferred: "regulated organisations needing audit evidence" from the brief sits inside this segment, but nothing fetched here names a regulator, an audit, or an evidence format. The link to audit evidence specifically is an inference, and thin.

### Segment 3 — Engineering teams delegating coding work

- Observed, second-hand: a blog post by Arcade.dev (a vendor of an MCP runtime, summarising Anthropic's "2026 State of AI Agents Report") says 91% of enterprises deploy AI coding tools in production and 57% deploy multi-step workflows. (https://www.arcade.dev/blog/5-takeaways-2026-state-of-ai-agents-claude/, read 2026-10-04) These are Anthropic's figures as relayed by Arcade; the original report was not fetched, and the Arcade post gives no sample size.
- Job to be done: hand a coding task to an agent and get back a change a human can review.
- Thin: one second-hand vendor source. Nothing here distinguishes long-running or standing coding agents from interactive coding assistants.

### Segment 4 — Platform and SRE teams

- No evidence found in this session. No fetched page describes SRE or incident-response teams running agents. Not searched for specifically; part 2 or a later pass should.
- Added by a later pass on 2026-10-04, which searched for this segment specifically. Evidence is still thin: one tool author and one vendor, no SRE team speaking for itself.
  - Observed: a Show HN post, "Nightwatch, The open-source, read-only AI SRE" (7 June 2026, 33 points, 10 comments), describes a tool that groups alert storms into incidents, flags noisy checks and has an agent investigate live systems for a root cause; its author writes "read-only for now, i don't trust it near prod yet and honestly neither should you." (https://hn.algolia.com/api/v1/items/48438180, read 2026-10-04)
  - Observed: Traversal, a vendor of an AI SRE product, writes on its blog (undated, titled for 2026) that "Speed to a wrong answer is negative value", that teams disabled AI runbooks which issued confident but incorrect commands during P1 incidents, and that high-impact changes should keep human approval. (https://www.traversal.com/blog/ai-in-incident-response-state-of-the-field-2026-sre, read 2026-10-04) Vendor claims; the post gives no sample and no named team for the disabled-runbook statement, and its adoption figures are other parties' (Gartner, Stanford HAI, Uptime Institute), not fetched here.
  - Job to be done: get from an alert storm to a probable cause faster, with evidence the on-call engineer can check, without the agent changing production by itself.
  - Inferred: both the open-source author and the vendor put the line in the same place, investigation yes, unattended remediation no. That is two sources with an interest in the answer, not a finding about SRE teams.

### Segment 5 — Solo developers and open-source self-hosters

- No direct evidence found in this session. Inferred, weakly: the issue trackers read for the next section (LangGraph, OpenAI Agents SDK) are where such users would appear, but the listings read do not identify who filed the issues.
- Added by a later pass on 2026-10-04. Still thin, and indirect: these are launch posts and bug titles, not self-hosters describing their work.
  - Observed: a Hacker News search for stories since April 2026 on self-hosted agents in production returned mostly launches of infrastructure around agents: "Runtime (YC P26) – Sandboxed coding agents for everyone on a team" (21 May 2026, 103 points), "Torrix, self hosted, LLM Observability (no Postgres, no Redis)" (13 May 2026, 74 points), "Cordium – FOSS self-hosted sandbox platform" (7 June 2026, 2 points), "strangeClaw – a self-hosted agent running inside a Firecracker microVM" (10 June 2026, 1 point), "FlowLink: MCP proxy blocking destructive AI agent commands" (26 May 2026, 1 point). (https://hn.algolia.com/api/v1/search?query=self-hosted%20agents%20production&tags=story&numericFilters=created_at_i%3E1775000000, read 2026-10-04) Only titles, points and dates were read from this listing.
  - Observed: n8n, a self-hostable workflow platform with an AI Agent node, has agent-related issues that read like those of people running it themselves against their own models: "Timeout setting does not work for Ollama node when it takes more than 5 minutes" (#25360, opened 5 February 2026, open) and "Custom OpenAI Endpoint does not work with non OpenAI models" (#9862, closed 17 March 2025). (https://github.com/n8n-io/n8n/issues?q=is%3Aissue+agent+sort%3Areactions-desc, read 2026-10-04) Titles only.
  - Job to be done, inferred: run an agent on one's own machines and models, contained in a sandbox, with few moving parts.
  - Inferred: the two best-received launches are about sandboxing and about observability that needs no extra database, which suggests containment and a small footprint matter to this audience. Points on a launch post measure interest in the post, not use.

## What they ask for and complain about

**Limits of this section.** Four pages were fetched: two GitHub issue listings sorted by reactions (primary, but only titles, numbers and dates were read, not the issue threads, and reaction counts were not returned), one vendor blog, and one secondary write-up of two vendor reports. Titles show what was asked for; they do not show why, or how many people asked.

### Human approval before consequential actions

- Observed: in the OpenAI Agents SDK tracker sorted by reactions, four of the 25 issues listed are requests for the same thing: "Human-In-The-Loop Architecture should be implemented on top priority!" (#636, closed 29 January 2026), "Human in the loop" (#109, closed 2 November 2025), "human-in-the-loop" (#378, closed 16 June 2025) and "Feature Request: Support Manual Interruption of OpenAI Agent During Execution" (#329, closed 29 January 2026). (https://github.com/openai/openai-agents-python/issues?q=is%3Aissue+sort%3Areactions-desc, read 2026-10-04)
- Observed: LangChain's engineering post "The Runtime Behind Production Deep Agents" (Sydney Runkle and Vivek Trivedy, 20 April 2026) puts the requirement as: "Before the agent executes a consequential action (sending an email, executing a financial transaction, deleting files), you want a human to see exactly what it's about to do." (https://www.langchain.com/blog/runtime-behind-production-deep-agents, read 2026-10-04) Vendor post describing its own product.
- Observed: 59.8% of LangChain's survey respondents say they rely on human review, against 53.3% using LLM-as-judge. (https://www.langchain.com/state-of-agent-engineering, read 2026-10-04; LangChain's figure)
- Inferred: all four SDK issues are closed, which suggests the request was met there; the threads were not read, so whether they closed as shipped or as duplicates is not known.

### Resuming after a pause or a crash without losing or repeating work

- Observed: LangGraph issues matching interrupt, checkpoint or resume include "Run Cancellation Causes Loss of Streamed State Not Yet Persisted as a Checkpoint" (#5672, opened 25 July 2025, open), "Unable to resume multiple interrupts from a single graph invoke" (#4028, closed 28 April 2025), "Resume value get reused when resuming parent graph which has subgraph with multiple interrupts" (#2870, closed 15 January 2025) and "Bug: AsyncRedisSaver._aload_pending_sends fails during HIL workflow resumption" (#5074, closed 12 June 2025). (https://github.com/langchain-ai/langgraph/issues?q=is%3Aissue+is%3Aopen+interrupt+OR+checkpoint+OR+resume+sort%3Areactions-desc, read 2026-10-04) The query asked for open issues but the listing returned closed ones too, so the filter did not apply as written.
- Observed: the same listing has several checkpoint-storage complaints against Postgres: an SSL "bad length" error "encountered across multiple version" (#3716, opened 6 March 2025, open), a request for a "Driver abstraction for checkpoint-postgres" (#7692, opened 3 May 2026, open) and for a "Configurable PostgreSQL schema" (#7345, opened 30 March 2026, open). (same URL, read 2026-10-04)
- Observed: Inngest's blog (Charly Poly, 19 February 2026; Inngest sells durable execution) describes the same pain from the vendor side: "if your serverless function times out, you lose everything", "re-execution means re-paying for tokens", and that a human "might respond in seconds, hours, or days". (https://www.inngest.com/blog/durable-execution-key-to-harnessing-ai-agents, read 2026-10-04)
- Observed: the LangChain post says "A crash, deploy, or transient failure anywhere in that loop shouldn't erase the work leading up to it." (https://www.langchain.com/blog/runtime-behind-production-deep-agents, read 2026-10-04)
- Inferred: pausing for a person and surviving a restart are the same requirement seen from two sides, and the bugs cluster where they meet (multiple interrupts, subgraphs, the storage driver).

### Output quality, and not being able to measure it

- Observed: LangChain's survey names quality as the top barrier to production (33%, described as "accuracy, relevance, consistency") and latency second (20%); 52.4% run offline evaluations and 37.3% online. (https://www.langchain.com/state-of-agent-engineering, read 2026-10-04; LangChain's figures)
- Observed: a secondary write-up by The Agent Report (23 May 2026) gives the quality figure as 32% rather than 33% and adds that large enterprises highlight "hallucinations and consistency of outputs". (https://the-agent-report.com/2026/05/state-of-agent-engineering-2026-langchain-datadog/, read 2026-10-04) The one-point difference between the two readings was not resolved.

### Provider failures, mainly rate limits

- Observed, second-hand: The Agent Report attributes to Datadog's "State of AI Engineering" (described as covering 1,000+ production customers) that about 5% of LLM call spans returned errors in February 2026 and that rate-limit errors account for roughly 60% of LLM call failures. (https://the-agent-report.com/2026/05/state-of-agent-engineering-2026-langchain-datadog/, read 2026-10-04) These are Datadog's figures from its own customers as relayed by a third party; Datadog's report was not fetched.
- Observed: Inngest lists "rate limits, timeouts, network errors, authentication expiration, and server errors" as the ways external APIs fail. (https://www.inngest.com/blog/durable-execution-key-to-harnessing-ai-agents, read 2026-10-04)

### Integration, data access, security and compliance

- Observed, second-hand: Arcade.dev's summary of Anthropic's report says "46% of respondents cite integration with existing systems as their primary challenge", 42% data access and data quality, and 40% security and compliance concerns. (https://www.arcade.dev/blog/5-takeaways-2026-state-of-ai-agents-claude/, read 2026-10-04) Anthropic's figures relayed by a vendor whose product addresses exactly these problems; no sample size given.
- Observed: the most-reacted issue listed in the OpenAI Agents SDK tracker is "Add MCP support" (#23, closed 26 March 2025); others ask for "A2A (Agent2Agent) support" (#472, closed 5 August 2026) and "Add OpenTelemetry format support for traces" (#18, closed 12 March 2025). (https://github.com/openai/openai-agents-python/issues?q=is%3Aissue+sort%3Areactions-desc, read 2026-10-04)
- Observed: the LangChain post lists tenant isolation ("User A's run should only touch User A's threads, and only read User A's memories"), sandboxed code execution, and scheduled runs without a user trigger among production requirements. (https://www.langchain.com/blog/runtime-behind-production-deep-agents, read 2026-10-04)

### Seeing what the agent did

- Observed: 89% of LangChain's respondents report having observability and 62% detailed tracing. (https://www.langchain.com/state-of-agent-engineering, read 2026-10-04; LangChain's figures, and LangChain sells an observability product)
- Observed: "Enhance `on_tool_start` Hook to Include Tool Call Arguments" (#252, closed 4 March 2026) and "Agent.as_tool hides nested tool-call events" (#864, closed 29 January 2026) in the OpenAI Agents SDK tracker ask for visibility into tool calls. (https://github.com/openai/openai-agents-python/issues?q=is%3Aissue+sort%3Areactions-desc, read 2026-10-04)

### Added by a later pass: forum threads and one self-hostable platform's tracker

Six further pages were fetched on 2026-10-04 to cover gaps named under "Not found" below (forum threads, SRE, self-hosters). Same limits: model-written extracts, titles rather than threads for the tracker, and short comment threads.

- **Timeouts on slow or local models.** Observed: three of the 25 n8n agent issues listed are the same complaint: "AI nodes timeout after 5m" (#11886, closed 15 January 2026), "Timeout setting does not work for Ollama node when it takes more than 5 minutes" (#25360, opened 5 February 2026, open) and "OpenAI Chat Model node hard-cuts at 300 s despite timeout settings" (#24496, closed 5 March 2026). (https://github.com/n8n-io/n8n/issues?q=is%3Aissue+agent+sort%3Areactions-desc, read 2026-10-04)
- **A failing tool should be reported to the agent, not end the run.** Observed: "AI Agent node: Tool node errors fail workflow instead of returning error to agent" (#24042, opened 8 January 2026, open). (same URL, read 2026-10-04)
- **MCP client and provider compatibility.** Observed: "MCP Client Tool sends additional parameter, that is not expected by MCP Server" (#21500, closed 16 March 2026), "MCP client does not support multi-parameter MCP server" (#21569, closed 9 December 2025), "AI Agent to MCP Client Node Invalid Request Issue" (#15603, closed 13 February 2026); and per-provider tool-calling breakage: "Google Gemini: Support for thought_signature missing in Tool Calls" (#22181, closed 5 December 2025), "DeepSeek AI Agent node fails with 400 error when using tools with thinking mode" (#29119, closed 17 August 2026), "Mistral Cloud Chat Model fails in AI Agent" (#37453, opened 31 August 2026, open). (same URL, read 2026-10-04) Inferred: by a count of the titles, roughly 12 of the 25 listed are a provider or MCP server behaving differently from what the client assumed.
- **Tool calls missing from memory.** Observed: the first issue listed is "AI Agent doesn't store the Tool usages in memory" (#14361, opened 2 April 2025, open). (same URL, read 2026-10-04)
- **Secrets, licence and policy checks for team coding agents.** Observed, in the Hacker News thread on Runtime (sandboxed coding agents for a team; 21 May 2026, 103 points, 30 comments): mritchie712, "Checked license it said copyrighted which makes this unsuable for me."; theahura, "keys are tricky...tools that do something like 'read a key from disk.'"; vorsken, "the generated code still needs to pass security policy checks before it merges."; cvolante, "If marketing sends me a pull request and I hate the code, what's the flow like for me to fix it?" (https://hn.algolia.com/api/v1/items/48225040, read 2026-10-04) Four individual comments, quoted as the extract gave them, ellipses included; not a count of opinion.
- **An SRE agent lacks context about neighbouring services.** Observed, in the Nightwatch thread: tam159, "LLM may not have enough context about the related services to investigate the errors", who suggests feeding it each service's README. (https://hn.algolia.com/api/v1/items/48438180, read 2026-10-04) One commenter in a ten-comment thread.
- **A confident wrong answer during an incident.** Observed: Traversal's claim, as under Segment 4, that teams turned off AI runbooks which issued confident but incorrect commands in P1 incidents, and that existing tools "stop at correlation". (https://www.traversal.com/blog/ai-in-incident-response-state-of-the-field-2026-sre, read 2026-10-04) Vendor claim with no named source.
- **Narrow, short-lived credentials and a check before the action.** Observed: a Cloud Security Alliance research note of 20 July 2026 on an intrusion at Hugging Face (detected the week of 14 July 2026, disclosed 16 July 2026), in which, by the note's account, an autonomous agent run by the attacker performed "more than 17,000 logged attacker actions", recommends "short-lived, per-task credentials rather than long-lived service accounts" and runtime controls that "intercept an agent's proposed action before execution, evaluate it against context-aware policy". (https://labs.cloudsecurityalliance.org/research/csa-research-note-huggingface-autonomous-agent-breach-202607/, read 2026-10-04) This is advice from a security body after an attack that used an agent; it is not a complaint from someone operating one, and Hugging Face's own disclosure was not fetched.
- Inferred: these additions repeat the earlier headings (approval before action, visibility into tool calls, integration) more than they add new ones. What is new is small and concrete: a fixed five-minute ceiling on model calls, a tool error ending the whole run, and the licence of the platform itself as a reason to reject it.

### Not found

- Nothing fetched speaks to cost as a current complaint beyond The Agent Report's remark that cost worries have "dropped significantly" (same URL as above, read 2026-10-04); no figure was given.
- No forum threads, changelogs or practitioner write-ups by non-vendors were read. No complaint here comes from an SRE team, a regulated organisation speaking for itself, or an identified self-hoster. (The later pass above read two short forum threads; the rest of this sentence still holds: no SRE team, no regulated organisation and no postmortem by someone running agents was read.)
- A search result claimed that 78% of enterprises have agent pilots and under 15% reach production scale; the page behind it was not fetched, so it is recorded here only as an unverified lead.

## Trends

**Scope and limits of this section.** This is the first discovery report, so there is no previous one to compare against; the aim was the last six months (April to October 2026). It falls well short of that. Eight fetches were made and only five returned usable content; of the five, two changelogs (Claude Code, LangSmith) returned only their most recent weeks, so nothing below describes what those products shipped between April and mid-August 2026. Two fetches were answered with a redirect to another host and one with HTTP 404 (listed under Sources). **No page was read for:** durable-execution runtimes (Temporal, Inngest, Restate), OpenAI's Codex, any hosted agent platform other than LangSmith, any AI SRE or incident tool, any agent-to-agent protocol, or any practitioner write-up. Pages were read through a fetch tool that returns a model-written extract, so quotations are as that extract gave them and were not checked against the raw page. Nothing here was tested.

### Comparable products

Only two products were actually read. One line each.

- **Claude Code** (Anthropic's coding-agent CLI, with background sessions and cloud-hosted scheduled "routines"). Observed, from its own changelog, versions 2.1.283 to 2.1.289, dated 25 September to 3 October 2026 (https://code.claude.com/docs/en/changelog, read 2026-10-04):
  - 28 September 2026 (2.1.284): "Changed interactive terminal and VS Code sessions to start in auto mode when no permission mode is configured, on every plan and provider"; 25 September 2026 (2.1.283): the same default for `claude -p` and Python Agent SDK sessions on third-party providers or with telemetry off.
  - 29 September 2026 (2.1.285): background shell commands now "stop after a time limit" (default 30 min, max 2 h); routine runs "whose cloud session never started" were "showing as Succeeded" and "now show as Failed". 30 September 2026 (2.1.286): fixed "background jobs showing done while waiting for your approval"; a late scheduled run now reads "Due".
  - 1 October 2026 (2.1.287): PreToolUse and PermissionRequest hooks that were "skipped when matching them failed" now block the call; a count such as "2 of 5" was added when permission requests stack up.
  - 28 September 2026 (2.1.284): MCP tool, WebFetch and WebSearch outputs added to the `tool.output` OpenTelemetry span event; 2 October 2026 (2.1.288): fixed permission asks that ended unanswered "emitting no `tool_decision` event".
  - Inferred: a widely used coding CLI now runs scheduled, unattended work and defaults to a classifier-gated autonomous mode, so "an agent that runs on a schedule" is no longer something Karakuri offers alone. The fixes that week are about a run's reported state being wrong (succeeded when it never started, done when it was waiting for approval) and about approval decisions missing from telemetry. Inferred: Karakuri's own checkpoint, digest and audit paths are exposed to the same class of fault, and what it has that this changelog does not show is a recorded ladder of earned autonomy and an evidence export. Whether Claude Code has equivalents elsewhere was not checked; only ten days of its changelog were read.
- **LangSmith Deployment / Managed Deep Agents** (LangChain's hosted runtime for LangGraph agents). Observed, from its changelog, entries for the weeks of 24 August to 21 September 2026 (https://docs.langchain.com/langsmith/changelog, read 2026-10-04):
  - Week of 14 to 21 September 2026: "Managed Deep Agents can discover OAuth settings from an MCP server's authentication challenge"; "Scale-to-zero deployments reject rollbacks to revisions below LangGraph API 0.13.0".
  - Week of 7 to 14 September 2026: "After a Managed Deep Agent run paused for credentials resumes, the Slack connect card is removed".
  - Week of 31 August to 7 September 2026: "LangSmith MCP connectors use Client ID Metadata Documents when supported".
  - Week of 24 to 31 August 2026: "Dedicated deployments now run at least two replicas".
  - Inferred: the hosted competitor's recent work is on MCP authorization (OAuth discovery, client metadata documents) and on runs that pause for a credential and resume, delivered through Slack. Inferred: authenticated remote MCP servers are becoming the normal case, and Karakuri's MCP tool support should be checked against that; this report did not check what Karakuri's MCP client supports for OAuth.
- **Not read:** OpenAI Codex (the changelog URL redirected to a different host, which was not followed), incident.io (HTTP 404), and every durable-execution runtime and AI SRE tool. No line is written for them rather than writing one from memory. Segment 4 of this report (platform and SRE teams) therefore still has no evidence.

### Standards and protocols

- **OpenTelemetry GenAI semantic conventions: still not stable, and they moved.** Observed, from a blog post by John Hodge dated 17 July 2026, a single individual's summary and not the OpenTelemetry project's own page (https://john-hodge.com/blog/opentelemetry-genai-semantic-conventions/, read 2026-10-04): "No GenAI-specific span, event, metric, or attribute in the dedicated repository is marked Stable"; the conventions moved to a dedicated repository, `open-telemetry/semantic-conventions-genai`, in June 2026, deprecated in the main repository at v1.42.0 and removed by v1.43.0; the new repository has no versioned releases yet; v1.41.0 (April 2026) restructured agent spans and added reasoning tokens; v1.40.0 (February 2026) added retrieval spans, cache attributes and agent versioning; no stabilization date is announced.
  - Inferred: Karakuri's GenAI telemetry (Phase 29) targets a specification that changed shape in April 2026 and changed home in June 2026. Which convention version Karakuri emits was not checked in this pass. The state as of 17 July 2026 may have changed since; the repository itself was not fetched.
- **MCP.** Observed: the draft specification changelog page contains only "Changes since the most recent release will accumulate here." (https://modelcontextprotocol.io/specification/draft/changelog, read 2026-10-04). Inferred, weakly: no changes are pending in the draft beyond the latest release, or the page is not kept current; the two cannot be told apart from this page, and the released revisions were not fetched. Observed indirectly: Claude Code on 1 October 2026 added "URL prompts from MCP servers on the 2025-11-25 protocol" (https://code.claude.com/docs/en/changelog, read 2026-10-04), which shows a client still adopting features of a revision named 2025-11-25. Whether a newer MCP revision was released in 2026 is **not established** here.
- **Agent-to-agent protocols.** Nothing fetched. The only evidence in this report is part 1's observation that the OpenAI Agents SDK closed its "A2A (Agent2Agent) support" issue on 5 August 2026.

### Governance and regulation

- **EU AI Act: high-risk obligations postponed; transparency duties not.** Observed, from a Gibson Dunn client alert dated 27 May 2026, a law firm's summary and not the legal text (https://www.gibsondunn.com/eu-ai-act-omnibus-agreement-postponed-high-risk-deadlines-and-other-key-changes/, read 2026-10-04): a provisional political agreement on the Digital Omnibus was reached on 6 May 2026 and confirmed by the Council on 13 May 2026; obligations for stand-alone high-risk systems (Annex III) move from 2 August 2026 to 2 December 2027, and for systems embedded in regulated products (Annex I) from 2 August 2027 to 2 August 2028; Article 50 transparency obligations still apply from 2 August 2026, with a grace period to 2 December 2026 for the watermarking requirement on existing systems. The extract says the alert does not mention any change to the logging, human-oversight or record-keeping duties themselves.
  - Not verified: the alert predates formal adoption, which it expected before 2 August 2026. A search-result snippet said the Council gave final approval on 29 June 2026; that page was not fetched, so adoption and entry into force are unconfirmed here.
  - Inferred: the date by which a deployer of a high-risk system would need audit-ready records and human-oversight design moved about sixteen months out, to December 2027. Inferred: that weakens "a deadline is coming" as a reason to adopt Karakuri's audit log and evidence export in 2026, and leaves the requirement itself in place. Whether any Karakuri user operates a high-risk system at all is unknown; part 1 found no user naming a regulator.
- No other jurisdiction or standard (US state laws, UK, ISO/IEC 42001, NIST) was read.

### Practitioner sentiment

- **Not sourced.** One search was run and no result page was fetched, because the fetch budget was spent. The snippets spoke of evaluation sets built before launch, a human on risky steps, tight permissions, retry loops burning token budgets, and moving from shadow mode to autonomy gradually; they come from vendor and consultancy blogs, were not read, and are recorded only as unverified leads. Part 1's pain-point section remains this report's only evidence of what people who run agents report.

### Added by a later pass: gaps named above

A later session on 2026-10-04 fetched the pages the first pass could not, in the brief's priority order. Same limits: pages read through a fetch tool, nothing tested. Items already written above are not repeated; where an item below settles something the first pass left open, it says so.

- **MCP revision 2026-07-28 exists, and it is a breaking one.** This settles the "not established" line under Standards above. Observed, from the specification's own "Key Changes" page for revision 2026-07-28, which lists changes since 2025-11-25 (https://modelcontextprotocol.io/specification/2026-07-28/changelog, read 2026-10-04). The page gives the revision name but no separate publication date, so 28 July 2026 is taken from the name.
  - Stateless core: "Make MCP stateless: remove the `initialize`/`notifications/initialized` handshake. Every request now carries its protocol version and client capabilities in `_meta`"; servers MUST implement a new `server/discover` RPC; protocol-level sessions and the `Mcp-Session-Id` header are removed from Streamable HTTP; `ping` and `logging/setLevel` are removed.
  - Server-to-client requests replaced: a "Multi Round-Trip Requests" pattern in which a server returns `resultType: "input_required"` and the client retries with `inputResponses`, replacing server-initiated `roots/list`, `sampling/createMessage` and `elicitation/create`. Every result now carries a required `resultType`.
  - No resumability: "A broken response stream loses the in-flight request; clients **MUST** re-issue it as a new request with a new request ID".
  - Tasks moved "out of the core protocol and into an official extension (`io.modelcontextprotocol/tasks`)", polled with `tasks/get`.
  - Authorization: clients MUST validate a present `iss` against the recorded issuer, MUST key persisted credentials by issuer and MUST NOT reuse them with another authorization server; Dynamic Client Registration is deprecated "in favor of Client ID Metadata Documents".
  - Deprecated with a minimum twelve-month window: Roots, Sampling, Logging, and the HTTP+SSE transport. The page's suggested replacement for Logging is to "log to `stderr` (stdio) or use OpenTelemetry"; it also documents trace-context propagation in `_meta` (`traceparent`, `tracestate`, `baggage`).
  - Relevance to Karakuri (inferred): an MCP client written against 2025-11-25 or earlier opens with a handshake that a 2026-07-28-only server no longer has, and may rely on sessions, sampling or elicitation callbacks that are removed or deprecated. Which revision Karakuri's MCP client speaks, and whether it falls back, was not checked in this pass. The trace-context keys would let a Karakuri GenAI trace continue into an MCP server; that is a possibility, not something examined.
- **OpenTelemetry GenAI conventions: the dedicated repository still has no release.** Observed: the releases page of `open-telemetry/semantic-conventions-genai` shows "There aren't any releases here." (https://github.com/open-telemetry/semantic-conventions-genai/releases, read 2026-10-04). This confirms from the project's own repository, as of today, what the first pass had only from an individual's blog post of 17 July 2026. The page says nothing about stability levels; those were not re-read.
  - Relevance to Karakuri (inferred): there is still no versioned target to pin Phase 29's telemetry to, so a claim of conformance can only name a commit or the last main-repository version.
- **EU AI Act Digital Omnibus: in force.** This settles the "not verified" line under Governance above. Observed, from an Orrick client alert of July 2026, a law firm's summary and not the legal text (https://www.orrick.com/en/Insights/2026/07/EU-AI-Act-Update-Digital-Omnibus-Finalizes-8-Compliance-Changes, read 2026-10-04): the final text was published in the Official Journal and is in force as of 29 July 2026, as the fetch extract reported the alert; high-risk dates are 2 December 2027 (Annex III) and 2 August 2028 (Annex I), with 2 August 2030 for systems used by public authorities. The alert's other listed changes include new prohibitions, a "small mid-cap" category with compliance relief, and stronger AI Office enforcement powers. It does not say whether the human-oversight, logging or record-keeping duties themselves changed; the dates of the Parliament and Council votes are not in it. The Official Journal text was not fetched.
  - Relevance to Karakuri (inferred): the first pass's reading stands and is now firmer: the high-risk deadline is December 2027 in law, not only in a provisional agreement.
- **Agent-to-agent: A2A moved under the same foundation umbrella as MCP's neighbours.** Fills the "nothing fetched" line under Standards above. Observed, from the Agentic AI Foundation's blog post of 17 August 2026 (https://aaif.io/blog/a2a-joins-aaif, read 2026-10-04): A2A joins the foundation as a hosted project, with "a neutral home where the community can collaborate on the standards that will enable agents from different vendors, frameworks, and organizations to work together"; the post places MCP at the layer of "how agents connect to and interact with tools, data sources, applications, and services" and A2A at the layer where agents "discover one another, communicate, delegate tasks, and exchange results across systems and organizational boundaries". Observed, from a Linux Foundation press release of 9 April 2026, which is before this report's three-month window and is the project's own promotion (https://www.linuxfoundation.org/press/a2a-protocol-surpasses-150-organizations-lands-in-major-cloud-platforms-and-sees-enterprise-production-use-in-first-year, read 2026-10-04): version 1.0 brings "Signed Agent Cards for cryptographic identity verification" and "enterprise-grade multi-tenancy"; the release claims over 150 supporting organizations and integration into Azure AI Foundry, Copilot Studio and Amazon Bedrock AgentCore Runtime. Those are the foundation's figures, not a measure of use. The A2A specification and its changelog were not fetched; a search snippet gave March 2026 for 1.0 and May 2026 for 1.0.1, unverified.
  - Relevance to Karakuri (inferred): Karakuri has MCP tools and no agent-to-agent surface. Nothing read here shows a user asking a standing-objective platform for A2A; the only demand signal in this report is the OpenAI Agents SDK issue closed on 5 August 2026. Signed agent cards are an identity mechanism that an audit log could record, if A2A were ever added.
- **AI SRE: an incident vendor ships investigation that starts at the alert, and stops before the fix.** Fills part of the "not read" line under Comparable products above. Observed, from Better Stack's own changelog post of 1 October 2026 (https://betterstack.com/community/blog/changelog-17-wake-up-your-ai-agent-before-getting-paged/, read 2026-10-04): "AI SRE starts investigating the moment the incident happens. It checks what's failing and what changed around the start time, then posts a short summary to the incident timeline and Slack."; users are told to "Focus on decision making and the fix itself, and leave the data analysis to AI SRE."; investigations can be shaped with custom prompts and draw context from connected tools (GitHub, GitLab, Sentry, Grafana, Datadog, Linear, Notion). The fetch extract found no mention of autonomous remediation, an approval step, or MCP in the post. One vendor, one changelog entry.
  - Not read: Dynatrace (a snippet dated its autonomous SRE agents to 27 July 2026), Splunk, incident.io, Datadog, Azure SRE Agent. Recorded as unverified leads only.
  - Relevance to Karakuri (inferred): this is a third source, after Nightwatch and Traversal in part 1, that draws the line at investigate-and-summarise. Karakuri's SRE path (Phase 32) would be compared on what these do by default: start on the alert, post into the incident timeline and chat. What Karakuri adds beyond them, a checkpoint before a remediation and a record of it, is exactly the step this vendor leaves to the human; whether buyers want that step automated at all is not shown.
- **Practitioner sentiment: still thin, and the one write-up read rests on a report that could not be traced.** Observed: a dev.to post by Tamiz Uddin dated 1 September, year not shown in the extract and taken as 2026 from its title (https://dev.to/tamizuddin/why-your-ai-agent-passed-every-test-but-still-failed-in-production-lessons-from-the-2026-agent-4e27, read 2026-10-04), names five ways agents that pass tests fail in production: drift in user inputs, fragile tool chains, context-window collapse, gaming of the evaluation metric, and no negative testing; and four responses: adversarial test generation, chaos engineering for agents, "production telemetry as ground truth", and formal policy verification. The author summarises others' failures, not a system of their own, and cites an "Agent Reliability Collective's 2026 report" (1,247 agents, 89 organizations, as the post states) without a link; that report was not found or fetched, so its figures are **not** repeated here as evidence. A first-hand postmortem on Medium ("The Agent That Burned $4,200 in 63 Hours") returned HTTP 403 and is not cited for content.
  - Relevance to Karakuri (inferred): "production telemetry as ground truth" matches the direction of Phase 30 (an evaluation set drawn from Karakuri's own runs). That is a match with one blogger's recommendation, not validation. No write-up by a team describing its own agents in production was read in either pass.

### What this section supports, and what it does not

- Supported by something read: scheduled and background agent runs are shipping in a mainstream coding CLI, with visible trouble reporting run state truthfully (late September 2026); a hosted runtime is working on MCP OAuth and pause-for-credential flows (August to September 2026); the OpenTelemetry GenAI conventions are unstable and relocated (June 2026); EU high-risk deadlines are agreed to move to December 2027 (May 2026).
- Not supported: any statement about the direction of the field as a whole, about durable-execution runtimes, about AI SRE tools, about agent-to-agent protocols, or about what practitioners now say. A later pass should fetch those first.
- After the later pass: also supported by something read are a breaking MCP revision named 2026-07-28, the Digital Omnibus being in force (29 July 2026, per a law firm), A2A joining the Agentic AI Foundation (17 August 2026), and one incident vendor shipping alert-triggered investigation (1 October 2026). Still not supported: anything about durable-execution runtimes, OpenAI Codex, LangGraph or other framework releases, more than one AI SRE tool, or first-hand practitioner accounts.

## Feasibility

Written by part 3 on 2026-10-04. Candidates were chosen by how much sourced evidence this report holds for them, not by how attractive they are. The repository was read (files, roadmap, ADR titles) and searched with `grep`; **nothing was built, run or tested**, so every statement about what the code does is a reading of the source, not an observed behaviour. No candidate was skipped as declined: "Earlier discovery pull requests" below lists no closed or merged discovery pull request.

### Candidate 1 — An MCP client that speaks revision 2026-07-28 and authenticated remote servers

Evidence in this report: the breaking MCP revision 2026-07-28 (Trends, later pass); LangSmith's recent work on MCP OAuth discovery and Client ID Metadata Documents (Comparable products); n8n's MCP client compatibility issues and "Add MCP support" as the most-reacted OpenAI Agents SDK issue listed (pain points). The evidence that the protocol changed is first-hand (the specification's own page); the evidence that users are hurt by it is indirect (issue titles from other products).

**What Karakuri already has.**

- Phase 28 (Completed) in `docs/roadmap.md` delivered an MCP client under `internal/platform/tools/mcp/` with stdio and streamable-HTTP transports, a per-instance allowlist, and Karakuri as an MCP server (read and propose only). The files are `client.go`, `protocol.go`, `transport.go`, `stdio.go`, `streamhttp.go`, `instance.go`, `environment.go`.
- Observed in the source: `internal/platform/tools/mcp/protocol.go` declares `const ProtocolVersion = "2025-06-18"` and a `MethodInitialize = "initialize"` method; `client.go` sends `protocolVersion` in that handshake and records the version the server answers with; `streamhttp.go` declares `const sessionHeader = "Mcp-Session-Id"`. Comments in `transport.go`, `streamhttp.go` and `client.go` say server-initiated requests (sampling, elicitation) are not implemented and that the client does not claim those capabilities.
- Observed: `instance_test.go` configures an instance with `Headers: map[string]string{"Authorization": "Bearer t0k"}`, so a static header is how a remote server is authenticated. A `grep` for `oauth` and `OAuth` under `internal/platform/tools/mcp/` returned nothing.
- Discovered tools are bounded by ADR 022 (`docs/adr/022-discovered-tools-are-bounded-four-ways.md`; title read, and its four bounds as Phase 28 states them) and their results are marked third-party per ADR 021.

**What it would take.** Inferred from the above and from the 2026-07-28 change list as this report records it:

- The client speaks a revision two behind the current one (2025-06-18, then 2025-11-25, then 2026-07-28). The 2026-07-28 revision removes the `initialize` handshake and the `Mcp-Session-Id` header that this client uses, and requires `server/discover`, a per-request version in `_meta`, and a `resultType` on every result. Supporting it is a second protocol path in `internal/platform/tools/mcp/` (`protocol.go`, `client.go`, `streamhttp.go`) with a fallback to the old handshake for older servers.
- Layers touched: `internal/platform/` only for the protocol work. Because the client never implemented sampling or elicitation, their deprecation costs nothing; the new `input_required` round-trip would need a decision on whether a tool asking for input becomes a checkpoint, which reaches `internal/feature/` and wants an ADR.
- OAuth for remote servers (issuer validation, credentials keyed by issuer, Client ID Metadata Documents) is larger and separate: a token store, a consent step a person completes, and configuration in `config/default.yaml`. It touches `internal/platform/`, `internal/api/` (a callback route, and `docs/openapi.yaml`) and probably the `auth` module.
- Rough size (an estimate, not measured): the protocol revision is one phase-sized slice of adapter work comparable to a Phase 28 step; OAuth is a phase of its own.
- The server side (Karakuri as an MCP server, Phase 28 step 5) would need the same revision work; it was not read in this pass.

**What could not be verified.**

- Whether a 2026-07-28-only server actually rejects this client: no server was run. The claim is a reading of two documents side by side.
- Whether any MCP server Karakuri's users would bind has dropped the old handshake yet. The specification gives deprecated features a minimum twelve-month window, but as this report records it that window is stated for Roots, Sampling, Logging and HTTP+SSE, not for the handshake; how fast servers move is unknown.
- Whether the client falls back when a server answers with a different version: `client.go` records the server's version, and the code that acts on a mismatch was not read.
- Karakuri's outbound MCP server code was not located or read.
- No user of Karakuri is on record asking for this; the demand evidence is from other products' trackers.

### Candidate 2 — An incident investigation that starts at the alert, stays read-only, and posts its findings where the on-call engineer is

Evidence in this report: Nightwatch (read-only by its author's choice), Traversal (keep humans on high-impact changes), Better Stack (investigate at incident start, post to the timeline and Slack, 1 October 2026), and one commenter on missing context about neighbouring services. Three of the four sources sell or publish an AI SRE tool; no SRE team speaks for itself in this report.

**What Karakuri already has.**

- Phase 32 (Completed) in `docs/roadmap.md`: `software.env.observability` is a real environment (`domains/software/observability_env.go`) over four adapters in `internal/platform/tools/observability/` (Prometheus, Loki, Datadog, PagerDuty); it serves `software.observe.fetch_logs` and `software.observe.fetch_metrics`; its snapshot wakes an incident objective when an alert starts, resolves or is acknowledged. These are the roadmap's statements; the adapter files themselves were not opened in this pass.
- The same phase states that an incident plan **always escalates**, because alert text is third-party material (ADR 021) and because `software.act.run_remediation` is on the SRE agent's approval list. So the "human approves the fix" line that all three sources draw is already where Karakuri puts it.
- A Slack adapter exists at `internal/platform/tools/messaging/slack.go` (found by `grep`; not opened).
- Standing objectives and reconciliation (Phase 20, ADR 015) are what would start the investigation without a person asking.

**What it would take.** Inferred:

- Phase 32's incident template couples investigation to a remediation that escalates. The comparable products deliver a finding first and leave the fix to a person. A read-only investigation objective (observe, fetch logs and metrics, write a root-cause summary, stop) is a new objective template and criteria in `domains/software/`, which is a pack change and touches no layer in `internal/`.
- Posting the summary to chat or to the incident itself: chat may be configuration only if the Slack adapter is reachable from the SRE agent, which was not checked. Writing to a PagerDuty or Datadog incident timeline is new adapter work in `internal/platform/tools/observability/` (the roadmap describes these adapters as reading only) plus a new capability, which would be on the approval list by default.
- The largest gap is one Phase 32 names itself as deferred: a twin binds one observability instance, so alerts from Prometheus and logs from Loki cannot be combined; and nothing was run against a live backend. Both are `internal/platform/` and pack work.
- Rough size (an estimate): the read-only template is small, days; multi-instance binding and live validation are the bulk and are already on Phase 32's deferred list, so this candidate is mostly "finish what Phase 32 deferred" and not a new capability.

**What could not be verified.**

- Whether the root-cause summary is any good. Phase 32 says the root-cause criterion is judged by a model and no judge is wired into its acceptance harness; Traversal's point in this report is that a fast wrong answer has negative value.
- Whether the Slack adapter can post from an incident objective today.
- Whether anyone wants this from a self-hosted Go server and not from the incident vendor they already pay; see Fit assessment.

### Candidate 3 — Pausing for a person and surviving a restart (already delivered; one named gap)

Evidence in this report: the heaviest of the three. Four human-in-the-loop issues in the OpenAI Agents SDK tracker, LangChain's post and survey figure on human review, LangGraph's interrupt, checkpoint and resume bugs, and Inngest's post. It is listed because it is the best-evidenced need, and the finding is that the roadmap already delivered it.

**What Karakuri already has.**

- Phase 11 (Completed), `docs/roadmap.md`: loop state persisted at iteration boundaries, `ResumeStoredLoops` called at boot, paused loops waiting for a fresh decision after a restart, with Restate and Celery executors in `internal/platform/executor/`. The phase states that a crash loses at most one iteration.
- Phase 13.5 (Completed): a checkpoint carries the planner's draft actions; a person can approve, reject or modify; each decision writes an audit row (`internal/feature/checkpoint/service.go`, `internal/core/checkpoint/checkpoint.go`).
- Phase 20 and ADR 016 (earned autonomy and digests, title read) and Phase 31 (the evidence pack, heading read) build on those.
- `grep` finds resume tests in `internal/feature/loop/resume_test.go` and `resume_stored_test.go`. They were not run.

**What it would take.** The capability exists, so the remaining work is narrow:

- Phase 11 defers active-active coordination: two replicas on one database could both re-launch the same loop, and leader election is left to the operator. LangSmith's "dedicated deployments now run at least two replicas" (this report, Trends) is the comparison. Closing it touches `internal/feature/loop/` and `internal/platform/storage/`. `grep` finds `lease` in `internal/feature/reconcile/`, so reconcile may already coordinate; that code was not read, and the size cannot be estimated until it is.
- Phase 11's "at most one iteration re-executes" means an action in that iteration can run twice after a crash, and the model call is paid for again. That is the "re-paying for tokens" and "resume value reused" class of complaint in this report. Whether Karakuri's actions are idempotent on re-execution was not checked.

**What could not be verified.**

- Any of the restart behaviour: no server was started or killed in this pass. The claims are the roadmap's own acceptance notes.
- Whether Phase 20 or later changed what Phase 11 deferred.
- The smaller complaints (a fixed five-minute model timeout, a tool error ending the run) were not checked against Karakuri's loop at all.

## Fit assessment

Written by part 3 on 2026-10-04. Everything in this section is an **assessment**, not a measurement: it sets the segments above against what the roadmap and source say Karakuri has. No user in any segment was asked, and no Karakuri deployment other than the project's own is known to this report.

- **Segment 1, small technology companies building agents into products. Weak to irrelevant.** Their leading use cases in this report are customer service and research; Karakuri is a server that holds standing objectives over software, with packs for domains, and is not a library to embed in a product. Their top stated barrier is output quality; Phase 30's evaluation set (`docs/roadmap.md`) is drawn from Karakuri's own runs and does not evaluate someone else's agent. Reasons they would not want it: a framework plus a hosted runtime already covers them, a Go server with a database is more to operate than a team under 100 wants, there is no ecosystem of integrations beyond the adapters in `internal/platform/tools/`, and the project is young.
- **Segment 2, large enterprises with governance as a condition. Strong on mechanism, unproven on everything else.** Review of risky writes, an audit row per decision, earned autonomy and an evidence export are what this segment's sources ask for, and Phases 13.5, 14, 16, 20 and 31 are those things. Reasons they would not want it: no vendor, support or certification stands behind it; the EU high-risk deadline moved to December 2027, which removes urgency; nothing in this report shows the evidence pack matches a format an auditor accepts; and MCP access to their systems would need the OAuth work in Candidate 1.
- **Segment 3, engineering teams delegating coding work. Mixed.** Strong where the work is standing and unattended: worktree delivery (ADR 003), checkpoints, digests, spend ceilings (Phase 23). Weak where it is interactive, which this report cannot separate from the rest of the segment. Reasons they would not want it: Claude Code now ships scheduled and background runs to people who already have it installed, so the unattended case is no longer Karakuri's alone; a team must run a server to get what a CLI gives them; and the Hacker News comments on secrets and merge policy checks name needs this pass did not check Karakuri against.
- **Segment 4, platform and SRE teams. Strong in design, weak in proof.** The line the sources draw, investigate freely and never remediate unapproved, is the line Phase 32 enforces. Reasons they would not want it: incident vendors ship investigation inside the tool the team already pages through; Phase 32 was never run against a live backend, binds one observability instance per twin, and remediates through a shell command on the server host; and a wrong root cause during an incident is the failure these teams punish, with no evidence here on Karakuri's accuracy.
- **Segment 5, solo developers and self-hosters. Plausible, thinly evidenced.** A single Go binary with SQLite by default (Phase 11's notes) suits "few moving parts", and MCP support gives tools without writing adapters. Reasons they would not want it: RBAC, federated identity, quotas and org units are weight a single user does not need; sandboxing is what the best-received launches offered, and Phase 32 itself says the shell denylist is not a security boundary; local-model behaviour (the five-minute timeout complaints) was not checked; and the platform's licence, which one commenter gave as a reason to reject a product outright, was not checked in this pass.

Taken together, as an assessment: Karakuri's fit is best where the buyer's condition is governance (Segments 2 and 4) and those are the segments with the least first-hand evidence in this report and the highest bar for maturity.

## What changed since the last report

This is the first discovery report: `ls docs/research/` on 2026-10-04 shows only `ai-use-cases-2026.md` and this file, so there is no baseline and nothing to confirm or contradict. The only earlier research is `docs/research/ai-use-cases-2026.md`, whose title and introduction describe it as the field evidence Phases 27 to 32 were proposed from, gathered in August 2026 from eight web searches and a read of the tree. Only its first forty lines were read in this pass, so no finding of it is compared here. The relation, as far as that reading supports: that document asked what the field was doing and produced phases; this report asks who would use the result and what they complain about, after those phases shipped. Both state the same limit, that most sources are vendor or secondary and adoption figures are directional. A later pass should read it in full and check whether MCP revision 2026-07-28 and the Digital Omnibus dates change any of its findings.

### Earlier discovery pull requests

Read with `gh pr list --state all --label karakuri:discovery` on 2026-10-04.

- #148, OPEN, https://github.com/bsenel/karakuri/pull/148: "Discovery 2026-10-04: report in progress, no phase proposed yet". This is the pull request for this report, on this branch; it had no reviews and no comments when read.
- No closed or merged discovery pull request was listed, so no proposed phase has been declined so far.

## Proposed phases

_Not yet written._

## Sources

Part 1 (segments and pain points). All eight fetches succeeded; none failed. Each page was read as a model-written extract of the fetched page, not as raw HTML.

- https://www.langchain.com/state-of-agent-engineering (read 2026-10-04): LangChain's survey; respondent make-up, production share, use cases, barriers, evaluation and human-review figures.
- https://www.arcade.dev/blog/5-takeaways-2026-state-of-ai-agents-claude/ (read 2026-10-04): vendor summary of Anthropic's 2026 State of AI Agents Report; coding-tool adoption and integration, data and security barriers, second-hand.
- https://www.langchain.com/blog/runtime-behind-production-deep-agents (read 2026-10-04): vendor post listing what long-running agents need in production; quotes on durability, human approval, tenancy.
- https://arxiv.org/abs/2605.10223 (read 2026-10-04): abstract only; the claim that agent frameworks lack governability for enterprise deployment.
- https://www.inngest.com/blog/durable-execution-key-to-harnessing-ai-agents (read 2026-10-04): vendor post; failure modes of agents without durable execution.
- https://github.com/langchain-ai/langgraph/issues?q=is%3Aissue+is%3Aopen+interrupt+OR+checkpoint+OR+resume+sort%3Areactions-desc (read 2026-10-04): issue titles, numbers and dates on interrupt, checkpoint and resume.
- https://the-agent-report.com/2026/05/state-of-agent-engineering-2026-langchain-datadog/ (read 2026-10-04): secondary write-up; Datadog's error and rate-limit figures second-hand, and a cross-check of LangChain's.
- https://github.com/openai/openai-agents-python/issues?q=is%3Aissue+sort%3Areactions-desc (read 2026-10-04): issue titles, numbers and dates; human-in-the-loop, MCP, A2A and tracing requests.

Two web searches were also run to find these pages; search-result snippets are not cited as evidence.

Part 1, later pass (SRE teams, self-hosters, forum threads). Seven fetches were made: six are cited below; one Hacker News search for AI SRE stories was used only to find the Nightwatch thread. Each page was read as a model-written extract.

- https://hn.algolia.com/api/v1/items/48438180 (read 2026-10-04): Hacker News thread, "Show HN: Nightwatch, The open-source, read-only AI SRE", 7 June 2026; the author's reason for read-only and one commenter's worry about context.
- https://www.traversal.com/blog/ai-in-incident-response-state-of-the-field-2026-sre (read 2026-10-04): AI SRE vendor's blog post; its claims on wrong answers during incidents and on keeping humans on high-impact changes.
- https://hn.algolia.com/api/v1/search?query=self-hosted%20agents%20production&tags=story&numericFilters=created_at_i%3E1775000000 (read 2026-10-04): Hacker News search listing; titles, points and dates of launches since April 2026.
- https://hn.algolia.com/api/v1/items/48225040 (read 2026-10-04): Hacker News thread on Runtime (YC P26), 21 May 2026; four commenters on licence, secrets, policy checks and review flow.
- https://github.com/n8n-io/n8n/issues?q=is%3Aissue+agent+sort%3Areactions-desc (read 2026-10-04): issue titles, numbers and dates; timeouts, tool errors, MCP client and provider compatibility.
- https://labs.cloudsecurityalliance.org/research/csa-research-note-huggingface-autonomous-agent-breach-202607/ (read 2026-10-04): Cloud Security Alliance research note of 20 July 2026; recommended controls after an intrusion that used an autonomous agent.

Two web searches were also run in this pass; their snippets (including figures attributed to Microsoft's Azure SRE Agent) are not cited, because the pages behind them were not fetched.

Part 2 (trends). Eight fetches were made: five returned content, two returned a redirect to another host, one returned HTTP 404. Each page was read as a model-written extract, not as raw HTML.

- https://code.claude.com/docs/en/changelog (read 2026-10-04): Claude Code changelog; only versions 2.1.283 to 2.1.289 (25 September to 3 October 2026) were returned; background sessions, routines, permission defaults, MCP and OpenTelemetry entries.
- https://docs.langchain.com/langsmith/changelog (read 2026-10-04): LangSmith changelog; only late August to September 2026 was returned; Managed Deep Agents, MCP OAuth, deployment entries. Reached by following the redirect below.
- https://john-hodge.com/blog/opentelemetry-genai-semantic-conventions/ (read 2026-10-04): an individual's blog post of 17 July 2026; status, repository move and version history of the OpenTelemetry GenAI semantic conventions.
- https://modelcontextprotocol.io/specification/draft/changelog (read 2026-10-04): MCP draft changelog; the page held one placeholder sentence and no changes.
- https://www.gibsondunn.com/eu-ai-act-omnibus-agreement-postponed-high-risk-deadlines-and-other-key-changes/ (read 2026-10-04): law-firm alert of 27 May 2026; EU AI Act Digital Omnibus dates.
- https://changelog.langchain.com/ (attempted 2026-10-04): returned a 301 redirect to docs.langchain.com, which was then fetched as above; no content read from this URL.
- https://developers.openai.com/codex/changelog (attempted 2026-10-04): returned a 308 redirect to a different host, not followed; no content read, nothing cited.
- https://incident.io/changelog (attempted 2026-10-04): HTTP 404; no content read, nothing cited.

Four web searches were also run in part 2 (frameworks, standards, EU AI Act, practitioner lessons); their snippets are not cited as evidence, and where one is mentioned above it is marked as an unverified lead.

Part 2, later pass (gaps in trends). Eight fetches were made: seven returned content, one returned HTTP 403. The MCP page came back as the page's own text; the others as model-written extracts.

- https://modelcontextprotocol.io/specification/2026-07-28/changelog (read 2026-10-04): MCP "Key Changes" for revision 2026-07-28 against 2025-11-25; stateless core, removed handshake and sessions, multi round-trip requests, tasks extension, authorization changes, deprecations.
- https://github.com/open-telemetry/semantic-conventions-genai/releases (read 2026-10-04): releases page of the OpenTelemetry GenAI conventions repository; no releases listed.
- https://www.orrick.com/en/Insights/2026/07/EU-AI-Act-Update-Digital-Omnibus-Finalizes-8-Compliance-Changes (read 2026-10-04): law-firm alert, July 2026; Digital Omnibus published and in force, high-risk dates, list of changes.
- https://aaif.io/blog/a2a-joins-aaif (read 2026-10-04): Agentic AI Foundation post of 17 August 2026; A2A becomes a hosted project, and how it relates to MCP.
- https://www.linuxfoundation.org/press/a2a-protocol-surpasses-150-organizations-lands-in-major-cloud-platforms-and-sees-enterprise-production-use-in-first-year (read 2026-10-04): Linux Foundation press release of 9 April 2026; A2A 1.0 features and the foundation's adoption figures.
- https://betterstack.com/community/blog/changelog-17-wake-up-your-ai-agent-before-getting-paged/ (read 2026-10-04): Better Stack changelog post of 1 October 2026; AI SRE investigation at incident start.
- https://dev.to/tamizuddin/why-your-ai-agent-passed-every-test-but-still-failed-in-production-lessons-from-the-2026-agent-4e27 (read 2026-10-04): individual's post dated 1 September; failure modes and responses, citing a report that is not linked.
- https://medium.com/@sattyamjain96/the-agent-that-burned-4-200-in-63-hours-a-production-ai-postmortem-d38fd9586a85 (attempted 2026-10-04): HTTP 403; no content read, nothing cited.

Seven web searches were also run in this pass (MCP, frameworks, OpenTelemetry, EU AI Act, A2A, AI SRE, production lessons); their snippets are not cited as evidence, and where one is mentioned above it is marked as an unverified lead.
