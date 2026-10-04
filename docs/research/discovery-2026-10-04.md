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

### Segment 5 — Solo developers and open-source self-hosters

- No direct evidence found in this session. Inferred, weakly: the issue trackers read for the next section (LangGraph, OpenAI Agents SDK) are where such users would appear, but the listings read do not identify who filed the issues.

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

### Not found

- Nothing fetched speaks to cost as a current complaint beyond The Agent Report's remark that cost worries have "dropped significantly" (same URL as above, read 2026-10-04); no figure was given.
- No forum threads, changelogs or practitioner write-ups by non-vendors were read. No complaint here comes from an SRE team, a regulated organisation speaking for itself, or an identified self-hoster.
- A search result claimed that 78% of enterprises have agent pilots and under 15% reach production scale; the page behind it was not fetched, so it is recorded here only as an unverified lead.

## Trends

_Not yet written in this pass._

## Feasibility

_Not yet written in this pass._

## Fit assessment

_Not yet written in this pass._

## What changed since the last report

_Not yet written in this pass._

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
