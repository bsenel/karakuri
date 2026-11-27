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

// "--org string   Organisation this belongs to" did not say whether it takes a
// name or an ID, nor where to look one up. Every flag that names an
// organisation must say it takes a name and point at krk org list.
func TestContainerOrgFlagHelp(t *testing.T) {
	cmds := append(teamCmd().Commands(), containerCreateCmd("org", "Create an organisation", false))
	checked := 0
	for _, cmd := range cmds {
		for _, name := range []string{"org", "to-org"} {
			f := cmd.Flags().Lookup(name)
			if f == nil {
				continue
			}
			checked++
			if !strings.Contains(f.Usage, "Name of") || !strings.Contains(f.Usage, "krk org list") {
				t.Errorf("%s --%s usage %q does not say it takes a name from krk org list", cmd.Name(), name, f.Usage)
			}
		}
	}
	// team create, list, rename, delete, move (two) and org create.
	if checked != 7 {
		t.Errorf("checked %d organisation flags, want 7", checked)
	}
}
