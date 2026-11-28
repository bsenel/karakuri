package command

import (
	"strings"
	"testing"
)

// "--since 30d" is the first window people try, and Go durations have no day
// unit; the error has to say what to type instead. It is raised before any
// request, so the API URL points at nothing.
func TestSinceInDaysSaysToUseHours(t *testing.T) {
	for _, args := range [][]string{
		{"cost", "report", "--since", "30d"},
		{"eval", "calibrate", "--since", "30d"},
	} {
		_, _, err := runKrkSplit(append([]string{"--api-url", "http://127.0.0.1:1"}, args...)...)
		if err == nil {
			t.Fatalf("krk %s: no error for --since 30d", strings.Join(args, " "))
		}
		for _, want := range []string{`invalid argument "30d" for "--since"`, "no day unit", "--since 720h", "krk " + args[0] + " " + args[1] + " --since 720h"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("krk %s: error %q does not mention %q", strings.Join(args, " "), err, want)
			}
		}
	}

	// Another flag's parse error is left as the parser wrote it.
	_, _, err := runKrkSplit("--api-url", "http://127.0.0.1:1", "cost", "report", "--limit", "many")
	if err == nil || strings.Contains(err.Error(), "day unit") {
		t.Errorf("krk cost report --limit many: error %v, want the parser's own message", err)
	}
}
