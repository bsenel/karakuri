package command

import (
	"strings"
	"testing"
)

// "--since duration" with a default printed as 720h0m0s invites "--since 30d",
// which the flag parser refuses; the usage has to name the units it accepts.
func TestCostReportHelp(t *testing.T) {
	cmd := costReportCmd()
	requireExample(t, cmd)

	since := cmd.Flags().Lookup("since").Usage
	for _, want := range []string{"hours", "24h", "no day unit", "720h"} {
		if !strings.Contains(since, want) {
			t.Errorf("--since usage %q does not mention %q", since, want)
		}
	}
	if err := cmd.Flags().Set("since", "30d"); err == nil {
		t.Error("--since 30d parsed; the usage says there is no day unit")
	}

	if limit := cmd.Flags().Lookup("limit").Usage; !strings.Contains(limit, "0 ") {
		t.Errorf("--limit usage %q does not say what the default 0 means", limit)
	}
}
