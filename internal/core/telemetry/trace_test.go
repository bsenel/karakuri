package telemetry

import (
	"context"
	"testing"
)

type ctxKey struct{}

// A deployment without tracing wired must be indistinguishable from one that
// never asked: the ctx it gets back is the ctx it passed in, values and all.
func TestNoopTracerReturnsTheSameContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKey{}, "kept")

	got, span := NoopTracer().Start(ctx, "invoke_agent test",
		Attribute{Key: GenAIOperationName, Value: OpInvokeAgent})

	if got != ctx {
		t.Error("NoopTracer.Start returned a different context")
	}
	if span == nil {
		t.Fatal("NoopTracer.Start returned a nil span; callers would have to nil-check every End")
	}
}

func TestNoopSpanMethodsDoNotPanic(t *testing.T) {
	_, span := NoopTracer().Start(context.Background(), "execute_tool test")

	span.SetAttributes(Attribute{Key: GenAIToolName, Value: "test.act.run"})
	span.SetAttributes()
	span.SetError("boom")
	span.End()
}
