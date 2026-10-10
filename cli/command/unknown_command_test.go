package command

import (
	"io"
	"strings"
	"testing"
)

// Cobra only runs its unknown-subcommand check while a command's Args is nil,
// so wrapping every command made `krk bogus` print the root help and exit 0.
func TestUnknownCommandIsAnError(t *testing.T) {
	root := NewRoot()
	root.SetArgs([]string{"bogus"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	err := root.Execute()
	if err == nil {
		t.Fatal("krk bogus: expected an error")
	}
	if want := `unknown command "bogus" for "krk"`; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not contain %q", err, want)
	}
}

// Leaving Args alone on the command groups must not cost the leaf commands
// their usage lines.
func TestLeafCommandsKeepUsageLines(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"quota", "set"},
			want: []string{`required flag(s) "reason", "tier" not set`, "Usage: krk quota set", "Run 'krk quota set --help' for the flags and examples"},
		},
		{
			args: []string{"objective", "get"},
			want: []string{"Usage: krk objective get", "Run 'krk objective get --help' for details"},
		},
	} {
		root := NewRoot()
		root.SetArgs(tc.args)
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		err := root.Execute()
		if err == nil {
			t.Fatalf("krk %s: expected an error", strings.Join(tc.args, " "))
		}
		for _, want := range tc.want {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("krk %s: error %q does not contain %q", strings.Join(tc.args, " "), err, want)
			}
		}
	}
}
