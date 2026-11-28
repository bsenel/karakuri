package software

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/capability"
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
// It keys a worktree on the identifiers every call carries, the objective and
// the loop/pass, so actions of one pass share their files and nothing else
// does; the orchestrator never sees a path.
//
// The worktrees are made and removed through the platform's git worktree
// manager. A worktree no signal released is removed once it has gone unused
// for longer than maxAge; expiry runs on every use, so it needs no goroutine.
type Workspaces struct {
	trees  *git.GoGitWorktreeManager
	maxAge time.Duration
	now    func() time.Time

	mu       sync.Mutex
	lastUsed map[string]time.Time // keyed on worktreeKey
	repoErr  error
}

// NewWorkspaces keeps worktrees of the repository at repoRoot, removing any
// older than maxAge by the clock now when no signal released it.
func NewWorkspaces(repoRoot string, maxAge time.Duration, now func() time.Time) *Workspaces {
	// HEAD is the base: the keeper has no branch of its own to start from, and
	// the repository's checkout is the one the server was started on.
	trees, err := git.NewGoGitWorktreeManager(config.GitConfig{
		RepoPath: repoRoot, WorktreeBase: packWorktreeBase, BaseBranch: "HEAD", BranchPrefix: packBranchPrefix,
	})
	return &Workspaces{trees: trees, maxAge: maxAge, now: now, lastUsed: map[string]time.Time{}, repoErr: err}
}

// worktreeKey is the manager's task ID for a call's objective and pass. It
// carries the whole objective ID because the manager keys its worktrees on the
// task ID alone.
func worktreeKey(call domain.CallContract) string {
	return fmt.Sprintf("%s.pass-%d", call.ObjectiveID, call.Pass)
}

// For is the worktree of the call's objective and pass, made on first use.
// The path stays inside the pack.
func (w *Workspaces) For(ctx context.Context, call domain.CallContract) (string, error) {
	wt, err := w.worktree(ctx, call)
	return wt.Path, err
}

// worktree is For's worktree whole, its branch with it, which the acting
// tools that push or open a pull request read.
func (w *Workspaces) worktree(ctx context.Context, call domain.CallContract) (git.Worktree, error) {
	if w.repoErr != nil {
		return git.Worktree{}, fmt.Errorf("software: the pack's repository: %w", w.repoErr)
	}
	if call.ObjectiveID == "" {
		return git.Worktree{}, errors.New("software: a call with no objective has no worktree of the pack")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.expireLocked(ctx)
	key := worktreeKey(call)
	wt, err := w.trees.Get(ctx, key)
	if err != nil {
		wt, err = w.trees.Create(ctx, git.WorktreeOptions{ObjectiveID: objective.ObjectiveID(call.ObjectiveID), TaskID: key})
		if err != nil {
			return git.Worktree{}, fmt.Errorf("software: worktree of objective %s pass %d: %w", call.ObjectiveID, call.Pass, err)
		}
	}
	w.lastUsed[key] = w.now()
	return wt, nil
}

// PassEnded removes the worktree of the call's objective and pass. A pass the
// pack never worked in, or one already removed, is no error.
func (w *Workspaces) PassEnded(ctx context.Context, call domain.CallContract) error {
	if w.repoErr != nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	key := worktreeKey(call)
	delete(w.lastUsed, key)
	return w.trees.Remove(ctx, key)
}

// ObjectiveEnded removes every worktree of the call's objective; repeating it
// finds nothing left and is no error.
func (w *Workspaces) ObjectiveEnded(ctx context.Context, call domain.CallContract) error {
	if w.repoErr != nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	prefix := call.ObjectiveID + ".pass-"
	for key := range w.lastUsed {
		if strings.HasPrefix(key, prefix) {
			delete(w.lastUsed, key)
		}
	}
	return w.trees.Prune(ctx, objective.ObjectiveID(call.ObjectiveID))
}

// Expire removes every worktree older than the age bound.
func (w *Workspaces) Expire(ctx context.Context) error {
	if w.repoErr != nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.expireLocked(ctx)
	return nil
}

// expireLocked removes every worktree unused for longer than maxAge; the
// caller holds w.mu. Removal is best effort, as the manager's is.
func (w *Workspaces) expireLocked(ctx context.Context) {
	now := w.now()
	for key, used := range w.lastUsed {
		if now.Sub(used) > w.maxAge {
			delete(w.lastUsed, key)
			_ = w.trees.Remove(ctx, key)
		}
	}
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

// fileWritingTools are the acting capabilities that write files, the ones
// capabilities.go marks with writes, so the ones whose Writes is set: the
// server serves them in the worktree of the call's objective and pass.
var fileWritingTools = map[capability.CapabilityID]bool{
	"software.act.create_pr":       true,
	"software.act.delegate_to_cli": true,
	"software.act.write_code":      true,
	"software.act.write_test":      true,
}

// inWorkspace runs act in the worktree of the call's objective and pass, and
// keeps the worktree's path out of what it returns.
//
// A verifier is handed it as params.workdir, when the call named none. A tool
// that writes files is handed it as the loop's provisioning hands it: the path
// as params.worktree_path, the plan's params.branch kept as
// params.planned_branch, the ref the pushed-refs check declares (Phase 40
// step 3), and the worktree's own branch as params.branch when the plan named
// none, the branch create_pr opens its pull request from.
func (s *MCPServer) inWorkspace(ctx context.Context, call domain.CallContract, tool capability.CapabilityID, envID environment.EnvironmentID, params map[string]any,
	act func(map[string]any) (environment.ActionResult, error)) (environment.ActionResult, error) {
	if s.workspaces == nil {
		return act(params)
	}
	if fileWritingTools[tool] {
		return s.inWritingWorktree(ctx, call, params, act)
	}
	if envID != workspaceEnv {
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

// inWritingWorktree runs a tool that writes files in the pass's worktree.
func (s *MCPServer) inWritingWorktree(ctx context.Context, call domain.CallContract, params map[string]any,
	act func(map[string]any) (environment.ActionResult, error)) (environment.ActionResult, error) {
	wt, err := s.workspaces.worktree(ctx, call)
	if err != nil {
		return environment.ActionResult{}, err
	}
	inTree := maps.Clone(params)
	if inTree == nil {
		inTree = map[string]any{}
	}
	inTree["worktree_path"] = wt.Path
	// The loop checked its worktree out on the plan's branch, so the branch it
	// put in params.branch was the plan's; the pack's worktree is the pass's,
	// so the plan's branch stays the one the action names.
	if planned, _ := params["branch"].(string); planned != "" {
		inTree["planned_branch"] = planned
	} else {
		inTree["branch"] = wt.Branch
	}
	res, err := act(inTree)
	return withoutWorkspace(res, wt.Path), err
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
