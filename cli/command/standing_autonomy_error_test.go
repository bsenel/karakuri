package command

import (
	"strings"
	"testing"
)

// A mistyped --autonomy or --ceiling is refused before any request, with
// the levels there are and a command that works. The API URL cannot be
// reached, so a check that ran after the request would fail otherwise.
func TestStandingUnknownAutonomyLevelSaysWhichLevelsExist(t *testing.T) {
	for _, flag := range []string{"--autonomy", "--ceiling"} {
		_, stderr, err := runKrkSplit("--api-url", "http://127.0.0.1:1", "objective", "standing", "obj_123", flag, "act_wtih_notice")
		if err == nil {
			t.Fatalf("%s act_wtih_notice exited zero", flag)
		}
		seen := err.Error() + stderr
		for _, want := range []string{
			`invalid ` + flag + ` "act_wtih_notice"`,
			"use sense, propose, act_with_notice or act",
			"`krk objective standing obj_123 " + flag + " propose`",
		} {
			if !strings.Contains(seen, want) {
				t.Errorf("%s: the user saw %q, want it to contain %q", flag, seen, want)
			}
		}
		if strings.Contains(seen, "not logged in") || strings.Contains(seen, "cannot reach") {
			t.Errorf("%s: the check ran after the request: %q", flag, seen)
		}
	}
}
