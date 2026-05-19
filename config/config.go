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
	Auth          AuthConfig          `yaml:"auth"`
	Domains       []DomainConfig      `yaml:"domains"`
	Memory        MemoryConfig        `yaml:"memory"`
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
	Name     string            `yaml:"name"`
	Enabled  bool              `yaml:"enabled"`
	Path     string            `yaml:"path,omitempty"`
	Formats  map[string]string `yaml:"formats,omitempty"`
	Rotation RotationConfig    `yaml:"rotation,omitempty"`
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

type DomainConfig struct {
	ID      string            `yaml:"id"`
	Enabled bool              `yaml:"enabled"`
	Options map[string]any    `yaml:"options,omitempty"`
}

type MemoryConfig struct {
	ConsolidationThreshold int `yaml:"consolidation_threshold"` // episodic entries before consolidation
	SemanticTopK           int `yaml:"semantic_top_k"`
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
	if cfg.Providers.Default == "" {
		cfg.Providers.Default = "claude"
	}
	if cfg.Memory.ConsolidationThreshold == 0 {
		cfg.Memory.ConsolidationThreshold = 20
	}
	if cfg.Memory.SemanticTopK == 0 {
		cfg.Memory.SemanticTopK = 5
	}
}

func Default() *Config {
	cfg := &Config{}
	setDefaults(cfg)
	cfg.Observability.Exporters = []ExporterConfig{{
		Name: "local", Enabled: true, Path: "./karakuri-obs/",
		Formats: map[string]string{"metrics": "ndjson", "logs": "ndjson"},
		Rotation: RotationConfig{MaxSizeMB: 100, MaxAgeDays: 30},
	}}
	cfg.Domains = []DomainConfig{
		{ID: "software", Enabled: true},
	}
	return cfg
}
