# Discovery 2026-10-25: weekly cycle

**Method.** This report is written in eight parts by separate sessions that are meant to run one after another, share no memory, and are not guaranteed to run in order; each part therefore guards itself and writes only between its own marker comments. It builds on `docs/research/discovery-2026-10-04.md` and `docs/research/discovery-2026-10-08.md`, refers to them rather than repeating them, and is meant to go deeper. Every source is graded: **[Q]** the fetch returned the words verbatim, and only then are they shown in quotation marks; **[E]** extract or paraphrase, never quoted; **[N]** not read (blocked, paywalled, 404, empty or rate-limited) and cited for nothing. "Observed:" marks what a fetched page says; "Inferred:" marks a conclusion drawn here. Nothing was built, run or tested. A vendor's or analyst's figure is theirs, not a measurement.

**Why this cycle runs.** The 2026-10-08 report's list 'Where this cycle is still thin', item 3, names sources that were not read and can be read now (a conference transcript of an SRE team, a GitHub Discussion of a self-hosted agent project, Anthropic's report PDF). The MCP specification's release feed shows nothing after 2026-07-28 (https://github.com/modelcontextprotocol/modelcontextprotocol/releases.atom, read 2026-10-25) [E], so the cycle runs on the thin spots, not on a new release.

## Who the users are and what they are trying to do

<!-- p1:start -->
Part 1 run: started

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
<!-- p1:end -->

## What they ask for and complain about

<!-- p2:start -->
_Pending: part 2._
<!-- p2:end -->

## Competitive teardown

<!-- p3:start -->
_Pending: part 3._
<!-- p3:end -->

<!-- p4a:start -->
_Pending: part 4._
<!-- p4a:end -->

## Capability matrix

<!-- p4b:start -->
_Pending: part 4._
<!-- p4b:end -->

## Technical frontier

<!-- p5:start -->
_Pending: part 5._
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
- `docs/research/success-criteria-2026-10-14.md` is on `main` but is not a discovery report; it is not counted here.
<!-- p1h:end -->

<!-- p7b:start -->
_Pending: part 7._
<!-- p7b:end -->

## Sources

### Sources, part 1

<!-- s1:start -->
_Pending: part 1._
<!-- s1:end -->

### Sources, part 2

<!-- s2:start -->
_Pending: part 2._
<!-- s2:end -->

### Sources, part 3

<!-- s3:start -->
_Pending: part 3._
<!-- s3:end -->

### Sources, part 4

<!-- s4:start -->
_Pending: part 4._
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
