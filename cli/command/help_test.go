package command

import (
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// requireExample fails when a command a new user reaches for has no Example:
// --help should show one invocation that can be copied and adapted.
func requireExample(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	if strings.TrimSpace(cmd.Example) == "" {
		t.Errorf("%q has no Example", cmd.Name())
	}
}

func TestCreateHelp(t *testing.T) {
	requireExample(t, objectiveCreateCmd())
	requireExample(t, twinCreateCmd())

	// A bare "Twin ID" / "Description" repeats the flag name and says nothing.
	flags := objectiveCreateCmd().Flags()
	if usage := flags.Lookup("twin").Usage; !strings.Contains(usage, "krk twin list") {
		t.Errorf("--twin usage %q does not say where a twin ID comes from", usage)
	}
	if usage := flags.Lookup("description").Usage; usage == "Description" {
		t.Errorf("--description usage %q only repeats the flag name", usage)
	}
}

// A wrong argument count has to show what the command expects: with
// SilenceUsage set, cobra's "accepts 1 arg(s), received 0" is all the user gets.
func TestMissingArgumentShowsUsage(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		usage string
	}{
		{[]string{"objective", "get"}, "Usage: krk objective get"},
		{[]string{"twin", "get"}, "Usage: krk twin get"},
		{[]string{"loop", "start"}, "Usage: krk loop start"},
	} {
		name := strings.Join(tc.args, " ")
		root := NewRoot()
		root.SetArgs(tc.args)
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		err := root.Execute()
		if err == nil {
			t.Fatalf("krk %s: no error for a missing argument", name)
		}
		msg := err.Error()
		if !strings.Contains(msg, "accepts 1 arg(s), received 0") {
			t.Errorf("krk %s: %q lost cobra's reason", name, msg)
		}
		if !strings.Contains(msg, tc.usage) {
			t.Errorf("krk %s: %q does not show %q", name, msg, tc.usage)
		}
		if want := "Run 'krk " + name + " --help'"; !strings.Contains(msg, want) {
			t.Errorf("krk %s: %q does not point at %q", name, msg, want)
		}
	}
}

func TestCheckpointHelp(t *testing.T) {
	for _, cmd := range []*cobra.Command{checkpointListCmd(), checkpointGetCmd(), checkpointResolveCmd()} {
		requireExample(t, cmd)
	}

	// --decision is required, so its usage has to name the values it accepts.
	usage := checkpointResolveCmd().Flags().Lookup("decision").Usage
	for _, choice := range []string{"approve", "reject", "modify"} {
		if !strings.Contains(usage, choice) {
			t.Errorf("--decision usage %q does not name %q", usage, choice)
		}
	}

	if short := checkpointCmd().Short; !strings.Contains(short, "human decision") {
		t.Errorf("checkpoint Short %q does not say what a checkpoint is", short)
	}
}

// The create examples and flag usages point at IDs that come from get and list.
func TestGetAndListHelp(t *testing.T) {
	for _, cmd := range []*cobra.Command{objectiveGetCmd(), objectiveListCmd(), twinGetCmd(), twinListCmd()} {
		requireExample(t, cmd)
	}
}

func TestLoopHelp(t *testing.T) {
	for _, cmd := range []*cobra.Command{loopStartCmd(), loopStatusCmd(), loopResumeCmd()} {
		requireExample(t, cmd)
	}

	if usage := loopStartCmd().Flags().Lookup("twin").Usage; !strings.Contains(usage, "krk twin list") {
		t.Errorf("--twin usage %q does not say where a twin ID comes from", usage)
	}

	// --decision is required, so its usage has to name the values it accepts.
	usage := loopResumeCmd().Flags().Lookup("decision").Usage
	for _, choice := range []string{"approve", "reject", "modify"} {
		if !strings.Contains(usage, choice) {
			t.Errorf("--decision usage %q does not name %q", usage, choice)
		}
	}
}
