package software

import (
	"context"
	"testing"
)

// The cli_agents instance's model option reaches every delegation, so the
// coding agents can run a cheaper model than the planner; with no option the
// delegation names none and the CLI keeps its own default.
func TestDelegationRunsTheInstanceModel(t *testing.T) {
	for _, model := range []string{"", "sonnet"} {
		cli := &recordingCLI{}
		env := &cliEnv{id: "software.env.cli_agent", cli: cli, twinID: "twin-1", model: model}
		res, err := env.Act(context.Background(), delegateAction(t))
		if err != nil {
			t.Fatalf("model %q: act: %v", model, err)
		}
		if !res.Success {
			t.Fatalf("model %q: delegation failed: %s", model, res.Error)
		}
		if len(cli.calls) != 1 {
			t.Fatalf("model %q: adapter calls = %d, want 1", model, len(cli.calls))
		}
		if got := cli.calls[0].Model; got != model {
			t.Errorf("DelegateInput.Model = %q, want %q", got, model)
		}
	}
}
