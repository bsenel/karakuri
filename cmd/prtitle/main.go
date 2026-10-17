// prtitle checks one pull request title against the form the software pack
// requires, and is what .github/workflows/pr-title.yml runs. It exists so the
// workflow calls software.ValidPullRequestTitle instead of repeating the
// pattern in shell, where the two would drift.
//
// Usage:
//
//	PR_TITLE='feat(cli): add a flag' go run ./cmd/prtitle
//	go run ./cmd/prtitle 'feat(cli): add a flag'
//
// The workflow uses the environment form: a title is text a stranger wrote,
// and it must never be put on a command line there. The argument is for
// trying a title by hand before opening the pull request.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/bsenel/karakuri/domains/software"
)

const help = `
The title becomes the squash commit on main, and the version is derived from it.
Types: feat (minor), fix, perf (patch), docs, refactor, test, build, ci, chore (no release).
Put ! before the colon for a breaking change. Examples:

  feat(mcp): attach karakuri tools to delegated agent runs (phase 34)
  fix(cli): shift audit export window to the requested hours (EB-003)
  docs(roadmap): propose phases 41 to 43 (discovery 2026-10-18)

Edit the title; the check reruns without a new commit.
`

func main() {
	os.Exit(run(os.Args[1:], os.Getenv("PR_TITLE"), os.Stderr))
}

func run(args []string, envTitle string, stderr io.Writer) int {
	title := envTitle
	if len(args) > 0 {
		title = args[0]
	}
	if err := software.ValidPullRequestTitle(title); err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprint(stderr, help)
		return 1
	}
	return 0
}
