# ADR 023 — Telemetry speaks the GenAI conventions, through a port core owns

**Status:** Accepted
**Date:** 2026-09-27
**Relates to:** [ADR 015](015-standing-objectives-and-reconciliation.md) (the loop converges once, and the supervisor calls it), [ADR 021](021-observations-carry-provenance.md) (a payload says who wrote it)

## Context

Phase 12 built the exporter chain: local file, AWS, Datadog, New Relic,
Elasticsearch, Loki, OTLP and Prometheus behind one `Exporter` interface, with
per-exporter isolation and a `RetryExporter` that short-circuits on
`ErrPermanent`. It moved metrics and logs, under names this project chose, and
no spans.

The OpenTelemetry GenAI semantic conventions have since settled how an agent
looks in telemetry: an `invoke_agent` span at the top, a `chat` span per model
call, an `execute_tool` span per tool call, and `gen_ai.*` attributes for
provider, model and token usage. The backends Karakuri already exports to build
their agent views from those names. Karakuri populated none of them.

Three constraints shaped the work:

- The loop lives in `internal/feature/loop`. The span tree follows the loop's
  own structure, so that is where spans have to open. `internal/core` has no
  vendor imports, and `internal/feature` depends only on core and platform.
- Eight exporters implement `Exporter`, and only one of them (OTLP) has a
  native span format.
- Operators have dashboards on `tokens_used`, `agent_latency_ms` and the other
  Phase 12 names.

## Decision 1 — a tracer port in core, with a noop default

`internal/core/telemetry/trace.go` defines `Tracer`, `Span` and `Attribute`,
and `NoopTracer()`. `Start(ctx, name, attrs...)` returns a context carrying the
new span, so work beneath it nests without the caller passing parents around.
`SetError` takes a message rather than an error, because a failed action is an
`ActionResult`, not a Go error. The attribute names are constants in
`internal/core/telemetry/genai.go`.

The loop and the agent factory take a `Tracer` by constructor injection. Nil
means the noop, so a test that builds the service as a struct literal traces
nothing instead of panicking. `*observability.OTel` is the only implementation,
and `internal/api/server.go` wires it in.

**Rejected: importing an OpenTelemetry SDK into core or feature.** It breaks
the rule that `internal/core` has no vendor imports, and it makes every loop
test carry an SDK and its global state to prove something unrelated to
tracing. The port is four methods, and it is all the loop needs.

## Decision 2 — spans are an optional capability of an exporter

`SpanExporter` is a separate interface with one method, `ExportSpans`.
`OTel.Flush` checks each active exporter for it with a type assertion. A
wrapper that implements `ExportSpans` for any inner exporter, like
`RetryExporter`, also answers `SupportsSpans`, so the assertion is not fooled
by the wrapper. `OTLPExporter` implements it and posts OTLP/JSON to
`/v1/traces`. No other exporter does.

An exporter that cannot take spans does not receive them. The first time that
happens, `Flush` logs a warning naming the exporter; later flushes drop that
exporter's spans without repeating it. A deployment learns which exporters it
is not tracing to, without a warning every ten seconds. A span export failure is logged and does
not stop the other exporters, the same isolation Phase 12 gave metrics.

**Rejected: widening `Exporter` with `ExportSpans`.** Every existing exporter
would stop compiling, and seven of them would get a stub that returns nil. A
stub that returns nil is a silent drop, and the operator would be told nothing.

## Decision 3 — metric names are aliased, never renamed

The legacy metrics keep their names and labels. The agent metrics add
`gen_ai.agent.name` beside `role`. `OTel.RecordChat` records one model call
under `tokens_used` and under the conventions' `gen_ai.client.token.usage` (one
record per `gen_ai.token.type`) and `gen_ai.client.operation.duration`, in
seconds. `TestOTel_LegacyMetricNamesStillEmitted` pins every name and label
that existed before. The Prometheus exporter maps dotted names into its
charset, so the aliases scrape as `gen_ai_client_token_usage` and
`gen_ai_client_operation_duration`.

**Rejected: renaming to the conventions' names.** It would break every
operator dashboard and alert built on the old names, in a phase whose purpose
is that existing tools work.

## Consequences

- **Telemetry now leaves the process.** Nothing in production ever called
  `OTel.Flush`. Since Phase 12, metrics and logs were buffered, never reached
  an exporter, and the buffer grew without bound. `startTelemetryFlush` in
  `internal/app/bootstrap.go` flushes every 10 seconds and once more on
  shutdown, bounded to 5 seconds on a fresh context. This is a behaviour
  change: a deployment with exporters configured starts sending metrics and
  logs it never sent before. A flush drains the buffers whether or not an
  exporter took them, so a failed export loses one interval instead of
  accumulating.
- **HTTP 400 from an OTLP endpoint is permanent.** This applies to metrics, logs
  and spans. The OTLP/HTTP spec treats a 400 as a rejected payload, and
  retrying it cannot succeed. Before, only 401 and 403 short-circuited, and a
  400 was retried like an outage.
- **Only OTLP carries traces.** The other seven exporters warn once and drop
  spans. A deployment that wants traces in Datadog or New Relic sends them
  through an OpenTelemetry Collector, which those backends already accept.
- **The aliases have no production caller yet.** `RecordChat` is defined and
  tested, and nothing outside its test calls it. The legacy agent metrics were
  in the same state before this phase. Model and token data reaches a backend
  today through the `chat` span's attributes.
- **`gen_ai.provider.name` is the registry name.** It is `claude` or `gemini`,
  not the conventions' well-known `anthropic` or `gcp.gemini`. A query on the
  conventions' provider values will not match yet.
- **No content, no sampling.** Spans carry no prompt or completion text, which
  the conventions make opt-in. Every iteration is traced.

## Alternatives considered

**An SDK tracer behind the port instead of a hand-built one.** This would give
the full OpenTelemetry SDK's batching and propagation for free. It was rejected
for this phase because the exporter chain already batches, isolates and retries.
A second pipeline beside it would have its own flush, and only OTLP would
benefit. The port leaves the swap open.

**Spans around every provider adapter's `Complete`.** This was rejected in
favour of the one call site in the agent factory that every provider passes
through. One span per model call, in one place, cannot be forgotten by the next
provider.

**`execute_tool` spans for refused and unrouted actions.** Rejected: no tool
ran, and a span saying one did would put a false entry in the operator's tool
view. The refusal is still recorded in the action's outcome and the loop's
iteration record.
