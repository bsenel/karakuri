package command

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// requireExample fails when a command a new user reaches for has no Example:
// --help should show one invocation that can be copied and adapted.
func requireExample(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	if strings.TrimSpace(cmd.Example) == "" {
		t.Errorf("%q has no Example", cmd.Name())
	}
}

func TestCheckpointHelp(t *testing.T) {
	for _, cmd := range []*cobra.Command{checkpointListCmd(), checkpointGetCmd(), checkpointResolveCmd()} {
		requireExample(t, cmd)
	}

	// --decision is required, so its usage has to name the values it accepts.
	usage := checkpointResolveCmd().Flags().Lookup("decision").Usage
	for _, choice := range []string{"approve", "reject", "modify"} {
		if !strings.Contains(usage, choice) {
			t.Errorf("--decision usage %q does not name %q", usage, choice)
		}
	}

	if short := checkpointCmd().Short; !strings.Contains(short, "human decision") {
		t.Errorf("checkpoint Short %q does not say what a checkpoint is", short)
	}
}
