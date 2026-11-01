package command

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestIDHelpNamesListCommand: a command that takes an ID must say which
// list command shows it, or a first-time user has nowhere to start.
func TestIDHelpNamesListCommand(t *testing.T) {
	for _, tc := range []struct {
		cmd  *cobra.Command
		want string
	}{
		{checkpointGetCmd(), "krk checkpoint list"},
		{checkpointResolveCmd(), "krk checkpoint list"},
	} {
		requireExample(t, tc.cmd)
		if !strings.Contains(tc.cmd.Long, tc.want) {
			t.Errorf("%s Long %q does not mention %q", tc.cmd.Name(), tc.cmd.Long, tc.want)
		}
	}
}
