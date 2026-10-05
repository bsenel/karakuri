// Package mcp speaks the Model Context Protocol, in both directions.
//
// Inbound, Client connects to a server somebody else runs — over stdio or
// streamable HTTP — lists its tools once, and calls them. It is the eleventh
// tool slot (ADR 006): named instances, a default, twin-bound through
// DigitalTwin.AdapterBindings, and a per-instance allowlist.
//
// Outbound, the wire types below are what internal/api/handler serves to a
// foreign runtime driving this deployment. The two directions share one set of
// types deliberately: a second copy of the envelope is a second thing to keep
// in step with a protocol neither end owns.
//
// Everything a server hands back is somebody else's writing — tool
// descriptions as much as tool results — so the environment in this package
// marks both with environment.TrustThirdParty. See ADR 022.
package mcp

import "encoding/json"

// ProtocolVersion is the MCP revision this implementation speaks.
//
// A server answering with a different one is not rejected: version negotiation
// in MCP is "the server states what it will speak", and refusing to talk to a
// server one revision ahead would make a working deployment fail on somebody
// else's release schedule. What is recorded is what it said, so /health shows
// the truth rather than this constant.
const ProtocolVersion = "2025-06-18"

// ModernProtocolVersion is the stateless revision: no handshake, no session,
// the version and the client's capabilities on every request instead. It is a
// second constant rather than a new value for the first because the two are
// different conversations, and the older one stays as the fallback.
const ModernProtocolVersion = "2026-07-28"

// MethodServerDiscover is what a 2026-07-28 server answers in place of the
// handshake.
const MethodServerDiscover = "server/discover"

// ResultTypeInputRequired is the resultType of a result that is a question
// rather than an answer: the server wants something from the client before it
// will finish.
const ResultTypeInputRequired = "input_required"

// CodeUnsupportedProtocol is what a 2026-07-28-only server answers `initialize`
// with, carrying the versions it does speak in the error's data. Observed
// against the Python SDK in Phase 33 step 1.
const CodeUnsupportedProtocol = -32022

// The names below were taken from the roadmap's description of revision
// 2026-07-28 ("every request carries its protocol version and client
// capabilities in `_meta`", "a required `resultType` on every result") and were
// NOT checked against the specification text. The roadmap states that the
// version and the capabilities travel in `_meta`; it does not state the keys
// they travel under, the resultType of an ordinary result, or the shape of a
// server/discover result. They are declared once here so that correcting one is
// a one-line change that the tests follow.
const (
	metaKeyProtocolVersion    = "protocolVersion"
	metaKeyClientCapabilities = "clientCapabilities"

	resultTypeComplete = "complete"

	// inputRequestsField is where an input_required result carries what the
	// server is asking for. The discovery pass names the client's half
	// (inputResponses) and not this one: it was not checked against the
	// specification text.
	inputRequestsField = "inputRequests"
)

// discoverResult is what a server returns from server/discover. Its field names
// carry the same caveat as the constants above: assumed, not read from the
// specification.
type discoverResult struct {
	ResultType        string   `json:"resultType"`
	SupportedVersions []string `json:"supportedVersions"`
	ServerInfo        Info     `json:"serverInfo"`
}

// Method names. Constants because they are matched in two places — the client
// sends them, the server dispatches on them — and a typo in either is a
// silently unreachable method.
const (
	MethodInitialize  = "initialize"
	MethodInitialized = "notifications/initialized"
	MethodToolsList   = "tools/list"
	MethodToolsCall   = "tools/call"
	MethodPing        = "ping"
)

// JSON-RPC 2.0 error codes used by both directions, plus the two MCP adds.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Request is one JSON-RPC 2.0 request or notification. ID is absent on a
// notification, which is the only structural difference between the two.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsNotification reports whether this message expects no reply. A server must
// not answer one, which is what stops an `initialized` notification from
// sitting in the response stream where the next result belongs.
func (r Request) IsNotification() bool { return len(r.ID) == 0 }

// Response is one JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error is a JSON-RPC error object.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *Error) Error() string { return e.Message }

// Tool is one tool as advertised by a server: what tools/list returns and what
// tools/call names.
//
// InputSchema is carried as a raw map rather than decoded into a typed schema
// because it is JSON Schema written by somebody else, and narrowing it to the
// fields this repository happens to model would quietly drop constraints a
// caller needs to satisfy.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// ToolResult is what tools/call returns. IsError is the protocol's way of
// reporting a tool that ran and failed, as distinct from a transport or
// dispatch failure, which is a JSON-RPC Error.
type ToolResult struct {
	Content []Content `json:"content,omitempty"`
	IsError bool      `json:"isError,omitempty"`
}

// Content is one block of a tool result. Only text is modelled: an image or an
// audio blob has no route into a planner prompt, and pretending to carry one
// would be a field nothing reads.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Text flattens the content blocks into the string that reaches the loop.
func (r ToolResult) Text() string {
	switch len(r.Content) {
	case 0:
		return ""
	case 1:
		return r.Content[0].Text
	}
	out := r.Content[0].Text
	for _, c := range r.Content[1:] {
		out += "\n" + c.Text
	}
	return out
}

// TextResult builds a successful single-block result.
func TextResult(text string) ToolResult {
	return ToolResult{Content: []Content{{Type: "text", Text: text}}}
}

// ErrorResult builds a result a tool failed to produce. It is a result rather
// than a JSON-RPC error because the caller asked a valid question and got a
// bad answer — the distinction matters to a client deciding whether to retry.
func ErrorResult(text string) ToolResult {
	return ToolResult{Content: []Content{{Type: "text", Text: text}}, IsError: true}
}

// InitializeResult is what a server returns from initialize.
type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
	ServerInfo      Info           `json:"serverInfo"`
	Instructions    string         `json:"instructions,omitempty"`
}

// Info names one end of the connection.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ListToolsResult is what a server returns from tools/list. The cursor is
// accepted and ignored: this client lists once at boot, and a server paginating
// its tools would need a discovery loop, which is deferred rather than
// pretended at.
type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// CallToolParams is what tools/call takes.
type CallToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}
