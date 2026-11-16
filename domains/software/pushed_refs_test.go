package software

import (
	"os"
	"os/exec"
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

// Phase 40 step 3, review blocker: other actors move refs on origin while a
// coding action runs. The owner merges a pull request, so main moves and, with
// delete_branch_on_merge, its head branch is deleted; other streams and
// dependabot push their own branches. A ref move counts against the action
// only when the action could have made it:
//
//   - an undeclared ref created or updated counts only when its new SHA is a
//     commit the action's worktree holds (cat-file -e <sha>^{commit}) and no
//     ref origin had before the action already reached it;
//   - an undeclared ref deleted counts only when the pre-push hook installed
//     for the delegated worktree logged that ref (best effort: --no-verify
//     skips the hook).
//
// The after read is `git ls-remote` (remoteRefs), which brings in no objects,
// so the existence check sees only what the action itself made or fetched.
// Nothing between the run and that check may fetch an undeclared ref into the
// worktree: containsTip fetches, but only a declared ref whose new tip is not
// local, and a fetch of anything else would bring in another actor's commit
// and make it look like the action's.

// pushedRefsOtherActor is a second clone of the fixture's origin, standing for
// the owner, another stream or dependabot. The action's worktree never fetches
// from it, so its commits are objects the worktree does not hold.
func pushedRefsOtherActor(t *testing.T, f pushedRefsFixture) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "other-actor")
	gitIn(t, filepath.Dir(dir), "clone", f.origin, dir)
	return dir
}

// pushedRefsMergedHead is a second undeclared branch on origin, for another
// actor to delete in the same run as the action deletes pushedRefsOther.
const pushedRefsMergedHead = "karakuri/ux-2026-11-15-merged"

// (a) Refs another actor moved during the run are not the action's: the
// action passes, carries no bounds violation, and its worktree never received
// the other actor's commits.
func TestDelegatedActionIsNotBlamedForARefAnotherActorMoved(t *testing.T) {
	cases := []struct {
		name string
		// other runs in the second clone during the action and returns the
		// commit it pushed, or "" when it pushed none the worktree lacks.
		other func(t *testing.T, f pushedRefsFixture, other string) string
	}{
		{
			// The owner merges a pull request: main moves to a commit the
			// worktree never had, and the merged head branch is deleted.
			name: "owner merges a pull request and its head branch is deleted",
			other: func(t *testing.T, _ pushedRefsFixture, other string) string {
				sha := commitIn(t, other, "merged.txt")
				gitIn(t, other, "push", "origin", "HEAD:refs/heads/main")
				gitIn(t, other, "push", "origin", "--delete", pushedRefsOther)
				return sha
			},
		},
		{
			// Another stream's pass, or dependabot, creates its own branch.
			name: "another actor creates its own branch",
			other: func(t *testing.T, _ pushedRefsFixture, other string) string {
				sha := commitIn(t, other, "bump.txt")
				gitIn(t, other, "push", "origin", "HEAD:refs/heads/dependabot/go_modules/example-1.2.3")
				return sha
			},
		},
		{
			// The worktree holds this commit, but origin already reached it
			// before the run, so the action did not bring it there.
			name: "another actor tags a commit origin already had",
			other: func(t *testing.T, f pushedRefsFixture, other string) string {
				gitIn(t, other, "tag", "v0.1.0-alpha.1", f.mainSHA)
				gitIn(t, other, "push", "origin", "refs/tags/v0.1.0-alpha.1")
				return ""
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPushedRefsFixture(t)
			other := pushedRefsOtherActor(t, f)
			var otherSHA string
			res := actWith(t, f.worktree, salvageAction(f.worktree), func(t *testing.T, wt string) error {
				commitIn(t, wt, "step.txt")
				gitIn(t, wt, "push", "origin", "HEAD:refs/heads/"+salvageItemBranch)
				otherSHA = tc.other(t, f, other)
				return nil
			})

			if !res.Success {
				t.Errorf("the action failed for a ref another actor moved: %q (origin now %v)", res.Error, f.originRefs(t))
			}
			if res.BoundsViolation != nil {
				t.Errorf("BoundsViolation = %+v, want none: another actor moved those refs, and blocking the stream for them stops it for something it did not do",
					res.BoundsViolation)
			}
			if otherSHA != "" {
				if err := exec.Command("git", "-C", f.worktree, "cat-file", "-e", otherSHA+"^{commit}").Run(); err == nil {
					t.Errorf("the worktree holds the other actor's commit %s after the check; nothing may fetch it in, or it reads as the action's", otherSHA)
				}
			}
		})
	}
}

// (b) A branch the action pushed its own commit to is still a violation, and
// a ref another actor moved in the same run is not named beside it.
func TestDelegatedActionStillFailsForAnUndeclaredBranchItPushed(t *testing.T) {
	const notMine = "karakuri/ux-not-mine"
	cases := []struct {
		name  string
		other bool // another actor moves main during the run
	}{
		{name: "alone"},
		{name: "while another actor moves main", other: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPushedRefsFixture(t)
			other := pushedRefsOtherActor(t, f)
			var ranSHA string
			res := actWith(t, f.worktree, salvageAction(f.worktree), func(t *testing.T, wt string) error {
				ranSHA = commitIn(t, wt, "elsewhere.txt")
				gitIn(t, wt, "push", "origin", "HEAD:refs/heads/"+notMine)
				if tc.other {
					commitIn(t, other, "merged.txt")
					gitIn(t, other, "push", "origin", "HEAD:refs/heads/main")
				}
				return nil
			})

			if res.Success {
				t.Fatalf("a run that pushed its own commit to %s passed (origin now %v)", notMine, f.originRefs(t))
			}
			if res.BoundsViolation == nil {
				t.Fatalf("a run that pushed its own commit to %s carried no BoundsViolation (Error %q)", notMine, res.Error)
			}
			if want := []string{"refs/heads/" + notMine}; !slices.Equal(res.BoundsViolation.Refs, want) {
				t.Errorf("BoundsViolation.Refs = %v, want exactly %v: the ref the action moved, not main, which another actor moved",
					res.BoundsViolation.Refs, want)
			}
			if !strings.Contains(res.Error, notMine) || !strings.Contains(res.Error, ranSHA) {
				t.Errorf("Error = %q, want it to name %s and the action's commit %s", res.Error, notMine, ranSHA)
			}
		})
	}
}

// (c) A branch the action deleted with a plain `git push origin --delete`
// from its worktree is a violation: the pre-push hook the pack installs for
// the delegated worktree logs the deleted ref. Another actor's deletion in the
// same run reaches no hook of this action's and is not named beside it.
func TestDelegatedActionFailsForAnUndeclaredBranchItDeleted(t *testing.T) {
	cases := []struct {
		name  string
		other bool // another actor deletes pushedRefsMergedHead during the run
	}{
		{name: "alone"},
		{name: "while another actor deletes a branch", other: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPushedRefsFixture(t)
			gitIn(t, f.clone, "push", "origin", "HEAD:refs/heads/"+pushedRefsMergedHead)
			other := pushedRefsOtherActor(t, f)
			res := actWith(t, f.worktree, salvageAction(f.worktree), func(t *testing.T, wt string) error {
				gitIn(t, wt, "push", "origin", "--delete", pushedRefsOther)
				if tc.other {
					gitIn(t, other, "push", "origin", "--delete", pushedRefsMergedHead)
				}
				return nil
			})

			if res.Success {
				t.Fatalf("a run that deleted %s passed (origin now %v)", pushedRefsOther, f.originRefs(t))
			}
			if res.BoundsViolation == nil {
				t.Fatalf("a run that deleted %s carried no BoundsViolation (Error %q)", pushedRefsOther, res.Error)
			}
			if want := []string{"refs/heads/" + pushedRefsOther}; !slices.Equal(res.BoundsViolation.Refs, want) {
				t.Errorf("BoundsViolation.Refs = %v, want exactly %v: the ref the action's own push deleted, not one another actor deleted",
					res.BoundsViolation.Refs, want)
			}
			if !strings.Contains(res.Error, pushedRefsOther) || !strings.Contains(res.Error, f.otherSHA) {
				t.Errorf("Error = %q, want it to name %s and its old SHA %s", res.Error, pushedRefsOther, f.otherSHA)
			}
			// The hook's log lives outside the worktree, so the run leaves no
			// file behind for the next pass to commit.
			if status := gitIn(t, f.worktree, "status", "--porcelain", "--ignored"); status != "" {
				t.Errorf("worktree status = %q after the check, want clean: the push log must live outside the worktree", status)
			}
		})
	}
}
