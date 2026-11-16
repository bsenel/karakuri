package software

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Phase 40 step 3, slice A: what was pushed is checked, not assumed. The tool
// allow-list bounds what a coding agent is told it may run, but the push
// happens inside its subprocess. Around a delegated coding action the software
// pack reads origin's refs before and after; a ref that moved (created, updated
// or deleted) outside the action's declared refs, which are params.branch
// (already held to the stream template's patterns by the decide step) and the
// pack's own salvage branch for this action, fails the action with a bounds
// violation naming the ref and its old and new SHA. A declared branch moved to
// a commit that does not contain its previous tip (a force push) is a
// violation too. This detects; it does not prevent.

// pushedRefsOther is a branch on origin that no action in these tests declares.
const pushedRefsOther = "karakuri/phase-40-someone-elses-group"

// pushedRefsFixture is the salvage fixture with one more branch on origin, so
// an undeclared update and an undeclared delete each have a ref to move.
type pushedRefsFixture struct {
	salvageFixture
	mainSHA  string
	otherSHA string
}

func newPushedRefsFixture(t *testing.T) pushedRefsFixture {
	t.Helper()
	f := pushedRefsFixture{salvageFixture: newSalvageFixture(t)}
	f.mainSHA = gitIn(t, f.clone, "rev-parse", "HEAD")
	gitIn(t, f.clone, "push", "origin", "HEAD:refs/heads/"+pushedRefsOther)
	f.otherSHA = f.mainSHA
	return f
}

// commitIn commits one new file in the worktree and returns the commit's SHA.
func commitIn(t *testing.T, wt, name string) string {
	t.Helper()
	writeFile(t, filepath.Join(wt, name), name+"\n")
	gitIn(t, wt, "add", name)
	gitIn(t, wt, "commit", "-m", "agent: "+name)
	return gitIn(t, wt, "rev-parse", "HEAD")
}

// A delegated run that pushes only to its declared branch, or to nothing,
// passes; one that moves any other ref on origin, or rewrites its own branch,
// fails with a bounds violation that names the ref and both SHAs.
func TestDelegatedActionFailsWhenItPushedOutsideItsDeclaredRefs(t *testing.T) {
	const newBranch = "karakuri/phase-40-step-03-new-group"
	type want struct {
		ref    string // empty: the action passes
		oldSHA func(f pushedRefsFixture) string
		newSHA func(f pushedRefsFixture, ran string) string
	}
	ran := func(_ pushedRefsFixture, sha string) string { return sha }
	cases := []struct {
		name   string
		branch string // the action's params.branch; empty means the item branch
		run    func(t *testing.T, f pushedRefsFixture, wt string) string
		want   want
	}{
		{
			name: "pushes only to its declared branch",
			run: func(t *testing.T, _ pushedRefsFixture, wt string) string {
				sha := commitIn(t, wt, "step.txt")
				gitIn(t, wt, "push", "origin", "HEAD:refs/heads/"+salvageItemBranch)
				return sha
			},
		},
		{
			// A first push of a new group: the declared ref had no tip to keep.
			name:   "creates its declared branch",
			branch: newBranch,
			run: func(t *testing.T, _ pushedRefsFixture, wt string) string {
				sha := commitIn(t, wt, "group.txt")
				gitIn(t, wt, "push", "origin", "HEAD:refs/heads/"+newBranch)
				return sha
			},
		},
		{
			name: "pushes nothing",
			run: func(t *testing.T, _ pushedRefsFixture, wt string) string {
				return commitIn(t, wt, "local-only.txt")
			},
		},
		{
			name: "pushes to main",
			run: func(t *testing.T, _ pushedRefsFixture, wt string) string {
				sha := commitIn(t, wt, "onto-main.txt")
				gitIn(t, wt, "push", "origin", "HEAD:refs/heads/main")
				return sha
			},
			want: want{
				ref:    "main",
				oldSHA: func(f pushedRefsFixture) string { return f.mainSHA },
				newSHA: ran,
			},
		},
		{
			name: "creates an undeclared branch",
			run: func(t *testing.T, _ pushedRefsFixture, wt string) string {
				sha := commitIn(t, wt, "elsewhere.txt")
				gitIn(t, wt, "push", "origin", "HEAD:refs/heads/karakuri/ux-not-mine")
				// The declared push alongside it does not excuse the other.
				gitIn(t, wt, "push", "origin", "HEAD:refs/heads/"+salvageItemBranch)
				return sha
			},
			want: want{ref: "karakuri/ux-not-mine", newSHA: ran},
		},
		{
			name: "deletes an undeclared branch",
			run: func(t *testing.T, _ pushedRefsFixture, wt string) string {
				gitIn(t, wt, "push", "origin", ":refs/heads/"+pushedRefsOther)
				return ""
			},
			want: want{ref: pushedRefsOther, oldSHA: func(f pushedRefsFixture) string { return f.otherSHA }},
		},
		{
			// The item branch's previous tip is no longer in its history.
			name: "force-rewrites its declared branch",
			run: func(t *testing.T, _ pushedRefsFixture, wt string) string {
				writeFile(t, filepath.Join(wt, "item.txt"), "rewritten\n")
				gitIn(t, wt, "commit", "--amend", "-a", "-m", "item commit, rewritten")
				gitIn(t, wt, "push", "--force", "origin", "HEAD:refs/heads/"+salvageItemBranch)
				return gitIn(t, wt, "rev-parse", "HEAD")
			},
			want: want{
				ref:    salvageItemBranch,
				oldSHA: func(f pushedRefsFixture) string { return f.itemSHA },
				newSHA: ran,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPushedRefsFixture(t)
			action := salvageAction(f.worktree)
			if tc.branch != "" {
				action.Params["branch"] = tc.branch
			}
			var ranSHA string
			res := actWith(t, f.worktree, action, func(t *testing.T, wt string) error {
				ranSHA = tc.run(t, f, wt)
				return nil
			})

			if tc.want.ref == "" {
				if !res.Success {
					t.Fatalf("a run that stayed inside its declared refs failed: %q (delta %v)", res.Error, res.StateDelta)
				}
				if res.BoundsViolation != nil {
					t.Errorf("BoundsViolation = %+v, want none for a run inside its declared refs", res.BoundsViolation)
				}
				return
			}
			if res.Success {
				t.Fatalf("a run that moved %s passed; want a failed result with a bounds violation (origin now %v)",
					tc.want.ref, f.originRefs(t))
			}
			// The loop records the violation, and reconcile blocks the stream,
			// on this typed field, never on Error's text.
			if res.BoundsViolation == nil {
				t.Fatalf("a run that moved %s carried no BoundsViolation, so the loop records none and the stream is not blocked (Error %q)",
					tc.want.ref, res.Error)
			}
			if !slices.Contains(res.BoundsViolation.Refs, "refs/heads/"+tc.want.ref) {
				t.Errorf("BoundsViolation.Refs = %v, want it to name refs/heads/%s", res.BoundsViolation.Refs, tc.want.ref)
			}
			if !strings.Contains(strings.ToLower(res.Error), "bounds violation") {
				t.Errorf("Error = %q, want it to name a bounds violation", res.Error)
			}
			if !strings.Contains(res.Error, tc.want.ref) {
				t.Errorf("Error = %q, want it to name the ref %s", res.Error, tc.want.ref)
			}
			if tc.want.oldSHA != nil {
				if old := tc.want.oldSHA(f); !strings.Contains(res.Error, old) {
					t.Errorf("Error = %q, want the ref's old SHA %s", res.Error, old)
				}
			}
			if tc.want.newSHA != nil {
				if moved := tc.want.newSHA(f, ranSHA); !strings.Contains(res.Error, moved) {
					t.Errorf("Error = %q, want the ref's new SHA %s", res.Error, moved)
				}
			}
		})
	}
}

// The salvage branch a failed run's edits are kept on is the pack's own push,
// declared for every coding action: a failed run that is salvaged keeps the
// run's own error and is not reported as a bounds violation.
func TestDelegatedActionSalvageIsNotABoundsViolation(t *testing.T) {
	f := newPushedRefsFixture(t)

	res := actWith(t, f.worktree, salvageAction(f.worktree), func(t *testing.T, wt string) error {
		writeFile(t, filepath.Join(wt, "unfinished.txt"), "left uncommitted\n")
		return errExit1
	})

	if res.Success {
		t.Fatal("a failed run must stay a failed result")
	}
	if res.BoundsViolation != nil {
		t.Errorf("BoundsViolation = %+v, want none for the pack's own salvage push", res.BoundsViolation)
	}
	if res.Error != errExit1.Error() {
		t.Errorf("Error = %q, want the delegate's own %q and no bounds violation for the salvage push", res.Error, errExit1.Error())
	}
	if got := res.StateDelta["salvage_branch"]; got != salvageWantBranch {
		t.Errorf("salvage_branch = %v, want %q (delta %v)", got, salvageWantBranch, res.StateDelta)
	}
}

// A remote whose refs cannot be read is not a check that passed (AGENTS.md
// rule 10): the action is not a success, and its error says why.
func TestDelegatedActionWithAnUnreadableRemoteIsNotAPass(t *testing.T) {
	f := newPushedRefsFixture(t)
	gone := filepath.Join(t.TempDir(), "gone.git")
	gitIn(t, f.worktree, "remote", "set-url", "origin", gone)
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Fatalf("fixture: %s should not exist (%v)", gone, err)
	}

	res := actWith(t, f.worktree, salvageAction(f.worktree), func(*testing.T, string) error {
		return nil
	})

	if res.Success {
		t.Fatalf("origin could not be read and the action passed; want a failed result (delta %v)", res.StateDelta)
	}
	if strings.TrimSpace(res.Error) == "" {
		t.Error("Error is empty, want the reason origin's refs could not be read")
	}
	// Not known is not a violation: only a ref seen to move blocks the stream.
	if res.BoundsViolation != nil {
		t.Errorf("BoundsViolation = %+v, want none when origin could not be read", res.BoundsViolation)
	}
}
