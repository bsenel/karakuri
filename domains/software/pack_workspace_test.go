package software

// Phase 39 step 4, slice A: the software pack's server keys its worktrees on
// the identifiers it is sent, the objective and the loop/pass of the call's
// contract, and the orchestrator never sees one.
//
// How the lifecycle signals are carried, settled here: as reserved tool names,
// domain.ToolPassEnded ("lifecycle.pass_ended") and domain.ToolObjectiveEnded
// ("lifecycle.objective_ended"), sent through the same CallTool path as every
// other call, with the call contract naming what ended. The step 3 server has
// CallTool and no notification channel, so a reserved tool is the one it can
// receive without a second transport; the gateway's audit, quota and authority
// apply to it as to any call. A notification was the alternative and would
// need a channel the in-process server does not have.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
)

const workspaceAgeBound = time.Hour

// packClock is a clock the test moves by hand, so expiry needs no sleep.
type packClock struct{ t time.Time }

func (c *packClock) now() time.Time          { return c.t }
func (c *packClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// packRepo is a real git repository with one commit, for the pack to make its
// worktrees in.
func packRepo(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		base := []string{"-c", "user.name=karakuri-test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false"}
		cmd := exec.Command("git", append(base, args...)...) // #nosec G204 -- fixed binary, test arguments
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("pack repo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "README.md")
	git("commit", "-q", "-m", "init")
	return root
}

func newTestWorkspaces(t *testing.T) (*Workspaces, *packClock, string) {
	t.Helper()
	repo := packRepo(t)
	clock := &packClock{t: time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)}
	return NewWorkspaces(repo, workspaceAgeBound, clock.now), clock, repo
}

func passCall(objectiveID string, pass int, actionID string) domain.CallContract {
	return domain.CallContract{
		TenantID: "tenant-1", TwinID: "twin-1",
		ObjectiveID: objectiveID, LoopID: "loop-" + objectiveID, Pass: pass, ActionID: actionID,
	}
}

func workspaceFor(t *testing.T, ws *Workspaces, call domain.CallContract) string {
	t.Helper()
	dir, err := ws.For(context.Background(), call)
	if err != nil {
		t.Fatalf("worktree for objective %s pass %d: %v", call.ObjectiveID, call.Pass, err)
	}
	if dir == "" {
		t.Fatalf("worktree for objective %s pass %d: empty path and no error", call.ObjectiveID, call.Pass)
	}
	return dir
}

func writeWork(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func gone(t *testing.T, dir string) bool {
	t.Helper()
	_, err := os.Stat(dir)
	return errors.Is(err, os.ErrNotExist)
}

func TestPackWorkspaceIsSharedByCallsOfTheSameObjectiveAndPass(t *testing.T) {
	ws, _, _ := newTestWorkspaces(t)

	write := workspaceFor(t, ws, passCall("obj-1", 1, "write-code"))
	writeWork(t, write, "feature.go", "package feature\n")

	runTests := workspaceFor(t, ws, passCall("obj-1", 1, "run-tests"))
	got, err := os.ReadFile(filepath.Join(runTests, "feature.go")) // #nosec G304 -- test path under t.TempDir
	if err != nil {
		t.Fatalf("the second action of the pass does not see the first one's file: %v", err)
	}
	if string(got) != "package feature\n" {
		t.Errorf("file seen by the second action = %q", got)
	}
	if _, err := os.Stat(filepath.Join(runTests, "README.md")); err != nil {
		t.Errorf("the worktree is not a checkout of the repository: %v", err)
	}
}

func TestPackWorkspaceOfAnotherPassOrObjectiveNeverSeesTheFiles(t *testing.T) {
	ws, _, _ := newTestWorkspaces(t)

	first := workspaceFor(t, ws, passCall("obj-1", 1, "write-code"))
	writeWork(t, first, "feature.go", "package feature\n")

	for name, call := range map[string]domain.CallContract{
		"next pass":       passCall("obj-1", 2, "write-code"),
		"other objective": passCall("obj-2", 1, "write-code"),
	} {
		other := workspaceFor(t, ws, call)
		if other == first {
			t.Errorf("%s: got the same worktree as objective obj-1 pass 1", name)
			continue
		}
		if _, err := os.Stat(filepath.Join(other, "feature.go")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: sees a file another pass wrote (stat err %v)", name, err)
		}
	}
}

func TestPackWorkspacePassEndedRemovesThatPassOnlyAndIsSafeToRepeat(t *testing.T) {
	for _, times := range []int{1, 3} {
		t.Run(fmt.Sprintf("sent %d times", times), func(t *testing.T) {
			ws, _, _ := newTestWorkspaces(t)
			ended := passCall("obj-1", 1, "write-code")
			kept := passCall("obj-1", 2, "write-code")

			endedDir := workspaceFor(t, ws, ended)
			writeWork(t, endedDir, "feature.go", "package feature\n")
			keptDir := workspaceFor(t, ws, kept)
			writeWork(t, keptDir, "other.go", "package other\n")

			for i := 0; i < times; i++ {
				if err := ws.PassEnded(context.Background(), ended); err != nil {
					t.Fatalf("pass ended, signal %d of %d: %v", i+1, times, err)
				}
			}
			if !gone(t, endedDir) {
				t.Errorf("the ended pass's worktree %s is still on disk", endedDir)
			}
			if _, err := os.Stat(filepath.Join(keptDir, "other.go")); err != nil {
				t.Errorf("ending pass 1 removed pass 2's work: %v", err)
			}
			again := workspaceFor(t, ws, ended)
			if _, err := os.Stat(filepath.Join(again, "feature.go")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("a call after the pass ended still finds its files (stat err %v)", err)
			}
		})
	}
}

func TestPackWorkspacePassEndedForAPassNeverSeenIsNoError(t *testing.T) {
	ws, _, _ := newTestWorkspaces(t)
	if err := ws.PassEnded(context.Background(), passCall("obj-9", 4, "")); err != nil {
		t.Errorf("pass ended for a pass the pack never worked in: %v", err)
	}
}

func TestPackWorkspaceObjectiveEndedRemovesEveryPassOfItAndIsSafeToRepeat(t *testing.T) {
	ws, _, _ := newTestWorkspaces(t)
	p1 := workspaceFor(t, ws, passCall("obj-1", 1, "a"))
	p2 := workspaceFor(t, ws, passCall("obj-1", 2, "a"))
	other := workspaceFor(t, ws, passCall("obj-2", 1, "a"))

	for i := 0; i < 3; i++ {
		if err := ws.ObjectiveEnded(context.Background(), passCall("obj-1", 0, "")); err != nil {
			t.Fatalf("objective ended, signal %d: %v", i+1, err)
		}
	}
	if !gone(t, p1) || !gone(t, p2) {
		t.Errorf("objective obj-1 ended and its worktrees remain: pass 1 gone=%v, pass 2 gone=%v", gone(t, p1), gone(t, p2))
	}
	if gone(t, other) {
		t.Error("ending objective obj-1 removed objective obj-2's worktree")
	}
}

func TestPackWorkspaceExpiresByAgeWithNoSignal(t *testing.T) {
	ws, clock, _ := newTestWorkspaces(t)
	old := workspaceFor(t, ws, passCall("obj-1", 1, "a"))
	clock.advance(workspaceAgeBound / 2)
	young := workspaceFor(t, ws, passCall("obj-2", 1, "a"))

	clock.advance(workspaceAgeBound/2 + time.Minute)
	if err := ws.Expire(context.Background()); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if !gone(t, old) {
		t.Errorf("a worktree older than the bound %s is still on disk", workspaceAgeBound)
	}
	if gone(t, young) {
		t.Errorf("a worktree younger than the bound %s was removed", workspaceAgeBound)
	}
}

// The server, given the keeper, runs a verifier in the worktree of the call's
// objective and pass, answers the lifecycle signals, and returns nothing that
// names where it worked.

func serverWithWorkspaces(t *testing.T) (*MCPServer, *Workspaces, string) {
	t.Helper()
	ws, _, repo := newTestWorkspaces(t)
	return packWithForge(nil).MCPServer(nil).WithWorkspaces(ws), ws, repo
}

func measureBundle(t *testing.T, srv *MCPServer, call domain.CallContract) (resText string, success bool) {
	t.Helper()
	res, err := srv.CallTool(context.Background(), call, capability.CapabilityID(capWebBundle), map[string]any{"dir": "dist"})
	if err != nil {
		t.Fatalf("call %s: %v", capWebBundle, err)
	}
	return fmt.Sprintf("%+v", res), res.Success
}

func TestPackWorkspaceTheServersVerifierRunsInThePassesWorktree(t *testing.T) {
	srv, ws, _ := serverWithWorkspaces(t)
	pass1 := passCall("obj-1", 1, "write-code")
	writeWork(t, workspaceFor(t, ws, pass1), "dist/app.js", "console.log(1)\n")

	text, ok := measureBundle(t, srv, passCall("obj-1", 1, "measure"))
	if !ok {
		t.Fatalf("the verifier of the same pass does not find the bundle written earlier in it: %s", text)
	}
	if text, ok := measureBundle(t, srv, passCall("obj-1", 2, "measure")); ok {
		t.Errorf("the verifier of another pass measured pass 1's bundle: %s", text)
	}
}

func TestPackWorkspaceTheServersResultCarriesNoWorkspaceReference(t *testing.T) {
	srv, ws, repo := serverWithWorkspaces(t)
	call := passCall("obj-1", 1, "measure")
	dir := workspaceFor(t, ws, call)
	writeWork(t, dir, "dist/app.js", "console.log(1)\n")

	res, err := srv.CallTool(context.Background(), call, capability.CapabilityID(capWebBundle), map[string]any{"dir": "dist"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.Success {
		t.Fatalf("the verifier failed in the pass's worktree: %s", res.Error)
	}
	for _, key := range []string{"workdir", "worktree_path", "worktree", "workspace", "path"} {
		if _, ok := res.StateDelta[key]; ok {
			t.Errorf("the result's state carries %q, a workspace reference the orchestrator would hold", key)
		}
	}
	text := fmt.Sprintf("%+v", res)
	for _, ref := range []string{dir, repo} {
		if strings.Contains(text, ref) {
			t.Errorf("the result names the pack's filesystem %q: %s", ref, text)
		}
	}
}

func TestPackWorkspacePassEndedThroughTheServerIsSafeToRepeat(t *testing.T) {
	srv, ws, _ := serverWithWorkspaces(t)
	call := passCall("obj-1", 1, "write-code")
	dir := workspaceFor(t, ws, call)

	for i := 0; i < 3; i++ {
		res, err := srv.CallTool(context.Background(), passCall("obj-1", 1, ""), domain.ToolPassEnded, nil)
		if err != nil {
			t.Fatalf("pass ended through the server, signal %d: %v", i+1, err)
		}
		if !res.Success {
			t.Fatalf("pass ended through the server, signal %d: %s", i+1, res.Error)
		}
	}
	if !gone(t, dir) {
		t.Errorf("pass ended through the server left the worktree %s", dir)
	}
	res, err := srv.CallTool(context.Background(), passCall("obj-1", 0, ""), domain.ToolObjectiveEnded, nil)
	if err != nil || !res.Success {
		t.Errorf("objective ended through the server: err %v, result %+v", err, res)
	}
}
