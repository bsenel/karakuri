package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunAcceptsATitleInTheFormFromTheEnvironmentOrAnArgument(t *testing.T) {
	const title = "ci(software): require conventional commit titles on pull requests"
	var stderr bytes.Buffer
	if code := run(nil, title, &stderr); code != 0 {
		t.Errorf("run with the title in the environment = %d, want 0; stderr: %s", code, &stderr)
	}
	if code := run([]string{title}, "", &stderr); code != 0 {
		t.Errorf("run with the title as an argument = %d, want 0; stderr: %s", code, &stderr)
	}
	if stderr.Len() != 0 {
		t.Errorf("an accepted title printed %q, want nothing", &stderr)
	}
}

// The workflow's log is where the author of a refused title looks, so it has
// to carry the reason, the form and the examples, not only a red cross.
func TestRunRefusesAnOldTitleAndShowsTheFormAndExamples(t *testing.T) {
	var stderr bytes.Buffer
	if code := run(nil, "Phase 33 — MCP After the Handshake", &stderr); code != 1 {
		t.Fatalf("run = %d, want 1", code)
	}
	for _, want := range []string{
		"<type>(<scope>): <summary>",
		"feat(mcp): attach karakuri tools to delegated agent runs (phase 34)",
		"fix(cli): shift audit export window to the requested hours (EB-003)",
		"docs(roadmap): propose phases 41 to 43 (discovery 2026-10-18)",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr does not carry %q:\n%s", want, &stderr)
		}
	}
}

func TestRunRefusesAnUnsetTitle(t *testing.T) {
	var stderr bytes.Buffer
	if code := run(nil, "", &stderr); code != 1 {
		t.Errorf("run with no title = %d, want 1", code)
	}
}
