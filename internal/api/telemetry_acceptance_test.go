package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/telemetry"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
	featurememory "github.com/bsenel/karakuri/internal/feature/memory"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/llm"
	"github.com/bsenel/karakuri/internal/platform/observability"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

const (
	accModel     = "fake-model-1"
	accInTokens  = 123
	accOutTokens = 45
	accPlan      = `{"actions":[{"capability":"acc.act","env_id":"acc.env","reason":"r"}],"confidence":0.95,"reasoning":"r"}`
)

// accProvider always plans the same single action and reports a fixed model
// and token usage. It embeds the interface for AsLLM, which the agent runtime
// never calls on this path.
type accProvider struct{ llm.ProviderAdapter }

func (accProvider) Name() string                   { return "acc" }
func (accProvider) Model() string                  { return accModel }
func (accProvider) Available(context.Context) bool { return true }

func (accProvider) Complete(context.Context, llm.CompletionRequest) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{
		Content:      accPlan,
		TokensUsed:   accInTokens + accOutTokens,
		InputTokens:  accInTokens,
		OutputTokens: accOutTokens,
	}, nil
}

func (accProvider) Stream(context.Context, llm.CompletionRequest) (<-chan llm.CompletionChunk, error) {
	ch := make(chan llm.CompletionChunk, 1)
	ch <- llm.CompletionChunk{Content: accPlan, Done: true}
	close(ch)
	return ch, nil
}

// accEnv succeeds at everything.
type accEnv struct{}

func (accEnv) ID() environment.EnvironmentID { return "acc.env" }
func (accEnv) Domain() string                { return "test" }

func (accEnv) Observe(context.Context, environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{EnvID: "acc.env", Version: "v1", Timestamp: time.Now().UTC()}, nil
}

func (accEnv) Act(context.Context, environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{Success: true}, nil
}

func (accEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return make(chan environment.EnvironmentEvent), nil
}

func (accEnv) Snapshot(context.Context) (environment.EnvironmentSnapshot, error) {
	return environment.EnvironmentSnapshot{EnvID: "acc.env"}, nil
}

// otlpSpan is the part of an OTLP/JSON span these assertions read.
type otlpSpan struct {
	TraceID      string `json:"traceId"`
	SpanID       string `json:"spanId"`
	ParentSpanID string `json:"parentSpanId"`
	Attributes   []struct {
		Key   string `json:"key"`
		Value struct {
			StringValue string `json:"stringValue"`
		} `json:"value"`
	} `json:"attributes"`
}

func (s otlpSpan) attr(key string) (string, bool) {
	for _, a := range s.Attributes {
		if a.Key == key {
			return a.Value.StringValue, true
		}
	}
	return "", false
}

type otlpTraces struct {
	ResourceSpans []struct {
		ScopeSpans []struct {
			Spans []otlpSpan `json:"spans"`
		} `json:"scopeSpans"`
	} `json:"resourceSpans"`
}

// One loop iteration, driven through the real loop, agent factory and OTel,
// reaches an OTLP collector as a single trace: the iteration's invoke_agent
// span is the parent of every model call and tool call inside it.
func TestOneIterationExportsOneTraceOverOTLP(t *testing.T) {
	var (
		mu     sync.Mutex
		bodies [][]byte
	)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/v1/traces" {
			mu.Lock()
			bodies = append(bodies, b)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()

	// The same construction bootstrap uses for an enabled otlp exporter.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "")
	exporters := observability.NewExporterRegistry()
	ot := observability.NewOTLPExporter()
	if !ot.Active() {
		t.Fatal("otlp exporter inactive with an endpoint set")
	}
	exporters.Register(observability.NewRetryExporter(ot, observability.RetryConfig{}))
	otel := observability.NewOTel(exporters)

	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "acc.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := storage.NewGORMStorage(db)
	hub := event.NewHub()

	providers := llm.NewRegistry(nil)
	providers.Register(accProvider{})

	envReg := environment.NewRegistry()
	if err := envReg.Register(environment.Factory{
		EnvID:  "acc.env",
		Domain: "test",
		Serves: []capability.CapabilityID{"acc.act"},
		Build: func(environment.BuildContext) (environment.Environment, error) {
			return accEnv{}, nil
		},
	}); err != nil {
		t.Fatalf("register env: %v", err)
	}

	factory := platformagent.NewFactory(providers, hub, otel, otel)
	svc := featureloop.NewService(store, factory, nil, envReg,
		featurememory.NewService(store, 5), featurecp.NewService(store, hub),
		nil, nil, hub, otel, nil, karakuriquota.Deps{}, otel)

	ctx := context.Background()
	obj := objective.Objective{ID: "obj-acc", Title: "trace one iteration", Domain: "test"}
	if err := store.SaveObjective(ctx, obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}
	res, err := svc.Run(ctx, coreloop.Request{
		Objective: obj,
		MaxIter:   1,
		Agent: coreagent.Definition{
			ID:                "acc-agent",
			Name:              "Acc Agent",
			Domain:            "test",
			ReasoningStrategy: coreagent.ReasoningChainOfThought,
			Authority:         coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions},
			LLMHints:          capability.LLMHints{PreferredProvider: "acc"},
		},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// The terminal state is persisted after the iteration's span has ended.
	deadline := time.Now().Add(10 * time.Second)
	for {
		st, err := store.GetLoopState(ctx, res.LoopID)
		if err == nil && st.Completed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("loop did not complete within 10s")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := otel.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	mu.Lock()
	got := append([][]byte(nil), bodies...)
	mu.Unlock()
	if len(got) == 0 {
		t.Fatal("collector received no /v1/traces request")
	}
	byOp := map[string][]otlpSpan{}
	for _, b := range got {
		var tr otlpTraces
		if err := json.Unmarshal(b, &tr); err != nil {
			t.Fatalf("decode OTLP body: %v", err)
		}
		for _, rs := range tr.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				for _, s := range ss.Spans {
					if op, ok := s.attr(telemetry.GenAIOperationName); ok {
						byOp[op] = append(byOp[op], s)
					}
				}
			}
		}
	}

	agents := byOp[telemetry.OpInvokeAgent]
	if len(agents) != 1 {
		t.Fatalf("invoke_agent spans = %d, want 1", len(agents))
	}
	root := agents[0]

	for _, op := range []string{telemetry.OpChat, telemetry.OpExecuteTool} {
		spans := byOp[op]
		if len(spans) == 0 {
			t.Errorf("no %s span exported", op)
		}
		for _, s := range spans {
			if s.TraceID != root.TraceID {
				t.Errorf("%s span traceId = %q, want invoke_agent's %q", op, s.TraceID, root.TraceID)
			}
			if s.ParentSpanID != root.SpanID {
				t.Errorf("%s span parentSpanId = %q, want invoke_agent's spanId %q", op, s.ParentSpanID, root.SpanID)
			}
		}
	}

	for _, s := range byOp[telemetry.OpChat] {
		for key, want := range map[string]string{
			telemetry.GenAIRequestModel:      accModel,
			telemetry.GenAIUsageInputTokens:  strconv.Itoa(accInTokens),
			telemetry.GenAIUsageOutputTokens: strconv.Itoa(accOutTokens),
		} {
			if v, ok := s.attr(key); !ok || v != want {
				t.Errorf("chat span %s = %q (present %v), want %q", key, v, ok, want)
			}
		}
	}
}
