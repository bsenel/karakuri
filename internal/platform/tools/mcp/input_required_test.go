package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// The two tools the modern-only fake answers by asking for input. Neither is in
// fakeTools: an allowlist names what may be called, and the tests that count
// what a server advertises are not this file's business.
const (
	askTool     = "ask_token"
	askLongTool = "ask_token_at_length"

	// askText is the server's own sentence, the part of the result a planner
	// has to be shown to know why the call did not answer.
	askText = "Paste the deploy token for prod-eu"
)

// askLongText is far more than any error should carry to a planner.
var askLongText = askText + strings.Repeat(" and say please", 70_000)

// inputRequiredResult is the modern-only fake's answer to a tools/call for one
// of the asking tools: no content, a resultType of input_required, and the
// request under inputRequestsField in the elicitation shape the revision
// folded into results. The shape below that field is assumed, like the field.
func inputRequiredResult(req Request) (json.RawMessage, bool) {
	if req.Method != MethodToolsCall {
		return nil, false
	}
	var p CallToolParams
	_ = json.Unmarshal(req.Params, &p)
	text := askText
	switch p.Name {
	case askTool:
	case askLongTool:
		text = askLongText
	default:
		return nil, false
	}
	result, _ := json.Marshal(map[string]any{
		"resultType": ResultTypeInputRequired,
		inputRequestsField: map[string]any{
			"token": map[string]any{
				"method": "elicitation/create",
				"params": map[string]any{"message": text},
			},
		},
	})
	return result, true
}

// callsSeen counts the tools/call requests the fake received.
func callsSeen(f *modernFake) int {
	n := 0
	for _, r := range f.requests() {
		if r.method == MethodToolsCall {
			n++
		}
	}
	return n
}

// askingInstance is an instance over a modern-only fake that may call the
// asking tools.
func askingInstance(t *testing.T) (*Instance, *modernFake) {
	t.Helper()
	fake := &modernFake{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	inst := NewInstance(context.Background(), "fs", Config{
		Transport:    TransportHTTP,
		URL:          srv.URL,
		AllowedTools: []string{"read_file", askTool, askLongTool},
	})
	t.Cleanup(func() { _ = inst.Close() })
	if inst.State() != StateConnected {
		t.Fatalf("instance is %s, want %s", inst.State(), StateConnected)
	}
	return inst, fake
}

// wantInputRequired checks an error is the named one, for the asking tool, and
// carries what the server said.
func wantInputRequired(t *testing.T, who string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s returned no error for a result of %s", who, ResultTypeInputRequired)
	}
	var asked *InputRequiredError
	if !errors.As(err, &asked) {
		t.Fatalf("%s error = %q (%T), want one errors.As matches to *InputRequiredError", who, err, err)
	}
	if asked.Tool != askTool {
		t.Errorf("%s InputRequiredError.Tool = %q, want %q", who, asked.Tool, askTool)
	}
	if !strings.Contains(asked.Request, askText) {
		t.Errorf("%s InputRequiredError.Request = %q, want the server's text %q", who, asked.Request, askText)
	}
}

func TestInputRequiredIsANamedErrorFromCallTool(t *testing.T) {
	c := negotiatedModern(t, &modernFake{})

	res, err := c.CallTool(context.Background(), askTool, nil)
	wantInputRequired(t, "CallTool", err)

	// A request for input read as an answer is a tool that returned nothing.
	if len(res.Content) != 0 || res.IsError {
		t.Errorf("CallTool result = %+v, want the zero value beside the error", res)
	}
	for _, want := range []string{ResultTypeInputRequired, "does not answer requests for input", askTool, askText} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("CallTool error = %q, want it to mention %q", err, want)
		}
	}
}

func TestInputRequiredRequestIsCapped(t *testing.T) {
	c := negotiatedModern(t, &modernFake{})

	_, err := c.CallTool(context.Background(), askLongTool, nil)
	var asked *InputRequiredError
	if !errors.As(err, &asked) {
		t.Fatalf("CallTool error = %.200q (%T), want one errors.As matches to *InputRequiredError", err, err)
	}
	if !strings.Contains(asked.Request, askText) {
		t.Errorf("Request does not carry what the server asked: %.80q", asked.Request)
	}
	// The cap's value is the implementation's to choose. What is asserted is
	// that there is one: a megabyte of a server's prose does not reach a
	// planner prompt by way of an error.
	if len(asked.Request) > len(askLongText)/2 {
		t.Errorf("Request is %d bytes of the %d the server sent, want it capped", len(asked.Request), len(askLongText))
	}
	if len(err.Error()) > len(askLongText)/2 {
		t.Errorf("error text is %d bytes, want it capped with the request", len(err.Error()))
	}
}

func TestInstanceCallPassesInputRequiredThrough(t *testing.T) {
	inst, _ := askingInstance(t)

	res, err := inst.Call(context.Background(), askTool, nil)
	wantInputRequired(t, "Instance.Call", err)
	if len(res.Content) != 0 || res.IsError {
		t.Errorf("Instance.Call result = %+v, want the zero value beside the error", res)
	}
}

func TestActReportsInputRequiredAsANamedFailure(t *testing.T) {
	inst, _ := askingInstance(t)
	env := NewEnvironment(inst)

	res, err := env.Act(context.Background(), environment.Action{
		CapabilityID: capability.MCPCapabilityID(inst.Name(), askTool),
	})
	// Act reports a call that failed in the result, not beside it.
	if err != nil {
		t.Fatalf("Act returned an error beside the result: %v", err)
	}
	if res.Success {
		t.Errorf("Act reported success for a tool that asked for input instead of answering")
	}
	if res.Error == "" {
		t.Fatalf("Act returned an empty result: no error text, StateDelta = %v", res.StateDelta)
	}
	for _, want := range []string{ResultTypeInputRequired, askText} {
		if !strings.Contains(res.Error, want) {
			t.Errorf("Act error = %q, want it to mention %q", res.Error, want)
		}
	}
	if output, _ := res.StateDelta["output"].(string); output != "" {
		t.Errorf("Act carried output %q for a call that produced none", output)
	}
	// The request is the server's writing wherever it is carried (ADR 021).
	if res.Trust != environment.TrustThirdParty {
		t.Errorf("Act result Trust = %q, want %q: it carries the server's own text", res.Trust, environment.TrustThirdParty)
	}
}

// One request, whatever is later retried: a result of input_required arrived
// whole, and sending the call again asks the server the same question.
func TestInputRequiredIsNotRetried(t *testing.T) {
	t.Run("client", func(t *testing.T) {
		fake := &modernFake{}
		c := negotiatedModern(t, fake)
		if _, err := c.CallTool(context.Background(), askTool, nil); err == nil {
			t.Fatalf("CallTool returned no error for a result of %s", ResultTypeInputRequired)
		}
		if got := callsSeen(fake); got != 1 {
			t.Errorf("server saw %d %s requests, want exactly 1", got, MethodToolsCall)
		}
	})

	t.Run("environment", func(t *testing.T) {
		inst, fake := askingInstance(t)
		res, err := NewEnvironment(inst).Act(context.Background(), environment.Action{
			CapabilityID: capability.MCPCapabilityID(inst.Name(), askTool),
		})
		if err != nil || res.Success {
			t.Fatalf("Act = success %v, %v; want a failure in the result", res.Success, err)
		}
		if got := callsSeen(fake); got != 1 {
			t.Errorf("server saw %d %s requests, want exactly 1", got, MethodToolsCall)
		}
	})
}

// The old fake's results carry no resultType. Nothing on the handshake path
// reads one, so nothing there can be mistaken for a request for input.
func TestLegacyPathNeverReportsInputRequired(t *testing.T) {
	c := httpClient(t, &httpFake{})
	ctx := context.Background()
	if got, err := c.Negotiate(ctx); err != nil || got.Path != PathInitialize {
		t.Fatalf("Negotiate = path %q, %v; want %q", got.Path, err, PathInitialize)
	}

	res, err := c.CallTool(ctx, "read_file", map[string]any{"path": "go.mod"})
	if err != nil || res.Text() != "contents of go.mod" {
		t.Fatalf("CallTool = %q, %v; want the old fake's answer accepted without a resultType", res.Text(), err)
	}

	// A tool that ran and failed is still a result, and a tool the server does
	// not have is still the server's error: neither became the new one.
	res, err = c.CallTool(ctx, "fail_tool", nil)
	if err != nil || !res.IsError || res.Text() != "the tool broke" {
		t.Errorf("CallTool(fail_tool) = %+v, %v; want an isError result", res, err)
	}
	_, err = c.CallTool(ctx, askTool, nil)
	var asked *InputRequiredError
	if err == nil || errors.As(err, &asked) {
		t.Errorf("CallTool(%s) on the old path = %v, want the server's own no-such-tool error", askTool, err)
	}
}
