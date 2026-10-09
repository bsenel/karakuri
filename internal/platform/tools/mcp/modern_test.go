package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// modernRequest is one request as the modern-only fake saw it.
type modernRequest struct {
	method  string
	id      string          // the JSON-RPC id, as it was sent
	tool    string          // params.name, for a tools/call
	meta    json.RawMessage // params._meta, nil when the request carried none
	session string          // the Mcp-Session-Id request header

	// The three routing headers, as they arrived.
	versionHeader, methodHeader, nameHeader string
}

// modernFake implements only revision 2026-07-28, in the shapes the official
// Python SDK (mcp 2.3.0) has and refusing what its server refuses (sdkLadder):
// no handshake, server/discover instead, a resultType on every result. It
// offers a session id on every reply although the revision has none, so a
// client that still echoes sessions is caught doing it.
type modernFake struct {
	// omitResultType makes tools/list and tools/call results arrive without
	// the field. Discovery keeps it, so the connection still opens.
	omitResultType bool

	// sse answers with an event stream: a progress notification, then the
	// result.
	sse bool

	// breakMethod names the method whose reply is a broken stream: an event
	// stream that sends a progress notification and ends without the result.
	// breakTimes is how many times it does so before answering normally, and a
	// negative count never answers. breakAfter is how long the stream stays
	// open before it ends.
	breakMethod string
	breakTimes  int
	breakAfter  time.Duration

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
		Name string          `json:"name"`
	}
	_ = json.Unmarshal(req.Params, &params)
	f.mu.Lock()
	f.seen = append(f.seen, modernRequest{
		method: req.Method, id: string(req.ID), tool: params.Name,
		meta: params.Meta, session: r.Header.Get(sessionHeader),
		versionHeader: r.Header.Get(sdkHeaderProtocolVersion),
		methodHeader:  r.Header.Get(sdkHeaderMethod),
		nameHeader:    r.Header.Get(sdkHeaderName),
	})
	f.mu.Unlock()

	// Without the version header the SDK's server takes the request for a
	// 2025-06-18 one, and one that is not initialize has no session to belong
	// to. Observed against mcp 2.3.0 on 2026-10-09.
	if r.Header.Get(sdkHeaderProtocolVersion) == "" && req.Method != MethodInitialize && !req.IsNotification() {
		body, _ := json.Marshal(Response{JSONRPC: "2.0", ID: req.ID, Error: &Error{Code: CodeInvalidRequest, Message: "Bad Request: Missing session ID"}})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(body)
		return
	}
	// A request the ladder refuses is refused before anything else happens to
	// it, a broken stream included.
	if req.Method != MethodInitialize && !req.IsNotification() {
		if refused := sdkLadder(req, r.Header); refused != nil {
			body, _ := json.Marshal(Response{JSONRPC: "2.0", ID: req.ID, Error: refused})
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(body)
			return
		}
	}

	f.mu.Lock()
	breaks := f.breakMethod != "" && req.Method == f.breakMethod && f.breakTimes != 0
	if breaks && f.breakTimes > 0 {
		f.breakTimes--
	}
	breakAfter := f.breakAfter
	f.mu.Unlock()

	w.Header().Set(sessionHeader, "bait")
	if req.IsNotification() {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if breaks {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(progressEvent))
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		select {
		case <-time.After(breakAfter):
		case <-r.Context().Done():
		}
		return
	}

	resp, ok := modernHandle(req, r.Header, f.omitResultType)
	if !ok {
		<-r.Context().Done()
		return
	}
	body, _ := json.Marshal(resp)
	if resp.Error != nil && resp.Error.Code != CodeMethodNotFound && req.Method != MethodInitialize {
		// The SDK answers every rung of its ladder with a 400.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(body)
		return
	}
	if f.sse {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(progressEvent + "event: message\ndata: " + string(body) + "\n\n"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// progressEvent is what a server sends on a stream ahead of its result.
const progressEvent = "event: message\n" +
	`data: {"jsonrpc":"2.0","method":"notifications/progress","params":{"progress":1}}` + "\n\n"

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
		_ = json.Unmarshal(meta[sdkMetaProtocolVersion], &version)
		if version != ModernProtocolVersion {
			t.Errorf("%s _meta[%s] = %q, want %q", r.method, sdkMetaProtocolVersion, version, ModernProtocolVersion)
		}
		var capabilities map[string]any
		if err := json.Unmarshal(meta[sdkMetaClientCapabilities], &capabilities); err != nil || capabilities == nil {
			t.Errorf("%s _meta[%s] = %s, want an object", r.method, sdkMetaClientCapabilities, meta[sdkMetaClientCapabilities])
		}
		// The server's ladder lets a request through without clientInfo; the
		// SDK's own client sends it on every request, and so must this one.
		var client Info
		_ = json.Unmarshal(meta[sdkMetaClientInfo], &client)
		if client.Name != ClientInfo.Name {
			t.Errorf("%s _meta[%s] = %s, want name %q", r.method, sdkMetaClientInfo, meta[sdkMetaClientInfo], ClientInfo.Name)
		}
		// The assumed names are not sent alongside the real ones.
		for _, bare := range []string{"protocolVersion", "clientCapabilities", "clientInfo"} {
			if _, ok := meta[bare]; ok {
				t.Errorf("%s _meta carries the bare key %q", r.method, bare)
			}
		}

		if r.versionHeader != ModernProtocolVersion {
			t.Errorf("%s %s = %q, want %q", r.method, sdkHeaderProtocolVersion, r.versionHeader, ModernProtocolVersion)
		}
		if r.methodHeader != r.method {
			t.Errorf("%s %s = %q, want the method", r.method, sdkHeaderMethod, r.methodHeader)
		}
		wantName := ""
		if r.method == MethodToolsCall {
			wantName = "read_file"
		}
		if r.nameHeader != wantName {
			t.Errorf("%s %s = %q, want %q", r.method, sdkHeaderName, r.nameHeader, wantName)
		}
	}
}

// A tool name that would not survive as a header value travels wrapped, the way
// the SDK's encode_header_value wraps it; the fake unwraps it and refuses a
// mismatch, so reaching the tool lookup is the header having been right.
func TestModernNameHeaderIsEncodedWhenItMustBe(t *testing.T) {
	fake := &modernFake{}
	c := negotiatedModern(t, fake)
	const name = "läs_fil"
	_, err := c.CallTool(context.Background(), name, nil)
	if err == nil || !strings.Contains(err.Error(), "no such tool") {
		t.Errorf("CallTool error = %v, want the server's own \"no such tool\"", err)
	}
	want := "=?base64?" + base64.StdEncoding.EncodeToString([]byte(name)) + "?="
	for _, r := range fake.requests() {
		if r.method == MethodToolsCall && r.nameHeader != want {
			t.Errorf("%s = %q, want %q", sdkHeaderName, r.nameHeader, want)
		}
	}
}

// stdio has no headers: the envelope alone carries the version, and a
// modern-only server over stdio is reached with the same three keys.
func TestModernStdioCarriesTheEnvelopeAndNoHeaders(t *testing.T) {
	cfg := stdioConfig()
	cfg.Env[fakeModernEnv] = "1"
	c := dial(t, cfg)
	ctx := context.Background()

	got, err := c.Negotiate(ctx)
	if err != nil {
		t.Fatalf("Negotiate against a 2026-07-28-only stdio server: %v", err)
	}
	if got.Path != PathDiscover || got.ProtocolVersion != ModernProtocolVersion || got.ServerInfo.Name != "modern-fs" {
		t.Errorf("negotiated %+v, want %s at %s with modern-fs", got, PathDiscover, ModernProtocolVersion)
	}
	if tools, err := c.ListTools(ctx); err != nil || len(tools) != len(fakeTools) {
		t.Fatalf("ListTools = %d tools, %v", len(tools), err)
	}
	// The stdio fake answers read_file only when the request's _meta carried
	// clientInfo, which it cannot be asked about afterwards.
	res, err := c.CallTool(ctx, "read_file", map[string]any{"path": "go.mod"})
	if err != nil || res.Text() != "contents of go.mod" {
		t.Fatalf("CallTool = %q, %v", res.Text(), err)
	}
}

// The fake refuses what the SDK's server refuses, with the SDK's codes. This is
// the fake held to the observations of 2026-10-09, not the client.
func TestModernFakeRefusesLikeTheSDK(t *testing.T) {
	srv := httptest.NewServer(&modernFake{})
	t.Cleanup(srv.Close)

	good := map[string]any{sdkMetaProtocolVersion: ModernProtocolVersion, sdkMetaClientCapabilities: map[string]any{}}
	cases := []struct {
		name    string
		method  string
		params  map[string]any
		headers map[string]string
		code    int
		message string
	}{
		{"no version header", MethodServerDiscover, map[string]any{"_meta": good}, nil, CodeInvalidRequest, "Missing session ID"},
		{"bare envelope keys", MethodServerDiscover,
			map[string]any{"_meta": map[string]any{"protocolVersion": ModernProtocolVersion, "clientCapabilities": map[string]any{}}},
			map[string]string{sdkHeaderProtocolVersion: ModernProtocolVersion, sdkHeaderMethod: MethodServerDiscover},
			CodeInvalidParams, sdkMetaProtocolVersion + ", " + sdkMetaClientCapabilities},
		{"method header disagrees", MethodToolsList, map[string]any{"_meta": good},
			map[string]string{sdkHeaderProtocolVersion: ModernProtocolVersion, sdkHeaderMethod: MethodServerDiscover},
			sdkCodeHeaderMismatch, "mcp-method"},
		{"name header disagrees", MethodToolsCall, map[string]any{"_meta": good, "name": "read_file"},
			map[string]string{sdkHeaderProtocolVersion: ModernProtocolVersion, sdkHeaderMethod: MethodToolsCall, sdkHeaderName: "delete_repo"},
			sdkCodeHeaderMismatch, "mcp-name"},
		{"version header disagrees", MethodServerDiscover, map[string]any{"_meta": good},
			map[string]string{sdkHeaderProtocolVersion: "2025-06-18", sdkHeaderMethod: MethodServerDiscover},
			sdkCodeHeaderMismatch, "mcp-protocol-version"},
		{"unsupported version", MethodServerDiscover,
			map[string]any{"_meta": map[string]any{sdkMetaProtocolVersion: "2031-01-01", sdkMetaClientCapabilities: map[string]any{}}},
			map[string]string{sdkHeaderProtocolVersion: "2031-01-01", sdkHeaderMethod: MethodServerDiscover},
			CodeUnsupportedProtocol, "Unsupported protocol version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params, _ := json.Marshal(tc.params)
			body, _ := json.Marshal(Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: tc.method, Params: params})
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL, bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			res, err := srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = res.Body.Close() }()
			var out Response
			if err := json.NewDecoder(res.Body).Decode(&out); err != nil || out.Error == nil {
				t.Fatalf("status %d, decode %v, error %v: want a JSON-RPC error", res.StatusCode, err, out.Error)
			}
			if res.StatusCode != http.StatusBadRequest || out.Error.Code != tc.code || !strings.Contains(out.Error.Message, tc.message) {
				t.Errorf("got %d %d %q, want 400 %d mentioning %q", res.StatusCode, out.Error.Code, out.Error.Message, tc.code, tc.message)
			}
			if tc.code == CodeUnsupportedProtocol {
				data, _ := json.Marshal(out.Error.Data)
				if string(data) != `{"requested":"2031-01-01","supported":["2026-07-28"]}` {
					t.Errorf("error data = %s, want the supported list and what was requested", data)
				}
			}
		})
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
