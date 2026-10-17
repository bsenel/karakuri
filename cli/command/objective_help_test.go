package command

import (
	"strings"
	"testing"
)

func TestObjectiveHelp(t *testing.T) {
	requireExample(t, objectiveGetCmd())
	requireExample(t, objectiveTemplatesCmd())

	// get takes an <id> its help never explained.
	if long := objectiveGetCmd().Long; !strings.Contains(long, "krk objective list") {
		t.Errorf("get Long %q does not say where <id> comes from", long)
	}
	// templates lists IDs; its help has to say where one goes.
	cmd := objectiveTemplatesCmd()
	if !strings.Contains(cmd.Long, "--template") || !strings.Contains(cmd.Example, "--template") {
		t.Errorf("templates Long %q or Example %q does not show where a template ID is used", cmd.Long, cmd.Example)
	}
}
