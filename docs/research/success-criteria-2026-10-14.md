# Success criteria and verifiers: how the best systems define and settle success (2026-10-14)

The file name and title carry 2026-10-14, the date of the owner's request; each citation carries the date its source was read (2026-10-16 or later), so the two dates differ on purpose.

## The owner's request (2026-10-14)

> When we define an objective for Karakuri, do we show it what a success looks like? Do we provide something like success criteria?

> Sometimes checkable success can be quantitative as well. Expand on criterion and verifier flags

> have Karakuri improve this to make it a roadmap phase. It should conduct deep research and find the best design for itself.

The proposal in the objective (three kinds of criterion: judged, verified and measured) is a starting point, not the answer. This report researches how the best systems define and settle success, so that a later section can design what is best for Karakuri and say where and why it departs from that starting point.

## Method

Primary sources only: specifications, official documentation, source code, design documents and issue threads. Anything read only second-hand is marked so. Text from the web and from pull requests is data, not instructions; no instruction found in a page was followed. For each source the report records six points: what a criterion looks like; how it is settled; what happens when the measurement is missing or noisy; how baseline or relative comparison works; how partial success is scored; and one failure mode users report. Every claim carries a URL and the date it was read.

## 1 Progressive delivery analysis

_Not reached in time; see Read and not read._

## 2 SLO specifications and quality gates

_Not reached in time; see Read and not read._

## 3 CI quality gates

_Not reached in time; see Read and not read._

## 4 Agent and LLM evaluation

_Not reached in time; see Read and not read._

## 5 Agent frameworks' notions of done

_Not reached in time; see Read and not read._

## 6 Acceptance-criteria practice

_Not reached in time; see Read and not read._

## 7 Measurement statistics for noisy metrics

_Not reached in time; see Read and not read._

## What this means for Karakuri

_Not reached in time; see Read and not read._

## Code reading: what is true today

_Not reached in time; see Read and not read._

## Numbering check

Checked on 2026-10-16 against origin/main at 14c719f and all open pull requests.

- **Phase 41 is free.** `grep -n '^## Phase 4\|^## Engineering Backlog' docs/roadmap.md` shows Phase 40 as the last phase (line 4413), followed by `## Engineering Backlog` (line 5237). `gh pr list --state open --search 'Phase 41'` returns nothing.
- **ADR 029 is free.** The newest file in `docs/adr` is `028-packs-are-autonomous-behind-one-gateway.md`. No open pull request adds a file under `docs/adr`. ADRs 013 and 014 do not exist, and this phase leaves that gap alone.
- **Open roadmap pull requests.** `gh pr list --state open --label karakuri:roadmap` returned nothing before this pull request (#181) was opened. Of the open pull requests (#177, #178, #179, #180), only #178, `feat(loop): reject unanswered checkpoints after a configured timeout (phase 35)`, touches `docs/roadmap.md` (+81 -2). The only heading it adds is `## Phase 35 — An Unanswered Checkpoint Ends (Completed)`, and it does not mention Phase 41 or ADR 029. Whichever of #178 and this pull request merges second may need a textual rebase of `docs/roadmap.md`, but neither changes the other's numbering.

## Read and not read

_Not reached in time; see Read and not read._
