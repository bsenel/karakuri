package observability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/telemetry"
)

type otlpAttr struct {
	Key   string         `json:"key"`
	Value map[string]any `json:"value"`
}

type otlpSpan struct {
	TraceID           string     `json:"traceId"`
	SpanID            string     `json:"spanId"`
	ParentSpanID      string     `json:"parentSpanId"`
	Name              string     `json:"name"`
	Kind              any        `json:"kind"`
	StartTimeUnixNano any        `json:"startTimeUnixNano"`
	EndTimeUnixNano   any        `json:"endTimeUnixNano"`
	Attributes        []otlpAttr `json:"attributes"`
	Status            struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
	} `json:"status"`
}

type otlpTraces struct {
	ResourceSpans []struct {
		Resource struct {
			Attributes []otlpAttr `json:"attributes"`
		} `json:"resource"`
		ScopeSpans []struct {
			Spans []otlpSpan `json:"spans"`
		} `json:"scopeSpans"`
	} `json:"resourceSpans"`
}

func hasOTLPStringAttr(attrs []otlpAttr, key, value string) bool {
	for _, a := range attrs {
		if a.Key == key && a.Value["stringValue"] == value {
			return true
		}
	}
	return false
}

func TestOTLPExportSpans(t *testing.T) {
	var (
		method, path string
		body         []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	e := &OTLPExporter{endpoint: srv.URL, service: "karakuri", client: srv.Client()}

	start := time.Unix(1_700_000_000, 123_456_789).UTC()
	parent := SpanRecord{
		TraceID: "0af7651916cd43dd8448eb211c80319c",
		SpanID:  "b7ad6b7169203331",
		Name:    telemetry.OpInvokeAgent,
		Kind:    SpanKindInternal,
		Start:   start,
		End:     start.Add(3 * time.Second),
		Status:  SpanStatus{Code: StatusOK},
	}
	child := SpanRecord{
		TraceID:      parent.TraceID,
		SpanID:       "00f067aa0ba902b7",
		ParentSpanID: parent.SpanID,
		Name:         telemetry.OpChat,
		Kind:         SpanKindClient,
		Start:        start.Add(time.Second),
		End:          start.Add(2 * time.Second),
		Attributes:   []telemetry.Attribute{{Key: telemetry.GenAIRequestModel, Value: "claude-sonnet-5"}},
		Status:       SpanStatus{Code: StatusError, Description: "rate limited"},
	}
	if err := e.ExportSpans(context.Background(), []SpanRecord{parent, child}); err != nil {
		t.Fatalf("ExportSpans: %v", err)
	}

	if method != http.MethodPost || path != "/v1/traces" {
		t.Fatalf("want POST /v1/traces, got %q %q", method, path)
	}
	var p otlpTraces
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("decode: %v\n%s", err, body)
	}
	if len(p.ResourceSpans) == 0 || len(p.ResourceSpans[0].ScopeSpans) == 0 {
		t.Fatalf("missing resourceSpans/scopeSpans: %s", body)
	}
	if !hasOTLPStringAttr(p.ResourceSpans[0].Resource.Attributes, "service.name", "karakuri") {
		t.Errorf("resource attributes lack service.name=karakuri: %+v", p.ResourceSpans[0].Resource.Attributes)
	}
	spans := p.ResourceSpans[0].ScopeSpans[0].Spans
	if len(spans) != 2 {
		t.Fatalf("want 2 spans, got %d: %s", len(spans), body)
	}
	byName := map[string]otlpSpan{}
	for _, s := range spans {
		byName[s.Name] = s
	}

	for _, want := range []SpanRecord{parent, child} {
		got, ok := byName[want.Name]
		if !ok {
			t.Errorf("no span named %q in %s", want.Name, body)
			continue
		}
		if got.TraceID != want.TraceID || got.SpanID != want.SpanID || got.ParentSpanID != want.ParentSpanID {
			t.Errorf("%s ids: want %s/%s/%q, got %s/%s/%q", want.Name,
				want.TraceID, want.SpanID, want.ParentSpanID, got.TraceID, got.SpanID, got.ParentSpanID)
		}
		if got.Kind != float64(want.Kind) {
			t.Errorf("%s kind: want integer %d, got %#v", want.Name, want.Kind, got.Kind)
		}
		if s := fmt.Sprintf("%d", want.Start.UnixNano()); got.StartTimeUnixNano != s {
			t.Errorf("%s startTimeUnixNano: want string %q, got %#v", want.Name, s, got.StartTimeUnixNano)
		}
		if s := fmt.Sprintf("%d", want.End.UnixNano()); got.EndTimeUnixNano != s {
			t.Errorf("%s endTimeUnixNano: want string %q, got %#v", want.Name, s, got.EndTimeUnixNano)
		}
		if got.Status.Code != float64(want.Status.Code) || got.Status.Message != want.Status.Description {
			t.Errorf("%s status: want {code:%d message:%q}, got %+v", want.Name, want.Status.Code, want.Status.Description, got.Status)
		}
	}
	if !hasOTLPStringAttr(byName[telemetry.OpChat].Attributes, telemetry.GenAIRequestModel, "claude-sonnet-5") {
		t.Errorf("chat span lacks %s attribute: %+v", telemetry.GenAIRequestModel, byName[telemetry.OpChat].Attributes)
	}
}

func TestOTLPExportSpansStatusErrors(t *testing.T) {
	cases := []struct {
		status    int
		permanent bool
	}{
		{http.StatusBadRequest, true},
		{http.StatusServiceUnavailable, false},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()
			e := &OTLPExporter{endpoint: srv.URL, service: "karakuri", client: srv.Client()}

			err := e.ExportSpans(context.Background(), []SpanRecord{{
				TraceID: "0af7651916cd43dd8448eb211c80319c",
				SpanID:  "b7ad6b7169203331",
				Name:    telemetry.OpChat,
				Kind:    SpanKindClient,
				Start:   time.Now(),
				End:     time.Now(),
			}})
			if err == nil {
				t.Fatalf("want an error for HTTP %d, got nil", tc.status)
			}
			if got := errors.Is(err, ErrPermanent); got != tc.permanent {
				t.Errorf("errors.Is(err, ErrPermanent): want %v, got %v (%v)", tc.permanent, got, err)
			}
		})
	}
}
