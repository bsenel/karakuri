package command

import (
	"strings"
	"testing"
)

// "Explicit tri-state filter" told a first-time user neither what the flag
// matches nor how to ask for the other two states.
func TestAuditHelpSaysWhatBoundsViolationShows(t *testing.T) {
	usage := auditCmd().Flags().Lookup("bounds-violation").Usage
	for _, want := range []string{"authority bounds", "--bounds-violation=false", "omit it to show both"} {
		if !strings.Contains(usage, want) {
			t.Errorf("--bounds-violation usage %q does not contain %q", usage, want)
		}
	}
	if strings.Contains(usage, "tri-state") {
		t.Errorf("--bounds-violation usage still says tri-state: %q", usage)
	}
}
