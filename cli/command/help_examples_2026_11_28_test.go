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

// A twin is the first thing a new user must create, and twin and twin create
// said only "Manage digital twins" and "Create a digital twin", without what a
// twin is or where its ID goes next.
func TestTwinHelpSaysWhatATwinIsAndWhereItsIDGoes(t *testing.T) {
	for _, want := range []string{"person, team or organization", "krk objective create --twin"} {
		if long := twinCmd().Long; !strings.Contains(long, want) {
			t.Errorf("twin Long %q does not contain %q", long, want)
		}
	}
	if long := twinCreateCmd().Long; !strings.Contains(long, "krk objective create --twin") {
		t.Errorf("twin create Long %q does not say where the new ID goes", long)
	}
	// "Twin name (required)" repeated the flag's name back.
	if usage := twinCreateCmd().Flags().Lookup("name").Usage; !strings.Contains(usage, "e.g.") {
		t.Errorf("--name usage %q gives no example", usage)
	}
}
