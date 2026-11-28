// Package gateway is the one path every tool call takes (ADR 028, Phase 39
// step 1). The reasoning loop and the served MCP surface both hand their call
// to Gateway.Call, which applies the capability quota, the audit row, the cost
// entry and the provenance of the result in one place.
//
// This file is scaffolding for the step's tests: Call does not yet do any of
// that, so the tests that hold it to the contract fail until it does.
package gateway

import (
	"context"
	"errors"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

// Caller names who made a call, so the audit row can say which door it came
// through.
type Caller string

const (
	// CallerLoop is the reasoning loop's act step.
	CallerLoop Caller = "loop"
	// CallerMCP is a tool call on the served MCP surface.
	CallerMCP Caller = "mcp"
)

// Call is one tool call as the gateway sees it.
type Call struct {
	Caller      Caller
	TwinID      string
	ObjectiveID string
	AgentID     string
	// Capability is the capability or tool name the call invokes.
	Capability string
	// EnvID is the environment or adapter that runs the call.
	EnvID  string
	Params map[string]any
}

// Perform is the work behind a call: the environment's Act, or the served
// tool's implementation.
type Perform func(ctx context.Context) (environment.ActionResult, error)

// ErrUnimplemented is what the scaffolding returns until the gateway applies
// its checks.
var ErrUnimplemented = errors.New("gateway: unimplemented")

// Gateway applies quota, audit, cost and provenance to every tool call.
type Gateway struct {
	store storage.StorageAdapter
	quota karakuriquota.Deps
	costs *karakuriquota.Recorder
}

// New builds a gateway over the audit store, the quota dependencies and the
// cost recorder.
func New(store storage.StorageAdapter, quota karakuriquota.Deps, costs *karakuriquota.Recorder) *Gateway {
	return &Gateway{store: store, quota: quota, costs: costs}
}

// Call runs one tool call through the gateway.
func (g *Gateway) Call(ctx context.Context, call Call, perform Perform) (environment.ActionResult, error) {
	return environment.ActionResult{Success: false, Error: ErrUnimplemented.Error()}, ErrUnimplemented
}
