package config

import (
	"os"
	"os/exec"
	"strconv"
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
	CLIAgents      SlotConfig `yaml:"cli_agents"`
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
	ConsolidationThreshold int                   `yaml:"consolidation_threshold"` // episodic entries before consolidation
	SemanticTopK           int                   `yaml:"semantic_top_k"`
	VectorBackend          string                `yaml:"vector_backend"` // "" | "sqlite-keyword" | "pgvector"
	EmbeddingDim           int                   `yaml:"embedding_dim"`  // dimensionality of embeddings (default 1536)
	Retention              MemoryRetentionConfig `yaml:"retention"`
}

// MemoryRetentionConfig controls the periodic retention sweep. When Enabled
// is true, MemoryService.RunRetention runs every IntervalMinutes and drops
// entries older than the per-tier TTLs or below the semantic confidence
// floor. Disabled by default — leave it off unless you've measured memory
// growth — once on, the deletions are irreversible.
type MemoryRetentionConfig struct {
	Enabled          bool    `yaml:"enabled"`
	IntervalMinutes  int     `yaml:"interval_minutes"`   // sweep interval; default 60
	WorkingTTLMinutes int    `yaml:"working_ttl_minutes"` // 0 = never
	EpisodicTTLDays  int     `yaml:"episodic_ttl_days"`  // 0 = never
	SemanticTTLDays  int     `yaml:"semantic_ttl_days"`  // 0 = never
	SemanticMinScore float64 `yaml:"semantic_min_score"` // drop semantic entries with confidence below this; 0 = no floor
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
	ensureGitHubToken()
	resolveEnvRefs(&cfg)
	overrideFromEnv(&cfg)
	return &cfg, nil
}

// ensureGitHubToken populates GITHUB_TOKEN from `gh auth token` when the
// env var is unset and the `gh` CLI is installed and authenticated. Runs
// before resolveEnvRefs so YAML `token_env: GITHUB_TOKEN` references pick
// up the value naturally. A failure (gh missing, not logged in, etc.) is
// a no-op — the github tool adapter will surface the missing token at
// startup the same way it does today.
func ensureGitHubToken() {
	if os.Getenv("GITHUB_TOKEN") != "" {
		return
	}
	cmd := exec.Command("gh", "auth", "token")
	out, err := cmd.Output()
	if err != nil {
		return
	}
	tok := strings.TrimSpace(string(out))
	if tok == "" {
		return
	}
	_ = os.Setenv("GITHUB_TOKEN", tok)
}

// overrideFromEnv lets Helm/Compose flip core settings without rewriting the
// static YAML. Currently honors:
//
//	KARAKURI_DATABASE_DRIVER         → cfg.Database.Driver  (e.g. "postgres")
//	KARAKURI_DATABASE_DSN            → cfg.Database.DSN
//	KARAKURI_MEMORY_VECTOR_BACKEND   → cfg.Memory.VectorBackend (e.g. "pgvector")
//	KARAKURI_MEMORY_EMBEDDING_DIM    → cfg.Memory.EmbeddingDim
func overrideFromEnv(cfg *Config) {
	if v := os.Getenv("KARAKURI_DATABASE_DRIVER"); v != "" {
		cfg.Database.Driver = v
	}
	if v := os.Getenv("KARAKURI_DATABASE_DSN"); v != "" {
		cfg.Database.DSN = v
	}
	if v := os.Getenv("KARAKURI_MEMORY_VECTOR_BACKEND"); v != "" {
		cfg.Memory.VectorBackend = v
	}
	if v := os.Getenv("KARAKURI_MEMORY_EMBEDDING_DIM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Memory.EmbeddingDim = n
		}
	}
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
		&cfg.Tools.CLIAgents,
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
	if cfg.Memory.VectorBackend == "" {
		cfg.Memory.VectorBackend = "sqlite-keyword"
	}
	if cfg.Memory.EmbeddingDim == 0 {
		cfg.Memory.EmbeddingDim = 1536
	}
}

func Default() *Config {
	cfg := &Config{}
	setDefaults(cfg)
	resolveEnvRefs(cfg)
	overrideFromEnv(cfg)
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
