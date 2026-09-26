package observability

import (
	"context"
	"sync"
	"time"

	"github.com/bsenel/karakuri/internal/core/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type OTel struct {
	meter     metric.Meter
	registry  *ExporterRegistry
	mu        sync.Mutex
	buffer    []MetricRecord
	logBuffer []LogRecord
}

func NewOTel(registry *ExporterRegistry) *OTel {
	meter := otel.Meter("karakuri")
	return &OTel{meter: meter, registry: registry}
}

func (o *OTel) RecordMetric(name string, value float64, labels map[string]string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.buffer = append(o.buffer, MetricRecord{Name: name, Value: value, Labels: labels, Timestamp: time.Now().UTC()})
}

func (o *OTel) RecordLog(level, message string, labels map[string]string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.logBuffer = append(o.logBuffer, LogRecord{Level: level, Message: message, Labels: labels, Timestamp: time.Now().UTC()})
}

func (o *OTel) Flush(ctx context.Context) error {
	o.mu.Lock()
	metrics := o.buffer
	logs := o.logBuffer
	o.buffer = nil
	o.logBuffer = nil
	o.mu.Unlock()
	for _, e := range o.registry.Active() {
		_ = e.ExportMetrics(ctx, metrics)
		_ = e.ExportLogs(ctx, logs)
		_ = e.Flush(ctx)
	}
	return nil
}

func (o *OTel) IncWorktreeCreated() { o.RecordMetric("worktree_created", 1, nil) }
func (o *OTel) IncWorktreeRemoved() { o.RecordMetric("worktree_removed", 1, nil) }
func (o *OTel) IncAgentInvocation(role string) {
	o.RecordMetric("agent_invocation", 1, agentLabels(role))
}

func (o *OTel) ObserveAgentLatency(role string, d time.Duration) {
	o.RecordMetric("agent_latency_ms", float64(d.Milliseconds()), agentLabels(role))
}

func (o *OTel) RecordTokens(role string, n int) {
	o.RecordMetric("tokens_used", float64(n), agentLabels(role))
}

// RecordChat records one LLM chat call under both the legacy tokens_used
// metric and the OpenTelemetry GenAI client metrics.
func (o *OTel) RecordChat(provider, model, role string, inTok, outTok int, d time.Duration) {
	o.RecordTokens(role, inTok+outTok)
	o.RecordMetric("gen_ai.client.token.usage", float64(inTok), chatLabels(provider, model, role, "input"))
	o.RecordMetric("gen_ai.client.token.usage", float64(outTok), chatLabels(provider, model, role, "output"))
	o.RecordMetric("gen_ai.client.operation.duration", d.Seconds(), chatLabels(provider, model, role, ""))
}

// agentLabels keeps the legacy role label and adds its GenAI equivalent.
func agentLabels(role string) map[string]string {
	return map[string]string{"role": role, telemetry.GenAIAgentName: role}
}

// chatLabels builds GenAI chat attributes; tokenType is omitted when empty.
func chatLabels(provider, model, role, tokenType string) map[string]string {
	m := map[string]string{
		telemetry.GenAIProviderName:  provider,
		telemetry.GenAIRequestModel:  model,
		telemetry.GenAIOperationName: telemetry.OpChat,
		telemetry.GenAIAgentName:     role,
	}
	if tokenType != "" {
		m[telemetry.GenAITokenType] = tokenType
	}
	return m
}

func (o *OTel) RecordMemoryRecall(tier string, count int, latencyMS int64) {
	o.RecordMetric("memory_recall_count", float64(count), map[string]string{"tier": tier})
	o.RecordMetric("memory_recall_latency_ms", float64(latencyMS), map[string]string{"tier": tier})
}

func (o *OTel) RecordMemoryConsolidation(promoted int) {
	o.RecordMetric("memory_consolidation_promoted", float64(promoted), nil)
}

func (o *OTel) RecordLoopIteration(domain string, step string, durationMS int64) {
	o.RecordMetric("loop_iteration_duration_ms", float64(durationMS), map[string]string{"domain": domain, "step": step})
}

func Attr(k, v string) attribute.KeyValue { return attribute.String(k, v) }
