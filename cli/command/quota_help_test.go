package command

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestQuotaHelp(t *testing.T) {
	requireExample(t, quotaConfigCmd())
	requireExample(t, quotaShowCmd())
	requireExample(t, quotaResetCmd())

	// show and reset refuse to run without --twin, so their examples must
	// show it and the flag must say where an ID comes from.
	for _, cmd := range []*cobra.Command{quotaShowCmd(), quotaResetCmd()} {
		if !strings.Contains(cmd.Example, "--twin") {
			t.Errorf("%s Example %q does not show the required --twin", cmd.Name(), cmd.Example)
		}
		if usage := cmd.Flags().Lookup("twin").Usage; !strings.Contains(usage, "krk twin list") {
			t.Errorf("%s --twin usage %q does not say where an ID comes from", cmd.Name(), usage)
		}
	}

	// reset behaves differently with and without --capability; the example
	// shows both and the flag names its default.
	if example := quotaResetCmd().Example; !strings.Contains(example, "--capability") {
		t.Errorf("reset Example %q does not show --capability", example)
	}
	usage := quotaResetCmd().Flags().Lookup("capability").Usage
	if !strings.Contains(usage, "krk domain capabilities") || !strings.Contains(usage, "default") {
		t.Errorf("--capability usage %q does not say where an ID comes from or name the default", usage)
	}
}
