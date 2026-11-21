package software

// Phase 40 step 11: when a review is asked for with a worktree and no body,
// the body is composed from commands run in that worktree, so what it says it
// ran is what ran and the output it quotes is the output it got.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bsenel/karakuri/internal/core/environment"
)

const (
	reviewBuild     = "go build ./..."
	reviewChangedCh = "go run ./cmd/krk bogus"
	reviewOutputMax = 1500
)

type reviewRunResult struct {
	cmd      string
	exitCode int
	output   string
}

func runReviewCommand(ctx context.Context, workdir, cmd string) (reviewRunResult, error) {
	res, err := newShellEnv("review-shell", workdir, 0).Act(ctx, environment.Action{
		CapabilityID: "software.act.shell_exec",
		Params:       map[string]any{"cmd": cmd, "workdir": workdir, "timeout_sec": 300},
	})
	if err != nil {
		return reviewRunResult{}, fmt.Errorf("run %q: %w", cmd, err)
	}
	code, _ := res.StateDelta["exit_code"].(int)
	out, _ := res.StateDelta["stdout"].(string)
	errOut, _ := res.StateDelta["stderr"].(string)
	out = strings.TrimSpace(out + errOut)
	if len(out) > reviewOutputMax {
		out = out[:reviewOutputMax] + "\n..."
	}
	return reviewRunResult{cmd: cmd, exitCode: code, output: out}, nil
}

func (r reviewRunResult) section() string {
	return fmt.Sprintf("- `%s` exited %d\n\n```\n%s\n```\n", r.cmd, r.exitCode, r.output)
}

// composeReviewBody builds the branch in workdir and, when it has a krk
// command, runs one that must be refused (an unknown command has to exit
// non-zero). It says what it did not run, since the rest is not verifiable here.
func composeReviewBody(ctx context.Context, workdir string) (string, error) {
	var ran, problems strings.Builder
	notRan := []string{"tests, lint and any binary other than the unknown-command check", "behaviour against a system that is not installed here", "how a web page looks"}

	build, err := runReviewCommand(ctx, workdir, reviewBuild)
	if err != nil {
		return "", err
	}
	ran.WriteString(build.section())
	if build.exitCode != 0 {
		fmt.Fprintf(&problems, "- the branch does not build: `%s` exited %d\n", build.cmd, build.exitCode)
		notRan = append([]string{"the binaries, since the build failed"}, notRan...)
	} else if info, statErr := os.Stat(filepath.Join(workdir, "cmd", "krk")); statErr == nil && info.IsDir() {
		bogus, err := runReviewCommand(ctx, workdir, reviewChangedCh)
		if err != nil {
			return "", err
		}
		ran.WriteString(bogus.section())
		if bogus.exitCode == 0 {
			fmt.Fprintf(&problems, "- `%s` exited 0; an unknown command should exit non-zero\n", bogus.cmd)
		}
	}

	var b strings.Builder
	b.WriteString("## What was run\n\n")
	b.WriteString(ran.String())
	b.WriteString("\n## What was not run\n\n")
	for _, n := range notRan {
		b.WriteString("- " + n + "\n")
	}
	if problems.Len() > 0 {
		b.WriteString("\n## Findings\n\n" + problems.String())
	}
	return b.String(), nil
}
