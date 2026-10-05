package command

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestArtifactHelp(t *testing.T) {
	for _, cmd := range []*cobra.Command{artifactListCmd(), artifactGetCmd(), artifactDiffCmd()} {
		if want := "krk artifact " + cmd.Name(); !strings.Contains(cmd.Example, want) {
			t.Errorf("%q Example %q does not show %q", cmd.Name(), cmd.Example, want)
		}
	}
}
