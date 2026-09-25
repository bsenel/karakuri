package tools

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/calendar"
	"github.com/bsenel/karakuri/internal/platform/tools/cliagent"
	"github.com/bsenel/karakuri/internal/platform/tools/design"
	"github.com/bsenel/karakuri/internal/platform/tools/email"
	"github.com/bsenel/karakuri/internal/platform/tools/mcp"
	"github.com/bsenel/karakuri/internal/platform/tools/messaging"
	"github.com/bsenel/karakuri/internal/platform/tools/observability"
	"github.com/bsenel/karakuri/internal/platform/tools/projectmgmt"
	"github.com/bsenel/karakuri/internal/platform/tools/research"
	"github.com/bsenel/karakuri/internal/platform/tools/testing"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// SlotInstances holds a typed set of named adapter instances for one slot plus
// the name of the default instance. Resolve("") returns the default; Resolve("x")
// returns the named instance or false if unknown.
type SlotInstances[T any] struct {
	defaultName string
	instances   map[string]instanceEntry[T]
}

type instanceEntry[T any] struct {
	typeName string // "github", "linear", "noop", …
	adapter  T
}

// Resolve returns the adapter for the given instance name. Empty name → default.
// Returns the zero value + false if unknown.
func (s SlotInstances[T]) Resolve(name string) (T, bool) {
	var zero T
	if name == "" {
		name = s.defaultName
	}
	if name == "" {
		return zero, false
	}
	e, ok := s.instances[name]
	if !ok {
		return zero, false
	}
	return e.adapter, true
}

// DefaultName returns the configured default instance name (may be "").
func (s SlotInstances[T]) DefaultName() string { return s.defaultName }

// Set installs an adapter under a name, becoming the default if the slot has
// none. Overwrites an existing entry with the same name.
//
// It exists because the slot could otherwise only be filled by buildXSlot,
// which switches on a type string and can only construct the shipped adapters.
// An adapter that is not one of those — a stub standing in for a coding-agent
// CLI or a forge — had no way into the registry at all, which is why the write
// path had no end-to-end test until Phase 26: the chain it needed could be
// described but not assembled.
func (s *SlotInstances[T]) Set(name, typeName string, adapter T) {
	if name == "" {
		return
	}
	if s.instances == nil {
		s.instances = map[string]instanceEntry[T]{}
	}
	s.instances[name] = instanceEntry[T]{typeName: typeName, adapter: adapter}
	if s.defaultName == "" {
		s.defaultName = name
	}
}

// Names returns instance names + their type, ordered as-is (map iteration).
// Used by /health to enumerate the topology.
func (s SlotInstances[T]) List() []InstanceInfo {
	out := make([]InstanceInfo, 0, len(s.instances))
	for name, e := range s.instances {
		out = append(out, InstanceInfo{Name: name, Type: e.typeName, IsDefault: name == s.defaultName})
	}
	return out
}

// InstanceInfo is the /health-shaped view of one configured instance.
type InstanceInfo struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	IsDefault bool   `json:"is_default"`
}

// ── Registry ─────────────────────────────────────────────────────────────────

type Registry struct {
	VC          SlotInstances[versioncontrol.VersionControlAdapter]
	ProjectMgmt SlotInstances[projectmgmt.ProjectManagementAdapter]
	Messaging   SlotInstances[messaging.MessagingAdapter]
	Design      SlotInstances[design.DesignAdapter]
	Testing     SlotInstances[testing.TestingAdapter]
	Calendar    SlotInstances[calendar.CalendarAdapter]
	Email       SlotInstances[email.EmailAdapter]
	CLIAgents   SlotInstances[cliagent.CLIAgentAdapter]

	// MCP is the eleventh slot and the only one whose adapters were not written
	// here: each instance is one MCP server, and what it offers is read off it
	// at boot rather than declared in this package (ADR 022).
	MCP SlotInstances[*mcp.Instance]

	// Single-instance slots — kept simple until use cases demand multi-instance.
	Observability observability.ObservabilityAdapter
	Research      research.ResearchAdapter

	mu sync.RWMutex
}

// AdapterStatus describes one configured (slot, instance) pair for /health.
type AdapterStatus struct {
	Slot      string `json:"slot"`
	Instance  string `json:"instance"`
	Type      string `json:"type"`
	Active    bool   `json:"active"`
	IsDefault bool   `json:"is_default"`
}

// NewRegistry returns an empty registry where every slot has no instances. Loop
// code resolving an unknown instance falls through to a slot's no-op adapter
// (added below in NewRegistryFromConfig as the implicit zero-value behavior).
func NewRegistry() *Registry {
	return &Registry{
		Observability: observability.NewNoOp(),
		Research:      research.NewHTTPScraper(),
	}
}

// NewRegistryFromConfig builds the registry from a config.ToolsConfig. Each
// slot's Instances are constructed via the per-slot dispatch tables below.
// Unknown adapter types log a warning and are silently skipped.
func NewRegistryFromConfig(cfg config.ToolsConfig) *Registry {
	r := NewRegistry()
	r.VC = buildVCSlot(cfg.VersionControl)
	r.ProjectMgmt = buildPMSlot(cfg.ProjectMgmt)
	r.Messaging = buildMessagingSlot(cfg.Messaging)
	r.Design = buildDesignSlot(cfg.Design)
	r.Testing = buildTestingSlot(cfg.Testing)
	r.Calendar = buildCalendarSlot(cfg.Calendar)
	r.Email = buildEmailSlot(cfg.Email)
	r.CLIAgents = buildCLIAgentSlot(cfg.CLIAgents)
	// Last, because it is the only slot builder that talks to anything: each
	// instance runs its handshake and its one tools/list here, so the registry
	// this returns already knows what every server offers.
	r.MCP = buildMCPSlot(context.Background(), cfg.MCP)
	return r
}

// MCPHealth is the per-instance topology /health reports, ordered by name so
// two boots of the same config read the same.
//
// Richer than the AdapterStatus row every slot gets, and deliberately so: an MCP
// instance has two things no other adapter has — a server that names itself, and
// an allowlist with a visible other side. "The server offers delete_repo and this
// deployment does not allow it" is the sentence an operator wants, and it does
// not fit in a boolean.
func (r *Registry) MCPHealth() []mcp.InstanceHealth {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]mcp.InstanceHealth, 0, len(r.MCP.List()))
	for _, info := range r.MCP.List() {
		inst, ok := r.MCP.Resolve(info.Name)
		if !ok {
			continue
		}
		out = append(out, inst.Health(info.IsDefault))
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

// MCPCapabilities is every discovered and allowed tool, as registry entries.
//
// Registered beside a pack's capabilities because the loop looks capabilities up
// by ID — for the workspace question, for quota, for the catalog — and a lookup
// that missed would fall back to defaults nobody chose. They are told apart by
// the reserved namespace rather than by which registry they live in, which is
// what makes the four bounds hold wherever the ID travels.
func (r *Registry) MCPCapabilities() []capability.Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []capability.Capability
	for _, info := range r.MCP.List() {
		if inst, ok := r.MCP.Resolve(info.Name); ok {
			out = append(out, inst.Capabilities()...)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out
}

// MCPEnvironmentFactories is one factory per configured instance, ordered by
// instance name so registration order does not depend on map iteration.
//
// Every instance gets one, including an unreachable server: it serves nothing,
// so it routes nothing, and it stays in /health where an operator can see why.
func (r *Registry) MCPEnvironmentFactories() []environment.Factory {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instances := r.MCP.List()
	sort.Slice(instances, func(a, b int) bool { return instances[a].Name < instances[b].Name })

	out := make([]environment.Factory, 0, len(instances))
	for _, info := range instances {
		inst, ok := r.MCP.Resolve(info.Name)
		if !ok {
			continue
		}
		out = append(out, mcp.NewFactory(inst, info.IsDefault))
	}
	return out
}

// CloseMCP releases every MCP server this registry started — the subprocesses,
// in practice. Called at shutdown; an instance that never connected is closed
// too, because "never connected" and "no process" are not the same thing.
func (r *Registry) CloseMCP() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, info := range r.MCP.List() {
		if inst, ok := r.MCP.Resolve(info.Name); ok {
			_ = inst.Close()
		}
	}
}

// Status returns one row per configured (slot, instance) plus one row per slot
// with no instances (showing as the no-op default). Used by /health.
func (r *Registry) Status() []AdapterStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []AdapterStatus
	collect := func(slot string, instances []InstanceInfo, active func(name string) bool) {
		if len(instances) == 0 {
			out = append(out, AdapterStatus{Slot: slot, Instance: "<noop>", Type: "noop", Active: false, IsDefault: true})
			return
		}
		for _, inst := range instances {
			out = append(out, AdapterStatus{
				Slot: slot, Instance: inst.Name, Type: inst.Type,
				Active: active(inst.Name), IsDefault: inst.IsDefault,
			})
		}
	}
	collect("versioncontrol", r.VC.List(), func(n string) bool {
		a, ok := r.VC.Resolve(n)
		return ok && a.Active()
	})
	collect("projectmgmt", r.ProjectMgmt.List(), func(n string) bool {
		a, ok := r.ProjectMgmt.Resolve(n)
		return ok && a.Active()
	})
	collect("messaging", r.Messaging.List(), func(n string) bool {
		a, ok := r.Messaging.Resolve(n)
		return ok && a.Active()
	})
	collect("design", r.Design.List(), func(n string) bool {
		a, ok := r.Design.Resolve(n)
		return ok && a.Active()
	})
	collect("testing", r.Testing.List(), func(n string) bool {
		a, ok := r.Testing.Resolve(n)
		return ok && a.Active()
	})
	collect("calendar", r.Calendar.List(), func(n string) bool {
		a, ok := r.Calendar.Resolve(n)
		return ok && a.Active()
	})
	collect("email", r.Email.List(), func(n string) bool {
		a, ok := r.Email.Resolve(n)
		return ok && a.Active()
	})
	collect("cli_agents", r.CLIAgents.List(), func(n string) bool {
		a, ok := r.CLIAgents.Resolve(n)
		return ok && a.Active()
	})
	// One row per MCP instance, the same shape as every other slot since
	// Phase 6. Active means the same thing it means everywhere else — this
	// instance can execute something — which for a server is: the handshake
	// succeeded and at least one tool survived the allowlist. MCPHealth carries
	// the rest.
	collect("mcp", r.MCP.List(), func(n string) bool {
		a, ok := r.MCP.Resolve(n)
		return ok && a.Active()
	})
	// Single-instance slots — show as one row each.
	out = append(out, AdapterStatus{Slot: "observability", Instance: "<default>", Type: "noop", Active: r.Observability.Active(), IsDefault: true})
	researchName := "http-scraper"
	if n, ok := r.Research.(interface{ Name() string }); ok {
		researchName = n.Name()
	}
	out = append(out, AdapterStatus{Slot: "research", Instance: "<default>", Type: researchName, Active: r.Research.Active(), IsDefault: true})
	return out
}

// ── Slot builders (one per slot — explicit dispatch on InstanceConfig.Type) ──

func buildVCSlot(cfg config.SlotConfig) SlotInstances[versioncontrol.VersionControlAdapter] {
	s := SlotInstances[versioncontrol.VersionControlAdapter]{
		defaultName: cfg.Default,
		instances:   map[string]instanceEntry[versioncontrol.VersionControlAdapter]{},
	}
	for name, inst := range cfg.Instances {
		switch inst.Type {
		case "github":
			s.instances[name] = instanceEntry[versioncontrol.VersionControlAdapter]{
				typeName: "github",
				adapter:  versioncontrol.NewGitHub(inst.OptString("token"), inst.OptString("repo")),
			}
		default:
			slog.Warn("unknown versioncontrol adapter type", "instance", name, "type", inst.Type)
		}
	}
	return s
}

func buildPMSlot(cfg config.SlotConfig) SlotInstances[projectmgmt.ProjectManagementAdapter] {
	s := SlotInstances[projectmgmt.ProjectManagementAdapter]{
		defaultName: cfg.Default,
		instances:   map[string]instanceEntry[projectmgmt.ProjectManagementAdapter]{},
	}
	for name, inst := range cfg.Instances {
		switch inst.Type {
		case "linear":
			s.instances[name] = instanceEntry[projectmgmt.ProjectManagementAdapter]{
				typeName: "linear",
				adapter:  projectmgmt.NewLinear(inst.OptString("api_key"), inst.OptString("team_id")),
			}
		default:
			slog.Warn("unknown projectmgmt adapter type", "instance", name, "type", inst.Type)
		}
	}
	return s
}

func buildMessagingSlot(cfg config.SlotConfig) SlotInstances[messaging.MessagingAdapter] {
	s := SlotInstances[messaging.MessagingAdapter]{
		defaultName: cfg.Default,
		instances:   map[string]instanceEntry[messaging.MessagingAdapter]{},
	}
	for name, inst := range cfg.Instances {
		switch inst.Type {
		case "slack":
			s.instances[name] = instanceEntry[messaging.MessagingAdapter]{
				typeName: "slack",
				adapter:  messaging.NewSlack(inst.OptString("bot_token"), inst.OptString("default_channel")),
			}
		default:
			slog.Warn("unknown messaging adapter type", "instance", name, "type", inst.Type)
		}
	}
	return s
}

func buildDesignSlot(cfg config.SlotConfig) SlotInstances[design.DesignAdapter] {
	s := SlotInstances[design.DesignAdapter]{
		defaultName: cfg.Default,
		instances:   map[string]instanceEntry[design.DesignAdapter]{},
	}
	for name, inst := range cfg.Instances {
		switch inst.Type {
		case "figma":
			s.instances[name] = instanceEntry[design.DesignAdapter]{
				typeName: "figma",
				adapter:  design.NewFigma(inst.OptString("token")),
			}
		default:
			slog.Warn("unknown design adapter type", "instance", name, "type", inst.Type)
		}
	}
	return s
}

func buildTestingSlot(cfg config.SlotConfig) SlotInstances[testing.TestingAdapter] {
	s := SlotInstances[testing.TestingAdapter]{
		defaultName: cfg.Default,
		instances:   map[string]instanceEntry[testing.TestingAdapter]{},
	}
	for name, inst := range cfg.Instances {
		switch inst.Type {
		case "playwright":
			s.instances[name] = instanceEntry[testing.TestingAdapter]{
				typeName: "playwright",
				adapter:  testing.NewPlaywright(inst.OptString("project_dir")),
			}
		default:
			slog.Warn("unknown testing adapter type", "instance", name, "type", inst.Type)
		}
	}
	return s
}

func buildCalendarSlot(cfg config.SlotConfig) SlotInstances[calendar.CalendarAdapter] {
	s := SlotInstances[calendar.CalendarAdapter]{
		defaultName: cfg.Default,
		instances:   map[string]instanceEntry[calendar.CalendarAdapter]{},
	}
	for name, inst := range cfg.Instances {
		switch inst.Type {
		case "google":
			s.instances[name] = instanceEntry[calendar.CalendarAdapter]{
				typeName: "google",
				adapter:  calendar.NewGoogle(inst.OptString("oauth_token"), inst.OptString("calendar_id")),
			}
		default:
			slog.Warn("unknown calendar adapter type", "instance", name, "type", inst.Type)
		}
	}
	return s
}

func buildCLIAgentSlot(cfg config.SlotConfig) SlotInstances[cliagent.CLIAgentAdapter] {
	s := SlotInstances[cliagent.CLIAgentAdapter]{
		defaultName: cfg.Default,
		instances:   map[string]instanceEntry[cliagent.CLIAgentAdapter]{},
	}
	for name, inst := range cfg.Instances {
		switch inst.Type {
		case "claude_code":
			s.instances[name] = instanceEntry[cliagent.CLIAgentAdapter]{
				typeName: "claude_code",
				adapter:  cliagent.NewClaudeCode(inst.OptString("binary")),
			}
		case "cursor_cli":
			s.instances[name] = instanceEntry[cliagent.CLIAgentAdapter]{
				typeName: "cursor_cli",
				adapter:  cliagent.NewCursorCLI(inst.OptString("binary")),
			}
		case "gemini_cli":
			s.instances[name] = instanceEntry[cliagent.CLIAgentAdapter]{
				typeName: "gemini_cli",
				adapter:  cliagent.NewGeminiCLI(inst.OptString("binary")),
			}
		case "copilot_cli":
			s.instances[name] = instanceEntry[cliagent.CLIAgentAdapter]{
				typeName: "copilot_cli",
				adapter:  cliagent.NewCopilotCLI(inst.OptString("binary")),
			}
		default:
			slog.Warn("unknown cli_agents adapter type", "instance", name, "type", inst.Type)
		}
	}
	return s
}

// buildMCPSlot dials every configured server and asks it what it offers.
//
// The one slot builder that does I/O, and the one that cannot avoid it: an MCP
// server's tools are not knowable from config, and Factory.Serves — the exact
// list the environment registry reverse-indexes once at Register — has to be
// known before registration. Discovery therefore completes here, at boot, per
// ADR 022.
//
// A server that is down does not fail the boot: mcp.NewInstance never returns an
// error, and the instance it returns serves nothing and reports why in /health.
// A whole deployment refusing to start because somebody else's filesystem server
// is not installed would be the wrong trade for every other slot too.
func buildMCPSlot(ctx context.Context, cfg config.SlotConfig) SlotInstances[*mcp.Instance] {
	s := SlotInstances[*mcp.Instance]{
		defaultName: cfg.Default,
		instances:   map[string]instanceEntry[*mcp.Instance]{},
	}
	// Sorted so instances are dialled in a fixed order — the logs of two boots
	// of one config are then comparable, which they are not under map iteration.
	names := make([]string, 0, len(cfg.Instances))
	for name := range cfg.Instances {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		inst := cfg.Instances[name]
		mcpCfg := mcp.Config{
			AllowedTools: inst.OptStrings("allowed_tools"),
			Timeout:      time.Duration(inst.OptInt("timeout_sec")) * time.Second,
		}
		switch inst.Type {
		case "stdio":
			mcpCfg.Transport = mcp.TransportStdio
			mcpCfg.Command = inst.OptString("command")
			mcpCfg.Args = inst.OptStrings("args")
			mcpCfg.WorkDir = inst.OptString("workdir")
			mcpCfg.Env = inst.OptStringMap("env")
		case "streamable_http":
			mcpCfg.Transport = mcp.TransportHTTP
			mcpCfg.URL = inst.OptString("url")
			mcpCfg.Headers = bearerHeaders(inst.OptStringMap("headers"), inst.OptString("bearer_token"))
		default:
			slog.Warn("unknown mcp adapter type", "instance", name, "type", inst.Type)
			continue
		}

		// Warned rather than refused, and the instance is still built: an
		// allowlist somebody forgot to write is a server with no tools, which
		// /health shows as connected and offering nothing. Dropping the instance
		// instead would make the mistake look like a typo in its name.
		if len(mcpCfg.AllowedTools) == 0 {
			slog.Warn("mcp instance allows no tools; none of its tools will be registered",
				"instance", name, "hint", "set allowed_tools")
		}

		s.instances[name] = instanceEntry[*mcp.Instance]{
			typeName: inst.Type,
			adapter:  mcp.NewInstance(ctx, name, mcpCfg),
		}
	}
	return s
}

// bearerHeaders puts the instance's credential where the transport expects it.
//
// Read as a top-level `bearer_token` option rather than out of `headers:`
// because config's `*_env` resolution only walks an instance's own keys — a
// secret referenced from inside a nested map would never be substituted, and
// would reach the server as the literal name of an environment variable.
func bearerHeaders(headers map[string]string, token string) map[string]string {
	if token == "" {
		return headers
	}
	out := make(map[string]string, len(headers)+1)
	for k, v := range headers {
		out[k] = v
	}
	out["Authorization"] = "Bearer " + token
	return out
}

func buildEmailSlot(cfg config.SlotConfig) SlotInstances[email.EmailAdapter] {
	s := SlotInstances[email.EmailAdapter]{
		defaultName: cfg.Default,
		instances:   map[string]instanceEntry[email.EmailAdapter]{},
	}
	for name, inst := range cfg.Instances {
		switch inst.Type {
		case "gmail":
			s.instances[name] = instanceEntry[email.EmailAdapter]{
				typeName: "gmail",
				adapter:  email.NewGmail(inst.OptString("oauth_token"), inst.OptString("from_address")),
			}
		case "outlook":
			s.instances[name] = instanceEntry[email.EmailAdapter]{
				typeName: "outlook",
				adapter:  email.NewOutlook(inst.OptString("oauth_token"), inst.OptString("from_address")),
			}
		case "smtp":
			port := inst.OptInt("port")
			if port == 0 {
				port = 587
			}
			s.instances[name] = instanceEntry[email.EmailAdapter]{
				typeName: "smtp",
				adapter: email.NewSMTP(inst.OptString("host"), port,
					inst.OptString("username"), inst.OptString("password"), inst.OptString("from_address")),
			}
		case "apple_mail":
			s.instances[name] = instanceEntry[email.EmailAdapter]{
				typeName: "apple_mail",
				adapter:  email.NewAppleMail(inst.OptString("from_address")),
			}
		default:
			slog.Warn("unknown email adapter type", "instance", name, "type", inst.Type)
		}
	}
	return s
}
