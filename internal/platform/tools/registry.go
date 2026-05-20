package tools

import (
	"log/slog"
	"sync"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/platform/tools/calendar"
	"github.com/bsenel/karakuri/internal/platform/tools/design"
	"github.com/bsenel/karakuri/internal/platform/tools/email"
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
	return r
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
