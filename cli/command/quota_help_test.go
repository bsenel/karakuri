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

func TestQuotaDecideHelp(t *testing.T) {
	for _, verb := range []string{"approve", "reject"} {
		cmd := quotaDecideCmd(verb)
		requireExample(t, cmd)

		// <request-id> appears nowhere else in the help, so the Long must
		// say where one comes from and the example must show one.
		if !strings.Contains(cmd.Long, "krk quota requests list") {
			t.Errorf("%s Long %q does not say where <request-id> comes from", verb, cmd.Long)
		}
		if !strings.Contains(cmd.Example, "krk quota requests "+verb+" qr_") {
			t.Errorf("%s Example %q does not show a qr_ request ID", verb, cmd.Example)
		}
		// A bare "why" names no value and no default.
		if usage := cmd.Flags().Lookup("note").Usage; !strings.Contains(usage, "recorded with the decision") || !strings.Contains(usage, "default") {
			t.Errorf("%s --note usage %q does not say what it is for or name the default", verb, usage)
		}
	}

	// reject used to print approve's Long, which describes writing an override.
	if long := quotaDecideCmd("reject").Long; strings.HasPrefix(long, "Approving") {
		t.Errorf("reject Long %q describes approving", long)
	}
}
