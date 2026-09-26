package observability

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/telemetry"
)

type capturingExporter struct {
	mu      sync.Mutex
	metrics []MetricRecord
}

func (c *capturingExporter) Name() string { return "capture" }
func (c *capturingExporter) ExportMetrics(_ context.Context, rs []MetricRecord) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics = append(c.metrics, rs...)
	return nil
}
func (c *capturingExporter) ExportLogs(context.Context, []LogRecord) error { return nil }
func (c *capturingExporter) Flush(context.Context) error                   { return nil }
func (c *capturingExporter) Shutdown(context.Context) error                { return nil }

// record runs fn against a fresh OTel, flushes, and returns what was exported.
func record(t *testing.T, fn func(o *OTel)) []MetricRecord {
	t.Helper()
	reg := NewExporterRegistry()
	c := &capturingExporter{}
	reg.Register(c)
	o := NewOTel(reg)
	fn(o)
	if err := o.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}
	return c.metrics
}

// findMetrics returns records named name whose labels include every pair in
// want. Extra labels are allowed: new GenAI attributes ride alongside the old
// ones rather than replacing them.
func findMetrics(rs []MetricRecord, name string, want map[string]string) []MetricRecord {
	var out []MetricRecord
	for _, r := range rs {
		if r.Name != name {
			continue
		}
		ok := true
		for k, v := range want {
			if r.Labels[k] != v {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, r)
		}
	}
	return out
}

func requireOne(t *testing.T, rs []MetricRecord, name string, labels map[string]string, value float64) {
	t.Helper()
	got := findMetrics(rs, name, labels)
	if len(got) != 1 {
		t.Fatalf("want exactly one %s%v, got %d in %+v", name, labels, len(got), rs)
	}
	if got[0].Value != value {
		t.Errorf("%s%v: want value %g, got %g", name, labels, value, got[0].Value)
	}
}

// Pre-existing metric names and labels are a contract with dashboards; adding
// GenAI conventions must not rename or drop them.
func TestOTel_LegacyMetricNamesStillEmitted(t *testing.T) {
	cases := []struct {
		name   string
		call   func(o *OTel)
		metric string
		labels map[string]string
		value  float64
	}{
		{"worktree created", func(o *OTel) { o.IncWorktreeCreated() }, "worktree_created", nil, 1},
		{"worktree removed", func(o *OTel) { o.IncWorktreeRemoved() }, "worktree_removed", nil, 1},
		{"agent invocation", func(o *OTel) { o.IncAgentInvocation("planner") }, "agent_invocation", map[string]string{"role": "planner"}, 1},
		{"agent latency", func(o *OTel) { o.ObserveAgentLatency("planner", 250*time.Millisecond) }, "agent_latency_ms", map[string]string{"role": "planner"}, 250},
		{"tokens", func(o *OTel) { o.RecordTokens("planner", 42) }, "tokens_used", map[string]string{"role": "planner"}, 42},
		{"memory recall count", func(o *OTel) { o.RecordMemoryRecall("episodic", 3, 17) }, "memory_recall_count", map[string]string{"tier": "episodic"}, 3},
		{"memory recall latency", func(o *OTel) { o.RecordMemoryRecall("episodic", 3, 17) }, "memory_recall_latency_ms", map[string]string{"tier": "episodic"}, 17},
		{"memory consolidation", func(o *OTel) { o.RecordMemoryConsolidation(5) }, "memory_consolidation_promoted", nil, 5},
		{"loop iteration", func(o *OTel) { o.RecordLoopIteration("software", "act", 900) }, "loop_iteration_duration_ms", map[string]string{"domain": "software", "step": "act"}, 900},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireOne(t, record(t, tc.call), tc.metric, tc.labels, tc.value)
		})
	}
}

func TestOTel_AgentMetricsCarryGenAIAgentName(t *testing.T) {
	cases := []struct {
		name   string
		call   func(o *OTel)
		metric string
	}{
		{"invocation", func(o *OTel) { o.IncAgentInvocation("reviewer") }, "agent_invocation"},
		{"latency", func(o *OTel) { o.ObserveAgentLatency("reviewer", time.Second) }, "agent_latency_ms"},
		{"tokens", func(o *OTel) { o.RecordTokens("reviewer", 7) }, "tokens_used"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := record(t, tc.call)
			want := map[string]string{"role": "reviewer", telemetry.GenAIAgentName: "reviewer"}
			if got := findMetrics(rs, tc.metric, want); len(got) != 1 {
				t.Errorf("want one %s%v, got %d in %+v", tc.metric, want, len(got), rs)
			}
		})
	}
}

func TestOTel_RecordChatEmitsLegacyAndGenAI(t *testing.T) {
	rs := record(t, func(o *OTel) {
		o.RecordChat("anthropic", "claude-sonnet-5", "implementer", 100, 40, 2*time.Second)
	})

	requireOne(t, rs, "tokens_used", map[string]string{"role": "implementer"}, 140)

	common := map[string]string{
		telemetry.GenAIProviderName:  "anthropic",
		telemetry.GenAIRequestModel:  "claude-sonnet-5",
		telemetry.GenAIOperationName: telemetry.OpChat,
	}
	with := func(k, v string) map[string]string {
		m := map[string]string{k: v}
		for ck, cv := range common {
			m[ck] = cv
		}
		return m
	}
	requireOne(t, rs, "gen_ai.client.token.usage", with(telemetry.GenAITokenType, "input"), 100)
	requireOne(t, rs, "gen_ai.client.token.usage", with(telemetry.GenAITokenType, "output"), 40)
	if got := findMetrics(rs, "gen_ai.client.token.usage", nil); len(got) != 2 {
		t.Errorf("want exactly two gen_ai.client.token.usage records, got %d", len(got))
	}
	requireOne(t, rs, "gen_ai.client.operation.duration", nil, 2)
}
