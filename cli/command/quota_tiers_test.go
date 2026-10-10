package command

import (
	"strings"
	"testing"
)

// Each tier takes different flags; the error for the wrong ones has to show
// the command to type, not only name the flag.
func TestQuotaSetErrorsShowExample(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{
			[]string{"--tier", "request", "--reason", "the SPA polls"},
			[]string{"--tier request is a rate", `krk quota set --tier request --per-minute 120 --burst 40 --reason "the SPA polls"`},
		},
		{
			[]string{"--tier", "llm-tokens", "--reason", "team grew"},
			[]string{"--tier llm-tokens is a daily cap", `krk quota set --tier llm-tokens --cap 2000 --reason "team grew"`},
		},
		{
			[]string{"--tier", "tokens", "--reason", "team grew"},
			[]string{`unknown tier "tokens"`, "adapter, capability, llm-tokens, request", "krk quota config"},
		},
	}
	for _, tc := range cases {
		cmd := quotaSetCmd()
		cmd.SetArgs(tc.args)
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		err := cmd.Execute()
		if err == nil {
			t.Fatalf("quota set %v = nil error, want one", tc.args)
		}
		for _, want := range tc.want {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("quota set %v: error %q does not contain %q", tc.args, err, want)
			}
		}
	}
}
