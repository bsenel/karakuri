package orchestrator

import (
	"context"
	"fmt"

	"github.com/bsenel/karakuri/internal/core/vfs"
)

type ExecutionPlan struct {
	SessionSHA string
	Tasks      []AgentTask
}

type AgentTask struct {
	ID                 string
	Role               string
	InstanceNum        int
	Provider           string
	DependsOn          []string
	Parallel           bool
	Inputs             []string
	Outputs            []string
	MaxRetries         int
	RequiresCheckpoint bool
	NeedsWorktree      bool
}

type Planner struct {
	workflowsDir string
}

func NewPlanner(workflowsDir string) *Planner {
	return &Planner{workflowsDir: workflowsDir}
}

func (p *Planner) Plan(ctx context.Context, mode, sessionSHA string, manifest vfs.Manifest) (*ExecutionPlan, error) {
	wf, err := LoadWorkflow(p.workflowsDir, mode)
	if err != nil {
		return nil, err
	}
	plan := &ExecutionPlan{SessionSHA: sessionSHA}
	for i, role := range wf.Roles {
		provider := role.Provider
		if provider == "" {
			provider = "claude"
		}
		instances := 1
		if role.Instances != nil {
			if max, ok := role.Instances["max"]; ok && max > 1 && role.Parallel {
				instances = max
			}
		}
		for inst := 0; inst < instances; inst++ {
			taskID := fmt.Sprintf("%s-%d", role.Role, inst)
			if instances > 1 {
				taskID = fmt.Sprintf("%s-inst-%d", role.Role, inst+1)
			}
			plan.Tasks = append(plan.Tasks, AgentTask{
				ID: taskID, Role: role.Role, InstanceNum: inst + 1,
				Provider: provider, DependsOn: role.DependsOn, Parallel: role.Parallel,
				Outputs: role.ArtifactsOut, MaxRetries: 2, NeedsWorktree: role.NeedsWorktree,
			})
			_ = i
		}
	}
	return plan, nil
}
