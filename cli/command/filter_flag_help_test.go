package command

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestObjectiveFilterSaysWhereIDComesFrom(t *testing.T) {
	// "Filter by objective ID" names the value but not where to find one.
	for _, cmd := range []*cobra.Command{artifactListCmd(), auditCmd()} {
		usage := cmd.Flags().Lookup("objective").Usage
		if !strings.Contains(usage, "krk objective list") {
			t.Errorf("%s --objective usage %q does not say where an ID comes from", cmd.Name(), usage)
		}
	}
}

func TestReportPreviewWindowShowsFormat(t *testing.T) {
	// report create's --window shows a duration; preview's said only
	// "How far back to look", which leaves the format a guess.
	usage := reportPreviewCmd().Flags().Lookup("window").Usage
	if !strings.Contains(usage, "e.g. 24h") {
		t.Errorf("preview --window usage %q does not show a duration", usage)
	}
}
