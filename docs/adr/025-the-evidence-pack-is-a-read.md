# ADR 025 — The evidence pack is a read, and it says what it cannot show

**Status:** Accepted
**Date:** 2026-10-03
**Relates to:** [ADR 015](015-standing-objectives-and-reconciliation.md) (the supervisor writes authority into the request, and the loop does not infer it), [ADR 016](016-earned-autonomy-and-digests.md) (a digest reads and accumulates nothing), [ADR 024](024-the-evaluation-set-is-recorded-history.md) (who labelled this deployment's checkpoints)

## Context

`tool_events` is the audit log. Since Phase 13 it has recorded every escalation
and execute, and since Phase 13.5 every approval, rejection and modification
with the approver account; Phase 20 added promotions and demotions. Phase 31
asked for the record an operator can hand to somebody who asks what a deployment
did in a past window. Four things were missing:

- Nothing said how long the log is kept. Memory has had a retention scheduler
  since Phase 13. The audit log had no stated minimum.
- A decision row named the agent and nothing else about how the decision was
  reached. The cost ledger carried the provider and model of the same call; the
  audit row did not.
- An objective template said nothing about how much it matters when work under
  it goes wrong.
- There was no export.

Three constraints shaped the work:

- `config` is a leaf package. It parses YAML and imports nothing from
  `internal/`.
- The loop does not infer authority. ADR 015 has the reconcile supervisor write
  `agent.AuthorityBounds` into the request, and the same has to hold for
  anything recorded about that authority.
- A record that is handed to an assessor is read by somebody who was not there.
  Anything it implies and cannot back is worse than a gap it names.

## Decision 1 — the floor is enforced where the delete happens

`internal/feature/audit/retention.go` declares `FloorDays = 183` and
`CheckRetention`, which refuses a floor below 183 and a retention below the
floor. Two config keys feed it: `audit.retention.floor_days`, configurable
upward only, and `audit.retention.days`, where 0 means never prune.

The check runs twice. `auditRetention` in `internal/app/bootstrap.go` runs it at
startup, before the database is opened, and the server refuses to start with a
message that names the floor and the value refused. `audit.Service.Prune` runs
it again before every delete. `Prune` is the only caller of
`GORMStorage.DeleteToolEventsBefore`, so a `Service` constructed with a
retention below the floor, by a test or by a future caller that skipped
bootstrap, deletes nothing and returns the error. `startAuditRetention` starts
the daily sweep only when `days` is set.

Package `config` parses the two keys and validates neither. The floor has one
definition, in the package that deletes.

**Rejected: validating only where config is parsed.** It is where a reader
expects it and it gives the earliest error. It also protects only the path that
goes through the config file. The delete would trust whoever constructed the
pruner, and the floor would be a property of startup, not of the log.

**Rejected: rounding a low value up to the floor.** The operator who wrote
`days: 30` believes rows are gone after 30 days. Keeping them for 183 without
saying so is a second misconfiguration on top of the first.

**Rejected: a floor constant in `config`.** `config` would then own a rule about
the audit log, and `internal/feature/audit` would import it or repeat it.

## Decision 2 — provenance is columns

Migration `000012_decision_provenance` adds `provider`, `model`, `template_id`
and `autonomy_rung` to `tool_events`, and `template_id` to `objectives`.
`stepContext.stampProvenance` in `internal/feature/loop/runner.go` writes the
four columns on escalation and execute rows, and puts the agent definition, the
reasoning strategy, the bounds applied and the template's risk class in the
payload. Both row kinds go through the one function.

The provider and model come from the plan, which carries those of the call that
produced it and carries neither when the call failed. The rung comes from
`loop.Request.AutonomyRung`, which `reconcileNow` writes beside the bounds it
derived. The loop copies it and a one-shot run leaves it empty.
`storage.ToolEventFilter`, `GET /api/v1/audit` and `krk audit` filter by
provider, model and template.

An empty column means "not recorded" on a row written before the migration. On
a row written after it, an empty provider and model mean no model drafted the
plan, and an empty rung means a one-shot run. The export does not try to tell
these apart: it counts a decision row with no provider, no model and no template
as having no provenance.

**Rejected: putting all of it in `payload_json`.** No migration, and the payload
already holds the bounds. But "every decision this model produced" would be a
scan of every row's JSON, written differently for SQLite and PostgreSQL. The
four things an auditor filters on are columns; the things they read once they
have the row stay in the payload.

**Rejected: having the loop work the rung out from the bounds.** Two rungs can
produce the same bounds, and a rule that maps bounds back to rungs would be a
second definition of the ladder that drifts from the first.

**Rejected: recording the configured default provider when the call failed.**
No model produced that plan. A guessed name on the row would be a false record.

## Decision 3 — risk classes are the deployment's own words

`objective.RiskClass` in `internal/core/objective/risk.go` has four values:
unclassified, `routine`, `consequential` and `high`. Each is defined by what the
pack author thinks a mistake costs: cheap to notice and undo; costly or slow to
undo; able to harm a person's health, safety, rights or livelihood.
`objective.Template.Risk` carries one. `conformance.CheckTemplateRisk` reports
each active template's class at boot. An unclassified template is a warning, a
value outside the set is a warning that names it, and neither refuses startup.
Each decision row records the class in force when the decision was taken,
because a template can be reclassified later.

**Rejected: the tiers of a regulation.** Whether a deployment is high-risk in a
regulatory sense depends on who runs it, for whom and to what end. A template
cannot know that. A field that used a regulation's vocabulary would read as a
determination nobody made, and a pack author marking a template `high` would
appear to have made a legal finding.

**Rejected: refusing to start on an unclassified template.** A pack written
before the field existed is unclassified. Saying so in the log is the point of
the step; a hard stop would turn a declaration into a gate, and ADR 015 allows
one gate.

## Decision 4 — the export is a read, with no generation time, and it refuses an open window

`audit.Exporter` in `internal/feature/audit/export.go` builds one JSON document
for the closed-open window `[from, to)`. It reads `tool_events` and checkpoints
and writes nothing, the way `internal/feature/report` builds a digest. Rows are
sorted by `created_at` then `id` inside the exporter so the order does not
depend on the store. Payloads are re-encoded with sorted keys. Struct field
order is the byte order, and `schema_version` changes when it does.

The current time is used once: a window whose `to` is after now is refused,
because an open window cannot produce the same bytes tomorrow. It never appears
in the document. The retention note is measured from the window's own end for
the same reason. A pending checkpoint is exported with its id, objective and
raise time only, so resolving it later does not change an earlier window.

`GET /api/v1/audit/export` writes the exporter's bytes unchanged. `krk audit
export` writes what it received, to stdout or to a file with mode 0600, and
prints the SHA-256 of those bytes to stderr.

**Rejected: a `generated_at` field.** It is the first thing anyone adds to an
export, and it makes two exports of one window differ. When the file was
produced is a fact about the file, and the operator's shell already has it.

**Rejected: exporting an open window and marking it partial.** Two partial
exports of "today so far" differ and both are correct, which defeats comparing
hashes at all.

**Rejected: a stored export.** A table of generated exports is a second record
that can disagree with the first, with its own retention. A failed export is
retried.

## Consequences

What this is not comes first, because the rest is read in its light.

- **It is not a compliance certification.** The phase produces a record. Whether
  a deployment is high-risk, and whether the record is enough for its assessor,
  are somebody else's determinations. The export says so in its own
  `not_a_certification` field. What can be said is which kinds of record it
  contains: decision rows, checkpoint resolutions, autonomy changes, counts by
  kind, the retention in force and the templates' declared classes.
- **The risk classes are not legal categories.** They are what a pack author
  wrote about their own templates.
- **The export shows which approver account resolved a checkpoint. It cannot
  show who or what was operating that account.** On this deployment the account
  was largely an AI assistant acting as operator on the owner's behalf. So
  "human oversight" here means the mechanism existed and was exercised under a
  named account, and nothing stronger. The export's `oversight.statement` ends:
  "The record shows which account decided; it cannot show who was operating that
  account." The boolean beside it is named `person_consulted`. The name claims
  more than the statement does, and it should be read as "an approver account
  acted in this window".
- **Provenance exists only for rows written after this phase.** Older rows are
  counted in `decisions.without_provenance`. They are not backfilled, because
  the provider and model of a past call are not recoverable from the row.
- **Byte-identity holds while the rows are retained.** Once a retention horizon
  prunes rows from a window, an export of that window changes. The export's
  `retention.note` says so without knowing today's date, because knowing it
  would make the bytes depend on when the export was asked for.

Then what was measured, on this deployment on 2026-10-03:

- **The same window twice gave the same bytes.** 2026-09-26T00:00:00Z to
  2026-10-03T12:00:00Z: 1,682,039 bytes each time, SHA-256
  `9a7d2df5a86362477e061176cc60722bb39d56e288b9b850fece7750654db0c5` both times.
- **An open window was refused.** "a window must have ended to be exported,
  because an open window cannot produce the same bytes tomorrow".
- **The window held** 41 escalations, 73 executes, 19 approvals, 32 rejections
  and 5 modifications; 41 checkpoints raised and 41 resolved.
- **114 of 114 decision rows had no recorded provenance.** Every one was written
  before migration 000012. The export counts them; it does not hide them.

And what the design costs or leaves open:

- **The prompt behind a decision is not recorded.** The roadmap's gap list named
  a prompt version; the steps did not. A row says which provider, model, agent
  definition, template, rung and bounds, and not what the model was asked.
- **The export is reproducible, not tamper-evident.** It is not signed and the
  rows are not hash-chained. A matching SHA-256 shows two exports agree. It does
  not show that neither was edited, or that the rows were not.
- **It is not scoped by tenant.** It covers every twin and is behind
  `audit:read`, like the audit listing. No new permission was added.
- **A window is assembled in memory.** Eight days was 1.7 MB here. There is no
  streaming and no size limit.
- **The floor guards one path.** `Prune` is the only delete in the code. A
  migration or an operator with database access is not checked by it.
- **The new columns are not indexed.** A filter by provider, model or template
  is a `WHERE` clause over an unindexed column.
- **Migration 000012 is additive.** Five columns, each `NOT NULL DEFAULT ''`.
- **Startup can now be refused by two more keys.** A deployment that sets
  `audit.retention.days` below 183 does not start.
- **The software delivery template's two review criteria are still served by no
  environment.** Unchanged by this phase; see Phase 29's deferred list.

## Alternatives considered

**A separate evidence table, written as decisions are taken.** It would hold
exactly the export's shape and make the export a `SELECT`. It was rejected for
the reason Phase 21 refused to accumulate digests: a second record can disagree
with the first, and an export that is a read of `tool_events` cannot.

**Backfilling provenance from the cost ledger.** The ledger has the provider and
model of each call, and a join on objective and time would fill many old rows.
It would also fill some of them wrongly, and a row cannot say that its
provenance is a guess. Counting them as having none is true.

**An `oversight` section that omits itself when nothing happened.** The
acceptance asks that a window with no checkpoint says so plainly. An absent
section reads as an export that forgot one.
