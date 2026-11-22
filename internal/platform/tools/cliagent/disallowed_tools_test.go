package cliagent

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// Phase 40 step 6a: the claude CLI's deny list takes precedence over its allow
// list, so it is where a broad `Bash(git:*)` is narrowed to exclude a rebase
// or a force push. The adapter has to pass DelegateInput.DisallowedTools on.

func disallowedTools(argv []string) []string {
	var lists []string
	for _, a := range argv {
		if v, ok := strings.CutPrefix(a, "--disallowed-tools="); ok {
			lists = append(lists, v)
		}
	}
	return lists
}

func TestClaudeCode_DisallowedToolsReachTheCommandLine(t *testing.T) {
	deny := []string{"Bash(git rebase:*)", "Bash(git push --force:*)", "Bash(git push origin +*)"}
	bin, trace := newStubCLI(t, 0)
	_, err := NewClaudeCode(bin).Delegate(context.Background(), DelegateInput{
		Prompt: "do the thing", WorktreePath: t.TempDir(),
		AllowedTools: []string{"Read", "Bash(git:*)"}, DisallowedTools: deny,
	})
	if err != nil {
		t.Fatalf("delegate: %v", err)
	}
	argv := trace.argv(t)
	lists := disallowedTools(argv)
	if len(lists) != 1 {
		t.Fatalf("want exactly one --disallowed-tools= argument, got %d; argv = %q", len(lists), argv)
	}
	if got := strings.Split(lists[0], ","); !slices.Equal(got, deny) {
		t.Errorf("--disallowed-tools = %q, want %q", got, deny)
	}
	if got := allowedTools(t, argv); !slices.Equal(got, []string{"Read", "Bash(git:*)"}) {
		t.Errorf("the allow list changed: %q", got)
	}
}

// Streamed runs go through a different method; the deny list must hold there too.
func TestClaudeCode_StreamCarriesDisallowedTools(t *testing.T) {
	bin, trace := newStubCLI(t, 0)
	ch, err := NewClaudeCode(bin).Stream(context.Background(), DelegateInput{
		Prompt: "do the thing", WorktreePath: t.TempDir(),
		AllowedTools: []string{"Bash(git:*)"}, DisallowedTools: []string{"Bash(git rebase:*)"},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	for range ch {
	}
	if lists := disallowedTools(trace.argv(t)); len(lists) != 1 || lists[0] != "Bash(git rebase:*)" {
		t.Errorf("--disallowed-tools arguments = %q, want one holding Bash(git rebase:*)", lists)
	}
}

func TestClaudeCode_NoDisallowedToolsAddsNoFlag(t *testing.T) {
	bin, trace := newStubCLI(t, 0)
	if _, err := NewClaudeCode(bin).Delegate(context.Background(), DelegateInput{Prompt: "x", WorktreePath: t.TempDir()}); err != nil {
		t.Fatalf("delegate: %v", err)
	}
	if lists := disallowedTools(trace.argv(t)); len(lists) != 0 {
		t.Errorf("argv carries %q with nothing to deny", lists)
	}
}
