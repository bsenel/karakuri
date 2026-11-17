package command

import (
	"strings"
	"testing"
)

// "Filter by kind" did not say that leaving the filter empty lists every twin.
func TestTwinListHelpSaysWhatAnEmptyFilterShows(t *testing.T) {
	cmd := twinListCmd()
	for flag, want := range map[string]string{
		"kind":   "default: every kind",
		"domain": "default: every domain",
	} {
		if usage := cmd.Flags().Lookup(flag).Usage; !strings.Contains(usage, want) {
			t.Errorf("--%s usage %q does not contain %q", flag, usage, want)
		}
	}
	if !strings.Contains(cmd.Example, "--domain software") {
		t.Errorf("twin list Example = %q, want a --domain line", cmd.Example)
	}
}
