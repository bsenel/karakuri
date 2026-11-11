package command

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// "krk org --help" listed four subcommands and no way to tell what a first
// call looks like. Each example must name a subcommand that really exists, so
// a renamed subcommand cannot leave a stale line behind.
func TestContainerParentHelp(t *testing.T) {
	for _, cmd := range []*cobra.Command{orgCmd(), teamCmd(), projectCmd()} {
		requireExample(t, cmd)
		for _, line := range strings.Split(cmd.Example, "\n") {
			fields := strings.Fields(line)
			if len(fields) < 3 || fields[0] != "krk" || fields[1] != cmd.Name() {
				t.Errorf("%s example line %q does not start with \"krk %s <subcommand>\"", cmd.Name(), line, cmd.Name())
				continue
			}
			sub, _, err := cmd.Find(fields[2:3])
			if err != nil || sub == cmd {
				t.Errorf("%s example line %q names no existing subcommand", cmd.Name(), line)
			}
		}
	}
}
