package config

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

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
	Quota         QuotaConfig         `yaml:"quota"`
	Reconcile     ReconcileConfig     `yaml:"reconcile"`
}

// ReconcileConfig bounds the supervisor that holds standing objectives at their
// declared state (Phase 20).
//
// Everything here is a ceiling rather than a target. A deployment with no
// standing objectives is unaffected by every value in it, and a deployment with
// a thousand is protected by all of them.
type ReconcileConfig struct {
	// Enabled is the kill switch. Off stops the supervisor from starting at
	// all; standing objectives keep their state and simply stop being due.
	// Default on, because the feature does nothing until somebody declares a
	// standing objective, and an operator who declared one meant it.
	Enabled *bool `yaml:"enabled"`

	// Tick is how often the supervisor asks which objectives are due. It
	// bounds scheduling precision, not cadence: a 30s tick with an hourly
	// cadence is accurate to half a minute, which is what a schedule
	// measured in hours deserves.
	Tick string `yaml:"tick"`

	// MaxConcurrent bounds reconciles running at once across this replica.
	// Without it, a hundred objectives coming due together would launch a
	// hundred concurrent model-calling loops — the failure mode is a bill
	// and a rate-limit wall, and it arrives all at once.
	MaxConcurrent int `yaml:"max_concurrent"`

	// LeaseTTL is how long a claim survives without renewal. It is the
	// window in which a crashed replica's work sits untouched, so it trades
	// recovery latency against the risk of two replicas both believing they
	// hold an objective whose renewals were merely slow.
	LeaseTTL string `yaml:"lease_ttl"`

	// BreakerFailures is how many consecutive failures pause an objective.
	// An escalation is not a failure — a loop that stopped to ask a question
	// did the right thing, and a breaker counting questions would trip on
	// the objectives being most careful.
	BreakerFailures int `yaml:"breaker_failures"`

	// StallReconciles is how many expensive runs may leave the criteria
	// score unmoved before the objective escalates instead of running again.
	// This is the brake the roadmap promised in Phase 1 and never built: the
	// only real limit until now was the iteration cap and the token budget.
	StallReconciles int `yaml:"stall_reconciles"`

	// DefaultMinInterval is the floor between reconciles for an objective
	// whose cadence did not name one. It is what stops a busy repository
	// from driving a paid loop every time somebody pushes.
	DefaultMinInterval string `yaml:"default_min_interval"`

	// MaxBackoff caps the exponential backoff after repeated failures, so a
	// long-broken objective still retries occasionally rather than
	// effectively never.
	MaxBackoff string `yaml:"max_backoff"`
}

// IsEnabled reports whether the supervisor should run. Nil means yes: the
// field is a pointer precisely so that "absent" and "explicitly false" are
// different, and only the latter turns the feature off.
func (r ReconcileConfig) IsEnabled() bool { return r.Enabled == nil || *r.Enabled }

func (r ReconcileConfig) TickDuration() time.Duration {
	return parseDurationOr(r.Tick, 30*time.Second)
}

func (r ReconcileConfig) LeaseTTLDuration() time.Duration {
	return parseDurationOr(r.LeaseTTL, 5*time.Minute)
}

func (r ReconcileConfig) DefaultMinIntervalDuration() time.Duration {
	return parseDurationOr(r.DefaultMinInterval, 10*time.Minute)
}

func (r ReconcileConfig) MaxBackoffDuration() time.Duration {
	return parseDurationOr(r.MaxBackoff, time.Hour)
}

// QuotaConfig configures rate limiting and quotas (ADR 008). Zero values mean
// "use the shipped default", so a deployment overrides only what it cares
// about.
type QuotaConfig struct {
	// Backend selects where counters live: memory, sql or valkey.
	//
	// memory is the default and is **per replica** — a limit of 60/min across
	// three replicas admits 180. That is fine for a single-instance
	// deployment and wrong behind a load balancer, where valkey is the only
	// option that is consistent across replicas.
	Backend string `yaml:"backend"`

	// ValkeyURL is required when Backend is valkey, e.g.
	// "valkey://:password@valkey:6379/0".
	ValkeyURL string `yaml:"valkey_url"`

	// RequestsPerMinute and RequestBurst cap how fast one principal can drive
	// the API. Burst defaults to RequestsPerMinute when unset.
	RequestsPerMinute int `yaml:"requests_per_minute"`
	RequestBurst      int `yaml:"request_burst"`

	// CapabilityPerDay caps invocations of one capability by one twin.
	CapabilityPerDay int `yaml:"capability_per_day"`

	// LLMTokensPerDay caps a twin's model spend, counted in tokens.
	LLMTokensPerDay int `yaml:"llm_tokens_per_day"`

	// AdapterPerDay caps calls to one external adapter.
	AdapterPerDay int `yaml:"adapter_per_day"`

	// LLMBudgetBackend selects who counts model spend: native or litellm.
	//
	// native counts tokens through this process and needs nothing else.
	// litellm delegates to a gateway that counts dollars against a maintained
	// per-model cost table, which is what most operators actually want to cap
	// — but it is a service to run, so it is opt-in.
	LLMBudgetBackend string `yaml:"llm_budget_backend"`

	// LiteLLMURL and LiteLLMKeyEnv locate the gateway. The key is named by
	// environment variable rather than inlined, matching the `*_env`
	// convention (ADR 006).
	LiteLLMURL    string `yaml:"litellm_url"`
	LiteLLMKeyEnv string `yaml:"litellm_key_env"`

	// Rates price what was consumed, so a spend report is in money rather than
	// in tokens.
	//
	// Parsed here rather than by the cost module, which takes a Go map: a price
	// table is configuration, and a module whose require block is empty should
	// not gain a YAML parser to read one.
	//
	// A rate with no model is the provider's fallback. Nothing here is a
	// default — a shipped price table would be wrong the week after it shipped,
	// and a report that invents money is worse than one that reports units.
	Rates []QuotaRateConfig `yaml:"rates"`

	// CostRetentionDays is how long individual cost events are kept. The daily
	// rollup survives pruning, so a shorter horizon costs the drill-down and
	// not the totals. Zero keeps everything.
	CostRetentionDays int `yaml:"cost_retention_days"`
}

// QuotaRateConfig is the price of one unit from one provider.
type QuotaRateConfig struct {
	Provider string `yaml:"provider"`

	// Model is optional. An entry without one is the provider's fallback, used
	// when no rate names the model exactly — providers ship models faster than
	// anybody updates a config file.
	Model string `yaml:"model"`

	// UnitKind defaults to tokens.
	UnitKind string `yaml:"unit_kind"`

	// PerUnit is the cost of a single unit. Model prices are quoted per million
	// tokens, so this is a small number: 15 dollars per million is 0.000015.
	PerUnit float64 `yaml:"per_unit"`
}

// LLM budget backends for QuotaConfig.LLMBudgetBackend.
const (
	LLMBudgetNative  = "native"
	LLMBudgetLiteLLM = "litellm"
)

// Backend names for QuotaConfig.Backend.
const (
	QuotaBackendMemory = "memory"
	QuotaBackendSQL    = "sql"
	QuotaBackendValkey = "valkey"
)

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

// AuthConfig configures JWT-based authentication and RBAC. There is no
// "disabled" mode and no static shared token: every request to /api/v1 carries
// a short-lived access token, and every route is gated by a permission.
type AuthConfig struct {
	JWT       JWTConfig           `yaml:"jwt"`
	Bootstrap AuthBootstrapConfig `yaml:"bootstrap"`
	Cookies   AuthCookieConfig    `yaml:"cookies"`

	// Provider selects the identity provider federated logins go through:
	// "bearer" (the default — local passwords only), "oidc" or "saml".
	//
	// Selecting one does not switch local password login off. It stays mounted
	// alongside, which is deliberate: it is the break-glass path when the
	// identity provider is unreachable, and re-adding a static shared token to
	// serve that purpose is exactly what Phase 14 removed.
	Provider string `yaml:"provider"`

	OIDC     AuthOIDCConfig     `yaml:"oidc"`
	SAML     AuthSAMLConfig     `yaml:"saml"`
	RoleMap  AuthRoleMapConfig  `yaml:"role_map"`
	Frontend AuthFrontendConfig `yaml:"frontend"`
}

// Federated identity providers Provider may name.
const (
	AuthProviderBearer = "bearer"
	AuthProviderOIDC   = "oidc"
	AuthProviderSAML   = "saml"
)

// AuthRoleMapConfig maps the groups an identity provider asserts onto Karakuri
// roles, optionally inside an organisation, team or project.
//
// One identity provider is configured at a time, so group names come from a
// single namespace and cannot collide. Federating two providers would need this
// map keyed by issuer; that is not supported.
type AuthRoleMapConfig struct {
	// Groups maps an asserted group name to what it grants.
	Groups map[string][]AuthRoleGrantConfig `yaml:"groups"`

	// Default is granted to a user who authenticated but matched no group.
	//
	// It is empty unless an operator sets it, and that is the important part:
	// everybody in a corporate directory can authenticate against a corporate
	// identity provider, so a default role here is a grant to the whole
	// company. Leaving it empty means such a user can log in and see nothing,
	// which is the correct shape — authentication is not authorization.
	Default []AuthRoleGrantConfig `yaml:"default"`
}

// AuthRoleGrantConfig is one mapped role and where it applies.
//
// Containers are named rather than identified because a human writes this file
// and nobody knows a team's ID when they do. The names are resolved to IDs once,
// at boot, and only the IDs ever reach a binding — see ADR 010 for why a policy
// must never carry a display name.
type AuthRoleGrantConfig struct {
	Role string `yaml:"role"`

	// Org, Team and Project name the container the role is granted over. At
	// most one of Team and Project may be set, and Team needs its Org to
	// disambiguate: two organisations may each have a team called
	// "Engineering", which is the whole reason names are scoped per parent.
	//
	// All empty means the role is granted over everything, which is what the
	// bare string form means and what every mapping meant before Phase 17.
	Org     string `yaml:"org,omitempty"`
	Team    string `yaml:"team,omitempty"`
	Project string `yaml:"project,omitempty"`
}

// UnmarshalYAML accepts both the bare form and the mapping form:
//
//	karakuri-admins: [admin]
//	acme-engineers:  [{role: operator, org: acme, team: eng}]
//
// The bare form is what every existing configuration file uses and it keeps
// meaning exactly what it meant: the role over everything. A config change is
// not a reasonable price for upgrading.
func (g *AuthRoleGrantConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&g.Role)
	}
	// A distinct type, so decoding the mapping does not call this method again.
	type grant AuthRoleGrantConfig
	var decoded grant
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*g = AuthRoleGrantConfig(decoded)
	return nil
}

// AuthOIDCConfig configures the OpenID Connect provider.
type AuthOIDCConfig struct {
	IssuerURL    string `yaml:"issuer_url"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret,omitempty"`

	// ClientSecretEnv names the environment variable the client secret is read
	// from, matching the `*_env` convention used for every other secret.
	ClientSecretEnv string `yaml:"client_secret_env,omitempty"`

	// RedirectURL is this server's own callback URL as the provider sees it —
	// it must match what is registered there. Defaults to PublicURL plus
	// /api/v1/auth/sso/callback.
	RedirectURL string `yaml:"redirect_url"`

	// Scopes requested at login. Empty means openid, profile and email.
	Scopes []string `yaml:"scopes"`

	// GroupsClaim, EmailClaim and NameClaim locate those values in the token's
	// claims, as dotted paths. There is no standard claim for group membership:
	// Keycloak nests it under "realm_access.roles", Okta and Auth0 use
	// "groups", Azure AD emits object IDs. Empty means "groups", "email" and
	// "name" respectively.
	GroupsClaim string `yaml:"groups_claim"`
	EmailClaim  string `yaml:"email_claim"`
	NameClaim   string `yaml:"name_claim"`
}

// AuthSAMLConfig configures the SAML 2.0 service provider.
type AuthSAMLConfig struct {
	// IDPMetadataURL or IDPMetadataFile supplies the identity provider's
	// metadata. Exactly one is needed; the URL is fetched once at startup.
	IDPMetadataURL  string `yaml:"idp_metadata_url"`
	IDPMetadataFile string `yaml:"idp_metadata_file"`

	// EntityID identifies this service provider. Defaults to the metadata URL.
	EntityID string `yaml:"entity_id"`

	// RoleAttribute, EmailAttribute and NameAttribute name the assertion
	// attributes to read. Each is matched against both an attribute's Name and
	// its FriendlyName, because providers populate one, the other, or both.
	RoleAttribute  string `yaml:"role_attribute"`
	EmailAttribute string `yaml:"email_attribute"`
	NameAttribute  string `yaml:"name_attribute"`

	// AllowIDPInitiated accepts logins this server did not start. Off by
	// default: without a request of ours to correlate against, the guarantee
	// that a response answers a request we actually sent is gone.
	AllowIDPInitiated bool `yaml:"allow_idp_initiated"`
}

// AuthFrontendConfig tells the server where to send a browser once a federated
// login succeeds.
type AuthFrontendConfig struct {
	// PublicURL is this server's externally reachable base URL. It is what
	// redirect and assertion-consumer URLs are derived from, and it cannot be
	// inferred from an inbound request: a proxied Host header is
	// attacker-controlled, and inferring a redirect target from one is how open
	// redirects happen.
	PublicURL string `yaml:"public_url"`

	// LoginRedirect is where the browser lands after a successful federated
	// login. Defaults to "/".
	LoginRedirect string `yaml:"login_redirect"`
}

// AuthCookieConfig tunes the httpOnly session cookies the browser client uses.
// Names, paths and lifetimes are derived from the JWT settings; the only thing
// left to decide is whether the deployment is allowed to weaken them.
type AuthCookieConfig struct {
	// InsecureAllowHTTP issues session cookies without the Secure attribute on
	// requests that did not arrive over TLS. It defaults to false and should
	// stay that way anywhere real: a session cookie without Secure travels in
	// the clear the first time anything downgrades a request to http://.
	//
	// It exists for plain-HTTP local development and for the integration
	// suite, whose clients — unlike browsers, which trust localhost — refuse to
	// send Secure cookies over http and would otherwise fail every login.
	InsecureAllowHTTP bool `yaml:"insecure_allow_http"`
}

// JWTConfig parameterises token issuance and verification.
type JWTConfig struct {
	Issuer   string `yaml:"issuer"`
	Audience string `yaml:"audience"`

	// AccessTTL and RefreshTTL are Go duration strings ("15m", "720h").
	AccessTTL  string `yaml:"access_ttl"`
	RefreshTTL string `yaml:"refresh_ttl"`

	// Keys lists every key a verifier accepts. Exactly one is Active and does
	// the signing; the rest keep previously-issued tokens verifiable, so
	// rotating the signer does not log everyone out.
	Keys []JWTKeyConfig `yaml:"keys"`
}

// JWTKeyConfig declares one signing or verification key. Secrets are never
// inlined in checked-in YAML — SecretEnv names an environment variable, matching
// the `*_env` convention the tool adapters already use (ADR 006).
type JWTKeyConfig struct {
	ID             string `yaml:"kid"`
	Algorithm      string `yaml:"algorithm"` // HS256 (default) or EdDSA
	Active         bool   `yaml:"active"`
	Secret         string `yaml:"secret,omitempty"`
	SecretEnv      string `yaml:"secret_env,omitempty"`
	PrivateKeyFile string `yaml:"private_key_file,omitempty"`
	PublicKeyFile  string `yaml:"public_key_file,omitempty"`
}

// AuthBootstrapConfig controls the first-boot administrator. On a database with
// no principals, the server mints this account so an operator can log in; on
// every later boot it is a no-op.
type AuthBootstrapConfig struct {
	// AdminID is the identifier given to that first administrator.
	AdminID string `yaml:"admin_id"`

	// EnvVar names the environment variable the first administrator's password
	// is read from. It holds the *name* of a variable and never the password
	// itself — no field on this struct is a credential, which is what makes it
	// safe to put in an error message telling an operator what to set.
	EnvVar string `yaml:"password_env"`
}

// AccessTTLDuration parses AccessTTL, falling back to 15 minutes.
func (c JWTConfig) AccessTTLDuration() time.Duration {
	return parseDurationOr(c.AccessTTL, 15*time.Minute)
}

// RefreshTTLDuration parses RefreshTTL, falling back to 30 days.
func (c JWTConfig) RefreshTTLDuration() time.Duration {
	return parseDurationOr(c.RefreshTTL, 30*24*time.Hour)
}

func parseDurationOr(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

type DomainConfig struct {
	ID      string         `yaml:"id"`
	Enabled bool           `yaml:"enabled"`
	Options map[string]any `yaml:"options,omitempty"`
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
	Enabled           bool    `yaml:"enabled"`
	IntervalMinutes   int     `yaml:"interval_minutes"`    // sweep interval; default 60
	WorkingTTLMinutes int     `yaml:"working_ttl_minutes"` // 0 = never
	EpisodicTTLDays   int     `yaml:"episodic_ttl_days"`   // 0 = never
	SemanticTTLDays   int     `yaml:"semantic_ttl_days"`   // 0 = never
	SemanticMinScore  float64 `yaml:"semantic_min_score"`  // drop semantic entries with confidence below this; 0 = no floor
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
//	KARAKURI_AUTH_ISSUER             → cfg.Auth.JWT.Issuer
//	KARAKURI_AUTH_AUDIENCE           → cfg.Auth.JWT.Audience
//	KARAKURI_AUTH_ACCESS_TTL         → cfg.Auth.JWT.AccessTTL   (e.g. "15m")
//	KARAKURI_AUTH_REFRESH_TTL        → cfg.Auth.JWT.RefreshTTL  (e.g. "720h")
//	KARAKURI_AUTH_JWT_SECRET         → an HS256 signing key, when no keys are
//	                                   declared in YAML (the common deployment)
//	KARAKURI_AUTH_BOOTSTRAP_ADMIN    → cfg.Auth.Bootstrap.AdminID
//	KARAKURI_AUTH_COOKIES_INSECURE   → cfg.Auth.Cookies.InsecureAllowHTTP,
//	                                   for plain-HTTP local development only
//	KARAKURI_QUOTA_BACKEND           → cfg.Quota.Backend (memory|sql|valkey)
//	KARAKURI_QUOTA_VALKEY_URL        → cfg.Quota.ValkeyURL
//	KARAKURI_QUOTA_REQUESTS_PER_MIN  → cfg.Quota.RequestsPerMinute
//	KARAKURI_QUOTA_LLM_TOKENS_PER_DAY → cfg.Quota.LLMTokensPerDay
//	KARAKURI_AUTH_PROVIDER           → cfg.Auth.Provider (bearer|oidc|saml)
//	KARAKURI_AUTH_PUBLIC_URL         → cfg.Auth.Frontend.PublicURL
//	KARAKURI_AUTH_OIDC_ISSUER_URL    → cfg.Auth.OIDC.IssuerURL
//	KARAKURI_AUTH_OIDC_CLIENT_ID     → cfg.Auth.OIDC.ClientID
//	KARAKURI_AUTH_OIDC_CLIENT_SECRET → cfg.Auth.OIDC.ClientSecret
//	KARAKURI_AUTH_OIDC_GROUPS_CLAIM  → cfg.Auth.OIDC.GroupsClaim
//	KARAKURI_AUTH_SAML_IDP_METADATA_URL  → cfg.Auth.SAML.IDPMetadataURL
//	KARAKURI_AUTH_SAML_IDP_METADATA_FILE → cfg.Auth.SAML.IDPMetadataFile
//	KARAKURI_AUTH_SAML_ROLE_ATTRIBUTE    → cfg.Auth.SAML.RoleAttribute
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
	if v := os.Getenv("KARAKURI_AUTH_ISSUER"); v != "" {
		cfg.Auth.JWT.Issuer = v
	}
	if v := os.Getenv("KARAKURI_AUTH_AUDIENCE"); v != "" {
		cfg.Auth.JWT.Audience = v
	}
	if v := os.Getenv("KARAKURI_AUTH_ACCESS_TTL"); v != "" {
		cfg.Auth.JWT.AccessTTL = v
	}
	if v := os.Getenv("KARAKURI_AUTH_REFRESH_TTL"); v != "" {
		cfg.Auth.JWT.RefreshTTL = v
	}
	if v := os.Getenv("KARAKURI_AUTH_BOOTSTRAP_ADMIN"); v != "" {
		cfg.Auth.Bootstrap.AdminID = v
	}
	for _, o := range []struct {
		env string
		dst *string
	}{
		{"KARAKURI_AUTH_PROVIDER", &cfg.Auth.Provider},
		{"KARAKURI_AUTH_PUBLIC_URL", &cfg.Auth.Frontend.PublicURL},
		{"KARAKURI_AUTH_OIDC_ISSUER_URL", &cfg.Auth.OIDC.IssuerURL},
		{"KARAKURI_AUTH_OIDC_CLIENT_ID", &cfg.Auth.OIDC.ClientID},
		{"KARAKURI_AUTH_OIDC_CLIENT_SECRET", &cfg.Auth.OIDC.ClientSecret},
		{"KARAKURI_AUTH_OIDC_GROUPS_CLAIM", &cfg.Auth.OIDC.GroupsClaim},
		{"KARAKURI_AUTH_SAML_IDP_METADATA_URL", &cfg.Auth.SAML.IDPMetadataURL},
		{"KARAKURI_AUTH_SAML_IDP_METADATA_FILE", &cfg.Auth.SAML.IDPMetadataFile},
		{"KARAKURI_AUTH_SAML_ROLE_ATTRIBUTE", &cfg.Auth.SAML.RoleAttribute},
	} {
		if v := os.Getenv(o.env); v != "" {
			*o.dst = v
		}
	}
	// Only an explicit truthy value turns Secure off; anything unparseable
	// leaves the safe default in place rather than guessing.
	if v := os.Getenv("KARAKURI_QUOTA_BACKEND"); v != "" {
		cfg.Quota.Backend = v
	}
	if v := os.Getenv("KARAKURI_QUOTA_VALKEY_URL"); v != "" {
		cfg.Quota.ValkeyURL = v
	}
	if v := os.Getenv("KARAKURI_QUOTA_LLM_BUDGET_BACKEND"); v != "" {
		cfg.Quota.LLMBudgetBackend = v
	}
	if v := os.Getenv("KARAKURI_QUOTA_LITELLM_URL"); v != "" {
		cfg.Quota.LiteLLMURL = v
	}
	// A limit that fails to parse leaves the default in place rather than
	// becoming zero, which Policy.Validate would reject at boot — a typo in an
	// env var should not stop the server.
	for _, o := range []struct {
		env string
		dst *int
	}{
		{"KARAKURI_QUOTA_REQUESTS_PER_MIN", &cfg.Quota.RequestsPerMinute},
		{"KARAKURI_QUOTA_REQUEST_BURST", &cfg.Quota.RequestBurst},
		{"KARAKURI_QUOTA_CAPABILITY_PER_DAY", &cfg.Quota.CapabilityPerDay},
		{"KARAKURI_QUOTA_LLM_TOKENS_PER_DAY", &cfg.Quota.LLMTokensPerDay},
		{"KARAKURI_QUOTA_ADAPTER_PER_DAY", &cfg.Quota.AdapterPerDay},
	} {
		if v := os.Getenv(o.env); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				*o.dst = n
			}
		}
	}
	if v := os.Getenv("KARAKURI_AUTH_COOKIES_INSECURE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Auth.Cookies.InsecureAllowHTTP = b
		}
	}
	// The single-secret path: most deployments set one env var rather than
	// declaring a keyring. Declared keys win, so an operator who has moved to
	// explicit rotation is not overridden by a leftover variable.
	if v := os.Getenv("KARAKURI_AUTH_JWT_SECRET"); v != "" && len(cfg.Auth.JWT.Keys) == 0 {
		cfg.Auth.JWT.Keys = []JWTKeyConfig{{
			ID:        "env",
			Algorithm: "HS256",
			Active:    true,
			Secret:    v,
		}}
	}
	// Resolve `secret_env` references the same way tool adapters do.
	for i, k := range cfg.Auth.JWT.Keys {
		if k.Secret == "" && k.SecretEnv != "" {
			cfg.Auth.JWT.Keys[i].Secret = os.Getenv(k.SecretEnv)
		}
	}
	if cfg.Auth.OIDC.ClientSecret == "" && cfg.Auth.OIDC.ClientSecretEnv != "" {
		cfg.Auth.OIDC.ClientSecret = os.Getenv(cfg.Auth.OIDC.ClientSecretEnv)
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
	if cfg.Auth.JWT.Issuer == "" {
		cfg.Auth.JWT.Issuer = "karakuri"
	}
	if cfg.Auth.JWT.Audience == "" {
		cfg.Auth.JWT.Audience = "karakuri-api"
	}
	if cfg.Auth.JWT.AccessTTL == "" {
		cfg.Auth.JWT.AccessTTL = "15m"
	}
	if cfg.Auth.JWT.RefreshTTL == "" {
		cfg.Auth.JWT.RefreshTTL = "720h"
	}
	if cfg.Auth.Bootstrap.AdminID == "" {
		cfg.Auth.Bootstrap.AdminID = "admin"
	}
	if cfg.Auth.Provider == "" {
		// Local passwords only. Federation is something an operator turns on,
		// not something a fresh install finds itself in.
		cfg.Auth.Provider = AuthProviderBearer
	}
	if cfg.Auth.Frontend.LoginRedirect == "" {
		cfg.Auth.Frontend.LoginRedirect = "/"
	}
	if cfg.Quota.LLMBudgetBackend == "" {
		cfg.Quota.LLMBudgetBackend = LLMBudgetNative
	}
	if cfg.Quota.LiteLLMKeyEnv == "" {
		cfg.Quota.LiteLLMKeyEnv = "LITELLM_MASTER_KEY"
	}
	if cfg.Reconcile.MaxConcurrent <= 0 {
		// Four rather than one: a deployment holding several standing
		// objectives should not have them queue behind each other, and a
		// number this small is still a ceiling somebody notices before the
		// bill does.
		cfg.Reconcile.MaxConcurrent = 4
	}
	if cfg.Reconcile.BreakerFailures <= 0 {
		cfg.Reconcile.BreakerFailures = 3
	}
	if cfg.Reconcile.StallReconciles <= 0 {
		cfg.Reconcile.StallReconciles = 3
	}
	if cfg.Quota.Backend == "" {
		// Per-replica, and documented as such. Anything else needs a
		// deployment decision, and defaulting to a backend that requires
		// infrastructure would stop a single-binary install from booting.
		cfg.Quota.Backend = QuotaBackendMemory
	}
	if cfg.Auth.Bootstrap.EnvVar == "" {
		cfg.Auth.Bootstrap.EnvVar = "KARAKURI_AUTH_BOOTSTRAP_PASSWORD"
	}
	// Note: no default signing key. A server with none refuses to start
	// (see internal/auth.Keyring) rather than falling back to something
	// predictable — a default JWT secret is a backdoor with a changelog entry.
}

func Default() *Config {
	cfg := &Config{}
	setDefaults(cfg)
	resolveEnvRefs(cfg)
	overrideFromEnv(cfg)
	cfg.Observability.Exporters = []ExporterConfig{{
		Name: "local", Enabled: true, Path: "./karakuri-obs/",
		Formats:  map[string]string{"metrics": "ndjson", "logs": "ndjson"},
		Rotation: RotationConfig{MaxSizeMB: 100, MaxAgeDays: 30},
	}}
	cfg.Domains = []DomainConfig{
		{ID: "software", Enabled: true},
	}
	return cfg
}
