package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// boundPath is one of the two ways an instance reaches its server, and what
// /health should say about each.
type boundPath struct {
	name    string
	handler func() http.Handler
	path    string
	version string
	server  string
}

// bothPaths is the modern-only fake, reached by discovery, and the old fake,
// which answers server/discover with method-not-found and is reached by the
// handshake. The old fake states a version of its own, so the assertion is on
// what the server said rather than this implementation's constant.
var bothPaths = []boundPath{
	{"modern", func() http.Handler { return &modernFake{} }, PathDiscover, ModernProtocolVersion, "modern-fs"},
	{"legacy", func() http.Handler { return &httpFake{} }, PathInitialize, "2099-01-01", "fake-fs"},
}

// pathInstance serves the path's fake behind a recorder and binds an instance
// to it. It fails the test unless discovery succeeded: none of the tests below
// has anything to say about an instance that did not connect.
func pathInstance(t *testing.T, p boundPath, timeout time.Duration, allowed ...string) (*Instance, *methodRecorder) {
	t.Helper()
	rec := &methodRecorder{next: p.handler()}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)
	inst := NewInstance(context.Background(), "acme_api", Config{
		Transport:    TransportHTTP,
		URL:          srv.URL,
		Timeout:      timeout,
		AllowedTools: allowed,
	})
	t.Cleanup(func() { _ = inst.Close() })
	if inst.State() != StateConnected {
		t.Fatalf("state = %q (%s), want connected", inst.State(), inst.Health(false).Error)
	}
	return inst, rec
}

func requestCount(m *methodRecorder) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.methods)
}

// The instance records which path it negotiated, beside the version, and the
// observation the planner reads says the same thing /health does.
func TestInstanceRecordsTheNegotiatedPath(t *testing.T) {
	for _, p := range bothPaths {
		t.Run(p.name, func(t *testing.T) {
			inst, _ := pathInstance(t, p, 0, "read_file", "fail_tool")

			if !inst.Active() {
				t.Error("a connected instance with allowed tools is not active")
			}
			h := inst.Health(true)
			if want := []string{"fail_tool", "read_file"}; !reflect.DeepEqual(h.Tools, want) {
				t.Errorf("tools = %v, want %v sorted", h.Tools, want)
			}
			if want := []string{"delete_repo", "hang"}; !reflect.DeepEqual(h.Filtered, want) {
				t.Errorf("filtered = %v, want %v", h.Filtered, want)
			}
			if h.ProtocolPath != p.path {
				t.Errorf("protocol path = %q, want %q", h.ProtocolPath, p.path)
			}
			if h.ProtocolVersion != p.version || h.Server != p.server {
				t.Errorf("protocol = %q server = %q, want %q from %q", h.ProtocolVersion, h.Server, p.version, p.server)
			}

			obs, err := NewEnvironment(inst).Observe(context.Background(), environment.ObservationQuery{})
			if err != nil {
				t.Fatal(err)
			}
			if obs.State["protocol_path"] != p.path || obs.State["protocol_version"] != p.version {
				t.Errorf("observed path = %v version = %v, want %q and %q",
					obs.State["protocol_path"], obs.State["protocol_version"], p.path, p.version)
			}
		})
	}
}

// The allowlist holds on both paths: a tool off it is not registered, and a
// call for it is refused before anything is sent.
func TestAllowlistHoldsOnBothPaths(t *testing.T) {
	for _, p := range bothPaths {
		t.Run(p.name, func(t *testing.T) {
			inst, rec := pathInstance(t, p, 0, "read_file")

			for _, tool := range inst.Tools() {
				if tool.Name != "read_file" {
					t.Errorf("tool %q is registered and is not on the allowlist", tool.Name)
				}
			}
			want := []capability.CapabilityID{capability.MCPCapabilityID("acme_api", "read_file")}
			if !reflect.DeepEqual(inst.CapabilityIDs(), want) {
				t.Errorf("CapabilityIDs = %v, want %v", inst.CapabilityIDs(), want)
			}
			if caps := inst.Capabilities(); len(caps) != 1 || caps[0].ID != want[0] {
				t.Errorf("capabilities = %+v, want only %s", caps, want[0])
			}

			before := requestCount(rec)
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
			if after := requestCount(rec); after != before {
				t.Errorf("%d request(s) reached the server for a refused tool", after-before)
			}
		})
	}
}

// An empty allowlist allows nothing, whichever way the server was reached.
func TestEmptyAllowlistAllowsNothingOnBothPaths(t *testing.T) {
	for _, p := range bothPaths {
		t.Run(p.name, func(t *testing.T) {
			inst, rec := pathInstance(t, p, 0)

			if len(inst.Tools()) != 0 || len(inst.Capabilities()) != 0 || len(inst.CapabilityIDs()) != 0 {
				t.Errorf("tools = %v, want none", inst.Tools())
			}
			if inst.Active() {
				t.Error("an instance allowing nothing reports active")
			}
			if len(inst.Health(false).Filtered) != len(fakeTools) {
				t.Errorf("filtered = %v, want every advertised tool", inst.Health(false).Filtered)
			}

			before := requestCount(rec)
			if _, err := inst.Call(context.Background(), "read_file", map[string]any{"path": "a"}); err == nil ||
				!strings.Contains(err.Error(), "allowlist") {
				t.Errorf("err = %v, want an allowlist refusal", err)
			}
			if after := requestCount(rec); after != before {
				t.Errorf("%d request(s) reached the server with nothing allowed", after-before)
			}
		})
	}
}

// What ADR 022's four bounds key on is the reserved namespace, so that is what
// has to survive the new path: the IDs, the domain, no workspace, and a factory
// that serves exactly what was discovered and allowed.
func TestDiscoveredToolsStayInTheReservedNamespaceOnBothPaths(t *testing.T) {
	for _, p := range bothPaths {
		t.Run(p.name, func(t *testing.T) {
			inst, _ := pathInstance(t, p, 0, "read_file", "fail_tool")

			caps := inst.Capabilities()
			if len(caps) != 2 {
				t.Fatalf("capabilities = %+v, want two", caps)
			}
			for _, c := range caps {
				if !capability.IsMCPCapability(c.ID) || c.Domain != capability.MCPDomain {
					t.Errorf("capability %q in %q is outside the reserved namespace", c.ID, c.Domain)
				}
				if c.GrantsWorkspace() {
					t.Errorf("capability %q grants a workspace", c.ID)
				}
			}
			read := caps[1]
			if read.ID != capability.MCPCapabilityID("acme_api", "read_file") || read.Description != "Reads a file." {
				t.Errorf("capability = %q %q, want read_file with the server's own text", read.ID, read.Description)
			}
			if !reflect.DeepEqual(read.InputSchema.Required, []string{"path"}) {
				t.Errorf("required = %v, want [path]", read.InputSchema.Required)
			}

			env := NewEnvironment(inst)
			if env.ID() != "mcp.env.acme_api" || env.Domain() != capability.MCPDomain {
				t.Errorf("env %q in %q", env.ID(), env.Domain())
			}
			f := NewFactory(inst, true)
			if f.Domain != capability.MCPDomain || !reflect.DeepEqual(f.Serves, inst.CapabilityIDs()) {
				t.Errorf("factory in %q serves %v, want %v", f.Domain, f.Serves, inst.CapabilityIDs())
			}
		})
	}
}

// The per-call timeout bounds a tool that never answers on both paths.
func TestCallThatHangsHitsTheTimeoutOnBothPaths(t *testing.T) {
	for _, p := range bothPaths {
		t.Run(p.name, func(t *testing.T) {
			inst, _ := pathInstance(t, p, 200*time.Millisecond, "read_file", "hang")

			start := time.Now()
			if _, err := inst.Call(context.Background(), "hang", nil); err == nil ||
				!strings.Contains(err.Error(), "deadline") {
				t.Fatalf("err = %v, want a deadline failure", err)
			}
			if took := time.Since(start); took > 3*time.Second {
				t.Errorf("the call returned after %s, want it bounded by the 200ms timeout", took)
			}
		})
	}
}

// Untrusted by construction on both paths: what the server advertises and what
// its tools return are somebody else's writing, and a tool that ran and failed
// is a result carrying the tool's own words.
func TestThirdPartyTrustHoldsOnBothPaths(t *testing.T) {
	for _, p := range bothPaths {
		t.Run(p.name, func(t *testing.T) {
			inst, _ := pathInstance(t, p, 0, "read_file", "fail_tool")
			env := NewEnvironment(inst)
			ctx := context.Background()

			obs, err := env.Observe(ctx, environment.ObservationQuery{})
			if err != nil {
				t.Fatal(err)
			}
			if obs.Trust != environment.TrustThirdParty {
				t.Errorf("observation trust = %q with tools advertised, want third party", obs.Trust)
			}

			res, err := env.Act(ctx, environment.Action{
				CapabilityID: capability.MCPCapabilityID("acme_api", "read_file"),
				Params:       map[string]any{"path": "go.mod"},
			})
			if err != nil {
				t.Fatalf("Act returned a Go error: %v", err)
			}
			if !res.Success || res.Trust != environment.TrustThirdParty {
				t.Errorf("result = %+v, want a third-party success", res)
			}
			if res.StateDelta["output"] != "contents of go.mod" || res.StateDelta["tool"] != "read_file" {
				t.Errorf("state delta = %v", res.StateDelta)
			}

			res, err = env.Act(ctx, environment.Action{CapabilityID: capability.MCPCapabilityID("acme_api", "fail_tool")})
			if err != nil {
				t.Fatalf("a tool that ran and failed is a result, not an error: %v", err)
			}
			if res.Success || res.Trust != environment.TrustThirdParty || !strings.Contains(res.Error, "the tool broke") {
				t.Errorf("result = %+v, want a third-party failure saying what the tool said", res)
			}
		})
	}
}

// The phase's acceptance: a server that speaks only 2026-07-28 is bound, its
// tools discovered and one of them called, with no handshake completed and no
// session echoed.
func TestAcceptance_ModernOnlyServer(t *testing.T) {
	fake := &modernFake{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	inst := NewInstance(context.Background(), "acme_api", Config{
		Transport:    TransportHTTP,
		URL:          srv.URL,
		AllowedTools: []string{"read_file"},
	})
	t.Cleanup(func() { _ = inst.Close() })

	h := inst.Health(true)
	if h.State != StateConnected {
		t.Fatalf("state = %q (%s), want connected", h.State, h.Error)
	}
	if !reflect.DeepEqual(h.Tools, []string{"read_file"}) {
		t.Errorf("tools = %v, want [read_file]", h.Tools)
	}
	if h.ProtocolPath != PathDiscover || h.ProtocolVersion != ModernProtocolVersion {
		t.Errorf("path = %q version = %q, want %q and %q", h.ProtocolPath, h.ProtocolVersion, PathDiscover, ModernProtocolVersion)
	}

	res, err := NewEnvironment(inst).Act(context.Background(), environment.Action{
		CapabilityID: capability.MCPCapabilityID("acme_api", "read_file"),
		Params:       map[string]any{"path": "go.mod"},
	})
	if err != nil {
		t.Fatalf("Act returned a Go error: %v", err)
	}
	if !res.Success || res.StateDelta["output"] != "contents of go.mod" || res.Trust != environment.TrustThirdParty {
		t.Errorf("result = %+v, want a third-party success with the tool's text", res)
	}

	if !fake.saw(MethodServerDiscover) || !fake.saw(MethodToolsCall) {
		t.Error("the server was not discovered and called")
	}
	for _, r := range fake.requests() {
		if r.session != "" {
			t.Errorf("%s carried session %q to a server that has none", r.method, r.session)
		}
	}
}

// The other half: a server that speaks only 2025-06-18 answers server/discover
// with method-not-found and is still bound, by the handshake.
func TestAcceptance_LegacyOnlyServer(t *testing.T) {
	rec := &methodRecorder{next: &httpFake{}}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	inst := NewInstance(context.Background(), "acme_api", Config{
		Transport:    TransportHTTP,
		URL:          srv.URL,
		AllowedTools: []string{"read_file"},
	})
	t.Cleanup(func() { _ = inst.Close() })

	h := inst.Health(true)
	if h.State != StateConnected {
		t.Fatalf("state = %q (%s), want connected", h.State, h.Error)
	}
	if !reflect.DeepEqual(h.Tools, []string{"read_file"}) {
		t.Errorf("tools = %v, want [read_file]", h.Tools)
	}
	if h.ProtocolPath != PathInitialize || h.ProtocolVersion != "2099-01-01" {
		t.Errorf("path = %q version = %q, want %q and what the server stated", h.ProtocolPath, h.ProtocolVersion, PathInitialize)
	}

	res, err := NewEnvironment(inst).Act(context.Background(), environment.Action{
		CapabilityID: capability.MCPCapabilityID("acme_api", "read_file"),
		Params:       map[string]any{"path": "go.mod"},
	})
	if err != nil {
		t.Fatalf("Act returned a Go error: %v", err)
	}
	if !res.Success || res.StateDelta["output"] != "contents of go.mod" || res.Trust != environment.TrustThirdParty {
		t.Errorf("result = %+v, want a third-party success with the tool's text", res)
	}
	if !rec.saw(MethodInitialize) || !rec.saw(MethodToolsCall) {
		t.Error("the server was not handshaken and called")
	}
}
