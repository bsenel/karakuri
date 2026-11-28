package command

import (
	"strings"
	"testing"
)

// A first-time user reaches objective and objective create before anything
// else; their help said "Manage objectives" and "Create an objective" and
// nothing about what an objective is or what to run after creating one.
func TestObjectiveHelpSaysWhatToRunNext(t *testing.T) {
	for _, want := range []string{"what a twin should achieve", "krk objective create", "krk loop start <id>"} {
		if long := objectiveCmd().Long; !strings.Contains(long, want) {
			t.Errorf("objective Long %q does not contain %q", long, want)
		}
	}
	for _, want := range []string{"print its ID", "krk loop start <id>", "krk objective criteria <id>"} {
		if long := objectiveCreateCmd().Long; !strings.Contains(long, want) {
			t.Errorf("objective create Long %q does not contain %q", long, want)
		}
	}
}
