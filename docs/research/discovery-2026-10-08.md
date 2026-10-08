# Discovery 2026-10-08: deeper cycle

**Method.** This report is written in eight parts by separate sessions that run one after another and share no memory; each part writes only its own sections and its own subheading under Sources. Branch in use: `karakuri/discovery-2026-10-08`, created by part 1 from `origin/main` (no open pull request with the label `karakuri:discovery` existed when part 1 started). Sources were fetched on 2026-10-08 through a fetch tool that passes pages through a model. Every source is graded: **[Q]** quoted verbatim by the fetch, and only then shown in quotation marks; **[E]** extract or paraphrase only; **[N]** not read (blocked, paywalled, error or redirect) and cited for nothing. "Observed:" marks what a fetched page says; "Inferred:" marks a conclusion drawn here. Nothing was built, run or tested. Vendor figures are the vendor's, not measurements. What `docs/research/discovery-2026-10-04.md` already says is referred to, not repeated.

## Who the users are and what they are trying to do

Written by part 1. The 2026-10-04 report's five segments stand; this section does not restate them. It adds first-hand sources for the three segments that report called thin, in the order it named them. Ten primary pages were read; quotations are fragments exactly as the fetch tool returned them, and the tool sometimes splits a long sentence into pieces, so a quoted fragment is not always a whole sentence. Nobody was interviewed.

### Platform and SRE teams

- **Brex, an in-house on-call agent.** Who speaks: Brex's own engineering journal; the extract shows no author, team or date. Job: take an incoming on-call ticket and give the engineer a head start on the investigation. Observed: "We encoded our oncall playbook into an agent."; the aim is to "hand the engineer a 70% complete investigation instead of a blank page."; "When a ticket arrives through an automated workflow or a direct @mention, it launches an investigation."; "By default it's read-only."; it "can search, query, and read, but it can't modify files, push commits, or change ticket states."; and "Upgrading the model improved investigation quality less than writing better runbooks." The post reports that, replayed against 2025 tickets, the agent "matched the correct root cause and mitigation on 91%"; that is Brex's own figure for its own tickets, not a measurement made here. The extract says write tools can be unlocked for a single reply when a follow-up asks for a write, and that the post describes no approval workflow. (https://www.brex.com/journal/how-we-built-an-ai-oncall-engineer.md, read 2026-10-08) [Q]
- **Zalando, LLMs reading postmortems.** Who speaks: Dmitry Kolesnikov, Senior Principal Engineer, Zalando Engineering Blog, 25 September 2025 (older than this cycle's window; kept because it is a team reporting its own error rates). Job: mine a backlog of datastore postmortems for recurring causes. Observed: "We adopted LLMs as intelligent postmortem review assistants."; "We have observed up to 40% probability for hallucination at summary and analysis phases."; "During the pipeline development, we conducted 100% human curation of output batches."; "As the system matured, we relaxed human curation to 10-20% of randomly sampled summaries from each output batch."; "While the goal of our solution is to reduce human involvement, human curation remains essential." Figures are Zalando's. (https://engineering.zalando.com/posts/2025/09/dead-ends-or-data-goldmines-ai-powered-postmortem-analysis.html, read 2026-10-08) [Q]
- **Two Hacker News threads on read-only production agents.** In the Show HN for Nightwatch (7 June 2026, 33 points, 10 comments) the author _mtxe, who built it after "we had a kubernetes upgrade that went wrong, and at some point a rollback wasn't possible anymore.", writes: "read-only for now, i don't trust it near prod yet and honestly neither should you." No commenter there described their own on-call work. (https://hn.algolia.com/api/v1/items/48438180, read 2026-10-08) [Q] In the Launch HN for HyperProbe (5 August 2026, 69 points; the extract counted roughly 45 comments), IgorVoytyuk, who the extract says describes running an autonomous pipeline for eight months: "Read-only in prod is the right constraint." and "A single confident answer that's wrong is worse than no answer"; doublerebel: "in a hurry to debug, it's easy to miss that a property should have been redacted"; akashy123 asks, per the extract, whether read-only is enforced or only a convention; denis-stable: "do you have plans to make an open source version for on-premises installation?" (https://hn.algolia.com/api/v1/items/49185389, read 2026-10-08) [Q] Individual comments, not a count of opinion.
- **One conference talk, weak.** Vladyslav Budichenko, Senior Software Engineer at Vocaly AI, Conf42 DevOps 2026 (22 January 2026), auto-generated transcript: "it's important to limit your like agents in scope and don't provide it, like to, don't allow it to do everything." The extract says the talk is general advice and describes no incident the speaker handled with an agent, so it is not evidence of an SRE team's practice. (https://www.conf42.com/DevOps_2026_Vladyslav_Budichenko_incidents_agents_security, read 2026-10-08) [Q]
- Inferred: the job in every source here is investigation handed to a person, started by a ticket or alert, read-only by default; the two teams that report results put the weight on runbooks and on sampled human review, not on the model. Phase 32's read-only SRE path and the checkpoint ladder are the same shape. Inferred, and not said by any source: the Zalando pattern of full review relaxed to a 10-20% sample is earned autonomy by sampling, which Karakuri's ladder does not express.
- **What would make this segment not want a platform like Karakuri.** Observed: Brex built its agent itself around its own runbooks and tickets, and the extract says the post gives no build-versus-buy reasoning. Inferred: a team with that much invested in its own playbook has little reason to adopt a general platform; and a team asking whether read-only is enforced or conventional (HyperProbe thread) would ask Karakuri the same question.

### Solo developers and self-hosters

- **Show HN: Pizza Bot, "An inbox for AI agents that work in the background"** (jd_, 15 September 2026, 61 points, 37 comments). Who speaks: the author and commenters about their own setups. Job: routine personal or small-business chores run overnight on a local model. Observed: the author built it out of "my frustration at having to manually log CRM activities through a browser form." and says "I've got scheduled agents that run (a little slowly) overnight using Qwen 3.8 27B that are ready for me by the morning."; nucleardog: "I run a small model locally as it's good enough for most of what I want to do (and cheap! and private!)"; scottydelta: "Why does it have to be a desktop app vs a self hostable web app?"; sgc: "I like to insert agents into deterministic workflows, so some way to easily have deterministic steps" and "I need to be able to enforce structured output easily (json, and xml if possible)."; boplicity: "would much, much prefer an open source project to manage various asynchronous tasks."; nzjrs: "Why bother depending on DeepAgents and LangChain etc."; esafak: "If it is about local work, how does it compare with simply having your agent monitor a directory for ticket files?" (https://hn.algolia.com/api/v1/items/49713894, read 2026-10-08) [Q] Nine comments chosen by the fetch out of 37, not a count of opinion.
- The Nightwatch author above is also a self-hoster describing their own cluster; see the SRE segment.
- Inferred: these people want scheduled work whose results wait for them in the morning, on a slow local model, in something they can host as a server and whose steps they can make deterministic. A cadence, a queue of results and a server process are what Karakuri has; whether it runs acceptably against a slow local model was not checked here, and the 2026-10-04 report left the same question open.
- **What would make this segment not want a platform like Karakuri.** Observed: esafak's question is the objection, that a directory of ticket files watched by an agent may be enough; nzjrs objects to framework dependencies. Inferred: Karakuri depends on langchaingo, and its governance features answer no need voiced in this thread.

### Teams running coding agents unattended

- **Spotify, a three-part series on its background coding agent.** Who speaks: Max Charas (Senior Staff Engineer) and Marc Bruggmann (Principal Engineer), Spotify Engineering, 6 November, 24 November and 9 December 2025 (older than this cycle's window; kept because it is the team that runs the thing). Job: fleet-wide code migrations, with nobody watching the run. Observed, part 1: "We replaced deterministic migration scripts with an agent that takes instructions from a prompt."; "we've already merged more than 1500 pull requests" (Spotify's figure); "Spotifiers can now kick off coding agent tasks from both Slack and GitHub Enterprise."; "agents can take a long time to produce a result, and their output can be unpredictable." (https://engineering.atspotify.com/2025/11/spotifys-background-coding-agent-part-1, read 2026-10-08) [Q] Part 2: "We keep our background coding agent very limited in terms of tools and hooks"; "Notably, we don't currently have code search or documentation tools exposed to our agent."; "The more tools you have, the more dimensions of unpredictability you introduce."; "It helps to clearly state in the prompt when not to take action." (https://engineering.atspotify.com/2025/11/context-engineering-background-coding-agents-part-2, read 2026-10-08) [Q] Part 3: the failure they name is "The background agent produces a PR that passes CI but is functionally incorrect."; the answer is "The verification loop consists of one or more independent verifiers." and "If one of the verifiers fails, the PR isn't opened and the user is presented with an error message."; "The agent itself has very limited access." The extract says an LLM judge compares the diff with the original prompt and vetoes about a quarter of sessions, per the authors. (https://engineering.atspotify.com/2025/12/feedback-loops-background-coding-agents-part-3, read 2026-10-08) [Q for the quoted fragments, E for the judge's veto rate]
- **Ask HN: "Why are software developers not using Background coding agents?"** (daemon_9009, 14 January 2026, 1 point, 8 comments). Who speaks: developers whose employers provide background agents and who do not use them. Observed: the poster: "they prefer IN-IDE agent even though the company is providing them with background agents."; lompad: "Generally, with the regular in-IDE agents you have the ability to easily intervene, correct and live-check."; speakingmoistly: "In my experience, having it run out of sight leads to heavier editing since smaller realignments" "couldn't be applied along the way."; thesuperbigfrog: "I do not trust an agent to give it unsupervised access to my systems."; Zekio: "they need too much hand holding still imho"; codyklimdev: "Using a background coding agent takes all of the tinkering and debugging out of it," (https://hn.algolia.com/api/v1/items/46615737, read 2026-10-08) [Q] A one-point thread with eight comments: eight people, no more.
- Inferred, and this is the distinction the 2026-10-04 report lacked: what separates an unattended coding agent from an interactive one in these two sources is not how long it runs. It is that nobody can correct it mid-run, so the correction has to be moved somewhere else. Spotify moves it before the run (a narrow prompt that says when not to act, few tools) and after it (independent verifiers and a judge that can stop a pull request from being opened). The developers who decline background agents decline for exactly the missing mid-run correction. Inferred for Karakuri: a checkpoint before a write is one such relocation; a verifier that can veto the result before a person is asked to look at it is another, and Spotify's account is that a quarter of runs needed it. Whether Karakuri has an equivalent was not checked in this part.
- Still not found: a team running a **standing** coding agent, one that wakes on a cadence and chooses its own work. Spotify's agent is started by a person or a migration campaign. Nothing read distinguishes standing from merely long-running.
- **What would make this segment not want a platform like Karakuri.** Observed: Spotify built its own on its existing Fleet Management system, and the extract says pushing code, Slack interaction and prompt authoring are handled by that surrounding infrastructure and not by the agent. Inferred: a team that already has fleet tooling and CI wants an agent that slots into it, not a second orchestrator; and the individual developers quoted do not want unattended runs at all.

### Regulated organisations, and the Anthropic report

- No first-hand source this cycle. One search for a bank, insurer, health or public-sector body describing its own agent deployment and what its risk or audit function required returned vendor and consultancy pages only; none was fetched and none is cited. Brex, above, is a financial-services company, but its post says nothing about a risk or audit function.
- Anthropic's "2026 State of AI Agents Report": the landing page was fetched and the extract says it holds a cover image and a download link, no findings and no date, and describes the report as based on a survey of 500+ technical leaders without stating a method. (https://resources.anthropic.com/ty-2026-state-of-ai-agents, read 2026-10-08) [E] The report itself was **not read** [N], so the 2026-10-04 report's use of it remains second-hand.

**Still thin:** (1) Platform and SRE: two teams speak for themselves (Brex, Zalando), neither at a conference and neither describing an agent that acts during an incident; no SREcon, KubeCon or QCon transcript was found, and the Brex post's date is unknown. (2) Self-hosters: one thread, with comments selected by the fetch tool; no GitHub Discussion of a self-hosted agent project was read, and no api.github.com request was made by this part. (3) Unattended coding: the best source is from late 2025 and is one company; no standing (cadence-driven, self-selecting) coding agent was found, and Spotify's veto rate is an extract, not a quotation. (4) Regulated organisations: nothing first-hand. (5) Anthropic's report PDF was not opened. (6) Quoted fragments were not checked against raw pages, and the fetch tool splits long sentences, so some quotations above are parts of sentences. (7) The 2026-10-04 segments 1 and 2 (product teams, large enterprises) got no new source.

## What they ask for and complain about

Not yet written (part 2).

## Competitive teardown

Not yet written (parts 3 and 4).

## Technical frontier

Not yet written (part 5).

## Governance and buying criteria

Not yet written (part 6).

## Feasibility

Not yet written (part 6).

## Fit and bets

Not yet written (part 7).

## What changed since the last report

Not yet written (part 7).

### How earlier proposals fared

Written by part 1 from `gh pr list --state all --label karakuri:discovery` on 2026-10-08.

- Pull request #148, "Discovery 2026-10-04: users, pain points, trends; proposes Phase 33 (MCP revision 2026-07-28)", head `karakuri/discovery-2026-10-04`: **merged** on 2026-10-05. Its proposal is on `main` as "Phase 33 — MCP After the Handshake (Planned)" in `docs/roadmap.md`, the highest phase there.
- No discovery pull request is closed unmerged, so no proposal has been declined by a human; none was open when this pass started.

## Sources

### Sources, part 1

Part 1 run: finished; fetches attempted 15; [Q] 10; [E] 5; [N] 0. (The Anthropic report PDF behind the landing page was not attempted and counts as not read. Five web searches were also run; their result snippets are cited for nothing.)

- https://www.brex.com/journal/how-we-built-an-ai-oncall-engineer.md, 2026-10-08, primary, [Q]
- https://engineering.zalando.com/posts/2025/09/dead-ends-or-data-goldmines-ai-powered-postmortem-analysis.html, 2026-10-08, primary, [Q]
- https://hn.algolia.com/api/v1/items/48438180 (Show HN: Nightwatch), 2026-10-08, primary, [Q]
- https://hn.algolia.com/api/v1/items/49185389 (Launch HN: HyperProbe), 2026-10-08, primary, [Q]
- https://www.conf42.com/DevOps_2026_Vladyslav_Budichenko_incidents_agents_security, 2026-10-08, primary (auto-generated talk transcript), [Q]
- https://hn.algolia.com/api/v1/items/49713894 (Show HN: Pizza Bot), 2026-10-08, primary, [Q]
- https://hn.algolia.com/api/v1/items/46615737 (Ask HN: background coding agents), 2026-10-08, primary, [Q]
- https://engineering.atspotify.com/2025/11/spotifys-background-coding-agent-part-1, 2026-10-08, primary, [Q]
- https://engineering.atspotify.com/2025/11/context-engineering-background-coding-agents-part-2, 2026-10-08, primary, [Q]
- https://engineering.atspotify.com/2025/12/feedback-loops-background-coding-agents-part-3, 2026-10-08, primary, [Q] (judge veto rate [E])
- https://resources.anthropic.com/ty-2026-state-of-ai-agents, 2026-10-08, primary (landing page only; no findings on it), [E]
- https://hn.algolia.com/api/v1/search?query=background%20coding%20agents&tags=story (with a created_at filter from 1 January 2026), 2026-10-08, index used to find threads, [E], cited for nothing
- https://hn.algolia.com/api/v1/search?query=AI%20SRE%20agent%20incident%20on-call&tags=story (same filter), 2026-10-08, index, [E], cited for nothing
- https://hn.algolia.com/api/v1/search?query=AI%20agents%20incident%20response%20on-call%20engineering&tags=story (same filter, points above 20), 2026-10-08, index, [E], cited for nothing
- https://hn.algolia.com/api/v1/search?query=self-hosted%20agent%20running%20unattended&tags=story (same filter), 2026-10-08, index, [E], returned no hits, cited for nothing

### Sources, part 2

### Sources, part 3

### Sources, part 4

### Sources, part 5

### Sources, part 6

### Sources, part 7

### Sources, part 8
