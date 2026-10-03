# ADR 024 — The evaluation set is recorded history, and the gate replays it

**Status:** Accepted
**Date:** 2026-10-03
**Relates to:** [ADR 015](015-standing-objectives-and-reconciliation.md) (the loop converges once, and nothing beside it gates), [ADR 020](020-a-declaration-is-verified-by-running-it.md) (a declaration is verified by running it), [ADR 021](021-observations-carry-provenance.md) (a payload says who wrote it)

## Context

`evaluateWithAgent` settles every verified criterion in every pack. Phase 25
found two defects in it: it was shown nothing the actions produced, and its
parser searched the reply for "pass", "met", "approved" or "yes", so "this does
not pass" scored as met. Both were fixed. How often the judge agrees with a
person was never measured.

The data to measure it already existed. Since Phase 13.5 every escalation writes
a checkpoint carrying the planner's proposed `Actions`, and every resolution
writes a human decision: approve, reject or modify. Those are human-labelled
plans, produced by people doing their jobs. Nothing read them.

`cmd/krk-bench` drives a seeded mock. It answers a question about strategies and
cannot answer "did this change make the system worse".

Three constraints shaped the work:

- A check that goes red on a defect has to exercise the code that had the
  defect. A harness with its own verdict parser would keep passing while the
  loop's regressed.
- CI cannot call a model. It would be non-deterministic, it would tie the cost
  of a pull request to upstream pricing, and a required job would fail on a
  provider outage.
- A checkpoint held the plan and not what the planner saw. Scoring the judge
  needs only the plan; replaying the planner needs the world state, and nothing
  persisted it.

## Decision 1 — calibrate the production parser, not a copy

`verdictIsPass` in `internal/feature/loop/verify.go` is exported as
`VerdictIsPass`, and `internal/feature/eval` calls it. `Service.Calibrate` lists
resolved checkpoints through `storage.ListResolvedCheckpoints`, puts each
drafted plan to a judge agent, and reads the reply with that one function.
`approve` is the positive label; `reject` and `modify` are negative.
`renderPlanTask` keeps `evaluateWithAgent`'s contract: judge only what is shown,
absence is FAIL, answer in one word. A judge error and an empty plan score as
FAIL, as they do in the loop.

Reintroducing a parser defect therefore changes the number calibration reports
and the result of the gate below. `TestCalibrate_UsesProductionParser` pins it.

The question is not the loop's question word for word. The loop asks whether a
criterion is met by what the actions produced; a checkpoint holds a plan that
has not run, so calibration asks whether the plan should be accepted as
proposed. The parser and the reply contract are shared. The prompt is not.

**Rejected: a parser inside the eval package.** It is the smaller change and
keeps `loop`'s surface closed. It also measures a judge nobody runs: the two
parsers drift, and the one under test is the one that does not grade anything.

## Decision 2 — the gate replays recorded replies, and calls no model

`internal/feature/eval/testdata/golden.v1.json` is a versioned set of judge
replies, each with a label, a `provenance` and a `note`. `Gate` runs a verdict
function over every reply and fails when agreement with the labels is below the
set's committed `baseline`. The baseline is 1.0, the agreement `VerdictIsPass`
reaches on the set, and a test asserts the two are equal so the baseline cannot
be lowered quietly. `LoadGoldenSet` refuses a set it could not score the way it
claims: no version, no entries, an entry with no provenance. An empty set fails.

It runs under `go test ./...` in CI's Test job. `TestGateGoesRedWithPhase25Parser`
re-implements the pre-Phase-25 parser and asserts the gate fails on it, the same
way ADR 020 verifies a declaration by running it.

**Rejected: calling the judge in CI.** It would measure the model as well as the
parser, which sounds like more. In practice a red build could mean a regression,
a provider change or sampling noise, and nobody could tell which. The live
measurement exists and is on demand: `krk eval calibrate`, behind `eval:run`.

## Decision 3 — the world state is recorded on the checkpoint

`Checkpoint.WorldState` holds the `loop.WorldState` the planner saw when it
escalated, in a `world_state_json` column on `checkpoints` (migration 000011).
`recordedWorldState` bounds it: 16 KiB of JSON per observation, 256 KiB in
total, with a `_truncated` marker in place of what was cut. Each observation's
`Trust` and the `Blind` list are kept unchanged, because a replay that forgot
which text was third-party would run without the notice ADR 021 gave the
original planner. A budget pause records none; it labels a spending decision,
not a plan. The field is `json:"-"` and is not on the REST response.

**Rejected: a new table of recorded world states.** The world state has no
meaning apart from the checkpoint whose plan it explains, and a second table
adds a join, a retention policy and a way for the two to disagree. The bound is
what keeps the row small, and it would be needed in either place.

**Rejected: putting it in the escalation audit payload.** `tool_events` is the
record an assessor reads. Third-party text does not belong in it at a quarter
of a megabyte a row.

## Decision 4 — operator-labelled history is not exported into the golden set

`ExportGolden` and `krk eval calibrate --export` turn calibration items into
golden entries. Nothing from this deployment was exported. All 18 entries in
`golden.v1.json` are constructed, and each note says which parser behaviour it
pins.

The reason is who labelled the history. Most of the 34 resolved checkpoints were
decided by an operator account driving Karakuri's own delivery, largely by an AI
assistant acting as that operator, and many rejections were procedural. A golden
entry is a claim that a label is right. These labels do not support that claim.

**Rejected: exporting them anyway, since they are real.** A gate whose baseline
is 55.9% agreement with procedural rejections would pin noise, and the first
honest improvement to the judge would turn it red.

## Consequences

- **The agreement rate is a number.** 55.9% (19 of 34), in `docs/benchmarks.md`
  with its confusion matrix. It is one measurement, taken on demand. Nothing
  tracks it over time.
- **That number is not a verdict on the judge.** Agreement is 93.8% on approvals
  and 15.4% on rejections, because the judge sees the objective and the plan and
  cannot see that the work was already done or that the plan was an error
  placeholder. What it shows is a judge that passes 29 of 34 plans.
- **Agreement is not correctness.** A human who approved a bad plan labels it
  approved.
- **The corpus over-represents hard cases.** Routine competence never escalates,
  so it never generates a label.
- **The gate measures the parser, not the model.** A judge that starts answering
  differently is invisible to it. It catches the Phase 25 regression and nothing
  wider.
- **Replay is not built.** The corpus for it is 2 checkpoints, because recording
  started with this phase. Checkpoints written before migration 000011 can never
  be replayed.
- **`loop.VerdictIsPass` is exported.** `internal/feature/loop` has one more
  public name, and `internal/feature/eval` imports a sibling feature package.
- **A new permission and a new column.** `eval:run` is admin only: the route
  spends a model call per checkpoint and reads checkpoints across twins. A
  checkpoint row can grow by up to roughly 256 KiB.
- **A run takes minutes.** One judge call per checkpoint; 34 took 2 minutes 57
  seconds through a CLI-backed provider. The CLI command has its own 30-minute
  bound, and the server stops judging when the caller disconnects.

## Alternatives considered

**Extending `cmd/krk-bench` instead of a new package.** The bench is a seeded
simulation with no storage and no provider. Calibration reads the database and
calls a model. They share nothing but the word "benchmark", and the synthetic
harness stays as it is.

**Treating `modify` as a positive label.** A modified plan did run, in some
form. It was rejected because the label is about the plan as drafted, which is
what the judge is shown, and the reviewer would not let that run.

**Skipping checkpoints the judge could not answer.** Rejected: in the loop an
error is a FAIL, and dropping those items would report a judge more agreeable
than the one in production. The report counts them separately as judge errors.
