package command

import (
	"strings"
	"testing"
)

func TestEvalCalibrateHelp(t *testing.T) {
	cmd := evalCalibrateCmd()
	requireExample(t, cmd)

	// --limit defaults to 0, which a first-time reader takes for "judge
	// nothing"; the usage must say 0 means every checkpoint in the window.
	if usage := cmd.Flags().Lookup("limit").Usage; !strings.Contains(usage, "0 judges every one") {
		t.Errorf("--limit usage %q does not say what the default 0 means", usage)
	}
	// The default prints as 720h0m0s; the usage must show the format and
	// say how long that is.
	if usage := cmd.Flags().Lookup("since").Usage; !strings.Contains(usage, "72h") || !strings.Contains(usage, "30 days") {
		t.Errorf("--since usage %q does not show the duration format or what the default spans", usage)
	}
}
