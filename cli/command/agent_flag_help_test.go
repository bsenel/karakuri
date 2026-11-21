package command

import (
	"strings"
	"testing"
)

func TestAgentFilterFlagHelp(t *testing.T) {
	// A bare "Filter by agent ID" does not say what is filtered or where
	// the ID is found.
	for name, usage := range map[string]string{
		"artifact list": artifactListCmd().Flags().Lookup("agent").Usage,
		"audit":         auditCmd().Flags().Lookup("agent").Usage,
	} {
		if !strings.Contains(usage, "Show only") || !strings.Contains(usage, "agent_id") {
			t.Errorf("%s --agent usage %q does not say what it narrows or the field to copy", name, usage)
		}
	}
}
