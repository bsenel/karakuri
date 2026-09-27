package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/tmc/langchaingo/llms"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/telemetry"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
)

// recordedSpan is what recordingTracer kept of one span. parent is the id of
// the span the ctx passed to Start carried, and zero for a root.
type recordedSpan struct {
	id       int
	name     string
	attrs    map[string]any
	parent   int
	ended    bool
	errored  bool
	errorMsg string
}

// spanKey is private to the fake, so the only way a span becomes a child is by
// being started from a ctx this tracer returned.
type spanKey struct{}

// recordingTracer is a telemetry.Tracer that remembers every span.
type recordingTracer struct {
	mu    sync.Mutex
	spans []*recordedSpan
}

func (t *recordingTracer) Start(ctx context.Context, name string, attrs ...telemetry.Attribute) (context.Context, telemetry.Span) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, _ := ctx.Value(spanKey{}).(int)
	s := &recordedSpan{id: len(t.spans) + 1, name: name, attrs: map[string]any{}, parent: parent}
	for _, a := range attrs {
		s.attrs[a.Key] = a.Value
	}
	t.spans = append(t.spans, s)
	return context.WithValue(ctx, spanKey{}, s.id), &recordingSpan{t: t, s: s}
}

// chatSpans copies the recorded chat spans, so assertions never read a span
// the agent might still be writing.
func (t *recordingTracer) chatSpans() []recordedSpan {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []recordedSpan
	for _, s := range t.spans {
		if s.name != telemetry.OpChat && !strings.HasPrefix(s.name, telemetry.OpChat+" ") {
			continue
		}
		c := *s
		c.attrs = make(map[string]any, len(s.attrs))
		for k, v := range s.attrs {
			c.attrs[k] = v
		}
		out = append(out, c)
	}
	return out
}

type recordingSpan struct {
	t *recordingTracer
	s *recordedSpan
}

func (r *recordingSpan) SetAttributes(attrs ...telemetry.Attribute) {
	r.t.mu.Lock()
	defer r.t.mu.Unlock()
	for _, a := range attrs {
		r.s.attrs[a.Key] = a.Value
	}
}

func (r *recordingSpan) SetError(msg string) {
	r.t.mu.Lock()
	defer r.t.mu.Unlock()
	r.s.errored = true
	r.s.errorMsg = msg
}

func (r *recordingSpan) End() {
	r.t.mu.Lock()
	defer r.t.mu.Unlock()
	r.s.ended = true
}

// fakeProvider answers every Complete with resp or err, and keeps the ctx it
// was called with so a test can see which span the call ran under.
type fakeProvider struct {
	name string
	resp llm.CompletionResponse
	err  error

	mu   sync.Mutex
	ctxs []context.Context
}

func (p *fakeProvider) Name() string { return p.name }

func (p *fakeProvider) Complete(ctx context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
	p.mu.Lock()
	p.ctxs = append(p.ctxs, ctx)
	p.mu.Unlock()
	return p.resp, p.err
}

func (p *fakeProvider) Stream(context.Context, llm.CompletionRequest) (<-chan llm.CompletionChunk, error) {
	ch := make(chan llm.CompletionChunk, 1)
	ch <- llm.CompletionChunk{Content: p.resp.Content, Done: true}
	close(ch)
	return ch, nil
}

func (p *fakeProvider) Available(context.Context) bool { return true }
func (p *fakeProvider) AsLLM() llms.Model              { return nil }

func (p *fakeProvider) lastCtx(t *testing.T) context.Context {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.ctxs) == 0 {
		t.Fatal("provider was never called")
	}
	return p.ctxs[len(p.ctxs)-1]
}

// modeledProvider is a fakeProvider that can say which model it served, the
// way modelOf asks.
type modeledProvider struct {
	*fakeProvider
	model string
}

func (p *modeledProvider) Model() string { return p.model }

const testModel = "fake-model-1"

func newModeled(resp llm.CompletionResponse, err error) *modeledProvider {
	return &modeledProvider{fakeProvider: &fakeProvider{name: "fakeprov", resp: resp, err: err}, model: testModel}
}

// runOnce builds an agent over p through a real Registry and Hub, and runs it
// once with ctx.
func runOnce(t *testing.T, ctx context.Context, p llm.ProviderAdapter, tracer telemetry.Tracer) (coreagent.Output, error) {
	t.Helper()
	a := newAgent(t, p, tracer)
	return a.Run(ctx, coreagent.Input{Task: "t"})
}

func newAgent(t *testing.T, p llm.ProviderAdapter, tracer telemetry.Tracer) coreagent.Agent {
	t.Helper()
	providers := llm.NewRegistry(nil)
	providers.Register(p)
	f := NewFactory(providers, event.NewHub(), observability.NewOTel(nil), tracer)
	a, err := f.New(context.Background(), coreagent.Definition{
		ID:       "a1",
		Name:     "tester",
		LLMHints: capability.LLMHints{PreferredProvider: p.Name()},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

func onlyChatSpan(t *testing.T, tr *recordingTracer) recordedSpan {
	t.Helper()
	spans := tr.chatSpans()
	if len(spans) != 1 {
		t.Fatalf("want exactly one chat span, got %d: %+v", len(spans), spans)
	}
	return spans[0]
}

func TestRun_OpensOneChatSpanPerModelCall(t *testing.T) {
	tr := &recordingTracer{}
	p := newModeled(llm.CompletionResponse{Content: "ok", TokensUsed: 19, InputTokens: 12, OutputTokens: 7}, nil)

	if _, err := runOnce(t, context.Background(), p, tr); err != nil {
		t.Fatalf("Run: %v", err)
	}

	s := onlyChatSpan(t, tr)
	if want := telemetry.OpChat + " " + testModel; s.name != want {
		t.Errorf("span name = %q, want %q", s.name, want)
	}
	want := map[string]any{
		telemetry.GenAIOperationName:     telemetry.OpChat,
		telemetry.GenAIProviderName:      p.Name(),
		telemetry.GenAIRequestModel:      testModel,
		telemetry.GenAIUsageInputTokens:  12,
		telemetry.GenAIUsageOutputTokens: 7,
	}
	for k, v := range want {
		if got, ok := s.attrs[k]; !ok || got != v {
			t.Errorf("attr %s = %v (present %v), want %v", k, got, ok, v)
		}
	}
	if !s.ended {
		t.Error("chat span was not ended")
	}
	if s.errored {
		t.Errorf("chat span marked failed on success: %q", s.errorMsg)
	}
}

func TestRun_ChatSpanNestsUnderCallerSpan(t *testing.T) {
	tr := &recordingTracer{}
	p := newModeled(llm.CompletionResponse{Content: "ok"}, nil)

	parentCtx, parent := tr.Start(context.Background(), telemetry.OpInvokeAgent+" tester")
	if _, err := runOnce(t, parentCtx, p, tr); err != nil {
		t.Fatalf("Run: %v", err)
	}
	parent.End()

	s := onlyChatSpan(t, tr)
	parentID, _ := parentCtx.Value(spanKey{}).(int)
	if s.parent != parentID {
		t.Errorf("chat span parent = %d, want caller span %d", s.parent, parentID)
	}
	if got, _ := p.lastCtx(t).Value(spanKey{}).(int); got != s.id {
		t.Errorf("provider ctx carries span %d, want chat span %d", got, s.id)
	}
}

func TestRun_TwoRunsGiveTwoChatSpans(t *testing.T) {
	tr := &recordingTracer{}
	p := newModeled(llm.CompletionResponse{Content: "ok"}, nil)
	a := newAgent(t, p, tr)

	for i := 0; i < 2; i++ {
		if _, err := a.Run(context.Background(), coreagent.Input{Task: "t"}); err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
	}

	spans := tr.chatSpans()
	if len(spans) != 2 {
		t.Fatalf("want 2 chat spans, got %d", len(spans))
	}
	for i, s := range spans {
		if !s.ended {
			t.Errorf("chat span %d was not ended", i)
		}
	}
}

func TestRun_ChatSpanOmitsUnreportedUsage(t *testing.T) {
	tr := &recordingTracer{}
	// A provider that reports only the total leaves the split at zero.
	p := newModeled(llm.CompletionResponse{Content: "ok", TokensUsed: 30}, nil)

	if _, err := runOnce(t, context.Background(), p, tr); err != nil {
		t.Fatalf("Run: %v", err)
	}

	s := onlyChatSpan(t, tr)
	for _, k := range []string{telemetry.GenAIUsageInputTokens, telemetry.GenAIUsageOutputTokens} {
		if v, ok := s.attrs[k]; ok {
			t.Errorf("unreported usage attr %s = %v, want absent", k, v)
		}
	}
}

func TestRun_ChatSpanWithoutModel(t *testing.T) {
	tr := &recordingTracer{}
	p := &fakeProvider{name: "fakeprov", resp: llm.CompletionResponse{Content: "ok"}}

	if _, err := runOnce(t, context.Background(), p, tr); err != nil {
		t.Fatalf("Run: %v", err)
	}

	s := onlyChatSpan(t, tr)
	if s.name != telemetry.OpChat {
		t.Errorf("span name = %q, want %q", s.name, telemetry.OpChat)
	}
	if v, ok := s.attrs[telemetry.GenAIRequestModel]; ok {
		t.Errorf("request model attr = %v, want absent when the provider cannot say", v)
	}
	if got := s.attrs[telemetry.GenAIProviderName]; got != p.Name() {
		t.Errorf("provider attr = %v, want %q", got, p.Name())
	}
}

func TestRun_ChatSpanRecordsProviderError(t *testing.T) {
	tr := &recordingTracer{}
	boom := errors.New("provider exploded")
	p := newModeled(llm.CompletionResponse{}, boom)

	_, err := runOnce(t, context.Background(), p, tr)
	if !errors.Is(err, boom) {
		t.Fatalf("Run err = %v, want %v", err, boom)
	}

	s := onlyChatSpan(t, tr)
	if !s.errored {
		t.Error("chat span not marked failed")
	}
	if !s.ended {
		t.Error("chat span was not ended on error")
	}
}

func TestNewFactory_NilTracer(t *testing.T) {
	p := newModeled(llm.CompletionResponse{Content: "ok"}, nil)

	out, err := runOnce(t, context.Background(), p, nil)
	if err != nil {
		t.Fatalf("Run with nil tracer: %v", err)
	}
	if out.Content != "ok" {
		t.Errorf("content = %q, want %q", out.Content, "ok")
	}
}

// capturingExporter keeps every metric record a Flush hands it.
type capturingExporter struct{ metrics []observability.MetricRecord }

func (c *capturingExporter) Name() string { return "capture" }
func (c *capturingExporter) ExportMetrics(_ context.Context, r []observability.MetricRecord) error {
	c.metrics = append(c.metrics, r...)
	return nil
}
func (c *capturingExporter) ExportLogs(context.Context, []observability.LogRecord) error { return nil }
func (c *capturingExporter) Flush(context.Context) error                                 { return nil }
func (c *capturingExporter) Shutdown(context.Context) error                              { return nil }

// runRecordingMetrics runs one call through a Factory whose OTel flushes into a
// capturingExporter, and returns what it captured by metric name.
func runRecordingMetrics(t *testing.T, resp llm.CompletionResponse) map[string][]observability.MetricRecord {
	t.Helper()
	exp := &capturingExporter{}
	reg := observability.NewExporterRegistry()
	reg.Register(exp)
	otel := observability.NewOTel(reg)

	providers := llm.NewRegistry(nil)
	p := newModeled(resp, nil)
	providers.Register(p)
	a, err := NewFactory(providers, event.NewHub(), otel, nil).New(context.Background(), coreagent.Definition{
		ID: "a1", Name: "tester", LLMHints: capability.LLMHints{PreferredProvider: p.Name()},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.Run(context.Background(), coreagent.Input{Task: "t"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := otel.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	byName := map[string][]observability.MetricRecord{}
	for _, m := range exp.metrics {
		byName[m.Name] = append(byName[m.Name], m)
	}
	return byName
}

// Phase 29 Step 1 added the GenAI client metrics beside the legacy names, but
// nothing called them: every model call now records both.
func TestRun_RecordsLegacyAndGenAIMetrics(t *testing.T) {
	got := runRecordingMetrics(t, llm.CompletionResponse{Content: "ok", TokensUsed: 19, InputTokens: 12, OutputTokens: 7})

	for _, name := range []string{"agent_invocation", "agent_latency_ms", "tokens_used", "gen_ai.client.operation.duration"} {
		if len(got[name]) != 1 {
			t.Errorf("%s recorded %d times, want 1", name, len(got[name]))
		}
	}
	if m := got["tokens_used"]; len(m) == 1 && (m[0].Value != 19 || m[0].Labels["role"] != "tester") {
		t.Errorf("tokens_used = %v %v, want 19 with role=tester", m[0].Value, m[0].Labels)
	}
	usage := map[string]float64{}
	for _, m := range got["gen_ai.client.token.usage"] {
		usage[m.Labels[telemetry.GenAITokenType]] = m.Value
		if m.Labels[telemetry.GenAIRequestModel] != testModel || m.Labels[telemetry.GenAIProviderName] != "fakeprov" {
			t.Errorf("token usage labels = %v, want model %q and provider fakeprov", m.Labels, testModel)
		}
	}
	if usage["input"] != 12 || usage["output"] != 7 || len(usage) != 2 {
		t.Errorf("gen_ai.client.token.usage by type = %v, want input 12 and output 7", usage)
	}
}

// A provider that reports only a total keeps it under tokens_used and claims no
// input/output split it does not have.
func TestRun_RecordsTotalOnlyWhenThereIsNoSplit(t *testing.T) {
	got := runRecordingMetrics(t, llm.CompletionResponse{Content: "ok", TokensUsed: 30})

	if m := got["tokens_used"]; len(m) != 1 || m[0].Value != 30 {
		t.Errorf("tokens_used = %+v, want one record of 30", m)
	}
	if n := len(got["gen_ai.client.token.usage"]); n != 0 {
		t.Errorf("gen_ai.client.token.usage recorded %d times without a split, want 0", n)
	}
}
