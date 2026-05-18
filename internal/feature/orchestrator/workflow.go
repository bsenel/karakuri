package orchestrator

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Workflow struct {
	Name        string              `yaml:"name"`
	Version     int                 `yaml:"version"`
	Constraints []string            `yaml:"constraints"`
	Checkpoints []CheckpointDef     `yaml:"checkpoints"`
	Roles       []RoleDef           `yaml:"roles"`
	Promotion   map[string]any      `yaml:"promotion,omitempty"`
	Loops       []LoopDef           `yaml:"loops,omitempty"`
}

type CheckpointDef struct {
	Before    string `yaml:"before,omitempty"`
	On        string `yaml:"on,omitempty"`
	Threshold int    `yaml:"threshold,omitempty"`
	Summary   string `yaml:"summary"`
}

type RoleDef struct {
	Role             string         `yaml:"role"`
	Provider         string         `yaml:"provider"`
	ProviderFallback string         `yaml:"provider_fallback"`
	Temperature      float64        `yaml:"temperature"`
	Parallel         bool           `yaml:"parallel"`
	NeedsWorktree    bool           `yaml:"needs_worktree"`
	DependsOn        []string       `yaml:"depends_on"`
	ArtifactsOut     []string       `yaml:"artifacts_out"`
	Instances        map[string]int `yaml:"instances,omitempty"`
}

type LoopDef struct {
	Name    string `yaml:"name"`
	Adapter string `yaml:"adapter"`
}

func LoadWorkflow(dir, name string) (*Workflow, error) {
	path := filepath.Join(dir, name+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wf Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, err
	}
	return &wf, nil
}
