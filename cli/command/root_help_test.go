package command

import (
	"strings"
	"testing"
)

func TestRootHelpHasExample(t *testing.T) {
	root := NewRoot()
	requireExample(t, root)

	// The first screen a new user sees must show the global flags in use and
	// a command that exists.
	for _, want := range []string{"krk ", "--api-url", "--output json"} {
		if !strings.Contains(root.Example, want) {
			t.Errorf("root Example %q does not show %q", root.Example, want)
		}
	}
	// A placeholder ID, never one from a real deployment: every user's help
	// would name an objective their server does not have.
	if !strings.Contains(root.Example, "krk objective get obj_123") || strings.Contains(root.Example, "76bba85ac9ea170c") {
		t.Errorf("root Example %q must use the placeholder obj_123, not a real objective ID", root.Example)
	}
	for _, line := range strings.Split(root.Example, "\n") {
		if _, ok := strings.CutPrefix(strings.TrimSpace(line), "krk "); !ok {
			continue
		}
		fields := strings.Fields(line)
		for _, f := range fields[1:] {
			if strings.HasPrefix(f, "-") {
				continue
			}
			if c, _, err := root.Find([]string{f}); err != nil || c == root {
				t.Errorf("root Example line %q names %q, which is not a command", line, f)
			}
			break
		}
	}
}
