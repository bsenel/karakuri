package software

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	gitlib "github.com/go-git/go-git/v5"

	"github.com/bsenel/karakuri/internal/core/environment"
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

// Phase 40 step 3, review of ecfae91: an action's worktree is made with `git
// worktree add` from the deployment's main repository, so every worktree of
// that repository shares ONE object store. A fetch by anyone sharing it during
// the run (the action's own `git fetch` or `git pull --rebase` before it
// pushes, another stream's worktree, the operator's `git fetch origin`) brings
// the owner's merge commit into the store, and "the worktree holds the commit"
// stops meaning "the action made it". A create or update of an undeclared ref
// is the action's only from evidence local to THIS worktree:
//
//   - the pre-push log installed for the worktree, which records every remote
//     ref a push sends (create, update and delete), and
//   - when the hook was skipped (--no-verify), the worktree's own HEAD reflog
//     (`git -C <worktree> log -g --format=%H HEAD`, kept per worktree): the
//     action made that commit here (a checkout or reset onto it is not
//     making it; see the third fix below).
//
// A ref moved to a commit neither names is somebody else's.

// sharedWorktreeSibling is the branch of the deployment repository's second
// worktree, standing for another stream's pass.
const sharedWorktreeSibling = "karakuri/ux-2026-11-16-sibling"

// sharedWorktreeFixture is the layout a deployment has: a bare origin, the
// deployment's main repository cloned from it, the action's worktree added
// from that repository on the item branch, and a sibling worktree of the same
// repository. All three checkouts share one object store. owner is a separate
// clone of origin, standing for the owner merging on GitHub.
type sharedWorktreeFixture struct {
	origin     string
	deployment string
	action     string
	sibling    string
	owner      string
}

func newSharedWorktreeFixture(t *testing.T) sharedWorktreeFixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	isolateGit(t)
	root := t.TempDir()
	f := sharedWorktreeFixture{
		origin:     filepath.Join(root, "origin.git"),
		deployment: filepath.Join(root, "deployment"),
		action:     filepath.Join(root, "worktrees", salvageObjectiveID, salvageTaskID),
		sibling:    filepath.Join(root, "worktrees", salvageObjectiveID, "sibling"),
		owner:      filepath.Join(root, "owner"),
	}
	gitIn(t, root, "init", "--bare", "-b", "main", f.origin)
	gitIn(t, root, "clone", f.origin, f.deployment)
	writeFile(t, filepath.Join(f.deployment, "tracked.txt"), "original\n")
	gitIn(t, f.deployment, "add", ".")
	gitIn(t, f.deployment, "commit", "-m", "base")
	gitIn(t, f.deployment, "push", "origin", "HEAD:refs/heads/main")
	gitIn(t, f.deployment, "push", "origin", "HEAD:refs/heads/"+pushedRefsOther)
	gitIn(t, f.deployment, "push", "origin", "HEAD:refs/heads/"+pushedRefsMergedHead)

	if err := os.MkdirAll(filepath.Dir(f.action), 0o750); err != nil {
		t.Fatal(err)
	}
	gitIn(t, f.deployment, "worktree", "add", "-b", salvageItemBranch, f.action, "main")
	commitIn(t, f.action, "item.txt")
	gitIn(t, f.action, "push", "origin", "HEAD:refs/heads/"+salvageItemBranch)
	gitIn(t, f.deployment, "worktree", "add", "-b", sharedWorktreeSibling, f.sibling, "main")
	gitIn(t, root, "clone", f.origin, f.owner)
	return f
}

// checkRun installs the push log on the action's worktree, reads origin's
// refs, runs the scenario and returns what checkPushedRefs reports for an
// action that declared its item branch.
func (f sharedWorktreeFixture) checkRun(t *testing.T, run func(t *testing.T)) *environment.BoundsViolation {
	t.Helper()
	return f.checkRunDeclaring(t, salvageItemBranch, run)
}

// checkRunDeclaring is checkRun for an action that declared branch. The run
// sees the environment installPushLog hands the agent, as a delegated run does.
func (f sharedWorktreeFixture) checkRunDeclaring(t *testing.T, branch string, run func(t *testing.T)) *environment.BoundsViolation {
	t.Helper()
	ctx := context.Background()
	log := installPushLog(ctx, f.action)
	t.Cleanup(func() {
		if err := log.remove(ctx); err != nil {
			t.Errorf("removing the push log: %v", err)
		}
	})
	before, beforeErr := readRemoteRefs(ctx, f.action)
	withEnv(t, log.env(nil), func() { run(t) })
	violation, err := checkPushedRefs(ctx, f.action, declaredRefs(branch, f.action), before, beforeErr, log)
	if err != nil {
		t.Fatalf("checkPushedRefs: %v; want a comparison, not an error", err)
	}
	return violation
}

// ownerMerges stands for the owner merging a pull request on GitHub: a new
// commit lands on origin's main from outside the deployment. It returns it.
func (f sharedWorktreeFixture) ownerMerges(t *testing.T) string {
	t.Helper()
	sha := commitIn(t, f.owner, "merged.txt")
	gitIn(t, f.owner, "push", "origin", "HEAD:refs/heads/main")
	return sha
}

// (a) The owner's merge commit fetched into the shared object store by
// anyone, the action included, is not the action's: main moved, but nothing
// local to the action's worktree names that commit.
func TestPushedRefsSharedWorktreeFetchIsNotTheActionsPush(t *testing.T) {
	cases := []struct {
		name    string
		fetchIn func(f sharedWorktreeFixture) []string
	}{
		{
			name:    "another stream's worktree and the action both fetch",
			fetchIn: func(f sharedWorktreeFixture) []string { return []string{f.sibling, f.action} },
		},
		{
			name:    "the operator fetches in the deployment checkout",
			fetchIn: func(f sharedWorktreeFixture) []string { return []string{f.deployment} },
		},
		{
			name:    "only the action fetches before it pushes",
			fetchIn: func(f sharedWorktreeFixture) []string { return []string{f.action} },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSharedWorktreeFixture(t)
			var mergeSHA string
			violation := f.checkRun(t, func(t *testing.T) {
				mergeSHA = f.ownerMerges(t)
				for _, dir := range tc.fetchIn(f) {
					gitIn(t, dir, "fetch", "origin")
				}
				commitIn(t, f.action, "step.txt")
				gitIn(t, f.action, "push", "origin", "HEAD:refs/heads/"+salvageItemBranch)
			})
			// The fixture must reproduce the defect: the owner's commit is now in
			// the object store the action's worktree reads.
			if err := exec.Command("git", "-C", f.action, "cat-file", "-e", mergeSHA+"^{commit}").Run(); err != nil {
				t.Fatalf("fixture: the action's worktree does not hold the owner's merge %s through the shared object store: %v", mergeSHA, err)
			}
			if violation != nil {
				t.Errorf("BoundsViolation = %+v, want none: main moved to the owner's merge %s, which reached the shared object store by a fetch; neither this worktree's push log nor its HEAD reflog names it, so blocking the stream for it stops it for something it did not do",
					violation, mergeSHA)
			}
		})
	}
}

// (b) and (c) A commit the action made in its own worktree and pushed to an
// undeclared branch is a violation, with or without the pre-push hook, and
// the owner's merge fetched into the shared store in the same run is not
// named beside it.
func TestPushedRefsSharedWorktreeUndeclaredPushIsAViolation(t *testing.T) {
	const notMine = "karakuri/ux-not-mine"
	cases := []struct {
		name     string
		noVerify bool // the push skips the pre-push hook; the HEAD reflog is the evidence
		owner    bool // the owner merges and the sibling worktree fetches during the run
	}{
		{name: "normal push"},
		{name: "normal push while the owner's merge is fetched", owner: true},
		{name: "push with --no-verify", noVerify: true},
		{name: "push with --no-verify while the owner's merge is fetched", noVerify: true, owner: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSharedWorktreeFixture(t)
			var ranSHA string
			violation := f.checkRun(t, func(t *testing.T) {
				if tc.owner {
					f.ownerMerges(t)
					gitIn(t, f.sibling, "fetch", "origin")
				}
				ranSHA = commitIn(t, f.action, "elsewhere.txt")
				args := []string{"push"}
				if tc.noVerify {
					args = append(args, "--no-verify")
				}
				gitIn(t, f.action, append(args, "origin", "HEAD:refs/heads/"+notMine)...)
			})
			how := "through the pre-push log"
			if tc.noVerify {
				how = "through the worktree's HEAD reflog, since --no-verify skipped the hook"
			}
			if violation == nil {
				t.Fatalf("no BoundsViolation for a commit %s the action made in its worktree and pushed to undeclared %s; want one, found %s",
					ranSHA, notMine, how)
			}
			if want := []string{headsPrefix + notMine}; !slices.Equal(violation.Refs, want) {
				t.Errorf("BoundsViolation.Refs = %v, want exactly %v: the ref the action pushed, not main, which the owner moved", violation.Refs, want)
			}
			if !strings.Contains(violation.Reason, notMine) || !strings.Contains(violation.Reason, ranSHA) {
				t.Errorf("Reason = %q, want it to name %s and the action's commit %s", violation.Reason, notMine, ranSHA)
			}
		})
	}
}

// (d) The pre-push log records every remote ref a push sends, not only
// deletions: a create and an update name the ref and the SHA sent, a deletion
// names the ref with "".
func TestPushLogRecordsEveryPushedRef(t *testing.T) {
	const created = "karakuri/ux-not-mine"
	f := newSharedWorktreeFixture(t)
	ctx := context.Background()
	log := installPushLog(ctx, f.action)
	t.Cleanup(func() {
		if err := log.remove(ctx); err != nil {
			t.Errorf("removing the push log: %v", err)
		}
	})

	var createdSHA, updatedSHA string
	withEnv(t, log.env(nil), func() {
		createdSHA = commitIn(t, f.action, "created.txt")
		gitIn(t, f.action, "push", "origin", "HEAD:refs/heads/"+created)
		updatedSHA = commitIn(t, f.action, "updated.txt")
		gitIn(t, f.action, "push", "origin", "HEAD:refs/heads/"+pushedRefsOther)
		gitIn(t, f.action, "push", "origin", "--delete", pushedRefsMergedHead)
	})

	pushed, err := log.pushed()
	if err != nil {
		t.Fatalf("pushed: %v; want the log of what the worktree's pushes sent", err)
	}
	want := map[string]string{
		headsPrefix + created:              createdSHA,
		headsPrefix + pushedRefsOther:      updatedSHA,
		headsPrefix + pushedRefsMergedHead: "",
	}
	for ref, sha := range want {
		got, ok := pushed[ref]
		switch {
		case !ok:
			t.Errorf("pushed() = %v, want it to name %s: the log must record every ref a push sends, so a create or update is attributed from this worktree's own pushes, not from the shared object store", pushed, ref)
		case got != sha:
			t.Errorf("pushed()[%s] = %q, want %q (the SHA the push sent; empty for a deletion)", ref, got, sha)
		}
	}
	deleted, err := log.deleted()
	if err != nil {
		t.Fatalf("deleted: %v", err)
	}
	if !deleted[headsPrefix+pushedRefsMergedHead] || len(deleted) != 1 {
		t.Errorf("deleted() = %v, want only %s: recording creates and updates must not make them read as deletions", deleted, pushedRefsMergedHead)
	}
}

// Phase 40 step 3, third fix. At f56f843 installPushLog ran `git config
// extensions.worktreeConfig true`, which lands in the SHARED config of the
// deployment's repository (every action worktree is a `git worktree add` of
// it), and go-git then refused to open that repository, so the server's
// worktree manager broke after the first delegated run. The hook reaches the
// agent through its process environment (GIT_CONFIG_COUNT/KEY/VALUE) instead,
// and no git config is written in any scope.

// gitConfigUnset fails the test unless `git -C dir config --get key` exits 1.
func gitConfigUnset(t *testing.T, dir, key, when string) {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "config", "--get", key).CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Errorf("%s: git -C %s config --get %s = %q (err %v), want exit 1 (unset): the push log must write no git config in any scope",
			when, dir, key, strings.TrimSpace(string(out)), err)
	}
}

// (a) Installing and removing the push log leaves every config of the
// deployment's repository untouched, and go-git still opens it.
func TestPushLogWritesNoSharedGitConfig(t *testing.T) {
	f := newSharedWorktreeFixture(t)
	ctx := context.Background()
	log := installPushLog(ctx, f.action)
	if len(log.env(nil)) == 0 {
		t.Errorf("installPushLog handed the agent no environment (err %v); want GIT_CONFIG_COUNT/KEY/VALUE naming the hook", log.err)
	}
	check := func(when string) {
		t.Helper()
		gitConfigUnset(t, f.deployment, "extensions.worktreeConfig", when)
		for _, dir := range []string{f.deployment, f.sibling, f.action} {
			gitConfigUnset(t, dir, "core.hooksPath", when)
		}
		repo, err := gitlib.PlainOpen(f.deployment)
		if err != nil {
			t.Errorf("%s: go-git cannot open the deployment repository: %v; the server's worktree manager opens it this way", when, err)
			return
		}
		if _, err := repo.Head(); err != nil {
			t.Errorf("%s: the deployment repository's HEAD does not resolve through go-git: %v", when, err)
		}
	}
	check("after installPushLog")
	if err := log.remove(ctx); err != nil {
		t.Fatalf("remove: %v", err)
	}
	check("after remove")
}

// (b) The hook reaches the agent's process only: a push from the action's
// worktree with the environment installPushLog returns is logged, a push from
// the sibling worktree (another stream's process) is not.
func TestPushLogLogsOnlyTheAgentsPushes(t *testing.T) {
	const mine, theirs = "karakuri/ux-mine", "karakuri/ux-theirs"
	f := newSharedWorktreeFixture(t)
	ctx := context.Background()
	log := installPushLog(ctx, f.action)
	t.Cleanup(func() {
		if err := log.remove(ctx); err != nil {
			t.Errorf("removing the push log: %v", err)
		}
	})
	var mineSHA string
	withEnv(t, log.env(nil), func() {
		mineSHA = commitIn(t, f.action, "mine.txt")
		gitIn(t, f.action, "push", "origin", "HEAD:refs/heads/"+mine)
	})
	commitIn(t, f.sibling, "theirs.txt")
	gitIn(t, f.sibling, "push", "origin", "HEAD:refs/heads/"+theirs)

	pushed, err := log.pushed()
	if err != nil {
		t.Fatalf("pushed: %v", err)
	}
	if got := pushed[headsPrefix+mine]; got != mineSHA {
		t.Errorf("pushed()[%s] = %q, want %q: the agent's push from its worktree, with the environment installPushLog returned, must run the hook", mine, got, mineSHA)
	}
	if _, ok := pushed[headsPrefix+theirs]; ok {
		t.Errorf("pushed() = %v names %s, pushed from the sibling worktree: the hook must reach the agent's process only", pushed, theirs)
	}
}

// (c) The action fetches after the owner merged and branches from origin/main
// (or resets onto it): its HEAD reflog then names the owner's merge, but as a
// checkout or reset, not a commit the worktree made, so main is not its doing.
func TestPushedRefsCheckoutOfTheOwnersMergeIsNotTheActions(t *testing.T) {
	const branch = "karakuri/phase-40-step-03-next-group"
	cases := []struct {
		name string
		move [][]string
	}{
		{name: "checkout -b from origin/main", move: [][]string{{"checkout", "-b", branch, "origin/main"}}},
		{name: "reset --hard onto origin/main", move: [][]string{{"checkout", "-b", branch}, {"reset", "--hard", "origin/main"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSharedWorktreeFixture(t)
			var mergeSHA string
			violation := f.checkRunDeclaring(t, branch, func(t *testing.T) {
				mergeSHA = f.ownerMerges(t)
				gitIn(t, f.action, "fetch", "origin")
				for _, args := range tc.move {
					gitIn(t, f.action, args...)
				}
				gitIn(t, f.action, "push", "origin", "HEAD:refs/heads/"+branch)
			})
			if reflog := gitIn(t, f.action, "log", "-g", "--format=%H", "HEAD"); !strings.Contains(reflog, mergeSHA) {
				t.Fatalf("fixture: the action's HEAD reflog does not name the owner's merge %s:\n%s", mergeSHA, reflog)
			}
			if violation != nil {
				t.Errorf("BoundsViolation = %+v, want none: main moved to the owner's merge %s, which the action only checked out; the worktree did not make that commit",
					violation, mergeSHA)
			}
		})
	}
}

// (d) A commit the action made on a branch it did not declare, pushed with
// --no-verify so the hook never ran, is still its violation: the commit is in
// its HEAD reflog as "commit".
func TestPushedRefsNoVerifyCommitOnAnUndeclaredBranchIsAViolation(t *testing.T) {
	const notMine = "karakuri/ux-not-mine"
	f := newSharedWorktreeFixture(t)
	var ranSHA string
	violation := f.checkRun(t, func(t *testing.T) {
		gitIn(t, f.action, "checkout", "-b", notMine)
		ranSHA = commitIn(t, f.action, "undeclared.txt")
		gitIn(t, f.action, "push", "--no-verify", "origin", notMine)
	})
	if violation == nil {
		t.Fatalf("no BoundsViolation for the action's commit %s on undeclared %s pushed with --no-verify; want one from its HEAD reflog", ranSHA, notMine)
	}
	if want := []string{headsPrefix + notMine}; !slices.Equal(violation.Refs, want) {
		t.Errorf("BoundsViolation.Refs = %v, want exactly %v", violation.Refs, want)
	}
	if !strings.Contains(violation.Reason, notMine) || !strings.Contains(violation.Reason, ranSHA) {
		t.Errorf("Reason = %q, want it to name %s and the action's commit %s", violation.Reason, notMine, ranSHA)
	}
}
