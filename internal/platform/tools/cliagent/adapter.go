// Package cliagent defines the adapter for subprocess-backed coding-agent CLIs
// (Claude Code, Cursor CLI, Gemini CLI, GitHub Copilot CLI). These CLIs
// bring their own tool loops — they read files, run shell commands, and
// iterate on edits autonomously — so they cannot be modelled as plain
// LLM completion endpoints. The adapter exposes a single Delegate() call
// that hands a task to the CLI and reports back what it produced.
package cliagent

import (
	"context"
)

// DelegateInput is what the loop's act step hands to a CLI agent.
type DelegateInput struct {
	// Prompt is the natural-language task description.
	Prompt string
	// WorktreePath is the working directory the CLI runs in. Edits are isolated here.
	WorktreePath string
	// Files is an optional list of explicit context files to seed the CLI with.
	Files []string
	// AllowedTools optionally constrains which built-in tools the CLI may use
	// (e.g. ["read", "edit", "bash"]). CLIs that don't support tool gating ignore it.
	AllowedTools []string
	// Env adds environment variables to the CLI's process (merged onto os.Environ()).
	Env map[string]string
	// Timeout caps the subprocess wall-clock; 0 means inherit ctx deadline.
	TimeoutSeconds int
}

// DelegateOutput is what the CLI agent returns to the loop.
type DelegateOutput struct {
	// Summary is the CLI's final textual summary of what it did.
	Summary string
	// ArtifactSHAs are SHAs of any blobs the loop should associate with this delegation;
	// adapters populate this from worktree diffs (caller may further hash + store).
	ArtifactSHAs []string
	// ToolUses surfaces tool-call events emitted by the CLI's internal loop.
	ToolUses []ToolUse
	// ExitCode of the CLI subprocess (0 = success).
	ExitCode int
	// RawOutput is the full stdout for callers that want to inspect it verbatim
	// (also written to episodic memory by the loop).
	RawOutput string
}

// DelegateChunk is one streamed event from a CLI agent's stdout.
type DelegateChunk struct {
	Kind    string // "text" | "tool_use" | "tool_result" | "error" | "done"
	Content string // for "text"
	Tool    *ToolUse
	Err     error
}

// ToolUse is a single tool invocation logged by the CLI.
type ToolUse struct {
	Name   string         `json:"name"`
	Input  map[string]any `json:"input,omitempty"`
	Result string         `json:"result,omitempty"`
	OK     bool           `json:"ok"`
}

// CLIAgentAdapter is the slot interface — one method to delegate a task,
// one method to stream its output. Multi-instance per ADR 006: an operator
// can configure several CLI agent instances (acme_claude, personal_cursor),
// each bound to a different twin via DigitalTwin.AdapterBindings["cli_agents"].
type CLIAgentAdapter interface {
	Name() string
	Active() bool
	Delegate(ctx context.Context, in DelegateInput) (DelegateOutput, error)
	Stream(ctx context.Context, in DelegateInput) (<-chan DelegateChunk, error)
}
