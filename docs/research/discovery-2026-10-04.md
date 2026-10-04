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

### What this section supports, and what it does not

- Supported by something read: scheduled and background agent runs are shipping in a mainstream coding CLI, with visible trouble reporting run state truthfully (late September 2026); a hosted runtime is working on MCP OAuth and pause-for-credential flows (August to September 2026); the OpenTelemetry GenAI conventions are unstable and relocated (June 2026); EU high-risk deadlines are agreed to move to December 2027 (May 2026).
- Not supported: any statement about the direction of the field as a whole, about durable-execution runtimes, about AI SRE tools, about agent-to-agent protocols, or about what practitioners now say. A later pass should fetch those first.

## Feasibility

_Not yet written in this pass._

## Fit assessment

_Not yet written in this pass._

## What changed since the last report

_Not yet written in this pass._

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
