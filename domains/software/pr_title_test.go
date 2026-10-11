package software

import (
	"strings"
	"testing"
)

// pullRequestTitleForm is the form a rejected title is told about. This
// repository squash-merges, so the title is the commit on main, and the
// version is derived from those commits.
const pullRequestTitleForm = "<type>(<scope>): <summary>"

func TestValidPullRequestTitleAcceptsTheForm(t *testing.T) {
	titles := []string{
		"feat(mcp): attach karakuri tools to delegated agent runs (phase 34)",
		"fix(cli): shift audit export window to the requested hours (EB-003)",
		"docs(roadmap): propose phases 41 to 43 (discovery 2026-10-18)",
		"ci(software): require conventional commit titles on pull requests",
		// The scope is optional, and `!` marks a breaking change with or
		// without one.
		"feat!: drop the legacy session endpoint",
		"feat(api)!: rename objective status field",
		// Dependabot's own titles, as they stand on main (#125 and #164).
		"gomod(deps): bump google.golang.org/grpc from 1.83.1 to 1.83.2",
		"npm(deps): bump source-map-js from 1.2.1 to 1.2.2 in /web",
	}
	for _, title := range titles {
		t.Run(title, func(t *testing.T) {
			if err := ValidPullRequestTitle(title); err != nil {
				t.Errorf("ValidPullRequestTitle(%q) = %v, want nil", title, err)
			}
		})
	}
}

// These are titles main carries from before the rule. Whoever wrote one needs
// to be shown the form, not only told the title is wrong.
func TestValidPullRequestTitleNamesTheFormWhenATitleIsNotInIt(t *testing.T) {
	titles := []string{
		"Phase 33 — MCP After the Handshake",
		"UX 2026-10-10: CLI help examples, error hints, delete output",
		"Roadmap: Phase 40, streams that run without a coordinator",
		"Engineering backlog: EB-004 audit export window",
	}
	for _, title := range titles {
		t.Run(title, func(t *testing.T) {
			err := ValidPullRequestTitle(title)
			if err == nil {
				t.Fatalf("ValidPullRequestTitle(%q) = nil, want an error", title)
			}
			if !strings.Contains(err.Error(), pullRequestTitleForm) {
				t.Errorf("error %q does not name the form %s", err, pullRequestTitleForm)
			}
		})
	}
}

func TestValidPullRequestTitleRejects(t *testing.T) {
	cases := []struct {
		name  string
		title string
		// keyword, when set, is one lower-case word the message must carry
		// (compared against the lower-cased message).
		keyword string
	}{
		{name: "an unknown type", title: "feature(cli): add a flag", keyword: "feature"},
		{name: "no space after the colon", title: "feat(cli):add a flag"},
		{name: "no colon", title: "feat(cli) add a flag"},
		{name: "an upper-case start to the summary", title: "feat(cli): Add a flag", keyword: "lower"},
		{name: "a trailing full stop", title: "feat(cli): add a flag.", keyword: "stop"},
		{name: "an empty title", title: ""},
		{name: "an upper-case scope", title: "feat(CLI): add a flag", keyword: "lower"},
		{name: "a summary far over the limit", title: "feat(cli): " + strings.TrimSpace(strings.Repeat("add a flag ", 11))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidPullRequestTitle(tc.title)
			if err == nil {
				t.Fatalf("ValidPullRequestTitle(%q) = nil, want an error", tc.title)
			}
			if tc.keyword != "" && !strings.Contains(strings.ToLower(err.Error()), tc.keyword) {
				t.Errorf("error %q does not mention %q", err, tc.keyword)
			}
		})
	}
}
