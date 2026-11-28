package software

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/git"
)

// packWorktreeBase is where, under the repository, the pack keeps its
// worktrees: inside the ignored worktrees/ directory, apart from the loop's
// own, which are keyed on an objective ID and never start with a dot.
const packWorktreeBase = "worktrees/.pack"

// packBranchPrefix names the branches of the pack's worktrees, apart from the
// loop's karakuri/ ones, so a pass of the pack never resets a delivery branch.
const packBranchPrefix = "karakuri-pack"

// workspaceEnv is the pack environment whose verifiers read params.workdir;
// a call to one of them with no workdir runs in the pass's worktree.
const workspaceEnv environment.EnvironmentID = "software.env.verify"

// Workspaces is the software pack's own keeper of worktrees (Phase 39 step 4).
// It keys a worktree on the identifiers every call carries, the objective, the
// loop run and the pass, so actions of one pass share their files and nothing
// else does; the orchestrator never sees a path.
//
// The worktrees are made and removed through the platform's git worktree
// manager. A worktree no signal released is removed once it has gone unused
// for longer than maxAge, and so is one an earlier process left under
// packWorktreeBase, found on disk at construction and aged by its directory's
// time. A call's expiry runs off the call's path, in its own goroutine.
type Workspaces struct {
	trees    *git.GoGitWorktreeManager
	repoRoot string
	maxAge   time.Duration
	now      func() time.Time

	mu       sync.Mutex
	known    map[string]packTree // keyed on worktreeKey
	expiring map[string]bool     // keys whose removal is under way
	repoErr  error
}

// packTree is what the keeper knows of one worktree: its objective, when it was
// last used and, for one an earlier process left, the path the manager does
// not know.
type packTree struct {
	objective string
	used      time.Time
	leftover  string
}

// packGitTimeout bounds one git call of the keeper's removal, so a hung git
// does not hold a signal or an expiry for ever.
const packGitTimeout = 30 * time.Second

// NewWorkspaces keeps worktrees of the repository at repoRoot, removing any
// older than maxAge by the clock now when no signal released it.
func NewWorkspaces(repoRoot string, maxAge time.Duration, now func() time.Time) *Workspaces {
	// HEAD is the base: the keeper has no branch of its own to start from, and
	// the repository's checkout is the one the server was started on.
	trees, err := git.NewGoGitWorktreeManager(config.GitConfig{
		RepoPath: repoRoot, WorktreeBase: packWorktreeBase, BaseBranch: "HEAD", BranchPrefix: packBranchPrefix,
	})
	w := &Workspaces{trees: trees, maxAge: maxAge, now: now,
		known: map[string]packTree{}, expiring: map[string]bool{}, repoErr: err}
	if err == nil {
		w.repoRoot, w.repoErr = filepath.Abs(repoRoot)
	}
	if w.repoErr == nil {
		w.findLeftovers()
	}
	return w
}

// findLeftovers records every worktree directory under packWorktreeBase, which
// at construction only an earlier process can have made, aged by its
// directory's time. It reads the directory and runs no git.
func (w *Workspaces) findLeftovers() {
	base := filepath.Join(w.repoRoot, packWorktreeBase)
	objectives, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, obj := range objectives {
		if !obj.IsDir() {
			continue
		}
		keys, err := os.ReadDir(filepath.Join(base, obj.Name()))
		if err != nil {
			continue
		}
		for _, key := range keys {
			info, err := key.Info()
			if err != nil || !key.IsDir() {
				continue
			}
			w.known[key.Name()] = packTree{
				objective: obj.Name(), used: info.ModTime(),
				leftover: filepath.Join(base, obj.Name(), key.Name()),
			}
		}
	}
}

// worktreeKey is the manager's task ID for a call's objective, loop run and
// pass. It carries the whole objective ID because the manager keys its
// worktrees on the task ID alone, and the loop run because a standing
// objective's pass numbers restart at 0 on every run. The loop ID is hashed,
// so the key stays one path segment and part of a valid branch name whatever
// the ID holds.
func worktreeKey(call domain.CallContract) string {
	loop := sha256.Sum256([]byte(call.LoopID))
	return fmt.Sprintf("%s.loop-%s.pass-%d", call.ObjectiveID, hex.EncodeToString(loop[:8]), call.Pass)
}

// For is the worktree of the call's objective, loop run and pass, made from
// the base on first use. The path stays inside the pack.
func (w *Workspaces) For(ctx context.Context, call domain.CallContract) (string, error) {
	if w.repoErr != nil {
		return "", fmt.Errorf("software: the pack's repository: %w", w.repoErr)
	}
	if call.ObjectiveID == "" {
		return "", errors.New("software: a call with no objective has no worktree of the pack")
	}
	if call.LoopID == "" {
		return "", errors.New("software: a call with no loop run has no worktree of the pack")
	}
	key := worktreeKey(call)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.expiring[key] {
		return "", fmt.Errorf("software: worktree of objective %s pass %d is being expired; call again", call.ObjectiveID, call.Pass)
	}
	if expired := w.takeExpiredLocked(key); len(expired) > 0 {
		go w.removeExpired(context.WithoutCancel(ctx), expired)
	}
	if tree, ok := w.known[key]; ok && tree.leftover != "" {
		// An earlier process's tree of this very key: its files are not this
		// process's to resume, so the pass starts again from the base.
		w.removeTree(ctx, key, tree)
	}
	wt, err := w.trees.Get(ctx, key)
	if err != nil {
		wt, err = w.trees.Create(ctx, git.WorktreeOptions{ObjectiveID: objective.ObjectiveID(call.ObjectiveID), TaskID: key})
		if err != nil {
			return "", fmt.Errorf("software: worktree of objective %s pass %d: %w", call.ObjectiveID, call.Pass, err)
		}
	}
	w.known[key] = packTree{objective: call.ObjectiveID, used: w.now()}
	return wt.Path, nil
}

// PassEnded removes the worktree of the call's objective, loop run and pass. A
// pass the pack never worked in, or one already removed, is no error.
func (w *Workspaces) PassEnded(ctx context.Context, call domain.CallContract) error {
	if w.repoErr != nil || call.LoopID == "" {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	key := worktreeKey(call)
	if tree, ok := w.known[key]; ok {
		delete(w.known, key)
		w.removeTree(ctx, key, tree)
	}
	return nil
}

// ObjectiveEnded removes every worktree of the call's objective, of every loop
// run, this process's and those an earlier one left; repeating it finds
// nothing left and is no error.
func (w *Workspaces) ObjectiveEnded(ctx context.Context, call domain.CallContract) error {
	if w.repoErr != nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for key, tree := range w.known {
		if tree.objective == call.ObjectiveID {
			delete(w.known, key)
			w.removeTree(ctx, key, tree)
		}
	}
	return w.trees.Prune(ctx, objective.ObjectiveID(call.ObjectiveID))
}

// Expire removes every worktree older than the age bound, and waits for it.
func (w *Workspaces) Expire(ctx context.Context) error {
	if w.repoErr != nil {
		return nil
	}
	w.mu.Lock()
	expired := w.takeExpiredLocked("")
	w.mu.Unlock()
	w.removeExpired(ctx, expired)
	return nil
}

// takeExpiredLocked takes out of the record every worktree unused for longer
// than maxAge, except the one keep names, which a call is using now, and marks
// them expiring until removeExpired is done; the caller holds w.mu.
func (w *Workspaces) takeExpiredLocked(keep string) map[string]packTree {
	now := w.now()
	expired := map[string]packTree{}
	for key, tree := range w.known {
		if key != keep && now.Sub(tree.used) > w.maxAge {
			delete(w.known, key)
			w.expiring[key] = true
			expired[key] = tree
		}
	}
	return expired
}

// removeExpired removes the worktrees takeExpiredLocked took, best effort, and
// clears their expiring mark.
func (w *Workspaces) removeExpired(ctx context.Context, expired map[string]packTree) {
	for key, tree := range expired {
		w.removeTree(ctx, key, tree)
	}
	w.mu.Lock()
	for key := range expired {
		delete(w.expiring, key)
	}
	w.mu.Unlock()
}

// removeTree removes one worktree and its karakuri-pack/ branch, so a tree
// made again on the same key starts at the base. One this process made goes
// through the manager; one an earlier process left goes through git, as the
// manager does not know it. Best effort, as the manager's removal is.
func (w *Workspaces) removeTree(ctx context.Context, key string, tree packTree) {
	if tree.leftover == "" {
		wt, err := w.trees.Get(ctx, key)
		_ = w.trees.Remove(ctx, key)
		if err == nil {
			w.dropBranch(ctx, wt.Branch)
		}
		return
	}
	branch := leftoverBranch(tree.leftover)
	_ = w.git(ctx, "worktree", "remove", "--force", tree.leftover)
	_ = os.RemoveAll(tree.leftover)
	_ = w.git(ctx, "worktree", "prune")
	w.dropBranch(ctx, branch)
}

// dropBranch deletes a branch of the pack's own; any other name is left alone.
func (w *Workspaces) dropBranch(ctx context.Context, branch string) {
	if strings.HasPrefix(branch, packBranchPrefix+"/") {
		_ = w.git(ctx, "branch", "-D", branch)
	}
}

// leftoverBranch reads the branch a leftover worktree has checked out from its
// own git files, with no git call: the worktree's .git names its git
// directory, whose HEAD names the branch. Empty when either cannot be read.
func leftoverBranch(dir string) string {
	link, err := os.ReadFile(filepath.Join(dir, ".git")) // #nosec G304 -- dir is under the pack's worktree base
	if err != nil {
		return ""
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(string(link), "gitdir:"))
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD")) // #nosec G304 -- the git directory the pack's worktree names
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(string(head), "ref: refs/heads/"))
}

// git runs one git command in the repository, bounded by packGitTimeout.
func (w *Workspaces) git(ctx context.Context, args ...string) error {
	gctx, cancel := context.WithTimeout(ctx, packGitTimeout)
	defer cancel()
	cmd := exec.CommandContext(gctx, "git", args...) // #nosec G204 -- fixed binary; worktree and branch subcommands built here
	cmd.Dir = w.repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), out, err)
	}
	return nil
}

// WithWorkspaces gives the server the pack's worktree keeper, so a verifier
// runs in the worktree of the call's objective and pass.
func (s *MCPServer) WithWorkspaces(w *Workspaces) *MCPServer {
	s.workspaces = w
	return s
}

// lifecycle answers the orchestrator's "pass ended" and "objective ended"
// signals. Both are safe to repeat; a server with no keeper has nothing to
// release and answers them all the same.
func (s *MCPServer) lifecycle(ctx context.Context, call domain.CallContract, tool string) (environment.ActionResult, error) {
	var err error
	if s.workspaces != nil {
		if tool == string(domain.ToolPassEnded) {
			err = s.workspaces.PassEnded(ctx, call)
		} else {
			err = s.workspaces.ObjectiveEnded(ctx, call)
		}
	}
	if err != nil {
		return environment.ActionResult{}, fmt.Errorf("software: %s: %w", tool, err)
	}
	return environment.ActionResult{
		Success: true, Trust: environment.TrustOperator,
		StateDelta: map[string]any{"action": tool, "status": "ok"},
	}, nil
}

// inWorkspace runs act with params.workdir set to the worktree of the call's
// objective and pass, when the tool reads one and the call named none, and
// keeps the worktree's path out of what it returns.
func (s *MCPServer) inWorkspace(ctx context.Context, call domain.CallContract, envID environment.EnvironmentID, params map[string]any,
	act func(map[string]any) (environment.ActionResult, error)) (environment.ActionResult, error) {
	if s.workspaces == nil || envID != workspaceEnv {
		return act(params)
	}
	if requested, _ := params["workdir"].(string); requested != "" {
		return act(params)
	}
	dir, err := s.workspaces.For(ctx, call)
	if err != nil {
		return environment.ActionResult{}, err
	}
	withDir := make(map[string]any, len(params)+1)
	for k, v := range params {
		withDir[k] = v
	}
	withDir["workdir"] = dir
	res, err := act(withDir)
	return withoutWorkspace(res, dir), err
}

// withoutWorkspace drops the keys that name where the pack worked and writes
// the worktree's path as "." wherever a text carries it, so the orchestrator
// holds no reference to the pack's filesystem.
func withoutWorkspace(res environment.ActionResult, dir string) environment.ActionResult {
	clean := func(s string) string {
		s = strings.ReplaceAll(s, dir+string(filepath.Separator), "")
		return strings.ReplaceAll(s, dir, ".")
	}
	if res.StateDelta != nil {
		delta := make(map[string]any, len(res.StateDelta))
		for k, v := range res.StateDelta {
			if k == "workdir" || k == "worktree_path" {
				continue
			}
			if text, ok := v.(string); ok {
				v = clean(text)
			}
			delta[k] = v
		}
		res.StateDelta = delta
	}
	res.Error = clean(res.Error)
	if len(res.Measurements) > 0 {
		ms := make([]environment.Measurement, len(res.Measurements))
		for i, m := range res.Measurements {
			m.Source = clean(m.Source)
			ms[i] = m
		}
		res.Measurements = ms
	}
	return res
}
