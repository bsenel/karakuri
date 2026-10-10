package mcp

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// streamClosedText is what readSSE says today when a stream ends without the
// reply. The re-issue has to keep it: it is the half of the error that says
// what went wrong, and reissuedText is the half that says what was done.
const (
	streamClosedText = "event stream closed before answering the request"
	reissuedText     = "re-issued once"
)

// seenFor returns the requests the fake received for one method, in order.
func seenFor(f *modernFake, method string) []modernRequest {
	var out []modernRequest
	for _, r := range f.requests() {
		if r.method == method {
			out = append(out, r)
		}
	}
	return out
}

// wantReissued asserts the fake saw a method exactly twice, under two request
// IDs, with _meta on both: the second request is a new request, not a replay.
func wantReissued(t *testing.T, f *modernFake, method string) {
	t.Helper()
	got := seenFor(f, method)
	if len(got) != 2 {
		t.Fatalf("server saw %d %s request(s), want 2: the first, and one re-issue after the stream broke", len(got), method)
	}
	if got[0].id == "" || got[1].id == "" {
		t.Errorf("%s ids = %q and %q, want both set", method, got[0].id, got[1].id)
	}
	if got[0].id == got[1].id {
		t.Errorf("%s was re-issued under the same request ID %s, want a new one", method, got[0].id)
	}
	for i, r := range got {
		if len(r.meta) == 0 {
			t.Errorf("%s request %d carried no _meta", method, i+1)
		}
	}
}

// brokenInstance is an instance over a modern-only fake in broken-stream mode.
func brokenInstance(t *testing.T, fake *modernFake, allowed ...string) *Instance {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	inst := NewInstance(context.Background(), "acme_api", Config{
		Transport:    TransportHTTP,
		URL:          srv.URL,
		AllowedTools: allowed,
	})
	t.Cleanup(func() { _ = inst.Close() })
	if inst.State() != StateConnected {
		t.Fatalf("state = %q (%s), want connected", inst.State(), inst.Health(false).Error)
	}
	return inst
}

// A stream that breaks once loses the request, and the client sends it again
// under a new ID.
func TestBrokenStreamReissuesAToolCallOnce(t *testing.T) {
	fake := &modernFake{breakMethod: MethodToolsCall, breakTimes: 1}
	c := negotiatedModern(t, fake)

	res, err := c.CallTool(context.Background(), "read_file", map[string]any{"path": "go.mod"})
	if err != nil {
		t.Fatalf("CallTool over a stream that broke once: %v", err)
	}
	if got, want := res.Text(), "contents of go.mod"; got != want {
		t.Errorf("CallTool text = %q, want %q", got, want)
	}
	wantReissued(t, fake, MethodToolsCall)
}

// One re-issue, not a loop: a stream that breaks every time is an error after
// the second request, and the error says both what happened and what was tried.
func TestBrokenStreamIsReissuedOnlyOnce(t *testing.T) {
	fake := &modernFake{breakMethod: MethodToolsCall, breakTimes: -1}
	c := negotiatedModern(t, fake)

	res, err := c.CallTool(context.Background(), "read_file", map[string]any{"path": "go.mod"})
	if err == nil {
		t.Fatalf("CallTool returned %q and no error over a stream that always breaks", res.Text())
	}
	if n := callsSeen(fake); n != 2 {
		t.Errorf("server saw %d tools/call request(s), want exactly 2", n)
	}
	for _, want := range []string{streamClosedText, reissuedText} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("CallTool error = %q, want it to contain %q", err, want)
		}
	}
}

// One deadline covers both attempts. The fake holds each stream open for most
// of the timeout before breaking it, so a second attempt given a timeout of
// its own would return well after the first one's deadline.
func TestReissueSharesOneDeadline(t *testing.T) {
	const (
		timeout    = 500 * time.Millisecond
		breakAfter = 400 * time.Millisecond
		// Two timeouts' worth would be 800ms: the second stream breaking.
		limit = 700 * time.Millisecond
	)
	fake := &modernFake{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	c := dial(t, Config{Transport: TransportHTTP, URL: srv.URL, Timeout: timeout})
	if _, err := c.Negotiate(context.Background()); err != nil {
		t.Fatalf("Negotiate: %v", err)
	}
	fake.mu.Lock()
	fake.breakMethod, fake.breakTimes, fake.breakAfter = MethodToolsCall, -1, breakAfter
	fake.mu.Unlock()

	start := time.Now()
	_, err := c.CallTool(context.Background(), "read_file", map[string]any{"path": "go.mod"})
	took := time.Since(start)
	if err == nil {
		t.Fatal("CallTool succeeded over a stream that always breaks")
	}
	if n := callsSeen(fake); n != 2 {
		t.Fatalf("server saw %d tools/call request(s), want 2: nothing was re-issued, so there is no second attempt to bound", n)
	}
	if took > limit {
		t.Errorf("CallTool returned after %s, want within about one %s timeout for both attempts", took, timeout)
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "deadline") {
		t.Errorf("CallTool error = %q, want the deadline that ended the second attempt", err)
	}
}

// The allowlist is checked before either attempt.
func TestAllowlistIsCheckedBeforeEitherAttempt(t *testing.T) {
	fake := &modernFake{breakMethod: MethodToolsCall, breakTimes: -1}
	inst := brokenInstance(t, fake, "read_file")

	before := len(fake.requests())
	if _, err := inst.Call(context.Background(), "delete_repo", nil); err == nil ||
		!strings.Contains(err.Error(), "allowlist") {
		t.Errorf("err = %v, want an allowlist refusal", err)
	}
	res, err := NewEnvironment(inst).Act(context.Background(), environment.Action{
		CapabilityID: capability.MCPCapabilityID("acme_api", "delete_repo"),
	})
	if err != nil {
		t.Fatalf("Act returned a Go error: %v", err)
	}
	if res.Success || !strings.Contains(res.Error, "allowlist") || res.Trust != environment.TrustThirdParty {
		t.Errorf("result = %+v, want a third-party allowlist refusal", res)
	}
	if after := len(fake.requests()); after != before {
		t.Errorf("%d request(s) reached the server for a refused tool", after-before)
	}
}

// What ADR 022 says of a call holds for the re-issued one: it is the same tool
// under the same reserved capability, it grants no workspace, and what comes
// back is third-party text whether the retry answered or not.
func TestReissuedCallKeepsTheADR022Bounds(t *testing.T) {
	t.Run("answered on the retry", func(t *testing.T) {
		fake := &modernFake{breakMethod: MethodToolsCall, breakTimes: 1}
		inst := brokenInstance(t, fake, "read_file")

		res, err := NewEnvironment(inst).Act(context.Background(), environment.Action{
			CapabilityID: capability.MCPCapabilityID("acme_api", "read_file"),
			Params:       map[string]any{"path": "go.mod"},
		})
		if err != nil {
			t.Fatalf("Act returned a Go error: %v", err)
		}
		if !res.Success {
			t.Errorf("result = %+v, want the re-issued call's answer", res)
		}
		if res.Trust != environment.TrustThirdParty {
			t.Errorf("trust = %q on a re-issued call's result, want third party", res.Trust)
		}

		wantReissued(t, fake, MethodToolsCall)
		for i, r := range seenFor(fake, MethodToolsCall) {
			if r.tool != "read_file" {
				t.Errorf("tools/call request %d named %q, want the allowed tool read_file", i+1, r.tool)
			}
		}
		for _, c := range inst.Capabilities() {
			if !capability.IsMCPCapability(c.ID) || c.Domain != capability.MCPDomain || c.GrantsWorkspace() {
				t.Errorf("capability %q in %q left the reserved namespace after a re-issue", c.ID, c.Domain)
			}
		}
	})

	t.Run("broken on the retry", func(t *testing.T) {
		fake := &modernFake{breakMethod: MethodToolsCall, breakTimes: -1}
		inst := brokenInstance(t, fake, "read_file")

		res, err := NewEnvironment(inst).Act(context.Background(), environment.Action{
			CapabilityID: capability.MCPCapabilityID("acme_api", "read_file"),
			Params:       map[string]any{"path": "go.mod"},
		})
		if err != nil {
			t.Fatalf("Act returned a Go error: %v", err)
		}
		if res.Success || res.Trust != environment.TrustThirdParty {
			t.Errorf("result = %+v, want a third-party failure", res)
		}
		if !strings.Contains(res.Error, reissuedText) {
			t.Errorf("result error = %q, want it to contain %q", res.Error, reissuedText)
		}
		if n := callsSeen(fake); n != 2 {
			t.Errorf("server saw %d tools/call request(s), want exactly 2", n)
		}
	})
}

// Only a broken stream is re-issued. An answer that arrived whole is an answer,
// whatever it says, also when it arrived on a stream.
func TestOnlyABrokenStreamIsReissued(t *testing.T) {
	cases := []struct {
		name  string
		tool  string
		check func(t *testing.T, res ToolResult, err error)
	}{
		{"input_required", askTool, func(t *testing.T, _ ToolResult, err error) {
			wantInputRequired(t, "CallTool", err)
		}},
		{"json-rpc error", "no_such_tool", func(t *testing.T, _ ToolResult, err error) {
			if err == nil || !strings.Contains(err.Error(), "server error") {
				t.Errorf("err = %v, want the server's JSON-RPC error", err)
			}
		}},
		{"isError result", "fail_tool", func(t *testing.T, res ToolResult, err error) {
			if err != nil || !res.IsError {
				t.Errorf("result = %+v, err = %v, want an isError result and no error", res, err)
			}
		}},
	}
	for _, sse := range []bool{false, true} {
		for _, tc := range cases {
			name := tc.name + " as json"
			if sse {
				name = tc.name + " on a stream"
			}
			t.Run(name, func(t *testing.T) {
				fake := &modernFake{sse: sse}
				c := negotiatedModern(t, fake)

				res, err := c.CallTool(context.Background(), tc.tool, nil)
				tc.check(t, res, err)
				if err != nil && strings.Contains(err.Error(), "re-issued") {
					t.Errorf("err = %q, want no mention of a re-issue", err)
				}
				if n := callsSeen(fake); n != 1 {
					t.Errorf("server saw %d tools/call request(s), want exactly 1", n)
				}
			})
		}
	}
}

// Re-issue is the 2026-07-28 rule. The older revisions had resumption, which
// this client never implemented, so there a broken stream fails as it always
// did: once.
func TestLegacyPathDoesNotReissueABrokenStream(t *testing.T) {
	rec := &methodRecorder{next: &httpFake{sse: true, breakStream: true}}
	c := httpClient(t, rec)
	got, err := c.Negotiate(context.Background())
	if err != nil {
		t.Fatalf("Negotiate: %v", err)
	}
	if got.Path != PathInitialize {
		t.Fatalf("Path = %q, want %q", got.Path, PathInitialize)
	}

	res, err := c.CallTool(context.Background(), "read_file", map[string]any{"path": "go.mod"})
	if err == nil {
		t.Fatalf("CallTool returned %q and no error over a broken stream", res.Text())
	}
	if !strings.Contains(err.Error(), streamClosedText) {
		t.Errorf("CallTool error = %q, want it to contain %q", err, streamClosedText)
	}
	if strings.Contains(err.Error(), "re-issued") {
		t.Errorf("CallTool error = %q, want no re-issue on the handshake path", err)
	}

	calls := 0
	rec.mu.Lock()
	for _, m := range rec.methods {
		if m == MethodToolsCall {
			calls++
		}
	}
	rec.mu.Unlock()
	if calls != 1 {
		t.Errorf("server saw %d tools/call request(s), want exactly 1", calls)
	}
}

// The rule is per request, not per tool call: tools/list is re-issued too, so
// one broken stream at boot does not leave an instance unreachable.
func TestBrokenStreamReissuesToolsListDuringDiscovery(t *testing.T) {
	t.Run("client", func(t *testing.T) {
		fake := &modernFake{breakMethod: MethodToolsList, breakTimes: 1}
		c := negotiatedModern(t, fake)

		tools, err := c.ListTools(context.Background())
		if err != nil {
			t.Fatalf("ListTools over a stream that broke once: %v", err)
		}
		if len(tools) != len(fakeTools) {
			t.Errorf("ListTools returned %d tools, want %d", len(tools), len(fakeTools))
		}
		wantReissued(t, fake, MethodToolsList)
	})

	t.Run("instance", func(t *testing.T) {
		fake := &modernFake{breakMethod: MethodToolsList, breakTimes: 1}
		inst := brokenInstance(t, fake, "read_file")

		if got := inst.CapabilityIDs(); len(got) != 1 || got[0] != capability.MCPCapabilityID("acme_api", "read_file") {
			t.Errorf("CapabilityIDs = %v, want only read_file", got)
		}
		wantReissued(t, fake, MethodToolsList)
	})
}
