package command

import (
	"strings"
	"testing"
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

func TestTwinGetHelp(t *testing.T) {
	if long := twinGetCmd().Long; !strings.Contains(long, "krk twin list") {
		t.Errorf("twin get Long %q does not say where <id> comes from", long)
	}
}
