package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gitlib "github.com/go-git/go-git/v5"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/objective"
)

type GoGitWorktreeManager struct {
	cfg       config.GitConfig
	mu        sync.RWMutex
	worktrees map[string]Worktree
}

func NewGoGitWorktreeManager(cfg config.GitConfig) (*GoGitWorktreeManager, error) {
	repoPath, err := filepath.Abs(cfg.RepoPath)
	if err != nil {
		return nil, err
	}
	if _, err := gitlib.PlainOpen(repoPath); err != nil {
		if _, err2 := gitlib.PlainInit(repoPath, false); err2 != nil {
			return nil, fmt.Errorf("init repo at %s: %w", repoPath, err)
		}
	}
	cfg.RepoPath = repoPath
	return &GoGitWorktreeManager{cfg: cfg, worktrees: make(map[string]Worktree)}, nil
}

func (m *GoGitWorktreeManager) repoRoot() string { return m.cfg.RepoPath }

func (m *GoGitWorktreeManager) Create(ctx context.Context, opts WorktreeOptions) (Worktree, error) {
	objID := string(opts.ObjectiveID)
	if len(objID) > 8 {
		objID = objID[:8]
	}
	branch := opts.BranchName
	if branch == "" {
		branch = fmt.Sprintf("%s/%s/%s", m.cfg.BranchPrefix, objID, opts.TaskID)
	}
	basePath := filepath.Join(m.repoRoot(), m.cfg.WorktreeBase, string(opts.ObjectiveID), opts.TaskID)
	if err := os.MkdirAll(filepath.Dir(basePath), 0o755); err != nil {
		return Worktree{}, err
	}
	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch = m.cfg.BaseBranch
	}
	_ = m.runGit(ctx, "branch", branch, baseBranch)
	if err := m.runGit(ctx, "worktree", "add", "-B", branch, basePath, branch); err != nil {
		if err2 := m.runGit(ctx, "worktree", "add", basePath, branch); err2 != nil {
			return Worktree{}, fmt.Errorf("worktree add: %w", err)
		}
	}
	wt := Worktree{
		TaskID: opts.TaskID, ObjectiveID: opts.ObjectiveID,
		Path: basePath, Branch: branch, CreatedAt: time.Now().UTC(),
	}
	m.mu.Lock()
	m.worktrees[opts.TaskID] = wt
	m.mu.Unlock()
	return wt, nil
}

func (m *GoGitWorktreeManager) Get(_ context.Context, taskID string) (Worktree, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	wt, ok := m.worktrees[taskID]
	if !ok {
		return Worktree{}, fmt.Errorf("worktree not found: %s", taskID)
	}
	return wt, nil
}

func (m *GoGitWorktreeManager) Remove(ctx context.Context, taskID string) error {
	m.mu.Lock()
	wt, ok := m.worktrees[taskID]
	if ok {
		delete(m.worktrees, taskID)
	}
	m.mu.Unlock()
	if !ok {
		return nil
	}
	_ = m.runGit(ctx, "worktree", "remove", "--force", wt.Path)
	_ = os.RemoveAll(wt.Path)
	return nil
}

func (m *GoGitWorktreeManager) List(_ context.Context, objectiveID objective.ObjectiveID) ([]Worktree, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Worktree
	for _, wt := range m.worktrees {
		if wt.ObjectiveID == objectiveID {
			out = append(out, wt)
		}
	}
	return out, nil
}

func (m *GoGitWorktreeManager) Prune(ctx context.Context, objectiveID objective.ObjectiveID) error {
	wts, _ := m.List(ctx, objectiveID)
	for _, wt := range wts {
		_ = m.Remove(ctx, wt.TaskID)
	}
	_ = m.runGit(ctx, "worktree", "prune")
	return nil
}

func (m *GoGitWorktreeManager) runGit(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = m.repoRoot()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), string(out), err)
	}
	return nil
}
