package telemetry

import "context"

// AttrKarakuriEnvironmentID names the environment an execute_tool span ran
// against. The GenAI conventions have no key for it, so it lives under the
// deployment's own namespace rather than borrowing one that means something
// else.
const AttrKarakuriEnvironmentID = "karakuri.environment.id"

// Attribute is one key/value on a span. Keys are the constants above and in
// genai.go, so every emitter spells them the same way.
type Attribute struct {
	Key   string
	Value any
}

// Tracer is the port through which the loop opens spans without importing a
// vendor SDK. It is implemented in internal/platform; nil at a call site means
// NoopTracer.
type Tracer interface {
	// Start opens a span as a child of whatever span ctx carries, and returns
	// a ctx carrying the new one so work beneath it nests.
	Start(ctx context.Context, name string, attrs ...Attribute) (context.Context, Span)
}

// Span is one open unit of traced work. End must be called exactly once.
type Span interface {
	SetAttributes(attrs ...Attribute)
	// SetError marks the span failed. A failed action is not a Go error, so
	// this takes the message rather than an error value.
	SetError(msg string)
	End()
}

// NoopTracer returns a Tracer that records nothing and hands ctx back
// unchanged, for a deployment that has not wired tracing.
func NoopTracer() Tracer { return noopTracer{} }

type noopTracer struct{}

func (noopTracer) Start(ctx context.Context, _ string, _ ...Attribute) (context.Context, Span) {
	return ctx, noopSpan{}
}

type noopSpan struct{}

func (noopSpan) SetAttributes(...Attribute) {}
func (noopSpan) SetError(string)            {}
func (noopSpan) End()                       {}
