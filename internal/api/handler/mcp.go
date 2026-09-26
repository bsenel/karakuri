package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	coretelemetry "github.com/bsenel/karakuri/internal/core/telemetry"
	featureobjective "github.com/bsenel/karakuri/internal/feature/objective"
	featurereconcile "github.com/bsenel/karakuri/internal/feature/reconcile"
	featurereport "github.com/bsenel/karakuri/internal/feature/report"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools/mcp"
)

// MCPHandler serves Karakuri itself as an MCP server over streamable HTTP, so a
// foreign runtime can read this deployment's work without a client written
// against the REST API.
//
// It is one route because the protocol dispatches inside the body rather than
// on the path, and it sits inside the authenticated group like every other
// /api/v1 route: the bearer token an MCP client sends is the same token `krk`
// sends, resolved by the same resolver. There is no MCP credential.
//
// Nor is there an MCP permission model. Every tool below declares the action the
// REST route answering the same question declares, against a resource reference
// built the same way — so a principal sees exactly what they would see through
// the API, and a scope set narrowed for one is narrowed for both. The tools are
// read-only for the same reason the phase is: proposing is a thing a runtime
// may do on its own, and approving is not (ADR 022).
type MCPHandler struct {
	Objectives *featureobjective.Service
	Reports    *featurereport.Service
	Reconcile  *featurereconcile.Service

	// Telemetry is the deployment's read-only view of itself. Nil on a
	// deployment that wired none, which the tool reports rather than hiding.
	Telemetry coretelemetry.Reader

	// Enforcer is used for its Authorizer and its OnDeny hook rather than as
	// middleware: the subject of an MCP call arrives inside a JSON-RPC body,
	// which no route-shaped check can see. The hook is what keeps a refusal
	// here in the same audit log as a refusal on a REST route.
	Enforcer *auth.Enforcer

	// Scopes narrows a listing to what the caller may read, exactly as
	// ObjectiveHandler.List does.
	Scopes karakuriauth.ScopeAuthorizer

	// Containers attaches the containers a named resource belongs to, so an
	// org-scoped binding reaches an objective inside it.
	Containers karakuriauth.ScopeLookup
}

// mcpServerInfo is what this deployment calls itself to a client. Deliberately
// not the deployment's own name: a client aggregating several servers names
// them itself, and the useful thing to state here is the implementation.
var mcpServerInfo = mcp.Info{Name: "karakuri", Version: "0.1"}

// mcpInstructions is the one place a foreign runtime is told what this server
// is and what it is not. A client puts it in front of its model, so it says the
// boundary out loud rather than leaving it to be discovered by a refusal.
const mcpInstructions = `Karakuri exposes its objectives, digests, reconcile state and self-telemetry here.

Every tool is read-only. Nothing offered here starts work, changes an
objective, or resolves a checkpoint: approving a decision Karakuri escalated is
a judgement about who may approve, and it stays with a person.

What you can see is bounded by the access token you presented — the same
permissions and the same tenancy that bound it in the REST API bind it here.`

// codeForbidden is this server's JSON-RPC code for "you may not". JSON-RPC
// reserves -32000..-32099 for implementation-defined errors, and none of the
// standard codes says this: a refused call is neither a bad request nor an
// internal failure, and reporting it as either would send a client retrying.
const codeForbidden = -32003

// maxMCPBody caps the JSON-RPC message this server will read. The global 1 MiB
// body ceiling already applies; this is the same bound stated locally so the
// reader is not relying on middleware registered three files away.
const maxMCPBody = 1 << 20

// mcpWithheld names tools a client will reasonably look for and will
// deliberately not find, with the reason.
//
// An unknown tool gets "unknown tool" and the list of what exists, which is the
// right answer for a typo. These are not typos: a runtime driving an objective
// will eventually try to clear the checkpoint blocking it, and "unknown tool"
// would read as an oversight to be worked around rather than a decision to be
// respected.
var mcpWithheld = map[string]string{
	"checkpoint_resolve": "resolving a checkpoint is a decision about who may approve, and it stays with a person",
	"checkpoint_approve": "approving a checkpoint is a decision about who may approve, and it stays with a person",
	"checkpoint_reject":  "rejecting a checkpoint is a decision about who may approve, and it stays with a person",
}

// mcpTool is one exposed tool: what a client sees, the permission it demands,
// the resource that permission is evaluated against, and what it reads.
type mcpTool struct {
	def    mcp.Tool
	action auth.Action

	// resource builds the reference the decision is made against. It is a
	// function of the arguments because an MCP call names its subject in the
	// body — the equivalent of a URL parameter on a REST route.
	resource func(ctx context.Context, h *MCPHandler, p auth.Principal, args mcpArgs) auth.ResourceRef

	// read runs the tool once the permission has been granted. It returns the
	// value to render as JSON; an error here is a bad answer to a fair
	// question (an objective that does not exist, a server that is not
	// standing) and comes back as a tool result, not a protocol error.
	read func(ctx context.Context, h *MCPHandler, p auth.Principal, args mcpArgs) (any, error)
}

// tools is the exposed surface, in the order a client sees it.
//
// A method rather than a package-level table because each entry closes over the
// services on the handler, and because the table is the readable form of the
// mapping onto the auth catalog — the same role internal/auth.Routes() plays for
// the REST surface.
func (h *MCPHandler) tools() []mcpTool {
	return []mcpTool{
		{
			def: mcp.Tool{
				Name:        "objectives_list",
				Description: "List the objectives this deployment is working on, newest first. Optionally narrowed to one twin or one status.",
				InputSchema: objectSchema(map[string]any{
					"twin_id": stringProperty("Only objectives belonging to this digital twin."),
					"status":  stringProperty("Only objectives in this status, e.g. active, converged, blocked."),
				}),
			},
			action: karakuriauth.ActionObjectiveRead,
			resource: func(ctx context.Context, h *MCPHandler, p auth.Principal, _ mcpArgs) auth.ResourceRef {
				// A collection names no resource, so the ref carries the
				// containers the caller holds the action over — otherwise a
				// team-scoped binding could read each objective by ID and never
				// list them. The same rule as the REST list route.
				return karakuriauth.ScopedCollectionRef(ctx, h.Scopes, p.ID,
					karakuriauth.ActionObjectiveRead, auth.Collection("objective"))
			},
			read: func(ctx context.Context, h *MCPHandler, p auth.Principal, args mcpArgs) (any, error) {
				visible, hidden, err := karakuriauth.ListFor(
					ctx, h.Scopes, p.ID, karakuriauth.ActionObjectiveRead, "objective")
				if err != nil {
					return nil, err
				}
				return h.Objectives.List(ctx, storage.ObjectiveFilter{
					TwinID:  args.str("twin_id"),
					Status:  args.str("status"),
					Visible: visible,
					Hidden:  hidden,
				})
			},
		},
		{
			def: mcp.Tool{
				Name:        "objective_read",
				Description: "Read one objective: its title, description, domain, status, autonomy and budget.",
				InputSchema: objectSchema(map[string]any{
					"objective_id": stringProperty("The objective to read."),
				}, "objective_id"),
			},
			action: karakuriauth.ActionObjectiveRead,
			resource: func(ctx context.Context, h *MCPHandler, _ auth.Principal, args mcpArgs) auth.ResourceRef {
				return karakuriauth.ScopedResource(ctx, h.Containers,
					auth.Resource("objective", args.str("objective_id")))
			},
			read: func(ctx context.Context, h *MCPHandler, _ auth.Principal, args mcpArgs) (any, error) {
				id, err := args.required("objective_id")
				if err != nil {
					return nil, err
				}
				return h.Objectives.Get(ctx, coreobjective.ObjectiveID(id))
			},
		},
		{
			def: mcp.Tool{
				Name:        "digest_read",
				Description: "Read the digest for one twin over a window: what was delivered, what was escalated, what it cost. Assembled on demand and delivered nowhere.",
				InputSchema: objectSchema(map[string]any{
					"twin_id": stringProperty("The digital twin to report on."),
					"window":  stringProperty("How far back to look, as a Go duration such as 24h or 168h. Defaults to 24h."),
				}, "twin_id"),
			},
			action: karakuriauth.ActionReportRead,
			resource: func(context.Context, *MCPHandler, auth.Principal, mcpArgs) auth.ResourceRef {
				// The same reference GET /reports/preview is gated on. A digest
				// is not stored per twin, so there is no narrower subject to
				// name without inventing one that the REST route does not have.
				return auth.Collection("report")
			},
			read: func(ctx context.Context, h *MCPHandler, _ auth.Principal, args mcpArgs) (any, error) {
				twinID, err := args.required("twin_id")
				if err != nil {
					return nil, err
				}
				window, err := args.duration("window", 24*time.Hour)
				if err != nil {
					return nil, err
				}
				return h.Reports.Preview(ctx, twinID, window)
			},
		},
		{
			def: mcp.Tool{
				Name:        "reconcile_status",
				Description: "Read the control-loop state of a standing objective and its recent reconcile history: when it last ran, whether it is paused or broken, and what each pass did.",
				InputSchema: objectSchema(map[string]any{
					"objective_id": stringProperty("The standing objective to report on."),
					"limit":        integerProperty("How many past reconciles to include. Defaults to 20, capped at 200."),
				}, "objective_id"),
			},
			action: karakuriauth.ActionObjectiveRead,
			resource: func(ctx context.Context, h *MCPHandler, _ auth.Principal, args mcpArgs) auth.ResourceRef {
				return karakuriauth.ScopedResource(ctx, h.Containers,
					auth.Resource("objective", args.str("objective_id")))
			},
			read: func(ctx context.Context, h *MCPHandler, _ auth.Principal, args mcpArgs) (any, error) {
				raw, err := args.required("objective_id")
				if err != nil {
					return nil, err
				}
				id := coreobjective.ObjectiveID(raw)
				state, err := h.Reconcile.State(ctx, id)
				if err != nil {
					return nil, fmt.Errorf("objective %s is not standing", id)
				}
				history, err := h.Reconcile.History(ctx, id, args.limit("limit", 20, 200))
				if err != nil {
					return nil, err
				}
				// The same pair the REST route returns, because they answer one
				// question: is this being looked after, and what has it done.
				return struct {
					State   reconcile.State     `json:"state"`
					History []reconcile.Outcome `json:"history"`
				}{State: state, History: history}, nil
			},
		},
		{
			def: mcp.Tool{
				Name:        "telemetry_read",
				Description: "Read what this deployment has been doing over a window: objectives by status, sense and reconcile counts, escalations and how they were resolved, spend, and the bottlenecks already ranked.",
				InputSchema: objectSchema(map[string]any{
					"twin_id": stringProperty("Narrow the snapshot to one digital twin. Omit for the whole deployment."),
					"window":  stringProperty("How far back to look, as a Go duration such as 168h. Defaults to 168h."),
				}),
			},
			// audit:read, because this is the record of what the deployment did
			// — escalations, who resolved them, what it spent. It is the
			// auditor's surface, and the role that holds it already holds
			// cost:read beside it.
			action: karakuriauth.ActionAuditRead,
			resource: func(context.Context, *MCPHandler, auth.Principal, mcpArgs) auth.ResourceRef {
				return auth.Collection("audit")
			},
			read: func(ctx context.Context, h *MCPHandler, _ auth.Principal, args mcpArgs) (any, error) {
				if h.Telemetry == nil {
					return nil, errors.New("no telemetry reader is wired into this deployment")
				}
				window, err := args.duration("window", 7*24*time.Hour)
				if err != nil {
					return nil, err
				}
				return h.Telemetry.Snapshot(ctx, coretelemetry.Query{
					Since:  time.Now().UTC().Add(-window),
					TwinID: args.str("twin_id"),
				})
			},
		},
	}
}

// ServeHTTP handles one JSON-RPC message.
//
// Streamable HTTP, minus the parts that have no caller: a POST carries one
// message and is answered with one JSON body. No SSE, because nothing here
// streams — every tool is a bounded read. No session header, because the bearer
// token already identifies the caller on every request, and a second identifier
// would be a second thing to expire, resume and get wrong.
//
// The GET and DELETE sides of the transport are answered by chi with 405, which
// is what the specification asks of a server offering neither.
func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		// Unreachable behind Authenticate, and answered rather than assumed:
		// this handler is the one place where a missing principal would mean
		// running a tool as nobody.
		authError(w, http.StatusUnauthorized, "unauthorized", "no principal")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxMCPBody))
	if err != nil {
		writeRPCError(w, http.StatusBadRequest, nil, mcp.CodeParseError, "could not read the request body")
		return
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		writeRPCError(w, http.StatusBadRequest, nil, mcp.CodeParseError, "empty request")
		return
	}
	if trimmed[0] == '[' {
		// Batching was removed in MCP 2025-06-18. Saying so beats decoding one
		// message out of an array and silently dropping the rest.
		writeRPCError(w, http.StatusBadRequest, nil, mcp.CodeInvalidRequest,
			"this server does not accept batched requests; send one message per POST")
		return
	}

	var req mcp.Request
	if err := json.Unmarshal([]byte(trimmed), &req); err != nil {
		writeRPCError(w, http.StatusBadRequest, nil, mcp.CodeParseError, "could not parse the JSON-RPC request")
		return
	}

	if req.IsNotification() {
		// A notification has no reply, and answering one would leave a response
		// in the stream where the next result belongs.
		w.WriteHeader(http.StatusAccepted)
		return
	}

	result, rpcErr := h.dispatch(r, principal, req)
	resp := mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rpcErr}
	writeJSON(w, resp)
}

// dispatch answers one request, returning either a result or a JSON-RPC error.
func (h *MCPHandler) dispatch(r *http.Request, p auth.Principal, req mcp.Request) (json.RawMessage, *mcp.Error) {
	ctx := r.Context()
	switch req.Method {
	case mcp.MethodInitialize:
		return marshalResult(mcp.InitializeResult{
			// What this server speaks, stated rather than echoed: a client
			// asking for a revision this implementation does not implement
			// should see the difference instead of being agreed with.
			ProtocolVersion: mcp.ProtocolVersion,
			Capabilities: map[string]any{
				// No listChanged notification: the tool list is fixed for the
				// life of the process, and what varies is per-principal.
				"tools": map[string]any{"listChanged": false},
			},
			ServerInfo:   mcpServerInfo,
			Instructions: mcpInstructions,
		})

	case mcp.MethodPing:
		return marshalResult(struct{}{})

	case mcp.MethodToolsList:
		return marshalResult(mcp.ListToolsResult{Tools: h.visibleTools(ctx, p)})

	case mcp.MethodToolsCall:
		return h.callTool(r, p, req)

	default:
		return nil, &mcp.Error{Code: mcp.CodeMethodNotFound, Message: "unknown method " + req.Method}
	}
}

// visibleTools is the catalog filtered to what this principal may actually
// call.
//
// Filtering rather than listing everything and refusing later: an advertised
// tool is a tool a model will plan around, and a plan built on one it cannot
// call is a wasted turn at best. A denial here is not audited — listing is not
// an attempt to act, and recording five refusals every time a client connects
// would bury the attempts that matter.
func (h *MCPHandler) visibleTools(ctx context.Context, p auth.Principal) []mcp.Tool {
	all := h.tools()
	out := make([]mcp.Tool, 0, len(all))
	for _, t := range all {
		decision, err := h.Enforcer.Authorizer.Authorize(ctx, p, t.action, t.resource(ctx, h, p, mcpArgs{}))
		if err != nil || !decision.Allowed {
			continue
		}
		out = append(out, t.def)
	}
	return out
}

// callTool authorizes and runs one tool.
func (h *MCPHandler) callTool(r *http.Request, p auth.Principal, req mcp.Request) (json.RawMessage, *mcp.Error) {
	var params mcp.CallToolParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &mcp.Error{Code: mcp.CodeInvalidParams, Message: "could not parse tool arguments"}
		}
	}
	if params.Name == "" {
		return nil, &mcp.Error{Code: mcp.CodeInvalidParams, Message: "no tool named"}
	}

	if reason, withheld := mcpWithheld[params.Name]; withheld {
		return nil, &mcp.Error{
			Code:    mcp.CodeInvalidParams,
			Message: fmt.Sprintf("%q is not exposed over MCP: %s", params.Name, reason),
		}
	}

	all := h.tools()
	var tool *mcpTool
	for i := range all {
		if all[i].def.Name == params.Name {
			tool = &all[i]
			break
		}
	}
	if tool == nil {
		return nil, &mcp.Error{
			Code:    mcp.CodeInvalidParams,
			Message: fmt.Sprintf("unknown tool %q; this server offers %s", params.Name, strings.Join(toolNames(all), ", ")),
		}
	}

	ctx := r.Context()
	args := mcpArgs(params.Arguments)
	res := tool.resource(ctx, h, p, args)

	decision, err := h.Enforcer.Authorizer.Authorize(ctx, p, tool.action, res)
	if err != nil {
		if h.Enforcer.OnError != nil {
			h.Enforcer.OnError(r, err)
		}
		return nil, &mcp.Error{Code: mcp.CodeInternalError, Message: "authorization could not be evaluated"}
	}
	if !decision.Allowed {
		if h.Enforcer.OnDeny != nil {
			// Named with the tool, so the audit row says which one was
			// attempted rather than only that something reached /mcp.
			h.Enforcer.OnDeny(toolRequest(r, params.Name), p, decision)
		}
		return nil, &mcp.Error{Code: codeForbidden, Message: decision.Reason}
	}

	value, err := tool.read(ctx, h, p, args)
	if err != nil {
		// A fair question with a bad answer: the protocol's own way of saying
		// the tool ran and failed, which is what lets a client tell this apart
		// from a refusal it should not retry.
		return marshalResult(mcp.ErrorResult(err.Error()))
	}

	rendered, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, &mcp.Error{Code: mcp.CodeInternalError, Message: "could not encode the result"}
	}
	return marshalResult(mcp.TextResult(string(rendered)))
}

// toolRequest is r with the tool appended to its path, for the audit hook. The
// hook records method and path, and every MCP call shares one of each.
func toolRequest(r *http.Request, tool string) *http.Request {
	clone := r.Clone(r.Context())
	clone.URL.Path = strings.TrimSuffix(r.URL.Path, "/") + "/" + tool
	return clone
}

func toolNames(tools []mcpTool) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.def.Name)
	}
	return out
}

func marshalResult(v any) (json.RawMessage, *mcp.Error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, &mcp.Error{Code: mcp.CodeInternalError, Message: "could not encode the result"}
	}
	return raw, nil
}

// writeRPCError answers a message that never reached dispatch — one that could
// not be parsed, or was not a single request. These carry an HTTP status as
// well as a JSON-RPC error because there is no request to attribute them to.
func writeRPCError(w http.ResponseWriter, status int, id json.RawMessage, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(mcp.Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &mcp.Error{Code: code, Message: message},
	})
}

// mcpArgs is one tool call's arguments, decoded from JSON — so every number is
// a float64 and every absent key is a zero value.
type mcpArgs map[string]any

func (a mcpArgs) str(key string) string {
	s, _ := a[key].(string)
	return strings.TrimSpace(s)
}

// required reads an argument a tool cannot run without.
func (a mcpArgs) required(key string) (string, error) {
	if v := a.str(key); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("%s is required", key)
}

// duration reads a Go duration argument, rejecting a non-positive one rather
// than treating it as the default: "window: -1h" is a mistake, and a report
// silently covering the last day instead is a mistake nobody notices.
func (a mcpArgs) duration(key string, fallback time.Duration) (time.Duration, error) {
	raw := a.str(key)
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a duration", key, raw)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return d, nil
}

// limit reads a row count, clamping rather than refusing: a client asking for
// a thousand reconciles wants as many as it can have, and the ceiling is this
// server's to set.
func (a mcpArgs) limit(key string, fallback, max int) int {
	n, ok := a[key].(float64)
	if !ok || n <= 0 {
		return fallback
	}
	if int(n) > max {
		return max
	}
	return int(n)
}

// objectSchema builds the JSON Schema a client validates arguments against.
//
// Written out here rather than derived from a Go type: the schema is what a
// model reads to decide what to send, and its descriptions are prose about this
// deployment that no struct tag would carry.
func objectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringProperty(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func integerProperty(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}
