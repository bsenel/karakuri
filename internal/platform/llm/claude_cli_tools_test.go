package llm

import (
	"slices"
	"testing"
)

// With no tools configured the CLI's command line is today's, so a print-mode
// call can use no tool that needs permission; configured tools are passed as
// one --allowed-tools flag.
func TestClaudeCLIArgsCarryOnlyConfiguredTools(t *testing.T) {
	cases := []struct {
		name  string
		tools []string
		want  []string
	}{
		{"none", nil, []string{"--print", "--output-format", "json"}},
		{"read-only gh", []string{"Bash(gh pr list:*)", "Bash(gh pr view:*)"},
			[]string{"--print", "--output-format", "json", "--allowed-tools=Bash(gh pr list:*),Bash(gh pr view:*)"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := newClaudeCLI("claude", tc.tools).args(); !slices.Equal(got, tc.want) {
				t.Errorf("args:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

// The provider option reaches the CLI fallback.
func TestClaudeProviderPassesCLIAllowedTools(t *testing.T) {
	_, dir := writeStubBinary(t, "claude", `printf '{"result":"ok"}\n'`)
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("PATH", dir)

	tools := []string{"Bash(gh pr view:*)"}
	p, err := NewClaudeProvider(WithCLIAllowedTools(tools))
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	if p.cli == nil || !slices.Equal(p.cli.allowedTools, tools) {
		t.Fatalf("CLI fallback allowed tools = %v, want %v", p.cli, tools)
	}
}
