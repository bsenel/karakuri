package command

import (
	"strings"
	"testing"
)

func TestWebHelp(t *testing.T) {
	cmd := webCmd()
	requireExample(t, cmd)

	if !strings.Contains(cmd.Long, "repo") || !strings.Contains(cmd.Long, "Node") {
		t.Errorf("web Long %q does not say where to run it or what it needs", cmd.Long)
	}

	// Backticks in a pflag usage become the value placeholder, which printed
	// "--install npm install" for a bool flag.
	if usages := cmd.Flags().FlagUsages(); strings.Contains(usages, "--install npm") {
		t.Errorf("--install renders a value placeholder; a bool flag has none:\n%s", usages)
	}
}
