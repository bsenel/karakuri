package cliagent

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// CopilotCLI is a CLIAgentAdapter for the GitHub Copilot CLI extension
// (`gh copilot suggest` / `gh copilot explain`). Unlike Claude Code and
// Cursor CLI, Copilot CLI does not edit files autonomously — it returns
// a textual suggestion that the loop's act step decides whether to apply.
//
// Mode (suggest vs explain) is picked from DelegateInput.Env["COPILOT_MODE"]
// (default "suggest"). The prompt is the query to pass through.
type CopilotCLI struct {
	bin string // path to gh binary; "gh" by default
}

func NewCopilotCLI(bin string) *CopilotCLI {
	if bin == "" {
		bin = "gh"
	}
	return &CopilotCLI{bin: bin}
}

func (c *CopilotCLI) Name() string { return "copilot_cli" }

// Active reports whether `gh` is on PATH AND the copilot extension is installed.
// We do not run `gh extension list` at health-check time (would be too slow);
// we only check `gh` presence, deferring extension-missing failures to first
// invocation.
func (c *CopilotCLI) Active() bool { return binaryAvailable(c.bin) }

func (c *CopilotCLI) Delegate(ctx context.Context, in DelegateInput) (DelegateOutput, error) {
	mode := strings.ToLower(in.Env["COPILOT_MODE"])
	if mode == "" {
		mode = "suggest"
	}
	if mode != "suggest" && mode != "explain" {
		return DelegateOutput{}, fmt.Errorf("copilot_cli: unsupported mode %q (expected 'suggest' or 'explain')", mode)
	}

	args := []string{"copilot", mode}
	if mode == "suggest" {
		// `gh copilot suggest -t shell|gh|git "query"` — leave -t default (shell).
		args = append(args, in.Prompt)
	} else {
		args = append(args, in.Prompt)
	}

	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Dir = in.WorktreePath
	cmd.Env = mergedEnv(in.Env)

	combined, err := cmd.CombinedOutput()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	output := string(combined)

	out := DelegateOutput{
		Summary:   output,
		RawOutput: output,
		ExitCode:  exitCode,
	}
	if err != nil && exitCode != 0 {
		return out, fmt.Errorf("copilot_cli: %w (output: %s)", err, truncate(output, 500))
	}
	return out, nil
}

func (c *CopilotCLI) Stream(ctx context.Context, in DelegateInput) (<-chan DelegateChunk, error) {
	ch := make(chan DelegateChunk, 2)
	go func() {
		defer close(ch)
		out, err := c.Delegate(ctx, in)
		if err != nil {
			ch <- DelegateChunk{Kind: "error", Err: err}
			return
		}
		ch <- DelegateChunk{Kind: "text", Content: out.Summary}
		ch <- DelegateChunk{Kind: "done"}
	}()
	return ch, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
