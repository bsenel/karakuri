package command

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestLoopStartHelp(t *testing.T) {
	cmd := loopStartCmd()
	requireExample(t, cmd)

	if !strings.Contains(cmd.Long, "krk objective list") {
		t.Errorf("start Long %q does not say where <objective-id> comes from", cmd.Long)
	}
	// --watch outlives the loop by declaring the objective standing; its
	// usage has to say so and say how to stop it.
	usage := cmd.Flags().Lookup("watch").Usage
	if !strings.Contains(usage, "standing") || !strings.Contains(usage, "krk objective pause") {
		t.Errorf("--watch usage %q does not say what it leaves running or how to stop it", usage)
	}
	if !strings.Contains(cmd.Example, "--watch") {
		t.Errorf("start Example %q does not show --watch", cmd.Example)
	}
}

// loop resume and checkpoint resolve take the same decisions: both show the
// one that needs extra flags, and both say what the -1 default means.
func TestResumeHelpShowsModify(t *testing.T) {
	for name, cmd := range map[string]func() *cobra.Command{
		"loop resume":        loopResumeCmd,
		"checkpoint resolve": checkpointResolveCmd,
	} {
		c := cmd()
		if !strings.Contains(c.Example, "--decision modify --constraint") {
			t.Errorf("%s Example %q does not show --decision modify", name, c.Example)
		}
		if usage := c.Flags().Lookup("revised-confidence").Usage; !strings.Contains(usage, "-1 sends no floor") {
			t.Errorf("%s --revised-confidence usage %q does not say what -1 means", name, usage)
		}
	}
}

func TestTwinGetHelp(t *testing.T) {
	if long := twinGetCmd().Long; !strings.Contains(long, "krk twin list") {
		t.Errorf("twin get Long %q does not say where <id> comes from", long)
	}
}
