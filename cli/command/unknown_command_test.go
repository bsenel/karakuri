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

// Cobra's unknown-subcommand check covers the root only, so `krk objective
// show` printed the objective help and exit 0.
func TestUnknownSubcommandOfAGroupIsAnError(t *testing.T) {
	root := NewRoot()
	root.SetArgs([]string{"objective", "lis"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	err := root.Execute()
	if err == nil {
		t.Fatal("krk objective lis: expected an error")
	}
	for _, want := range []string{`unknown command "lis" for "krk objective"`, "Did you mean list?", "Available commands: create, get, list", "Run 'krk objective --help' for details"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// A group with no argument still shows its help and succeeds.
func TestBareGroupShowsHelp(t *testing.T) {
	root := NewRoot()
	root.SetArgs([]string{"objective"})
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil {
		t.Fatalf("krk objective: %v", err)
	}
	if !strings.Contains(out.String(), "Available Commands:") {
		t.Errorf("krk objective printed %q, want its help", out.String())
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
