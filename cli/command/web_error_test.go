package command

import (
	"strings"
	"testing"
)

// Outside the repo root, krk web must say where to run it and show how.
func TestWebOutsideRepoRootSaysWhereToRun(t *testing.T) {
	t.Chdir(t.TempDir())

	err := webCmd().RunE(nil, nil)
	if err == nil {
		t.Fatal("krk web outside the repo root returned no error")
	}
	for _, want := range []string{"web/package.json not found", "Karakuri repo root", "e.g. cd ", "krk web --install"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}
