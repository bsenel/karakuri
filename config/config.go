package config

import (
	"os"
	"strings"

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
	Tools         ToolsConfig         `yaml:"tools"`
}

// ToolsConfig holds a SlotConfig per adapter category. Every slot has the same
// shape: a Default instance name (used when a twin has no binding for the slot)
// and a map of named Instances. See ADR 006 for the rationale.
type ToolsConfig struct {
	VersionControl SlotConfig `yaml:"versioncontrol"`
	ProjectMgmt    SlotConfig `yaml:"projectmgmt"`
	Messaging      SlotConfig `yaml:"messaging"`
	Design         SlotConfig `yaml:"design"`
	Testing        SlotConfig `yaml:"testing"`
	Calendar       SlotConfig `yaml:"calendar"`
	Email          SlotConfig `yaml:"email"`
}

// SlotConfig is the uniform per-slot shape (Pattern B).
type SlotConfig struct {
	Default   string                    `yaml:"default"`
	Instances map[string]InstanceConfig `yaml:"instances"`
}

// InstanceConfig declares an adapter instance: an opaque Type that selects an
// implementation (e.g. "github", "gmail", "smtp") plus arbitrary provider-
// specific Options. Options may carry `*_env` keys whose values are env var
// names — resolveEnvRefs copies the env value to the bare key at load time.
type InstanceConfig struct {
	Type    string         `yaml:"type"`
	Options map[string]any `yaml:",inline"`
}

// OptString reads a string option by key, returning "" if missing or wrong type.
func (i InstanceConfig) OptString(key string) string {
	if v, ok := i.Options[key].(string); ok {
		return v
	}
	return ""
}

// OptInt reads an int option by key, returning 0 if missing.
func (i InstanceConfig) OptInt(key string) int {
	switch v := i.Options[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
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
	resolveEnvRefs(&cfg)
	return &cfg, nil
}

// resolveEnvRefs walks every InstanceConfig.Options and, for any key ending
// `_env`, copies os.Getenv(value) into the corresponding bare key. Allows
// checked-in YAML to reference secrets by env var name without inlining them.
//
// Example: `token_env: ACME_GITHUB_TOKEN` becomes `token: <env value>` at load.
func resolveEnvRefs(cfg *Config) {
	slots := []*SlotConfig{
		&cfg.Tools.VersionControl,
		&cfg.Tools.ProjectMgmt,
		&cfg.Tools.Messaging,
		&cfg.Tools.Design,
		&cfg.Tools.Testing,
		&cfg.Tools.Calendar,
		&cfg.Tools.Email,
	}
	for _, slot := range slots {
		for name, inst := range slot.Instances {
			if inst.Options == nil {
				continue
			}
			for k, v := range inst.Options {
				envName, isRef := strings.CutSuffix(k, "_env")
				if !isRef {
					continue
				}
				envKey, ok := v.(string)
				if !ok || envKey == "" {
					continue
				}
				if val := os.Getenv(envKey); val != "" {
					inst.Options[envName] = val
				}
			}
			slot.Instances[name] = inst
		}
	}
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
	resolveEnvRefs(cfg)
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
