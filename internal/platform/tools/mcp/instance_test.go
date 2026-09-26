package mcp

import (
	"context"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
)

// A stdio server is launched, handshaken and listed at construction, and what
// /health reports is what it offered, narrowed by the allowlist — with the
// other side of the allowlist visible rather than silent.
func TestStdioInstanceDiscoversAllowedToolsAndReportsTheRest(t *testing.T) {
	inst := NewInstance(context.Background(), "acme_files", stdioConfig("read_file", "fail_tool"))
	t.Cleanup(func() { _ = inst.Close() })

	h := inst.Health(true)
	if h.State != StateConnected {
		t.Fatalf("state = %q (%s), want connected", h.State, h.Error)
	}
	if want := []string{"fail_tool", "read_file"}; !reflect.DeepEqual(h.Tools, want) {
		t.Errorf("tools = %v, want %v sorted", h.Tools, want)
	}
	if want := []string{"delete_repo", "hang"}; !reflect.DeepEqual(h.Filtered, want) {
		t.Errorf("filtered = %v, want %v", h.Filtered, want)
	}
	// What the server said, not this implementation's constant.
	if h.Server != "fake-fs" || h.ProtocolVersion != "2099-01-01" {
		t.Errorf("server = %q protocol = %q, want what the server stated", h.Server, h.ProtocolVersion)
	}
	if h.Transport != TransportStdio || !h.IsDefault {
		t.Errorf("transport = %q default = %v", h.Transport, h.IsDefault)
	}
	if !inst.Active() {
		t.Error("a connected instance with allowed tools is not active")
	}
}

// The registry entries carry the reserved namespace, the server's text, and the
// part of its JSON Schema the planner reads.
func TestInstanceCapabilitiesAreNamespacedWithTheServersSchema(t *testing.T) {
	inst := NewInstance(context.Background(), "acme_files", stdioConfig("read_file"))
	t.Cleanup(func() { _ = inst.Close() })

	caps := inst.Capabilities()
	if len(caps) != 1 {
		t.Fatalf("capabilities = %+v, want one", caps)
	}
	c := caps[0]
	if c.ID != capability.MCPCapabilityID("acme_files", "read_file") || c.Domain != capability.MCPDomain {
		t.Errorf("capability %q in %q, want mcp.acme_files.read_file in mcp", c.ID, c.Domain)
	}
	if c.Description != "Reads a file." {
		t.Errorf("description = %q, want the server's own", c.Description)
	}
	if p := c.InputSchema.Properties["path"]; p.Type != "string" || p.Description != "Which file." {
		t.Errorf("path property = %+v", p)
	}
	if !reflect.DeepEqual(c.InputSchema.Required, []string{"path"}) {
		t.Errorf("required = %v, want [path]", c.InputSchema.Required)
	}
	if !reflect.DeepEqual(inst.CapabilityIDs(), []capability.CapabilityID{c.ID}) {
		t.Errorf("CapabilityIDs = %v, want [%s]", inst.CapabilityIDs(), c.ID)
	}
}

// An empty allowlist allows nothing: the server is connected and offers no
// tool this deployment will register.
func TestEmptyAllowlistAllowsNothing(t *testing.T) {
	inst := NewInstance(context.Background(), "acme_files", stdioConfig())
	t.Cleanup(func() { _ = inst.Close() })

	if inst.State() != StateConnected {
		t.Fatalf("state = %q, want connected", inst.State())
	}
	if len(inst.Tools()) != 0 || len(inst.CapabilityIDs()) != 0 {
		t.Errorf("tools = %v, want none", inst.Tools())
	}
	if inst.Active() {
		t.Error("an instance allowing nothing reports active")
	}
	if len(inst.Health(false).Filtered) != len(fakeTools) {
		t.Errorf("filtered = %v, want every advertised tool", inst.Health(false).Filtered)
	}
}

func TestStdioCallRoundTrips(t *testing.T) {
	inst := NewInstance(context.Background(), "acme_files", stdioConfig("read_file", "fail_tool"))
	t.Cleanup(func() { _ = inst.Close() })

	res, err := inst.Call(context.Background(), "read_file", map[string]any{"path": "README.md"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || res.Text() != "contents of README.md" {
		t.Errorf("result = %+v", res)
	}

	res, err = inst.Call(context.Background(), "fail_tool", nil)
	if err != nil {
		t.Fatalf("a tool that ran and failed is a result, not an error: %v", err)
	}
	if !res.IsError || res.Text() != "the tool broke" {
		t.Errorf("result = %+v, want an error result", res)
	}
}

// Calls made at once each get their own reply. The stdio transport reads one
// pipe and skips IDs it did not send, so without the client holding one
// exchange at a time two calls discard each other's replies and time out.
func TestConcurrentStdioCallsEachGetTheirOwnReply(t *testing.T) {
	cfg := stdioConfig("read_file")
	cfg.Timeout = 5 * time.Second
	inst := NewInstance(context.Background(), "acme_files", cfg)
	t.Cleanup(func() { _ = inst.Close() })

	const n = 16
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := fmt.Sprintf("file-%d", i)
			res, err := inst.Call(context.Background(), "read_file", map[string]any{"path": path})
			if err != nil {
				errs <- err
				return
			}
			if want := "contents of " + path; res.Text() != want {
				errs <- fmt.Errorf("call %d got %q, want %q", i, res.Text(), want)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// The allowlist is checked at call time too: a capability ID can outlive the
// list that admitted it.
func TestCallRefusesAToolOffTheAllowlist(t *testing.T) {
	inst := NewInstance(context.Background(), "acme_files", stdioConfig("read_file"))
	t.Cleanup(func() { _ = inst.Close() })

	if _, err := inst.Call(context.Background(), "delete_repo", nil); err == nil ||
		!strings.Contains(err.Error(), "allowlist") {
		t.Errorf("err = %v, want an allowlist refusal", err)
	}
}

// A server that does not answer in time takes the transport down rather than
// leaving the stream one reply out of step; the instance says why.
func TestStdioCallThatTimesOutBreaksTheTransport(t *testing.T) {
	cfg := stdioConfig("read_file", "hang")
	cfg.Timeout = 200 * time.Millisecond
	inst := NewInstance(context.Background(), "acme_files", cfg)
	t.Cleanup(func() { _ = inst.Close() })

	if _, err := inst.Call(context.Background(), "hang", nil); err == nil ||
		!strings.Contains(err.Error(), "deadline") {
		t.Fatalf("err = %v, want a deadline failure", err)
	}
	if _, err := inst.Call(context.Background(), "read_file", map[string]any{"path": "x"}); err == nil {
		t.Error("a transport out of step with its server answered another call")
	}
}

// A command that cannot be started is a configuration mistake /health can
// show, not a boot failure.
func TestInstanceThatCannotBeDialledIsMisconfigured(t *testing.T) {
	for _, cfg := range []Config{
		{Transport: TransportStdio, Command: "/nonexistent/karakuri-mcp-server"},
		{Transport: TransportStdio},
		{Transport: TransportHTTP},
		{Transport: "carrier-pigeon"},
		{},
	} {
		inst := NewInstance(context.Background(), "broken", cfg)
		if inst.State() != StateMisconfigured {
			t.Errorf("config %+v: state = %q, want misconfigured", cfg, inst.State())
		}
		if inst.Health(false).Error == "" || len(inst.Tools()) != 0 {
			t.Errorf("config %+v: health = %+v, want an error and no tools", cfg, inst.Health(false))
		}
		if _, err := inst.Call(context.Background(), "read_file", nil); err == nil {
			t.Errorf("config %+v: an undialled instance ran a tool", cfg)
		}
	}
}

func TestInstanceWhoseServerIsDownIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(&httpFake{})
	url := srv.URL
	srv.Close()

	inst := NewInstance(context.Background(), "gone", Config{Transport: TransportHTTP, URL: url, AllowedTools: []string{"read_file"}})
	h := inst.Health(false)
	if h.State != StateUnreachable || h.Error == "" || len(h.Tools) != 0 {
		t.Errorf("health = %+v, want unreachable with an error and no tools", h)
	}
}

// Both legal replies to a POST — a JSON body and an event stream with progress
// ahead of the result — and the session the server issued at initialize
// echoed on everything after it, beside the configured credential.
func TestHTTPInstanceSpeaksJSONAndSSE(t *testing.T) {
	for _, sse := range []bool{false, true} {
		t.Run(fmt.Sprintf("sse=%v", sse), func(t *testing.T) {
			fake := &httpFake{sse: sse}
			srv := httptest.NewServer(fake)
			t.Cleanup(srv.Close)

			inst := NewInstance(context.Background(), "acme_api", Config{
				Transport:    TransportHTTP,
				URL:          srv.URL,
				Headers:      map[string]string{"Authorization": "Bearer t0k"},
				AllowedTools: []string{"read_file"},
			})
			t.Cleanup(func() { _ = inst.Close() })

			if inst.State() != StateConnected {
				t.Fatalf("state = %q (%s)", inst.State(), inst.Health(false).Error)
			}
			res, err := inst.Call(context.Background(), "read_file", map[string]any{"path": "a"})
			if err != nil || res.Text() != "contents of a" {
				t.Fatalf("call = %+v, %v", res, err)
			}

			fake.mu.Lock()
			defer fake.mu.Unlock()
			for _, s := range fake.sessions {
				if s != "session-1" {
					t.Errorf("a request after initialize carried session %q, want session-1", s)
				}
			}
			for _, a := range fake.auth {
				if a != "Bearer t0k" {
					t.Errorf("Authorization = %q, want the configured bearer", a)
				}
			}
		})
	}
}
