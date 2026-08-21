# AI Use Cases, 2026 — The Evidence Behind Phases 27–32

## What this document is for

`software.reason.analyse_repo` reads the roadmap's deferred lists as evidence
because each line is work somebody already justified in prose (Phase 25). This
file is the same thing for the phases that have no history to stand on yet: the
field evidence Phases 27–32 were proposed from, recorded once, with sources, so
a later phase can cite it rather than re-deriving the landscape from a model's
recollection.

It is written to the same rule the karakuri pack works under. A claim about the
field and a claim about this tree are different claims and are not presented
identically: every finding below states what the field reports, then what this
repository actually does, with a path. The second half was verified by reading
the code on 21 August 2026 and is the half that decides anything.

## Method, and its limits

Eight searches over the public web in August 2026, plus a read of this tree.

The limits are worth stating plainly, because several of the numbers below are
the kind that get repeated until they sound measured:

- **Most sources are secondary.** Vendor blogs and analyst summaries citing
  surveys, not the surveys. Where two sources disagree — pilot-to-production
  failure is variously 86%, 88% and 95%, over different denominators — the
  spread is reported rather than the most quotable figure.
- **Adoption statistics are directional at best.** They establish that
  evaluation and governance rank above model capability as blockers. They do not
  establish the magnitude, and nothing below should be built because of a
  percentage.
- **The tree facts are not directional.** They were read, they carry paths, and
  they are what the phases actually rest on. A finding whose field half is
  strong and whose tree half is empty produced no phase.

---

## Finding 1 — Agents are redirected through what they read

**The field.** OWASP's Top 10 for Agentic Applications 2026 places Agent Goal
Hijack at ASI01: an agent's objectives or task selection manipulated through the
natural-language content it ingests. The framework's stated distinction from the
LLM Top 10 is the one that matters here — it covers what happens when the model
becomes an actor with credentials, tools, memory and the autonomy to chain
actions. Its recurring mitigation is human approval for consequential actions
rather than better prompting.

**This tree.** `stepObserve` (`internal/feature/loop/observe.go`) fans out
across every environment and merges the results into one flat `WorldState`.
Nothing on `environment.Observation` says where a payload came from, so every
fact in it is equally trusted by the planner that reads it next. And Karakuri
acts on what it concludes — it opens pull requests and sends mail.

**What actually reaches the planner is narrower than the threat model suggests,
and reading the code changed the shape of the phase.** `gitEnv` carries commit
lists and `PRSummary` records — title, URL, check state, failing checks — with
no body and no issue operation. `commsEnv` reads `q.Filter["channel"]`, and
`stepObserve` passes `ObservationQuery{Limit: 20}` with no filter, so Slack
message bodies are reachable and never fetched. `researchEnv.Observe` returns
only whether an adapter is wired, deliberately and with a comment saying why;
scraped pages arrive on the **act** path as `ActionResult.StateDelta`. No domain
pack registers an email environment at all — `Registry.Email` is consumed only
by digest delivery.

So the widest untrusted surface today is action results rather than
observations. A phase that marked only `Observation` — which is what the field's
framing points at — would leave the larger hole open.

A second defect sits in the same loop. An environment whose `Observe` returns an
error is skipped with a bare `continue`, so an environment that went blind is
indistinguishable from one that saw nothing. Phase 20 refused exactly this
conflation on the outer loop — a blind environment is named in the outcome and
its objective is driven by its schedule instead — and the inner loop never
learned it.

**What follows.** Phase 27. The fix belongs on `AuthorityBounds.Decide`, the
policy Phase 24 extracted so it could be run rather than read; a second gate
beside it would contradict ADR 015.

## Finding 2 — The tool layer standardised, and this one did not

**The field.** MCP was created by Anthropic and donated to the Linux
Foundation's Agentic AI Foundation in December 2025; secondary sources put the
ecosystem past 110 million monthly downloads. The two-layer shape — MCP for
vertical tool access, A2A for horizontal agent-to-agent delegation — is
described consistently across 2026 surveys as the default enterprise
architecture, with a joint MCP/A2A specification reported as the first formal
bridge between them.

**This tree.** `grep -ri "mcp\|a2a"` over Go, Markdown and YAML returns nothing.
The tool layer is ten hand-written adapters across seven slots (Phase 6), and
adding an integration means a Go file plus a case in a type-string switch —
`tools.SlotInstances.Set` exists precisely because that switch could not be
extended from outside, and it was added for a test, not for an integration.

The shape to carry MCP is already here, which is the argument for the phase
rather than against it: ADR 006's per-slot `default` plus named `instances`,
twin-bound through `AdapterBindings`, is what an MCP client configuration looks
like. What does not fit is the capability model. Pack capabilities are declared
statically and conformance-checked at boot; MCP tools are discovered at runtime.
That is a real design decision, not a wiring exercise, and it is the one thing
in Phase 28 that wants an ADR.

**What follows.** Phase 28, after Phase 27 — a third-party tool *description*
reaches the planner's context, which is the ASI01 surface again, arriving from a
server nobody in this repository wrote.

## Finding 3 — Everyone agreed what an agent span looks like

**The field.** The OpenTelemetry GenAI semantic conventions, under CNCF, now
cover LLM client spans, agent spans, tool execution, content capture and
evaluation. The span tree is `invoke_agent` at the top with `chat` children per
model call and `execute_tool` per tool invocation. Datadog, New Relic and
Honeycomb consume the conventions natively; LangChain, CrewAI and AutoGen emit
them. The stated benefit is uniformity — a span from one framework looks like a
span from another.

**This tree.** `internal/platform/observability/otel.go` records metrics and
logs and emits no spans; `grep` for `gen_ai` returns nothing. The names are
Karakuri's own: `loop_iteration_duration_ms`, `agent_invocation`,
`agent_latency_ms`, `tokens_used`, `memory_recall_count`.

The gap is worth more here than it would be in most projects, because Phase 12
already shipped exporters to Datadog, New Relic, Elasticsearch, Loki, OTLP and
Prometheus with per-exporter isolation and retry. Every one of those backends
now ships agent dashboards that Karakuri's telemetry does not populate — for
naming reasons alone. This is the cheapest finding in the document to act on and
the only one that is purely additive.

**What follows.** Phase 29. Existing metric names are kept and aliased; renaming
them for tidiness would break dashboards operators already have.

## Finding 4 — Evaluation is the blocker, and Karakuri is sitting on a labelled set

**The field.** Reported pilot-to-production failure runs 86–95% depending on
source and denominator, and the blockers named ahead of model capability are
consistent across them: evaluation, governance friction, integration brittleness
and unclear process ownership. The received practice by 2026 is a versioned
golden set of a few hundred cases built from *real* failures rather than
synthetic ones, trajectory scoring alongside final-answer scoring — an agent
that reaches the right answer through a dangerous sequence of tool calls is a
production failure — and an LLM judge calibrated to roughly 85–90% agreement
with human labels before it is trusted to gate anything.

**This tree.** The golden set exists and nothing reads it. Every escalation
writes a checkpoint carrying the planner's proposed `Actions`
(`internal/core/checkpoint`), and every resolution writes a human verdict —
`kind=approval`, `rejection` or `modification` in `tool_events`
(`internal/platform/storage/adapter.go:108`), with `Decision.Choice`,
`Approver` and `Modifications` alongside. Those are human-labelled *plans*,
accumulating in every deployment, produced by people doing their jobs rather
than by an annotation budget.

**Not trajectories, though, and the difference decides the phase.** What the
agent saw is not kept: `loop_states` holds counters and the request, the
escalation audit payload holds actions and confidence, and the checkpoint holds
neither. So calibrating the judge — would it have said what the person said —
works on stored rows today, and replaying a *planner* against the same situation
does not, because the situation was never written down. Phase 30 has to record
that before it can replay anything, and saying otherwise would be claiming a
corpus this deployment does not have.

What exists instead is `cmd/krk-bench`, which says in its own header that it
drives a seeded stochastic mock rather than a real provider — reproducible and
free, and answering a different question. And `evaluateWithAgent`, the judge
that settles every criterion in every pack, has never been calibrated against
anything. Phase 25 gave it the evidence it was previously judging without; how
often it agrees with a person is still unmeasured.

**What follows.** Phase 30, and the phase text has to state what the set is not:
a resolved checkpoint labels one plan in one situation, agreement is not
correctness, and a corpus drawn from escalations over-represents hard cases by
construction — routine competence never escalates, so it never generates a
label.

## Finding 5 — The high-risk deadline passed three weeks ago

**The field.** The EU AI Act's high-risk obligations became enforceable on
2 August 2026: Article 12 automatic event logging over the system's lifetime
with a six-month minimum retention, Article 14 human oversight that a person can
actually exercise and interrupt, plus risk management, data governance,
transparency, cybersecurity and post-market monitoring. Sources analysing agent
systems specifically read the action layer — API calls, third-party platforms,
MCP servers — as in scope, and note that in a chain of agents the boundary
extends to every agent performing a high-risk function. Penalties are cited at
up to €15M or 3% of worldwide turnover.

**This tree.** Karakuri is closer than most and short in specific places.
Already present: `tool_events` records every escalation, approval, rejection,
modification, promotion and demotion with an approver; checkpoints are
designed-in human interruption rather than a bolted-on review step; digests
(Phase 21) are post-market monitoring under another name, and are reproducible
for any past window because they accumulate nothing; HALA already commits the
project to human oversight of consequential decisions.

Missing, and each is narrow: no declared retention floor on audit rows — memory
has a retention scheduler (Phase 13), the audit log has no stated minimum; no
model, provider or prompt version recorded against the decision it produced,
though the cost ledger carries provider and model for the same call; no risk
classification on an objective template; and no export.

**What follows.** Phase 31, built the way `internal/feature/report` builds a
digest — read-only, accumulating nothing, the same window producing the same
bytes tomorrow.

## Finding 6 — The vertical that matches this architecture is the one it cannot reach

**The field.** Autonomous incident response is where agentic systems actually
went into production in 2026. Reported figures: Tier-1 incidents handled at
around 90% accuracy, MTTR down more than 40% in Kubernetes-native deployments,
AWS and Microsoft both shipping GA reliability agents in March 2026. The
consistent caveat is that production change stays human-approved, and that the
limits are trust, topology quality and governance rather than reasoning.

The fit with this architecture is unusually exact, and not because of the
subject matter. A standing objective is a desired state held rather than a task
finished; sensing is cheap and reconciliation is rare; autonomy is earned under
an operator's ceiling and one rejection demotes. That is a description of a
self-healing control loop with a human gate, and Phase 20 already built it.

**This tree.** The use case is declared throughout and served nowhere.
`ObservabilityAdapter` (`internal/platform/tools/observability/adapter.go`) is
an interface and a no-op; Phase 6 shipped ten adapter implementations and none
of them was this one. It is also the only slot that never became multi-instance
— `Registry.Observability` is a bare interface field rather than a
`SlotInstances[T]`, so it consults no `AdapterBindings` — and the interface
exposes `GetAlerts` and `Active` only, while the capabilities to be served are
`fetch_logs` and `fetch_metrics`. `software.env.observability`
(`domains/software/environments.go:113`) is a `noopFactory` — it declares no
`Serves` and builds a `noopEnv`. The `sre` agent
(`domains/software/agents.go:75`) lists `software.observe.fetch_logs` and
`software.observe.fetch_metrics`; both are declared in
`domains/software/capabilities.go` and served by that no-op. The
`incident_response` template (`domains/software/objectives.go:66`) has existed
since Phase 2.

This is the defect class Phases 24, 25 and 26 each closed an instance of —
declared, planned by models, served by nothing — and it is sitting in the part
of the namespace where the check does not look. The property tests Phase 26
added cover `software.act.*`; Phase 25 recorded that `reason.*` is uncovered and
called it the ninth instance. `observe.*` is uncovered on the same grounds.

**What follows.** Phase 32 — a slot brought up to ADR 006, a widened adapter
interface, its implementations, and one environment. No new engine, but more
than the "just adapter work" it looks like from the outside, which is worth
saying before somebody sizes it from the outside.

## Finding 7 — Memory is where this tree is ahead

**The field.** Context rot — degradation as irrelevant tool output and stale
intermediate state accumulate, silently and without error — is the dominant
2026 concern for long-horizon agents, with reported accuracy losses across a
wide band. The practices are compaction with sliding windows, hierarchical
summarisation, and keeping full state outside the model while compiling only
what each turn needs.

**This tree.** Four memory tiers with consolidation, retention TTL and a
confidence floor (Phases 1, 3, 13), and — more to the point — a loop that
terminates. Phase 20 was explicit that anything which must keep running belongs
to the reconcile supervisor, which *calls* the loop rather than extending it, so
the long-running thing here is a schedule rather than a context window. The
exposure that remains is narrower than the field's: a single loop's iterations
accumulate, and nothing measures how much.

**What follows.** Nothing yet. This is recorded as deferred rather than phased,
because the practice being recommended is mostly already the architecture, and
a phase that restates it would be the kind of speculative work AGENTS.md's YAGNI
rule exists to prevent. Worth revisiting with a measurement first.

---

## Recorded and deliberately not phased

**Agent identity and scoped credentials.** The 2026 pattern gives an agent its
own principal and issues per-invocation delegation tokens carrying the human's
authority separately, so an action is attributed to both; a survey of 900+
practitioners is reported as finding 93% of agent projects using unscoped API
keys and 74% of agents over-permissioned. Karakuri's outbound side matches the
weak pattern: adapters authenticate with static `*_env` credentials shared
across every twin, and a `tool_events` row records the adapter, not on whose
authority the call was made. The inbound side is the opposite — Phases 14–18
built RBAC, scope sets, hierarchical containers and federated identity.

It is left out of this slate for sequencing, not merit. It touches every adapter
in `internal/platform/tools/`, and doing it before Phase 28 would mean doing it
twice, since MCP brings its own authorisation story. The natural home is after
MCP has landed and there is one credential model to design rather than two.

**Further verticals.** Legal, mechanical and consulting are 30-line stubs.
Legal and consulting are document-centric and would want a retrieval and
ingestion layer this tree does not have, so proposing them now would be
proposing that layer under another name. SRE is proposed instead because it
needs no primitive Karakuri lacks.

**A2A.** Real, and the horizontal half of the standard stack. Phase 28's
outbound MCP server covers the case that matters first — another runtime driving
a Karakuri objective — through auth that already exists. A2A after there is
demand for the direction this deployment does not yet have.

---

## Sources

Secondary unless marked. Retrieved 21 August 2026.

**Agentic security**
- [OWASP Top 10 for Agentic Applications 2026](https://genai.owasp.org/resource/owasp-top-10-for-agentic-applications-for-2026/) — primary (OWASP GenAI Security Project)
- [OWASP Top 10 for Agentic Applications 2026: Key Takeaways](https://goteleport.com/blog/owasp-top-10-agentic-applications/)

**Interoperability**
- [A Survey of Agent Interoperability Protocols: MCP, ACP, A2A, ANP](https://arxiv.org/html/2505.02279v1) — primary (preprint)
- [Governance Gaps in Agent Interoperability Protocols](https://arxiv.org/pdf/2606.31498) — primary (preprint)
- [Agent Interoperability Protocols 2026: MCP, A2A, ACP and the Path to Convergence](https://zylos.ai/research/2026-03-26-agent-interoperability-protocols-mcp-a2a-acp-convergence/)

**Observability**
- [Inside the LLM Call: GenAI Observability with OpenTelemetry](https://opentelemetry.io/blog/2026/genai-observability/) — primary (OpenTelemetry)
- [Datadog Agent Observability natively supports OpenTelemetry GenAI Semantic Conventions](https://www.datadoghq.com/blog/llm-otel-semantic-convention/)
- [How OpenTelemetry Traces LLM Calls, Agent Reasoning, and MCP Tools](https://greptime.com/blogs/2026-05-09-opentelemetry-genai-semantic-conventions)

**Evaluation and adoption**
- [The 2025 AI Agent Report: Why AI Pilots Fail in Production](https://composio.dev/content/why-ai-agent-pilots-fail-2026-integration-roadmap)
- [Building an AI Agent Evaluation Pipeline: 2026 Methodology](https://www.digitalapplied.com/blog/ai-agent-evaluation-pipeline-2026-testing-methodology)
- [LLM Agent Evaluation Metrics in 2026: Tool Calling, Task Completion, Trace-Based Evals](https://www.confident-ai.com/blog/llm-agent-evaluation-complete-guide)
- [An Empirical Study of Automating Agent Evaluation](https://arxiv.org/pdf/2605.11378) — primary (preprint)
- [Why 88% of AI Agents Fail Production](https://www.digitalapplied.com/blog/88-percent-ai-agents-never-reach-production-failure-framework)

**Regulation**
- [What the EU AI Act requires for AI agent logging](https://www.helpnetsecurity.com/2026/04/16/eu-ai-act-logging-requirements/)
- [EU AI Act Enforcement August 2026 Guide](https://trussed.ai/resources/eu-ai-act-enforcement-august-2026-guide)
- [EU AI Act Compliance for AI Agents: Technical Checklist for 2026](https://thebrightbyte.com/playbook/expertise/eu-ai-act-compliance-ai-agents)

**SRE and incident response**
- [Agentic SRE: How Self-Healing Infrastructure Is Redefining Enterprise AIOps in 2026](https://www.unite.ai/agentic-sre-how-self-healing-infrastructure-is-redefining-enterprise-aiops-in-2026/)
- [AI in Incident Response 2026: The Data](https://www.traversal.com/blog/ai-in-incident-response-state-of-the-field-2026-sre)

**Identity**
- [AI Identity: Standards, Gaps, and Directions](https://arxiv.org/pdf/2604.23280) — primary (preprint)
- [Agent Authentication & Delegated Access: OAuth Flows, Scoped Tokens, Identity Patterns](https://zylos.ai/research/2026-04-11-agent-authentication-delegated-access-oauth-scoped-tokens)
- [Giving Agents Their Own Identity, Not Your Credentials](https://www.digitalapplied.com/blog/agent-identity-credentials-non-human-access-2026-playbook)

**Memory and context**
- [AMA-Bench: Evaluating Long-Horizon Memory for Agentic Applications](https://arxiv.org/pdf/2602.22769) — primary (preprint)
- [Agent Context Compaction for Long-Running Sessions](https://zylos.ai/research/2026-04-21-agent-context-compaction-long-running-sessions/)
- [Self-GC: Self-Governing Context for Long-Horizon LLM Agents](https://arxiv.org/pdf/2607.00692) — primary (preprint)
