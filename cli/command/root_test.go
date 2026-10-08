package command

import (
	"io"
	"testing"
)

// An unknown --output value has to be refused with the accepted ones named,
// not silently printed as pretty.
func TestInvalidOutputFormat(t *testing.T) {
	root := NewRoot()
	root.SetArgs([]string{"twin", "list", "--output", "yaml"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	t.Cleanup(func() { output = "pretty" })
	err := root.Execute()
	if err == nil {
		t.Fatal("--output yaml: expected an error")
	}
	if want := `invalid --output "yaml": use json, pretty or quiet`; err.Error() != want {
		t.Errorf("--output yaml: error %q, want %q", err, want)
	}
}
