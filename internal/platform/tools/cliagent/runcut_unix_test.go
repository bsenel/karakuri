//go:build unix

package cliagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A stopped delegation reaches the caller as a cut through every adapter's
// wrapping, so the action's result can say so.
func TestRunCut_SeenThroughEachAdapter(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "slow-agent")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nsleep 30\n"), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if err := os.Chmod(bin, 0o700); err != nil { // #nosec G302 -- the script must be executable
		t.Fatalf("chmod script: %v", err)
	}
	adapters := []CLIAgentAdapter{NewClaudeCode(bin), NewCursorCLI(bin), NewGeminiCLI(bin), NewCopilotCLI(bin)}
	for _, a := range adapters {
		t.Run(a.Name(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			start := time.Now()
			_, err := a.Delegate(ctx, DelegateInput{Prompt: "p", WorktreePath: t.TempDir()})
			if elapsed := time.Since(start); elapsed > 3*time.Second {
				t.Errorf("Delegate took %s after a 200ms timeout", elapsed)
			}
			cut, groupExited := RunCut(err)
			if !cut || !groupExited {
				t.Errorf("RunCut(%v) = (%v, %v), want (true, true)", err, cut, groupExited)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("err %v does not carry the context's reason", err)
			}
		})
	}
}

func TestRunCut_OtherErrors(t *testing.T) {
	if cut, _ := RunCut(errors.New("exit 1")); cut {
		t.Error("an ordinary error reads as a cut")
	}
	if cut, _ := RunCut(nil); cut {
		t.Error("nil reads as a cut")
	}
}
