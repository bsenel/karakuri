package loop

import (
	"context"
	"strings"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// stubEnv is a minimal Environment implementation for catalog tests.
// We only need ID() and Domain() — Observe/Act/Subscribe/Snapshot are
// never called by buildReasonCatalog.
type stubEnv struct {
	id     environment.EnvironmentID
	domain string
}

func (e *stubEnv) ID() environment.EnvironmentID { return e.id }
func (e *stubEnv) Domain() string                { return e.domain }
func (e *stubEnv) Observe(_ context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{}, nil
}
func (e *stubEnv) Act(_ context.Context, _ environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{}, nil
}
func (e *stubEnv) Subscribe(_ context.Context, _ environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return nil, nil
}
func (e *stubEnv) Snapshot(_ context.Context) (environment.EnvironmentSnapshot, error) {
	return environment.EnvironmentSnapshot{}, nil
}

func newCatalogContext(envs []environment.Environment, caps []capability.Capability, domains []string) *stepContext {
	reg := capability.NewRegistry()
	for _, c := range caps {
		_ = reg.Register(c)
	}
	svc := &serviceImpl{hub: event.NewHub(), capReg: reg}
	obj := objective.Objective{Title: "test"}
	if len(domains) > 0 {
		obj.Domain = domains[0]
		if len(domains) > 1 {
			obj.AdditionalDomains = domains[1:]
		}
	}
	return &stepContext{
		envs:     envs,
		obj:      obj,
		svc:      svc,
		agentDef: coreagent.Definition{Domain: obj.Domain},
	}
}

func TestBuildReasonCatalog_ListsRegisteredEnvs(t *testing.T) {
	sc := newCatalogContext(
		[]environment.Environment{
			&stubEnv{id: "software.env.codebase", domain: "software"},
			&stubEnv{id: "software.env.git", domain: "software"},
		},
		nil,
		[]string{"software"},
	)
	got := buildReasonCatalog(sc)
	if !strings.Contains(got, "software.env.codebase") {
		t.Errorf("expected env_id software.env.codebase in catalog, got:\n%s", got)
	}
	if !strings.Contains(got, "software.env.git") {
		t.Errorf("expected env_id software.env.git in catalog, got:\n%s", got)
	}
	if !strings.Contains(got, "use env_id values from this list") {
		t.Errorf("expected explicit instruction to use only listed env_ids, got:\n%s", got)
	}
}

func TestBuildReasonCatalog_ListsCapabilitiesForDomain(t *testing.T) {
	sc := newCatalogContext(
		nil,
		[]capability.Capability{
			{ID: "repo.read", Domain: "software", Description: "Read a file or directory"},
			{ID: "repo.write", Domain: "software", Description: "Write or scaffold a file"},
			{ID: "irrigation.plan", Domain: "agriculture"}, // out of domain — must be excluded
		},
		[]string{"software"},
	)
	got := buildReasonCatalog(sc)
	if !strings.Contains(got, "repo.read") || !strings.Contains(got, "Read a file") {
		t.Errorf("expected repo.read with description, got:\n%s", got)
	}
	if !strings.Contains(got, "repo.write") {
		t.Errorf("expected repo.write, got:\n%s", got)
	}
	if strings.Contains(got, "irrigation.plan") {
		t.Errorf("agriculture capability should not appear for software objective, got:\n%s", got)
	}
}

func TestBuildReasonCatalog_CrossDomainUnion(t *testing.T) {
	sc := newCatalogContext(
		nil,
		[]capability.Capability{
			{ID: "repo.read", Domain: "software"},
			{ID: "patient.lookup", Domain: "healthcare"},
		},
		[]string{"software", "healthcare"},
	)
	got := buildReasonCatalog(sc)
	if !strings.Contains(got, "repo.read") {
		t.Errorf("expected software capability in cross-domain catalog, got:\n%s", got)
	}
	if !strings.Contains(got, "patient.lookup") {
		t.Errorf("expected healthcare capability in cross-domain catalog, got:\n%s", got)
	}
}

func TestBuildReasonCatalog_StableSortAcrossRuns(t *testing.T) {
	// Prompt caching cares about stability. Two contexts with the
	// same capability set in different registration order must
	// produce the same catalog string.
	caps := []capability.Capability{
		{ID: "z.last"},
		{ID: "a.first"},
		{ID: "m.middle"},
	}
	for i := range caps {
		caps[i].Domain = "software"
	}
	first := buildReasonCatalog(newCatalogContext(nil, caps, []string{"software"}))

	// Reverse the registration order
	reversed := []capability.Capability{caps[2], caps[0], caps[1]}
	second := buildReasonCatalog(newCatalogContext(nil, reversed, []string{"software"}))

	if first != second {
		t.Errorf("catalog must be order-stable across registration orderings\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestBuildReasonCatalog_EmptyWhenNothingRegistered(t *testing.T) {
	sc := newCatalogContext(nil, nil, nil)
	got := buildReasonCatalog(sc)
	if got != "" {
		t.Errorf("expected empty catalog when no envs and no capabilities, got:\n%s", got)
	}
}

func TestBuildReasonCatalog_NilSafe(t *testing.T) {
	if got := buildReasonCatalog(nil); got != "" {
		t.Errorf("expected empty catalog for nil stepContext, got %q", got)
	}
	svc := &serviceImpl{}
	sc := &stepContext{svc: svc} // capReg nil, envs nil
	if got := buildReasonCatalog(sc); got != "" {
		t.Errorf("expected empty catalog when capReg is nil and envs are nil, got %q", got)
	}
}

func TestBuildReasonCatalog_WarnsAgainstInvention(t *testing.T) {
	sc := newCatalogContext(
		[]environment.Environment{&stubEnv{id: "x", domain: "software"}},
		nil,
		[]string{"software"},
	)
	got := buildReasonCatalog(sc)
	if !strings.Contains(got, "invented") || !strings.Contains(got, "unrouted") {
		t.Errorf("expected catalog to warn against inventing env_ids/capabilities, got:\n%s", got)
	}
}
