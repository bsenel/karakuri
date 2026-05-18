package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Database      DatabaseConfig      `yaml:"database"`
	Git           GitConfig           `yaml:"git"`
	Observability ObservabilityConfig `yaml:"observability"`
	Providers     ProvidersConfig     `yaml:"providers"`
	Executor      string              `yaml:"executor"`
	WorkflowsDir  string              `yaml:"workflows_dir"`
	Auth          AuthConfig          `yaml:"auth"`
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
}

type DatabaseConfig struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

type GitConfig struct {
	RepoPath     string `yaml:"repo_path"`
	WorktreeBase string `yaml:"worktree_base"`
	BaseBranch   string `yaml:"base_branch"`
	BranchPrefix string `yaml:"branch_prefix"`
	AutoPrune    bool   `yaml:"auto_prune"`
}

type ObservabilityConfig struct {
	Exporters []ExporterConfig `yaml:"exporters"`
}

type ExporterConfig struct {
	Name    string            `yaml:"name"`
	Enabled bool              `yaml:"enabled"`
	Path    string            `yaml:"path,omitempty"`
	Formats map[string]string `yaml:"formats,omitempty"`
	Rotation RotationConfig   `yaml:"rotation,omitempty"`
}

type RotationConfig struct {
	MaxSizeMB  int `yaml:"max_size_mb"`
	MaxAgeDays int `yaml:"max_age_days"`
}

type ProvidersConfig struct {
	Default  string            `yaml:"default"`
	Fallback map[string]string `yaml:"fallback"`
}

type AuthConfig struct {
	Token string `yaml:"token"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	setDefaults(&cfg)
	return &cfg, nil
}

func setDefaults(cfg *Config) {
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}
	if cfg.Database.Driver == "" {
		cfg.Database.Driver = "sqlite"
	}
	if cfg.Database.DSN == "" {
		cfg.Database.DSN = "karakuri.db"
	}
	if cfg.Git.RepoPath == "" {
		cfg.Git.RepoPath = "."
	}
	if cfg.Git.WorktreeBase == "" {
		cfg.Git.WorktreeBase = "worktrees"
	}
	if cfg.Git.BaseBranch == "" {
		cfg.Git.BaseBranch = "main"
	}
	if cfg.Git.BranchPrefix == "" {
		cfg.Git.BranchPrefix = "karakuri"
	}
	if cfg.Executor == "" {
		cfg.Executor = "local"
	}
	if cfg.WorkflowsDir == "" {
		cfg.WorkflowsDir = "workflows"
	}
	if cfg.Providers.Default == "" {
		cfg.Providers.Default = "claude"
	}
}

func Default() *Config {
	cfg := &Config{}
	setDefaults(cfg)
	cfg.Observability.Exporters = []ExporterConfig{{
		Name: "local", Enabled: true, Path: "./karakuri-obs/",
		Formats: map[string]string{"metrics": "ndjson", "logs": "ndjson"},
	}}
	return cfg
}
