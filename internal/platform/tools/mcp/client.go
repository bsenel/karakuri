package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// DefaultTimeout bounds a single MCP call.
//
// Thirty seconds because the two things a call does are both bounded in
// practice: discovery lists tools a server already knows, and a tool call
// reaches a filesystem or an API. A server that needs longer than this to answer
// is one an operator should notice, and the failure is reported per instance
// rather than taken out of the loop's own budget.
const DefaultTimeout = 30 * time.Second

// ClientInfo is what this implementation calls itself to a server.
var ClientInfo = Info{Name: "karakuri", Version: "0.1"}

// Config declares one MCP server: which transport reaches it, and what this
// deployment will let it offer.
//
// One struct for both transports rather than two, because the config file has
// one shape per ADR 006 and the fields that do not apply are simply empty. A
// stdio instance with a URL is a config mistake, not a second type.
type Config struct {
	// Transport is TransportStdio or TransportHTTP.
	Transport string

	// stdio: the command to run, its arguments, its working directory, and the
	// variables added to this process's environment.
	Command string
	Args    []string
	WorkDir string
	Env     map[string]string

	// http: the endpoint and any headers it needs — an Authorization bearer,
	// most often.
	URL     string
	Headers map[string]string

	// AllowedTools is the allowlist. A tool a server advertises and this list
	// does not name is never registered, never reaches the planner's catalog,
	// and cannot be called.
	//
	// An empty list allows nothing. That is the safe reading and the deliberate
	// one: "empty means everything" is a default an operator discovers by
	// finding a tool in the audit log they never approved, and a server can add
	// a tool between one boot and the next.
	AllowedTools []string

	// Timeout bounds one call. Zero means DefaultTimeout.
	Timeout time.Duration
}

// Client is one connection to one MCP server.
//
// It speaks the three methods this deployment needs — initialize, tools/list,
// tools/call — over either transport. What it deliberately does not do is
// reconnect: an instance discovers at boot and reports what it found, and a
// client that silently re-established a session would make /health describe a
// server it is no longer talking to.
type Client struct {
	transport transport

	mu      sync.Mutex
	nextID  int64
	timeout time.Duration

	server   Info
	protocol string
}

// NewClient dials the server a Config names. The transport is established here,
// so a command that does not exist or a URL that cannot be parsed fails at
// construction rather than at the first call.
func NewClient(cfg Config) (*Client, error) {
	t, err := newTransport(cfg)
	if err != nil {
		return nil, err
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Client{transport: t, timeout: timeout}, nil
}

// Transport names the transport in use, for /health.
func (c *Client) Transport() string { return c.transport.Kind() }

// ServerInfo returns what the server called itself, and the protocol revision it
// said it speaks. Empty before Initialize.
func (c *Client) ServerInfo() (Info, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.server, c.protocol
}

// Initialize performs the MCP handshake: initialize, then the initialized
// notification the server waits for before it will answer anything else.
//
// The protocol version the server states is recorded rather than checked. MCP
// negotiation is the server declaring what it will speak, and refusing to talk
// to one a revision ahead would break a working deployment on somebody else's
// release schedule. /health shows what it said.
func (c *Client) Initialize(ctx context.Context) (InitializeResult, error) {
	params, err := json.Marshal(map[string]any{
		"protocolVersion": ProtocolVersion,
		// No capabilities are declared because none are implemented: this client
		// does not offer sampling, roots or elicitation, and claiming them would
		// invite requests nothing answers.
		"capabilities": map[string]any{},
		"clientInfo":   ClientInfo,
	})
	if err != nil {
		return InitializeResult{}, err
	}

	var out InitializeResult
	if err := c.call(ctx, MethodInitialize, params, &out); err != nil {
		return InitializeResult{}, err
	}

	c.mu.Lock()
	c.server = out.ServerInfo
	c.protocol = out.ProtocolVersion
	c.mu.Unlock()

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	if err := c.transport.Notify(ctx, Request{JSONRPC: "2.0", Method: MethodInitialized}); err != nil {
		return out, fmt.Errorf("initialized notification: %w", err)
	}
	return out, nil
}

// ListTools asks the server what it offers.
//
// One page. A server that paginates its tools would need a discovery loop, and
// NextCursor is accepted and ignored rather than half-handled — see the note on
// ListToolsResult.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	var out ListToolsResult
	if err := c.call(ctx, MethodToolsList, nil, &out); err != nil {
		return nil, err
	}
	return out.Tools, nil
}

// CallTool invokes one tool by its server-side name.
//
// The name is the server's, not the capability ID: the caller has already
// stripped the mcp.<instance>. prefix, because a server knows nothing about how
// this deployment namespaces what it discovered.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (ToolResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	params, err := json.Marshal(CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return ToolResult{}, err
	}
	var out ToolResult
	if err := c.call(ctx, MethodToolsCall, params, &out); err != nil {
		return ToolResult{}, err
	}
	return out, nil
}

// Close releases the subprocess or the connection.
func (c *Client) Close() error { return c.transport.Close() }

// call sends one request, checks the envelope, and decodes the result.
//
// Serialised on the client's mutex: the stdio transport is a single pipe in each
// direction, and two calls interleaved on it would return each other's replies.
func (c *Client) call(ctx context.Context, method string, params json.RawMessage, out any) error {
	c.mu.Lock()
	c.nextID++
	id := json.RawMessage(strconv.FormatInt(c.nextID, 10))
	c.mu.Unlock()

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	resp, err := c.transport.Send(ctx, Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	if resp.Error != nil {
		return fmt.Errorf("%s: server error %d: %s", method, resp.Error.Code, resp.Error.Message)
	}
	if out == nil || len(resp.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(resp.Result, out); err != nil {
		return fmt.Errorf("%s: decode result: %w", method, err)
	}
	return nil
}

// withTimeout applies the per-call bound, leaving a caller's shorter deadline
// alone.
func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < c.timeout {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.timeout)
}
