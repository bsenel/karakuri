package software

import (
	"context"
	"slices"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// Phase 40 step 6a (ii): "a plan that rebases or force-pushes is refused".
// Step 2's limits admit `Bash(git:*)` and cannot read a git subcommand, so the
// enforcement that is real is the claude CLI's deny list, which takes
// precedence over its allow list: every coding delegation carries deny entries
// for a rebase and a force push, whatever tools the stream's brief allows.
// Chosen over a new limit in step 2 because the limits match whole entries,
// and the entry that admits a rewrite is the broad `Bash(git:*)` every stream
// needs.

var gitRewriteDenied = []string{
	"Bash(git rebase:*)",
	"Bash(git push --force:*)",
	"Bash(git push -f:*)",
	"Bash(git push --force-with-lease:*)",
	"Bash(git push origin +*)",
}

func TestEveryStreamsDelegationDeniesRebaseAndForcePush(t *testing.T) {
	for _, b := range streamBriefs() {
		for _, id := range codingCapabilities {
			t.Run(b.templateID+"/"+string(id), func(t *testing.T) {
				cli := &recordingCLI{}
				env := &cliEnv{id: "software.env.cli_agent", cli: cli}
				a := codingAction(id, b.tools, b.branches[0])
				a.Params["worktree_path"] = delegateAction(t).Params["worktree_path"]
				res, err := env.Act(context.Background(), a)
				if err != nil {
					t.Fatalf("act: %v", err)
				}
				if !res.Success {
					t.Fatalf("delegation failed: %s", res.Error)
				}
				if len(cli.calls) != 1 {
					t.Fatalf("adapter calls = %d, want 1", len(cli.calls))
				}
				got := cli.calls[0].DisallowedTools
				for _, want := range gitRewriteDenied {
					if !slices.Contains(got, want) {
						t.Errorf("DisallowedTools lacks %q: %q", want, got)
					}
				}
				if !slices.Contains(cli.calls[0].AllowedTools, "Bash(git:*)") {
					t.Errorf("the brief's Bash(git:*) was dropped: %q", cli.calls[0].AllowedTools)
				}
			})
		}
	}
}

// Step 2's limits already refuse a plan that lists a rebase or force-push entry
// of its own, because the entry is not one of the stream's exact entries. This
// keeps that true; the violation's Value names the entry.
func TestStreamLimitsRefuseARebaseOrForcePushToolEntry(t *testing.T) {
	entries := append([]string{"Bash(git rebase -i main)", "Bash(git push origin +main)"}, gitRewriteDenied...)
	for _, b := range streamBriefs() {
		bounds := templateBounds(streamTemplate(t, b.templateID))
		for _, id := range codingCapabilities {
			for _, entry := range entries {
				a := codingAction(id, append(slices.Clone(b.tools), entry), b.branches[0])
				if !violates(bounds.CheckActionLimits([]environment.Action{a}), "allowed_tools", entry) {
					t.Errorf("%s/%s: %q is inside the limits", b.templateID, id, entry)
				}
			}
		}
	}
}
