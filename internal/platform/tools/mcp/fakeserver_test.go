package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
)

// fakeServerEnv switches the test binary into a stdio MCP server. The stdio
// transport launches a real subprocess, so the honest fake is this binary
// re-executed with the variable set.
const fakeServerEnv = "KARAKURI_MCP_FAKE_SERVER"

func TestMain(m *testing.M) {
	if os.Getenv(fakeServerEnv) == "1" {
		serveStdio(os.Stdin, os.Stdout)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeTools is what the fake server advertises. delete_repo exists so there is
// something for an allowlist to refuse.
var fakeTools = []Tool{
	{Name: "read_file", Description: "Reads a file.", InputSchema: map[string]any{
		"type":       "object",
		"properties": map[string]any{"path": map[string]any{"type": "string", "description": "Which file."}},
		"required":   []any{"path"},
	}},
	{Name: "fail_tool", Description: "Always fails."},
	{Name: "delete_repo", Description: "Deletes the repository."},
	{Name: "hang", Description: "Never answers."},
}

// fakeHandle answers one request. ok is false for a notification, and for the
// one tool that never answers.
func fakeHandle(req Request) (Response, bool) {
	if req.IsNotification() {
		return Response{}, false
	}
	resp := Response{JSONRPC: "2.0", ID: req.ID}
	var result any
	switch req.Method {
	case MethodInitialize:
		result = InitializeResult{ProtocolVersion: "2099-01-01", ServerInfo: Info{Name: "fake-fs", Version: "9"}}
	case MethodToolsList:
		result = ListToolsResult{Tools: fakeTools}
	case MethodToolsCall:
		var p CallToolParams
		_ = json.Unmarshal(req.Params, &p)
		switch p.Name {
		case "read_file":
			result = TextResult(fmt.Sprintf("contents of %v", p.Arguments["path"]))
		case "fail_tool":
			result = ErrorResult("the tool broke")
		case "hang":
			return Response{}, false
		default:
			resp.Error = &Error{Code: CodeInvalidParams, Message: "no such tool " + p.Name}
		}
	default:
		resp.Error = &Error{Code: CodeMethodNotFound, Message: "unknown method " + req.Method}
	}
	if result != nil {
		resp.Result, _ = json.Marshal(result)
	}
	return resp, true
}

// serveStdio is the fake over newline-delimited JSON. It writes a banner and a
// notification ahead of each reply, because real servers do both and the
// transport has to skip them.
func serveStdio(in io.Reader, out io.Writer) {
	w := bufio.NewWriter(out)
	fmt.Fprintln(w, "fake-fs starting up")
	_ = w.Flush()

	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		resp, ok := fakeHandle(req)
		if !ok {
			continue
		}
		fmt.Fprintln(w, `{"jsonrpc":"2.0","method":"notifications/progress","params":{"progress":1}}`)
		line, _ := json.Marshal(resp)
		fmt.Fprintln(w, string(line))
		_ = w.Flush()
	}
}

// stdioConfig launches this test binary as the fake server.
func stdioConfig(allowed ...string) Config {
	return Config{
		Transport:    TransportStdio,
		Command:      os.Args[0],
		Env:          map[string]string{fakeServerEnv: "1"},
		AllowedTools: allowed,
	}
}

// httpFake is the fake over streamable HTTP. sse switches the reply from a JSON
// body to an event stream with a progress notification ahead of the result.
type httpFake struct {
	sse bool

	mu       sync.Mutex
	sessions []string // Mcp-Session-Id seen on each request after initialize
	auth     []string
}

func (f *httpFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	if req.Method != MethodInitialize {
		f.sessions = append(f.sessions, r.Header.Get(sessionHeader))
	}
	f.mu.Unlock()

	if req.Method == MethodInitialize {
		w.Header().Set(sessionHeader, "session-1")
	}
	resp, ok := fakeHandle(req)
	if !ok {
		if req.IsNotification() {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		<-r.Context().Done()
		return
	}
	body, _ := json.Marshal(resp)
	if !f.sse {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	var b strings.Builder
	b.WriteString("event: message\n")
	b.WriteString(`data: {"jsonrpc":"2.0","method":"notifications/progress","params":{"progress":1}}` + "\n\n")
	b.WriteString("event: message\n")
	b.WriteString("data: " + string(body) + "\n\n")
	_, _ = w.Write([]byte(b.String()))
}
