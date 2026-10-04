# ADR 026 — An environment that cannot see says so

**Status:** Accepted
**Date:** 2026-10-04
**Relates to:** [ADR 006](006-multi-instance-tool-adapters.md) (named tool instances bound per twin), [ADR 015](015-standing-objectives-and-reconciliation.md) (one gate: authority is written into the request), [ADR 017](017-karakuri-as-a-domain-pack.md) (a snapshot SHA answers "is this worth waking up for"), [ADR 019](019-capabilities-declare-what-they-need.md) (a declared capability has to be served), [ADR 021](021-observations-carry-provenance.md) (trust is set from what the payload holds)

## Context

The software pack has declared `software.env.observability`, an SRE agent and an
incident-response template since Phase 2. Until Phase 32 none of it could see
anything. `internal/platform/tools/observability/noop.go` was the only
`ObservabilityAdapter`, `Registry.Observability` was a bare field holding it,
and the environment was a `noopFactory` that built a `noopEnv`.

`noopEnv.Snapshot` returned the constant SHA `noop-snapshot`. Phase 20's
reconcile supervisor compares snapshot SHAs to decide whether the world moved.
A constant SHA is a world that never moves, so a standing incident objective on
a deployment with no observability backend would have observed once, found
nothing, and been left alone for good. The health endpoint agreed with it: the
no-op adapter had a hard-coded row.

Three constraints shaped the work:

- Phase 20 already has a way to say "could not look". The loop lists an
  environment whose `Observe` failed in `WorldState.Blind`, and reconcile's
  fingerprint lists an environment whose snapshot has no SHA as blind.
- The loop decides nothing about authority. ADR 015 has one gate,
  `agent.AuthorityBounds`, and an environment does not add a second.
- An alert's text is written by whoever wrote the alert rule or the incident
  title. ADR 021 says the observation carrying it is third-party.

## Decision 1 — blind is an error from Observe and an empty SHA from Snapshot

`observabilityEnv` in `domains/software/observability_env.go` holds the adapter
the twin's `AdapterBindings["observability"]` resolves to, or a true nil. When
no instance is bound, when the bound instance is not active, or when the
adapter cannot answer alerts, `Observe` returns an error and `Snapshot` returns
an `EnvironmentSnapshot` with an empty SHA. The loop then lists the environment
in `WorldState.Blind` and reconcile lists it as blind in its fingerprint. No new
mechanism was added; both lists are Phase 20's.

`noop.go` is deleted. `tools.Registry.Observability` is a
`SlotInstances[observability.ObservabilityAdapter]` built from the
`tools.observability` config (a default and named instances), and
`AdapterStatuses` reports a row per configured instance and no row when there
are none. This is not the top-level `observability:` config section, which
configures telemetry exporters.

**Rejected: an observation that says `status: noop`.** It is a successful
observation. The planner would have to read a field to learn that it knows
nothing, and reconcile would still get a SHA that never changes.

**Rejected: a constant SHA.** This is what was there. It reads as a quiet
world.

**Rejected: keeping a no-op adapter as the fallback.** The slot could not be
made multi-instance with it in place: the no-op was what made an unconfigured
deployment look healthy.

## Decision 2 — an adapter returns ErrUnsupported for a signal it does not have

`ObservabilityAdapter` in `internal/platform/tools/observability/adapter.go` has
`GetAlerts`, `FetchLogs` and `FetchMetrics`. No shipped backend except Datadog
has all three. An adapter returns `observability.ErrUnsupported`, wrapped with
its own name, for the ones it lacks: `prometheus.go` for logs, `loki.go` for
alerts and metrics, `pagerduty.go` for logs and metrics.

The same rule covers a response the adapter could not read whole. Each adapter
reads at most 8 MiB (`body.go`) and a larger body is an error, a non-2xx status
is an error, and PagerDuty refuses a list when the API reports more incidents
than the one page it read.

**Rejected: returning an empty slice.** An empty result means the adapter
looked and found nothing. From a backend that has no such signal it is
indistinguishable from "nothing is wrong", and `alerts_resolved` would pass on
it.

## Decision 3 — the Snapshot SHA hashes the open alert set, not the observation

`observabilityFingerprint` hashes the open alerts as sorted IDs each with its
state, and the per-severity counts bucketed by order of magnitude. Messages,
timestamps, values and raw counts are left out. `Alert.ID` is the provider's
fingerprint when it has one and is otherwise derived from the alert's name and
sorted labels, never from a timestamp or a value, so the same alert has the same
ID on the next poll. This is [ADR 017](017-karakuri-as-a-domain-pack.md)'s
rule: an alert starting, resolving or being acknowledged is worth waking up
for; the same alerts still firing are not.

**Rejected: hashing the observation.** An alert's message is re-rendered with
the current value on every evaluation interval. A hash over it would move every
interval and an incident objective would reconcile continuously on one unchanged
alert.

## Decision 4 — remediation is a capability with no approval logic in it

`software.act.run_remediation` is served by `software.env.remediation`
(`domains/software/remediation_env.go`). It runs a command through the shell
executor, so the denylist, the workdir confinement and the timeout are the
shell's. It refuses to run without `alert_id`, `rationale` and `cmd`, and it
records the first two on the result, so an executed remediation names the alert
it was for and why.

It does not decide whether it may run. The SRE agent in
`domains/software/agents.go` lists the capability in
`AuthorityBounds.RequiresApprovalFor`, and that is the gate
([ADR 015](015-standing-objectives-and-reconciliation.md)).

**Rejected: a second gate inside the environment.** An environment that checks
for an approval is a second definition of who may act, and the two drift. The
loop already escalates and already records the approver.

## Decision 5 — "remediation applied" is verified by looking at the alerts

`software.verify.alerts_resolved`, served by the observability environment,
takes `alert_ids` and succeeds only when none of them is still open. It fails
when it cannot look: no instance bound, an inactive instance, or an adapter
error. The incident template in `domains/software/objectives.go` verifies its
remediation criterion with it.

**Rejected: `software.verify.run_tests`.** This is what the template used. A
passing test run says nothing about whether the incident is over.

## Consequences

- **Nothing here was run against a live Prometheus, Loki, Datadog or
  PagerDuty.** The adapters are tested against `httptest` servers built from
  those APIs' documented shapes. The roadmap's acceptance says "observes real
  alerts". What is shown is the whole path, with a scripted observability
  instance, in `internal/feature/loop/incident_test.go`.
- **A twin binds one observability instance.** Prometheus has no logs and Loki
  has no alerts or metrics. A twin bound to Loki alone is blind, because it
  cannot answer alerts, and a twin bound to Prometheus cannot fetch logs.
  Datadog is the only shipped type that answers all three. Binding several
  instances to one twin is not possible yet.
- **`run_remediation` is a shell command on the machine the server runs on.**
  It sits behind the shell denylist, which `domains/software/shell_env.go`
  describes as a guard against accidental harm and not a security boundary.
  There is no Kubernetes, cloud or runbook adapter. The safety of a remediation
  rests on the human approval, which is why it is always escalated.
- **An incident plan always escalates, for two independent reasons.** An alert's
  text in the evidence is third-party material
  ([ADR 021](021-observations-carry-provenance.md)); provenance is checked
  first and is the reason the audit row records. And `run_remediation` is on
  the SRE agent's approval list. So earned autonomy never applies to a
  remediation. This is a property: the phase adds no new way to say yes.
- **`alerts_resolved` trusts the IDs it is given.** It reports an alert
  resolved when its ID is not in the open set, and it cannot tell an alert that
  cleared from an ID that never named one. A plan that passes the wrong ID
  meets the criterion. The IDs are in the plan a person approves, and the
  verifier keeps no history to check them against.
- **A verification in the same plan as its remediation looks immediately.** A
  backend that takes an evaluation interval to clear an alert will still show
  it open, the criterion is not met in that iteration, and a later iteration
  has to look again.
- **The acceptance test scores the verifier-backed criterion only.** The
  root-cause criterion is judged by a model, and no judge is wired into that
  harness.
- **Datadog events are not read**, because nothing in the interface carries
  one. **Opsgenie is not implemented**; the roadmap said "PagerDuty or
  Opsgenie".
- **The served-capability check stays a test in the software pack**
  (`domains/software/routing_test.go`). It now covers `software.observe.*` as
  well as `software.act.*`. It is not a conformance check, because other packs'
  observe capabilities are deliberately served by placeholder environments
  ([ADR 019](019-capabilities-declare-what-they-need.md)). `reason.*`,
  `decide.*` and `learn.*` remain uncovered, as Phase 25 recorded.
- **The vendor figures the roadmap cites for the field are not measured here.**
  Nothing in this phase measures how many incidents are handled without a
  person or how long resolution takes.
- **A deployment with no observability instance now reports nothing for the
  slot** in `/health`, where it used to report an active no-op.

## Alternatives considered

**A `Blind` field on `Observation`.** It would say the same thing in a typed
way. Phase 20 already treats a failed `Observe` and an empty SHA as blind, and a
field would be a second way to say it that an environment could forget to set.

**One environment per signal, each bound to its own instance.** It would let a
twin use Prometheus for alerts and Loki for logs. It is what lifting the
one-instance limit probably looks like, and it was left out because the binding
map holds one name per slot today.

**Approval recorded by the remediation environment.** The approver is already
on the `tool_events` approval row the loop writes; a second record could
disagree with it.
