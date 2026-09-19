package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/bsenel/karakuri/internal/core/capability"
)

// SlotName is the ADR 006 slot these instances fill, and the key a twin's
// AdapterBindings uses to name one: `krk twin bindings <id> --set mcp=acme_files`.
const SlotName = "mcp"

// Connection states reported in /health.
const (
	// StateConnected means the handshake succeeded and tools/list answered.
	StateConnected = "connected"
	// StateUnreachable means it did not. The instance still exists, still
	// appears in /health, and registers no tools — see the note on Discover.
	StateUnreachable = "unreachable"
	// StateMisconfigured means the instance could not even be dialled: no
	// command, no URL, an unknown transport.
	StateMisconfigured = "misconfigured"
)

// Instance is one configured MCP server: a client, an allowlist, and the result
// of asking it once what it offers.
//
// Discovery happens when the instance is built, and the outcome is fixed from
// then on. That is the routing decision in ADR 022: environment.Factory.Serves
// is an exact list of capability IDs reverse-indexed once at Register, so the
// list has to be knowable before Register is called. An instance whose server was
// down at boot serves nothing and says so, rather than registering a promise the
// registry would route actions to.
type Instance struct {
	name   string
	cfg    Config
	client *Client

	// allow is the allowlist as a set, for the two places that check it:
	// discovery, which drops what is not on it, and Call, which refuses a
	// capability ID built before an operator narrowed the list.
	allow map[string]bool

	mu sync.RWMutex

	state    string
	failure  string
	server   Info
	protocol string

	// tools are the ones both discovered and allowed, sorted by name so
	// /health, the planner's catalog and Serves all read the same across boots.
	tools []Tool

	// filtered names what the server advertised and the allowlist refused. It is
	// reported rather than dropped: "the server has a tool called delete_repo
	// and this deployment does not allow it" is the sentence an operator wants
	// from /health, and silence reads like the tool does not exist.
	filtered []string
}

// InstanceHealth is the /health-shaped view of one instance: which transport,
// what state the connection is in, and the tools discovered and allowed.
//
// The same three things every Phase 6 slot reports, plus the two MCP adds: a
// server names itself, and an allowlist has a visible other side.
type InstanceHealth struct {
	Name            string   `json:"name"`
	Transport       string   `json:"transport"`
	State           string   `json:"state"`
	IsDefault       bool     `json:"is_default"`
	Server          string   `json:"server,omitempty"`
	ProtocolVersion string   `json:"protocol_version,omitempty"`
	Tools           []string `json:"tools"`
	Filtered        []string `json:"filtered,omitempty"`
	Error           string   `json:"error,omitempty"`
}

// NewInstance builds an instance and completes discovery against its server.
//
// It never returns an error, and that is the point. Every failure — a command
// that is not installed, a URL nothing listens on, a handshake that times out —
// leaves an instance in StateUnreachable with zero tools, which is a thing
// /health can describe and routing can ignore. Returning an error would make one
// unreachable server either fail the boot or vanish from the topology, and an
// operator debugging "why is my filesystem server not there" would have nothing
// to read.
func NewInstance(ctx context.Context, name string, cfg Config) *Instance {
	inst := &Instance{name: name, cfg: cfg, allow: allowSet(cfg.AllowedTools)}

	client, err := NewClient(cfg)
	if err != nil {
		inst.state = StateMisconfigured
		inst.failure = err.Error()
		return inst
	}
	inst.client = client
	inst.discover(ctx)
	return inst
}

// discover runs the handshake and the one tools/list, and records what came back.
func (i *Instance) discover(ctx context.Context) {
	init, err := i.client.Initialize(ctx)
	if err != nil {
		i.fail(fmt.Errorf("initialize: %w", err))
		return
	}

	tools, err := i.client.ListTools(ctx)
	if err != nil {
		i.fail(fmt.Errorf("tools/list: %w", err))
		return
	}

	var allowed []Tool
	var filtered []string
	for _, t := range tools {
		if t.Name == "" {
			continue
		}
		if i.allow[t.Name] {
			allowed = append(allowed, t)
			continue
		}
		filtered = append(filtered, t.Name)
	}
	sort.Slice(allowed, func(a, b int) bool { return allowed[a].Name < allowed[b].Name })
	sort.Strings(filtered)

	i.mu.Lock()
	defer i.mu.Unlock()
	i.state = StateConnected
	i.server = init.ServerInfo
	i.protocol = init.ProtocolVersion
	i.tools = allowed
	i.filtered = filtered
}

// fail records why this instance has nothing to offer and closes the transport,
// so a half-open subprocess is not left behind for the life of the process.
func (i *Instance) fail(err error) {
	i.mu.Lock()
	i.state = StateUnreachable
	i.failure = err.Error()
	i.tools = nil
	i.mu.Unlock()
	if i.client != nil {
		_ = i.client.Close()
	}
}

// Name is the instance name from config — the value a twin binding names.
func (i *Instance) Name() string { return i.name }

// Transport reports stdio or http, or "" for an instance that could not be
// dialled at all.
func (i *Instance) Transport() string {
	if i.client == nil {
		return i.cfg.Transport
	}
	return i.client.Transport()
}

// Active reports whether this instance can execute anything, which is the same
// question every other slot's adapter answers for /health: connected, and with
// at least one tool the allowlist permits.
func (i *Instance) Active() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.state == StateConnected && len(i.tools) > 0
}

// State returns the connection state.
func (i *Instance) State() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.state
}

// Tools returns the discovered and allowed tools, sorted by name.
func (i *Instance) Tools() []Tool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return append([]Tool(nil), i.tools...)
}

// Capabilities renders the discovered tools as registry entries.
//
// The description is the server's own text, carried through unedited: it is what
// the planner reads to decide whether to use the tool, and rewriting it here
// would be this repository putting words in a third party's mouth. It arrives in
// the prompt marked as somebody else's writing instead (ADR 022, Decision 3).
func (i *Instance) Capabilities() []capability.Capability {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]capability.Capability, 0, len(i.tools))
	for _, t := range i.tools {
		out = append(out, capability.NewMCPCapability(i.name, t.Name, t.Description, schemaOf(t)))
	}
	return out
}

// CapabilityIDs is Capabilities reduced to what Factory.Serves needs: the exact
// list, in a stable order, known before Register is called.
func (i *Instance) CapabilityIDs() []capability.CapabilityID {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]capability.CapabilityID, 0, len(i.tools))
	for _, t := range i.tools {
		out = append(out, capability.MCPCapabilityID(i.name, t.Name))
	}
	return out
}

// Health renders the per-instance topology /health reports.
func (i *Instance) Health(isDefault bool) InstanceHealth {
	i.mu.RLock()
	defer i.mu.RUnlock()

	names := make([]string, 0, len(i.tools))
	for _, t := range i.tools {
		names = append(names, t.Name)
	}
	return InstanceHealth{
		Name:            i.name,
		Transport:       i.Transport(),
		State:           i.state,
		IsDefault:       isDefault,
		Server:          i.server.Name,
		ProtocolVersion: i.protocol,
		Tools:           names,
		Filtered:        append([]string(nil), i.filtered...),
		Error:           i.failure,
	}
}

// Call invokes one discovered tool.
//
// The allowlist is checked here as well as at discovery. The two checks answer
// different questions: discovery decides what is registered, and this decides
// what may run — a capability ID can outlive the list that admitted it, in a
// stored plan or a checkpoint an operator approves after the config narrowed.
func (i *Instance) Call(ctx context.Context, tool string, args map[string]any) (ToolResult, error) {
	i.mu.RLock()
	state, client := i.state, i.client
	i.mu.RUnlock()

	if state != StateConnected || client == nil {
		return ToolResult{}, fmt.Errorf("MCP instance %q is %s", i.name, state)
	}
	if !i.allow[tool] {
		return ToolResult{}, fmt.Errorf("tool %q is not on instance %q's allowlist", tool, i.name)
	}
	return client.CallTool(ctx, tool, args)
}

// Close releases the server. Called for every instance at shutdown, including
// the ones that never connected.
func (i *Instance) Close() error {
	if i.client == nil {
		return nil
	}
	return i.client.Close()
}

// allowSet turns the configured allowlist into a set, dropping blanks and
// trimming the whitespace a YAML list picks up.
func allowSet(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			out[n] = true
		}
	}
	return out
}

// schemaOf narrows a server's JSON Schema to the fields capability.Schema
// models, for the one reader that needs them: the planner, deciding what
// arguments to pass.
//
// It is lossy by design and the loss is bounded to the prompt. Nothing validates
// an MCP call against this schema — the server does that, and it is the only
// party that has the whole of it — so a constraint dropped here costs a less
// well-informed planner and never a call that should have been refused.
func schemaOf(t Tool) capability.Schema {
	out := capability.Schema{Type: "object"}
	if t.InputSchema == nil {
		return out
	}
	if s, ok := t.InputSchema["type"].(string); ok && s != "" {
		out.Type = s
	}
	props, ok := t.InputSchema["properties"].(map[string]any)
	if ok && len(props) > 0 {
		out.Properties = make(map[string]capability.SchemaProperty, len(props))
		for name, raw := range props {
			spec, _ := raw.(map[string]any)
			prop := capability.SchemaProperty{}
			if v, ok := spec["type"].(string); ok {
				prop.Type = v
			}
			if v, ok := spec["description"].(string); ok {
				prop.Description = v
			}
			for _, e := range asAnySlice(spec["enum"]) {
				if s, ok := e.(string); ok {
					prop.Enum = append(prop.Enum, s)
				}
			}
			out.Properties[name] = prop
		}
	}
	for _, r := range asAnySlice(t.InputSchema["required"]) {
		if s, ok := r.(string); ok {
			out.Required = append(out.Required, s)
		}
	}
	sort.Strings(out.Required)
	return out
}

func asAnySlice(v any) []any {
	out, _ := v.([]any)
	return out
}
