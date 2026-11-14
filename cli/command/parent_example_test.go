package command

import (
	"testing"

	"github.com/spf13/cobra"
)

// "krk objective --help" is the first page a new user reads after login; it
// shows a first call, and every call names a subcommand that exists.
func TestFirstRunParentsShowAFirstCall(t *testing.T) {
	for _, cmd := range []*cobra.Command{objectiveCmd()} {
		requireExample(t, cmd)
		requireExampleNamesSubcommands(t, cmd)
	}
}
