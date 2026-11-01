package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// Phase 41 step 4: the gateway carries a verifier's measurements through MCP
// results as structured content, so a number is never scraped out of the text
// (docs/roadmap.md, Phase 41 decision 2, 'Measurements').
//
// The shape is the smallest that carries a Measurement: the result's
// structuredContent holds a "measurements" array, and each entry has a
// "name", a "unit" and a "value" that is a JSON number. Source and trust are
// not the server's to state; this deployment sets them.
//
//	{"content": [...], "structuredContent": {"measurements": [
//	    {"name": "coverage", "value": 0.82, "unit": "ratio"}
//	]}}

const (
	// measureTool answers with measurements in structuredContent, and with a
	// text block that names a different number.
	measureTool = "measure"
	// proseTool answers with a number in its text and no structuredContent.
	proseTool = "measure_in_prose"
)

// measuringServer is httpFake with two more tools. They are not in fakeTools:
// the allowlist names what may be called, and the tests that count what a
// server advertises are not this file's business.
type measuringServer struct{ httpFake }

func (s *measuringServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var req Request
	_ = json.Unmarshal(raw, &req)
	var p CallToolParams
	_ = json.Unmarshal(req.Params, &p)

	var result map[string]any
	switch {
	case req.Method != MethodToolsCall:
	case p.Name == measureTool:
		result = map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "coverage 0.10, duration 99"}},
			"structuredContent": map[string]any{
				"measurements": []any{
					map[string]any{"name": "coverage", "value": 0.82, "unit": "ratio"},
					map[string]any{"name": "duration", "value": 12, "unit": "s"},
				},
			},
		}
	case p.Name == proseTool:
		result = map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "coverage: 0.99 ratio"}},
		}
	}
	if result == nil {
		r.Body = io.NopCloser(bytes.NewReader(raw))
		s.httpFake.ServeHTTP(w, r)
		return
	}
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func measuringEnv(t *testing.T) *Environment {
	t.Helper()
	srv := httptest.NewServer(&measuringServer{})
	t.Cleanup(srv.Close)
	inst := NewInstance(context.Background(), "metrics", Config{
		Transport:    TransportHTTP,
		URL:          srv.URL,
		AllowedTools: []string{"read_file", measureTool, proseTool},
	})
	t.Cleanup(func() { _ = inst.Close() })
	if inst.State() != StateConnected {
		t.Fatalf("instance is %s: %s", inst.State(), inst.Health(false).Error)
	}
	return NewEnvironment(inst)
}

// Each measurement in structuredContent arrives as a typed Measurement with
// its value a float64 from the JSON number, not from the text beside it. A
// number an external MCP server returned is somebody else's figure, so each
// is TrustThirdParty (AGENTS.md rule 9), like the result that carries it.
func TestStructuredContentMeasurementsBecomeTypedThirdPartyMeasurements(t *testing.T) {
	env := measuringEnv(t)

	res, err := env.Act(context.Background(), environment.Action{
		CapabilityID: capability.MCPCapabilityID("metrics", measureTool),
	})
	if err != nil {
		t.Fatalf("Act returned a Go error: %v", err)
	}
	if !res.Success {
		t.Fatalf("the call failed: %s", res.Error)
	}

	want := map[string]struct {
		value float64
		unit  string
	}{
		"coverage": {0.82, "ratio"},
		"duration": {12, "s"},
	}
	if len(res.Measurements) != len(want) {
		t.Fatalf("measurements = %+v, want %d", res.Measurements, len(want))
	}
	for _, m := range res.Measurements {
		w, ok := want[m.Name]
		if !ok {
			t.Errorf("unexpected measurement %+v", m)
			continue
		}
		if m.Value != w.value || m.Unit != w.unit {
			t.Errorf("%s = %v %s, want %v %s", m.Name, m.Value, m.Unit, w.value, w.unit)
		}
		if m.Trust != environment.TrustThirdParty {
			t.Errorf("%s trust = %q, want third party", m.Name, m.Trust)
		}
	}
}

// A result with no structuredContent carries no measurement, however plainly
// its text states a number, and that is not an error.
func TestAResultWithoutStructuredContentCarriesNoMeasurements(t *testing.T) {
	env := measuringEnv(t)

	res, err := env.Act(context.Background(), environment.Action{
		CapabilityID: capability.MCPCapabilityID("metrics", proseTool),
	})
	if err != nil {
		t.Fatalf("Act returned a Go error: %v", err)
	}
	if !res.Success || res.Error != "" {
		t.Errorf("success = %v, error = %q; want a plain success", res.Success, res.Error)
	}
	if len(res.Measurements) != 0 {
		t.Errorf("measurements = %+v, scraped from the text", res.Measurements)
	}
}
