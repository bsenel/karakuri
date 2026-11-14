package command

import (
	"strings"
	"testing"
)

func TestMemoryRecallTopKHelp(t *testing.T) {
	cmd := memoryRecallCmd()
	requireExample(t, cmd)

	// A bare "Maximum results" does not say what is counted.
	if usage := cmd.Flags().Lookup("top-k").Usage; !strings.Contains(usage, "memory entries") {
		t.Errorf("--top-k usage %q does not say it counts memory entries", usage)
	}
}
