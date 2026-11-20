package cliagent

import (
	"context"
	"slices"
	"testing"
)

// A delegation that names a model runs Claude Code on it; one that names none
// leaves the CLI on its own default, so the command line is today's.
func TestClaudeCode_ModelFlag(t *testing.T) {
	cases := []struct {
		name  string
		model string
		want  []string
	}{
		{"no model", "", []string{"--print", "--output-format=stream-json", "--verbose", "do the thing"}},
		{"model named", "sonnet", []string{"--print", "--output-format=stream-json", "--verbose", "--model=sonnet", "do the thing"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin, trace := newStubCLI(t, 0)
			_, err := NewClaudeCode(bin).Delegate(context.Background(), DelegateInput{
				Prompt: "do the thing", WorktreePath: t.TempDir(), Model: tc.model,
			})
			if err != nil {
				t.Fatalf("Delegate: %v", err)
			}
			if argv := trace.argv(t); !slices.Equal(argv, tc.want) {
				t.Errorf("argv:\n got %q\nwant %q", argv, tc.want)
			}
		})
	}
}
