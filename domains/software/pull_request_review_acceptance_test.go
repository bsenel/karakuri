package software

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 11 acceptance (docs/roadmap.md): against a fixture repository
// with a pull request that breaks the build and one whose changed command
// exits with the wrong code, the pass posts a review naming each finding with
// the command run and its output; on a clean pull request it posts what it
// ran and no finding; a plan to push is refused by its limits.
//
// The push refusal is already proved for this template by
// TestPullRequestReviewRefusesAnyPush and TestPullRequestReviewShellCannotPushOrMerge
// (pull_request_review_test.go), so it is not repeated here.
//
// A scripted planner cannot show the rest: it writes whatever body it is told
// to, so a review that "names the command and its output" would only prove the
// script. These tests run the commands for real in a worktree of each branch
// and ask the product to post the review, with nothing from the planner but
// the pull request's branch.

const reviewFixtureGoMod = "module fixture\n\ngo 1.22\n"

const reviewFixtureMain = `package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "status" {
		fmt.Fprintln(os.Stderr, "krk: unknown command")
		os.Exit(2)
	}
	fmt.Println("ok")
}
`

// reviewBranches are the three branches of the fixture, each a change to
// cmd/krk/main.go on top of the clean base.
var reviewBranches = map[string]string{
	"breaks-build": strings.Replace(reviewFixtureMain, `fmt.Println("ok")`, `fmt.Println(Frobnicate())`, 1),
	"wrong-exit":   strings.Replace(reviewFixtureMain, "os.Exit(2)", "os.Exit(0)", 1),
	"clean":        strings.Replace(reviewFixtureMain, `"ok"`, `"ok, still"`, 1),
}

// reviewFixture builds the repository and one worktree per branch, as the
// reviewer's own worktree at the pull request's head would be.
func reviewFixture(t *testing.T) map[string]string {
	t.Helper()
	isolateGit(t)
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	cmdDir := filepath.Join(repo, "cmd", "krk")
	if err := os.MkdirAll(cmdDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "go.mod"), reviewFixtureGoMod)
	writeFile(t, filepath.Join(cmdDir, "main.go"), reviewFixtureMain)
	gitIn(t, repo, "init", "-q", "-b", "main")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-q", "-m", "base")
	trees := map[string]string{}
	for name, src := range reviewBranches {
		tree := filepath.Join(root, "wt-"+name)
		gitIn(t, repo, "worktree", "add", "-q", "-b", name, tree, "main")
		writeFile(t, filepath.Join(tree, "cmd", "krk", "main.go"), src)
		gitIn(t, tree, "commit", "-q", "-am", name)
		trees[name] = tree
	}
	return trees
}

// reviewRun is one command run in a worktree through the shell capability.
type reviewRun struct {
	cmd      string
	exitCode int
	output   string
}

func runInWorktree(t *testing.T, tree, cmd string) reviewRun {
	t.Helper()
	res, err := newShellEnv("shell", tree, 0).Act(context.Background(), environment.Action{
		CapabilityID: "software.act.shell_exec", Params: map[string]any{"cmd": cmd, "timeout_sec": 120},
	})
	if err != nil {
		t.Fatalf("shell_exec %q: %v", cmd, err)
	}
	code, _ := res.StateDelta["exit_code"].(int)
	out, _ := res.StateDelta["stdout"].(string)
	errOut, _ := res.StateDelta["stderr"].(string)
	return reviewRun{cmd: cmd, exitCode: code, output: out + errOut}
}

// What a reviewer must run on each branch, and what the fixture says it sees.
const (
	reviewBuildCmd = "go build ./..."
	reviewBogusCmd = "go run ./cmd/krk bogus"
)

// The fixture itself: the commands behave as the tests below depend on, so a
// failure there is the product's and not the fixture's.
func TestPullRequestReviewAcceptanceFixtureBehavesAsDescribed(t *testing.T) {
	trees := reviewFixture(t)
	if r := runInWorktree(t, trees["breaks-build"], reviewBuildCmd); r.exitCode == 0 || !strings.Contains(r.output, "Frobnicate") {
		t.Errorf("breaks-build: %+v, want a failing build naming Frobnicate", r)
	}
	if r := runInWorktree(t, trees["wrong-exit"], reviewBogusCmd); r.exitCode != 0 {
		t.Errorf("wrong-exit: `krk bogus` exit code %d, want the wrong code 0", r.exitCode)
	}
	if r := runInWorktree(t, trees["clean"], reviewBuildCmd); r.exitCode != 0 {
		t.Errorf("clean: build %+v, want success", r)
	}
	if r := runInWorktree(t, trees["clean"], reviewBogusCmd); r.exitCode == 0 {
		t.Errorf("clean: `krk bogus` exits 0, want a non-zero code")
	}
}

// The limits say the reviewer may "run the binaries in its worktree"; with the
// build and test commands only, it cannot run the changed command, so it could
// never find that `krk bogus` exits 0.
func TestPullRequestReviewAcceptanceMayRunTheChangedCommand(t *testing.T) {
	v := decideReview(reviewBounds(t), reviewAction("software.act.shell_exec", map[string]any{"cmd": reviewBogusCmd, "workdir": "."}))
	if v.Refuse {
		t.Fatalf("the reviewer's limits refuse %q (%s): reviewShellCommands holds only build, test and lint commands, "+
			"so the reviewer cannot run a binary the pull request changes and the wrong-exit-code finding cannot be made", reviewBogusCmd, v.Reason)
	}
}

// Each broken branch gets a review naming the finding with the command and its
// output; the clean one says what was run and has no finding. The review is
// requested with the branch alone: the body must come from the results of the
// runs against the worktree, not from the planner's text.
func TestPullRequestReviewAcceptanceReviewNamesCommandAndOutput(t *testing.T) {
	trees := reviewFixture(t)
	for _, tc := range []struct {
		branch   string
		cmd      string
		finding  bool
		wantText string // output the review must quote
	}{
		{"breaks-build", reviewBuildCmd, true, "Frobnicate"},
		{"wrong-exit", reviewBogusCmd, true, "exit"},
		{"clean", reviewBuildCmd, false, ""},
	} {
		t.Run(tc.branch, func(t *testing.T) {
			run := runInWorktree(t, trees[tc.branch], tc.cmd)
			failed := (tc.branch == "breaks-build" && run.exitCode != 0) || (tc.branch == "wrong-exit" && run.exitCode == 0)
			if failed != tc.finding {
				t.Fatalf("fixture: %+v does not show the expected finding=%v", run, tc.finding)
			}
			forge := &fakeForge{prs: []versioncontrol.PRSummary{draftPR("body")}}
			// The worktree and the runs are all the product has; no body.
			res := actPostReview(t, forge, map[string]any{"branch": streamBranch, "repo": "acme/widgets", "workdir": trees[tc.branch]})

			if !res.Success || len(forge.reviews) != 1 {
				t.Fatalf("software.act.post_review given a branch and its worktree and no body: success=%v error=%q reviews=%d. "+
					"The product has no code that composes a review body from the results of software.act.shell_exec or "+
					"software.verify.run_tests (postReview takes params.body verbatim from the planner), so a review "+
					"that names the command run and its output exists only if the planner writes it", res.Success, res.Error, len(forge.reviews))
			}
			body := forge.reviews[0].body
			if !strings.Contains(body, tc.cmd) {
				t.Errorf("the review does not name the command run %q:\n%s", tc.cmd, body)
			}
			if tc.wantText != "" && !strings.Contains(body, tc.wantText) {
				t.Errorf("the review does not quote the output %q:\n%s", tc.wantText, body)
			}
			if has := strings.Contains(strings.ToLower(body), "finding"); has != tc.finding {
				t.Errorf("finding in review = %v, want %v:\n%s", has, tc.finding, body)
			}
		})
	}
}
