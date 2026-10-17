package cliagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newPrintingStubCLI writes a script that prints body to stdout and exits with
// exitCode, standing in for a Claude Code run that fails.
func newPrintingStubCLI(t *testing.T, body string, exitCode int) string {
	t.Helper()
	script := "#!/bin/sh\ncat <<'OUT'\n" + body + "\nOUT\nexit " + map[bool]string{true: "1", false: "0"}[exitCode != 0] + "\n"
	bin := filepath.Join(t.TempDir(), "stub-cli")
	if err := os.WriteFile(bin, []byte(script), 0o600); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	if err := os.Chmod(bin, 0o700); err != nil { // #nosec G302 -- the stub must be executable
		t.Fatalf("chmod stub: %v", err)
	}
	return bin
}

// A usage limit reaches the coding agent as a synthetic assistant message and
// an exit of 1 with nothing on stderr. The error has to carry that message, or
// the audit log only says "exit 1 (stderr: )".
func TestClaudeCode_ExitWithEmptyStderrCarriesTheLastOutput(t *testing.T) {
	bin := newPrintingStubCLI(t,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"You have hit your session limit · resets 9pm (Europe/Paris)"}]}}`+"\n"+
			`{"type":"result","subtype":"success","is_error":true,"result":"You have hit your session limit · resets 9pm (Europe/Paris)"}`, 1)

	_, err := NewClaudeCode(bin).Delegate(context.Background(), DelegateInput{Prompt: "p", WorktreePath: t.TempDir()})

	if err == nil {
		t.Fatal("expected an error from a CLI that exits 1")
	}
	if !strings.Contains(err.Error(), "session limit") {
		t.Errorf("error %q does not carry the CLI's message", err)
	}
	if !strings.Contains(err.Error(), "exit 1") {
		t.Errorf("error %q no longer names the exit status", err)
	}
}

// Output is truncated so a run that printed a lot does not put all of it into
// one error string.
func TestClaudeCode_ExitErrorKeepsOnlyTheTailOfTheOutput(t *testing.T) {
	long := strings.Repeat("x", 5000) + " the reason"
	bin := newPrintingStubCLI(t, `{"type":"result","is_error":true,"result":"`+long+`"}`, 1)

	_, err := NewClaudeCode(bin).Delegate(context.Background(), DelegateInput{Prompt: "p", WorktreePath: t.TempDir()})

	if err == nil {
		t.Fatal("expected an error from a CLI that exits 1")
	}
	if !strings.Contains(err.Error(), "the reason") {
		t.Errorf("error does not keep the end of the output: %.120q", err)
	}
	if len(err.Error()) > 1000 {
		t.Errorf("error is %d bytes, want it bounded", len(err.Error()))
	}
}
