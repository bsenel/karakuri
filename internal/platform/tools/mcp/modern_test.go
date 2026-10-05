package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// modernRequest is one request as the modern-only fake saw it.
type modernRequest struct {
	method  string
	meta    json.RawMessage // params._meta, nil when the request carried none
	session string          // the Mcp-Session-Id request header
}

// modernFake implements only revision 2026-07-28, as the roadmap describes it:
// no handshake, server/discover instead, a resultType on every result. It
// offers a session id on every reply although the revision has none, so a
// client that still echoes sessions is caught doing it.
type modernFake struct {
	// omitResultType makes tools/list and tools/call results arrive without
	// the field. Discovery keeps it, so the connection still opens.
	omitResultType bool

	mu   sync.Mutex
	seen []modernRequest
}

func (f *modernFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	var params struct {
		Meta json.RawMessage `json:"_meta"`
	}
	_ = json.Unmarshal(req.Params, &params)
	f.mu.Lock()
	f.seen = append(f.seen, modernRequest{method: req.Method, meta: params.Meta, session: r.Header.Get(sessionHeader)})
	f.mu.Unlock()

	w.Header().Set(sessionHeader, "bait")
	if req.IsNotification() {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	resp := Response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case MethodInitialize:
		resp.Error = &Error{
			Code:    CodeUnsupportedProtocol,
			Message: "unsupported protocol version",
			Data:    map[string]any{"supported": []string{ModernProtocolVersion}},
		}
	case MethodServerDiscover:
		resp.Result, _ = json.Marshal(discoverResult{
			ResultType:        resultTypeComplete,
			SupportedVersions: []string{ModernProtocolVersion},
			ServerInfo:        Info{Name: "modern-fs", Version: "1"},
		})
	default:
		var ok bool
		resp, ok = fakeHandle(req)
		if !ok {
			<-r.Context().Done()
			return
		}
		if resp.Error == nil && !f.omitResultType {
			var result map[string]json.RawMessage
			_ = json.Unmarshal(resp.Result, &result)
			result["resultType"], _ = json.Marshal(resultTypeComplete)
			resp.Result, _ = json.Marshal(result)
		}
	}
	body, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (f *modernFake) requests() []modernRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]modernRequest(nil), f.seen...)
}

func (f *modernFake) saw(method string) bool {
	for _, r := range f.requests() {
		if r.method == method {
			return true
		}
	}
	return false
}

// methodRecorder notes the method of each request on its way to another
// handler. httpFake does not record methods, and the fallback test needs to
// know the handshake was actually sent.
type methodRecorder struct {
	next http.Handler

	mu      sync.Mutex
	methods []string
}

func (m *methodRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var req Request
	_ = json.Unmarshal(raw, &req)
	m.mu.Lock()
	m.methods = append(m.methods, req.Method)
	m.mu.Unlock()
	r.Body = io.NopCloser(bytes.NewReader(raw))
	m.next.ServeHTTP(w, r)
}

func (m *methodRecorder) saw(method string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, got := range m.methods {
		if got == method {
			return true
		}
	}
	return false
}

// httpClient serves a handler and dials it.
func httpClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return dial(t, Config{Transport: TransportHTTP, URL: srv.URL})
}

func dial(t *testing.T, cfg Config) *Client {
	t.Helper()
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// negotiatedModern returns a client that has opened a connection to a
// modern-only fake.
func negotiatedModern(t *testing.T, fake *modernFake) *Client {
	t.Helper()
	c := httpClient(t, fake)
	if _, err := c.Negotiate(context.Background()); err != nil {
		t.Fatalf("Negotiate against a 2026-07-28-only server: %v", err)
	}
	return c
}

func TestNegotiateDiscoversAModernOnlyServer(t *testing.T) {
	fake := &modernFake{}
	c := httpClient(t, fake)

	got, err := c.Negotiate(context.Background())
	if err != nil {
		t.Fatalf("Negotiate: %v", err)
	}
	if got.Path != PathDiscover {
		t.Errorf("Path = %q, want %q", got.Path, PathDiscover)
	}
	if got.ProtocolVersion != ModernProtocolVersion {
		t.Errorf("ProtocolVersion = %q, want %q", got.ProtocolVersion, ModernProtocolVersion)
	}
	if got.ServerInfo.Name != "modern-fs" {
		t.Errorf("ServerInfo.Name = %q, want modern-fs", got.ServerInfo.Name)
	}

	if !fake.saw(MethodServerDiscover) {
		t.Errorf("server never saw %s", MethodServerDiscover)
	}
	// A server that has discovery is never offered the handshake it removed.
	for _, method := range []string{MethodInitialize, MethodInitialized} {
		if fake.saw(method) {
			t.Errorf("server saw %s, which revision %s does not have", method, ModernProtocolVersion)
		}
	}
}

func TestModernPathListsAndCallsTools(t *testing.T) {
	c := negotiatedModern(t, &modernFake{})
	ctx := context.Background()

	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != len(fakeTools) {
		t.Fatalf("ListTools returned %d tools, want %d", len(tools), len(fakeTools))
	}
	for i, tool := range tools {
		if tool.Name != fakeTools[i].Name {
			t.Errorf("tool %d = %q, want %q", i, tool.Name, fakeTools[i].Name)
		}
	}

	res, err := c.CallTool(ctx, "read_file", map[string]any{"path": "go.mod"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if got, want := res.Text(), "contents of go.mod"; got != want {
		t.Errorf("CallTool text = %q, want %q", got, want)
	}
}

func TestModernRequestsCarryMetaAndNoSession(t *testing.T) {
	fake := &modernFake{}
	c := negotiatedModern(t, fake)
	ctx := context.Background()
	if _, err := c.ListTools(ctx); err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if _, err := c.CallTool(ctx, "read_file", map[string]any{"path": "go.mod"}); err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	for _, method := range []string{MethodServerDiscover, MethodToolsList, MethodToolsCall} {
		if !fake.saw(method) {
			t.Errorf("server never saw %s", method)
		}
	}
	for _, r := range fake.requests() {
		// The server offered a session on every reply. Sending it back is the
		// stateful habit the revision removed.
		if r.session != "" {
			t.Errorf("%s carried %s: %q, want no session header", r.method, sessionHeader, r.session)
		}

		if len(r.meta) == 0 {
			t.Errorf("%s carried no _meta in params", r.method)
			continue
		}
		var meta map[string]json.RawMessage
		if err := json.Unmarshal(r.meta, &meta); err != nil {
			t.Errorf("%s _meta is not an object: %s", r.method, r.meta)
			continue
		}
		var version string
		_ = json.Unmarshal(meta[metaKeyProtocolVersion], &version)
		if version != ModernProtocolVersion {
			t.Errorf("%s _meta.%s = %q, want %q", r.method, metaKeyProtocolVersion, version, ModernProtocolVersion)
		}
		var capabilities map[string]any
		if err := json.Unmarshal(meta[metaKeyClientCapabilities], &capabilities); err != nil || capabilities == nil {
			t.Errorf("%s _meta.%s = %s, want an object", r.method, metaKeyClientCapabilities, meta[metaKeyClientCapabilities])
		}
	}
}

func TestModernResultWithoutResultTypeIsAnError(t *testing.T) {
	c := negotiatedModern(t, &modernFake{omitResultType: true})
	ctx := context.Background()

	// An unread resultType is how an input_required result would be mistaken
	// for an answer with nothing in it.
	tools, err := c.ListTools(ctx)
	if err == nil {
		t.Errorf("ListTools returned %d tools and no error for a result with no resultType", len(tools))
	} else if !strings.Contains(err.Error(), "resultType") {
		t.Errorf("ListTools error = %q, want it to say the result carried no resultType", err)
	}

	res, err := c.CallTool(ctx, "read_file", map[string]any{"path": "go.mod"})
	if err == nil {
		t.Errorf("CallTool returned %q and no error for a result with no resultType", res.Text())
	} else if !strings.Contains(err.Error(), "resultType") {
		t.Errorf("CallTool error = %q, want it to say the result carried no resultType", err)
	}
}

func TestNegotiateFallsBackToInitialize(t *testing.T) {
	ctx := context.Background()

	check := func(t *testing.T, c *Client) {
		t.Helper()
		got, err := c.Negotiate(ctx)
		if err != nil {
			t.Fatalf("Negotiate against a server with only the handshake: %v", err)
		}
		if got.Path != PathInitialize {
			t.Errorf("Path = %q, want %q", got.Path, PathInitialize)
		}
		// What the old fake answers initialize with; nothing else it does
		// states either.
		if got.ProtocolVersion != "2099-01-01" || got.ServerInfo.Name != "fake-fs" {
			t.Errorf("negotiated %q with %q, want 2099-01-01 with fake-fs", got.ProtocolVersion, got.ServerInfo.Name)
		}
		// The old fake reads no _meta and answers with no resultType, so these
		// succeeding is the old path asking for neither.
		if tools, err := c.ListTools(ctx); err != nil || len(tools) != len(fakeTools) {
			t.Fatalf("ListTools = %d tools, %v", len(tools), err)
		}
		res, err := c.CallTool(ctx, "read_file", map[string]any{"path": "go.mod"})
		if err != nil || res.Text() != "contents of go.mod" {
			t.Fatalf("CallTool = %q, %v", res.Text(), err)
		}
	}

	t.Run("http", func(t *testing.T) {
		fake := &httpFake{}
		rec := &methodRecorder{next: fake}
		check(t, httpClient(t, rec))

		if !rec.saw(MethodInitialize) {
			t.Errorf("server never saw %s", MethodInitialize)
		}
		// The session opened at initialize is still echoed: tools/list and
		// tools/call are the last two requests the fake recorded.
		fake.mu.Lock()
		sessions := append([]string(nil), fake.sessions...)
		fake.mu.Unlock()
		if len(sessions) < 2 {
			t.Fatalf("sessions = %v, want at least tools/list and tools/call", sessions)
		}
		for _, sid := range sessions[len(sessions)-2:] {
			if sid != "session-1" {
				t.Errorf("sessions = %v, want session-1 on the requests after initialize", sessions)
				break
			}
		}
	})

	// The stdio fake is a subprocess and cannot be asked what it saw; that it
	// was sent initialize is read from the answer only initialize gives.
	t.Run("stdio", func(t *testing.T) {
		check(t, dial(t, stdioConfig()))
	})
}

func TestNegotiateNamesBothFailures(t *testing.T) {
	c := httpClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req Request
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.IsNotification() {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		resp := Response{JSONRPC: "2.0", ID: req.ID, Error: &Error{Code: CodeInternalError, Message: "no answer for " + req.Method}}
		switch req.Method {
		case MethodServerDiscover:
			resp.Error.Message = "discovery is on fire"
		case MethodInitialize:
			resp.Error.Message = "handshake is flooded"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))

	_, err := c.Negotiate(context.Background())
	if err == nil {
		t.Fatal("Negotiate succeeded against a server that refuses both paths")
	}
	// Either failure alone sends an operator to the wrong half of the problem.
	for _, want := range []string{MethodServerDiscover, "discovery is on fire", MethodInitialize, "handshake is flooded"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Negotiate error = %q, want it to mention %q", err, want)
		}
	}
}
