package procenv

import (
	"strings"
	"testing"
)

func TestScrubNestedSession(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"CLAUDECODE=1",
		"CLAUDE_CODE_SESSION_ID=abc",
		"CLAUDE_CODE_CHILD_SESSION=1",
		"HOME=/home/x",
		"ANTHROPIC_API_KEY=sk-xxx", // must survive — it's not a session marker
	}
	out := ScrubNestedSession(in)
	joined := strings.Join(out, "\n")

	for _, banned := range []string{"CLAUDECODE=", "CLAUDE_CODE_SESSION_ID=", "CLAUDE_CODE_CHILD_SESSION="} {
		if strings.Contains(joined, banned) {
			t.Errorf("nested-session var %q should have been stripped, env: %v", banned, out)
		}
	}
	for _, keep := range []string{"PATH=/usr/bin", "HOME=/home/x", "ANTHROPIC_API_KEY=sk-xxx"} {
		if !strings.Contains(joined, keep) {
			t.Errorf("non-session var %q must be preserved, env: %v", keep, out)
		}
	}
}
