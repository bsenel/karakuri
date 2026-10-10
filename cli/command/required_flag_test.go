package command

import (
	"io"
	"strings"
	"testing"
)

// Cobra's `required flag(s) "reason", "tier" not set` names the flags but not
// what they take; the usage line and the pointer to --help have to follow it.
func TestMissingRequiredFlagShowsUsage(t *testing.T) {
	root := NewRoot()
	root.SetArgs([]string{"quota", "set"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	err := root.Execute()
	if err == nil {
		t.Fatal("quota set without flags: expected an error")
	}
	for _, want := range []string{`required flag(s) "reason", "tier" not set`, "Usage: krk quota set", "Run 'krk quota set --help' for the flags and examples"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// --help must still win over a missing required flag.
func TestHelpIgnoresMissingRequiredFlag(t *testing.T) {
	root := NewRoot()
	root.SetArgs([]string{"quota", "set", "--help"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil {
		t.Fatalf("quota set --help = %v, want nil", err)
	}
}
