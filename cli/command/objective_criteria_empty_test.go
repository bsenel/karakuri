package command

import (
	"strings"
	"testing"
)

// An objective with no success criteria shows "-" under Required rather than
// a heading with nothing beneath it.
func TestObjectiveCriteriaWithNoneShowsDash(t *testing.T) {
	lines := showCriteriaOf(t, `{"id": "obj_42", "title": "Speed up search", "success_criteria": []}`)

	got := strings.Join(lines, "\n")
	want := "obj_42  Speed up search\n\nRequired:\n  -\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}
