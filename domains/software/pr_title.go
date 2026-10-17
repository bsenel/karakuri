package software

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// prTitleForm is shown in every refusal: whoever wrote the title needs the
// form, not only the news that theirs is wrong.
const prTitleForm = "<type>(<scope>): <summary>"

// prTitleSummaryMax keeps the squash commit's first line readable in
// `git log --oneline` once GitHub appends " (#123)".
const prTitleSummaryMax = 72

// prTitleTypes is what a title may start with, and so what a release is
// derived from: feat is a minor version, fix and perf a patch, the rest no
// release. `!` before the colon makes any of them a major.
var prTitleTypes = map[string]bool{
	"feat": true, "fix": true, "perf": true,
	"docs": true, "refactor": true, "test": true,
	"build": true, "ci": true, "chore": true,
}

// prTitleDependabotTypes is an explicit allow-list, not more types to choose
// from: Dependabot titles its pull requests `gomod(deps): ...` and
// `npm(deps): ...`, and those are already on main. `actions` is the third
// prefix .github/dependabot.yml configures, and it is on main as well (#151),
// so it is listed with them. Nobody writes these by hand, so the two checks on
// how the summary is written are relaxed for them, here and nowhere else:
// Dependabot has started one with "Bump" (#23) and does not bound its length.
var prTitleDependabotTypes = map[string]bool{"gomod": true, "npm": true, "actions": true}

var (
	prTitlePattern = regexp.MustCompile(`^([A-Za-z]+)(?:\(([^()]*)\))?(!?): (.*)$`)
	prTitleScope   = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*$`)
)

// ValidPullRequestTitle reports why title is not a Conventional Commits
// header, or nil when it is one. This repository squash-merges, so the title
// becomes the commit on main, and the version is derived from those commits.
//
// It is the only decider: .github/workflows/pr-title.yml runs it through
// cmd/prtitle rather than repeating the pattern in shell.
func ValidPullRequestTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return prTitleError("the title is empty")
	}
	m := prTitlePattern.FindStringSubmatch(title)
	if m == nil {
		return prTitleError("%q does not start with a type, an optional scope in parentheses, and a colon followed by one space", title)
	}
	typ, scope, summary := m[1], m[2], m[4]
	hasScope := strings.Contains(title[:len(title)-len(summary)], "(")

	dependabot := prTitleDependabotTypes[typ]
	if !prTitleTypes[typ] && !dependabot {
		return prTitleError("%q is not a type; use one of feat, fix, perf, docs, refactor, test, build, ci, chore", typ)
	}
	if hasScope && !prTitleScope.MatchString(scope) {
		return prTitleError("the scope %q must be the area in lower case, such as cli or mcp", scope)
	}
	if summary == "" || summary != strings.TrimSpace(summary) {
		return prTitleError("the summary must follow the colon after exactly one space, with none trailing")
	}
	if strings.HasSuffix(summary, ".") {
		return prTitleError("the summary must not end with a full stop")
	}
	if dependabot {
		return nil
	}
	if first, _ := utf8.DecodeRuneInString(summary); unicode.IsUpper(first) {
		return prTitleError("the summary must start in lower case, got %q", string(first))
	}
	if n := utf8.RuneCountInString(summary); n > prTitleSummaryMax {
		return prTitleError("the summary is %d characters, at most %d", n, prTitleSummaryMax)
	}
	return nil
}

func prTitleError(format string, args ...any) error {
	return fmt.Errorf("pull request title: "+format+"; the form is "+prTitleForm, args...)
}
