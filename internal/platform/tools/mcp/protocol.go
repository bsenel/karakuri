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
// with, carrying the versions it does speak in the error's data
// (UNSUPPORTED_PROTOCOL_VERSION in mcp_types/jsonrpc.py).
const CodeUnsupportedProtocol = -32022

// The names below were read from the official Python SDK, mcp 2.3.0, on
// 2026-10-09, and each says where. The specification text was not read: where
// the SDK and the specification page could differ, the name here is the SDK's.
const (
	// PROTOCOL_VERSION_META_KEY, CLIENT_CAPABILITIES_META_KEY and
	// CLIENT_INFO_META_KEY in mcp_types/_types.py. classify_inbound_request in
	// mcp/shared/inbound.py requires the first two on every request and reads
	// the third as optional (SHOULD-include).
	metaKeyProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	metaKeyClientCapabilities = "io.modelcontextprotocol/clientCapabilities"
	metaKeyClientInfo         = "io.modelcontextprotocol/clientInfo"

	// SERVER_INFO_META_KEY in mcp_types/_types.py: where a result names the
	// server that produced it. A server/discover result has no top-level
	// serverInfo.
	metaKeyServerInfo = "io.modelcontextprotocol/serverInfo"

	// MCP_PROTOCOL_VERSION_HEADER, MCP_METHOD_HEADER and MCP_NAME_HEADER in
	// mcp/shared/inbound.py, which declares them lowercase; HTTP field names
	// are case-insensitive. Streamable HTTP only: stdio has no headers.
	headerProtocolVersion = "MCP-Protocol-Version"
	headerMethod          = "Mcp-Method"
	headerName            = "Mcp-Name"

	// "complete" and "input_required" (ResultTypeInputRequired above) are the
	// resultType values in mcp_types/_v2026_07_28/__init__.py;
	// is_input_required in mcp_types/methods.py compares against the second.
	resultTypeComplete = "complete"

	// InputRequiredResult.input_requests, alias inputRequests, in
	// mcp_types/_v2026_07_28/__init__.py: a map from the server's own key to a
	// request (CreateMessageRequest, ListRootsRequest or ElicitRequest), each
	// with `method` and `params`. Its sibling requestState and the client's
	// inputResponses are not modelled: this client answers no requests for
	// input (ADR 027).
	inputRequestsField = "inputRequests"
)

// nameBearingMethods is NAME_BEARING_METHODS in mcp/shared/inbound.py: the
// methods whose request repeats one of its params in the Mcp-Name header, and
// which param. Only tools/call is sent by this client; the other two are here
// so the table is the SDK's and not a subset that looks complete.
var nameBearingMethods = map[string]string{
	MethodToolsCall:  "name",
	"prompts/get":    "name",
	"resources/read": "uri",
}

// discoverResult is what a server returns from server/discover: DiscoverResult
// in mcp_types/_v2026_07_28/__init__.py. The server names itself under
// `_meta`, not at the top level.
type discoverResult struct {
	ResultType        string          `json:"resultType"`
	SupportedVersions []string        `json:"supportedVersions"`
	Capabilities      map[string]any  `json:"capabilities"`
	Instructions      string          `json:"instructions,omitempty"`
	Meta              map[string]Info `json:"_meta,omitempty"`

	// The SDK's model requires both and this client caches nothing, so they are
	// read tolerantly: absent is fine, and neither is acted on.
	CacheScope string `json:"cacheScope"`
	TTLMs      int64  `json:"ttlMs"`
}

// Discovery builds the server/discover result for a server that speaks
// versions and describes itself as server does in the handshake. It lives here
// rather than in the handler so the server answers in the shape the client
// decodes, from one declaration of it.
func Discovery(server InitializeResult, versions []string) any {
	capabilities := server.Capabilities
	if capabilities == nil {
		capabilities = map[string]any{}
	}
	return discoverResult{
		ResultType:        resultTypeComplete,
		SupportedVersions: versions,
		Capabilities:      capabilities,
		Instructions:      server.Instructions,
		Meta:              map[string]Info{metaKeyServerInfo: server.ServerInfo},
		// "private" and zero: not to be shared across callers and not to be
		// reused, which is what a server that caches nothing should say.
		CacheScope: "private",
	}
}

// RequestVersion reads the protocol version a request states in its params'
// `_meta`, the way withMeta writes it. Empty means the request states none,
// which is a 2025-06-18 client: that revision settles the version once in the
// handshake. A version that is there and is not a string comes back as its raw
// JSON, so a server comparing it to what it speaks refuses it rather than
// mistaking it for absent.
func RequestVersion(params json.RawMessage) string {
	var body struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	if len(params) == 0 || json.Unmarshal(params, &body) != nil {
		return ""
	}
	raw, ok := body.Meta[metaKeyProtocolVersion]
	if !ok {
		return ""
	}
	var version string
	if err := json.Unmarshal(raw, &version); err != nil {
		return string(raw)
	}
	return version
}

// CompleteResult marks a result as a finished answer, which revision 2026-07-28
// requires of every result and checkResultType refuses to go without. The
// result's own fields are carried through as they were encoded.
func CompleteResult(result json.RawMessage) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(result, &fields); err != nil {
		return nil, err
	}
	fields["resultType"], _ = json.Marshal(resultTypeComplete)
	return json.Marshal(fields)
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
