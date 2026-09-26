package loop

import (
	"context"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// toolSourceEnv is an environment whose capabilities were discovered: it
// answers ProvidedCapabilities the way an MCP environment does.
type toolSourceEnv struct {
	stubEnv
	provides []capability.CapabilityID
}

func (e *toolSourceEnv) ProvidedCapabilities() []capability.CapabilityID { return e.provides }

// A discovered tool belongs to no domain an objective declares, so the catalog
// reaches it through the environments built for this twin — and only those. A
// tool registered for an instance this twin is not bound to stays out of the
// list the planner is told is exhaustive.
func TestBuildReasonCatalog_ListsToolsFromBuiltToolSources(t *testing.T) {
	mine := capability.NewMCPCapability("acme_files", "read_file", "Reads a file.", capability.Schema{})
	theirs := capability.NewMCPCapability("other_files", "read_file", "Reads their file.", capability.Schema{})

	sc := newCatalogContext(
		[]environment.Environment{
			&stubEnv{id: "software.env.codebase", domain: "software"},
			&toolSourceEnv{
				stubEnv:  stubEnv{id: "mcp.env.acme_files", domain: capability.MCPDomain},
				provides: []capability.CapabilityID{mine.ID},
			},
		},
		[]capability.Capability{
			{ID: "software.act.run_tests", Domain: "software", Description: "Runs the tests."},
			mine, theirs,
		},
		[]string{"software"},
	)
	got := buildReasonCatalog(sc)

	if !strings.Contains(got, "mcp.acme_files.read_file — Reads a file.") {
		t.Errorf("the bound instance's tool is missing from the catalog:\n%s", got)
	}
	if strings.Contains(got, "other_files") {
		t.Errorf("another instance's tool reached this twin's catalog:\n%s", got)
	}
	if !strings.Contains(got, "software.act.run_tests") {
		t.Errorf("the objective's own capabilities are missing:\n%s", got)
	}
}

// The second bound at runtime. Boot only warns about templates, so a criterion
// naming a discovered tool can reach verify — and a successful run of that tool
// must not settle it. The control criterion shows the same outcome shape does
// settle a pack's verifier.
func TestStepVerify_DiscoveredToolNeverSettlesACriterion(t *testing.T) {
	mcpTool := capability.MCPCapabilityID("acme_files", "read_file")
	sc := &stepContext{
		svc: &serviceImpl{hub: event.NewHub()},
		obj: objective.Objective{
			ID: "obj-1",
			SuccessCriteria: []objective.Criterion{
				{ID: "third-party", Verifier: mcpTool},
				{ID: "ours", Verifier: "test.act.run_tests"},
			},
		},
	}
	score, allMet := stepVerify(context.Background(), sc, []actionOutcome{
		{CapabilityID: string(mcpTool), Result: environment.ActionResult{Success: true, Trust: environment.TrustThirdParty}},
		{CapabilityID: "test.act.run_tests", Result: environment.ActionResult{Success: true}},
	})
	if allMet {
		t.Fatal("a successful MCP call settled a criterion")
	}
	if score != 0.5 {
		t.Errorf("score = %v, want 0.5: the pack's verifier met, the discovered one refused", score)
	}
}

// MCP environments are built whatever the objective's domains are; what
// confines them is the factory's binding check, not the objective's subject.
func TestBuildEnvironments_IncludesToolSourcesOutsideTheObjectivesDomains(t *testing.T) {
	reg := environment.NewRegistry()
	for _, f := range []environment.Factory{
		{EnvID: "software.env.codebase", Domain: "software"},
		{EnvID: "mcp.env.acme_files", Domain: capability.MCPDomain,
			Serves: []capability.CapabilityID{capability.MCPCapabilityID("acme_files", "read_file")}},
	} {
		id, domain := f.EnvID, f.Domain
		f.Build = func(environment.BuildContext) (environment.Environment, error) {
			return &stubEnv{id: id, domain: domain}, nil
		}
		if err := reg.Register(f); err != nil {
			t.Fatal(err)
		}
	}

	envs := BuildEnvironments(context.Background(), nil, reg, nil, objective.Objective{Domain: "software"})
	got := map[environment.EnvironmentID]bool{}
	for _, e := range envs {
		got[e.ID()] = true
	}
	if !got["software.env.codebase"] || !got["mcp.env.acme_files"] || len(envs) != 2 {
		t.Errorf("built %v, want the software environment and the MCP one", got)
	}
}

// Routing through the index, not the fallback: an MCP action reaches its
// instance's environment because that environment's Serves names it, even when
// the plan named a different env_id.
func TestResolveEnv_MCPActionRoutesThroughTheIndex(t *testing.T) {
	mcpTool := capability.MCPCapabilityID("acme_files", "read_file")
	reg, envs := routeRegistry(t,
		struct {
			id     environment.EnvironmentID
			serves []capability.CapabilityID
		}{"software.env.codebase", []capability.CapabilityID{"software.act.run_tests"}},
		struct {
			id     environment.EnvironmentID
			serves []capability.CapabilityID
		}{"mcp.env.acme_files", []capability.CapabilityID{mcpTool}},
	)
	s := &serviceImpl{envReg: reg}

	for _, envID := range []string{"", "software.env.codebase"} {
		env, routedBy := s.resolveEnv(envs, plannedAction{CapabilityID: string(mcpTool), EnvID: envID})
		if env == nil || env.ID() != "mcp.env.acme_files" {
			t.Fatalf("env_id %q: routed to %v, want mcp.env.acme_files", envID, env)
		}
		if routedBy != "capability" {
			t.Errorf("env_id %q: routed by %q, want the index (capability) rather than a fallback", envID, routedBy)
		}
	}
}

// A discovered tool never gets a worktree, even if its registry entry says it
// needs one.
func TestNeedsWorkspace_NeverForADiscoveredTool(t *testing.T) {
	reg := capability.NewRegistry()
	forged := capability.NewMCPCapability("acme_files", "write_file", "Writes files.", capability.Schema{})
	forged.NeedsWorkspace = true
	_ = reg.Register(forged)
	_ = reg.Register(capability.Capability{ID: "test.act.edit", Domain: "test", NeedsWorkspace: true})

	s := &serviceImpl{capReg: reg}
	if s.needsWorkspace(string(forged.ID)) {
		t.Error("a discovered tool was given a workspace")
	}
	if !s.needsWorkspace("test.act.edit") {
		t.Error("a pack capability needing a workspace was refused one")
	}
}
