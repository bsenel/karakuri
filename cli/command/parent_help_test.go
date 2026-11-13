package command

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// A parent command's --help is where a first-time user lands; it shows a
// first call, and every call it shows names a subcommand that exists, so the
// example cannot drift from the tree.
func TestParentCommandsShowAFirstCall(t *testing.T) {
	for _, cmd := range []*cobra.Command{checkpointCmd(), artifactCmd(), twinCmd()} {
		requireExample(t, cmd)
		requireExampleNamesSubcommands(t, cmd)
	}
}

func requireExampleNamesSubcommands(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	prefix := "krk " + cmd.Name() + " "
	for _, line := range strings.Split(cmd.Example, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rest, ok := strings.CutPrefix(line, prefix)
		if !ok {
			t.Errorf("%s example line %q does not start with %q", cmd.Name(), line, prefix)
			continue
		}
		sub := strings.Fields(rest)[0]
		if found, _, err := cmd.Find([]string{sub}); err != nil || found == cmd {
			t.Errorf("%s example line %q names no subcommand %q", cmd.Name(), line, sub)
		}
	}
}
