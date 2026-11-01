package app

import (
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/config"
	domainsw "github.com/bsenel/karakuri/domains/software"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/feature/delegation"
	featureobjective "github.com/bsenel/karakuri/internal/feature/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/mcp"
)

// Phase 41 step 9 slice 9c: no capability the loop can execute writes a
// criterion's target, baseline, reducer, samples or sustain (docs/roadmap.md,
// Phase 41 step 9, "No loop capability can write a target, and a test
// enumerates the capabilities", and decision 6 'Targets belong to their
// author').
//
// The only writers are the person-facing methods of the objective service,
// featureobjective.Service.SetTarget and SetBaseline, reached through
// PUT /objectives/{id}/criteria/{criterion_id}/target and .../baseline behind
// objective:update, and the store's SaveObjective under them. Reducer, samples
// and sustain have no writer after create at all. A capability can reach none
// of them because:
//
//  1. every capability the loop can execute is executed by an environment's
//     Act, and no environment built for the software pack or the reserved MCP
//     namespace holds, anywhere in its reachable values, something with a
//     SetTarget, SetBaseline or SaveObjective method;
//  2. the BuildContext an environment is built from carries no such handle
//     either, so a future environment cannot be given one through it;
//  3. the one way back into Karakuri a capability has, the cli agent's
//     delegated credential, is bound to the viewer role, which grants no
//     objective:update.
//
// Closures are opaque to reflection: a Factory.Build that captured the
// objective store would escape check 1. The capability list below is what
// catches that: a new capability fails the test until somebody has read what
// it executes and added it.

// targetWriterMethods are the method names that write a criterion after
// create. TestTheTargetWritersAreThePersonFacingServiceMethods keeps them the
// real ones.
var targetWriterMethods = []string{"SetTarget", "SetBaseline", "SaveObjective"}

// reviewedLoopCapabilities is every capability the loop can execute in a
// deployment with the software pack and its default tools, each read and found
// to write no criterion. Adding one here is a statement that its execution
// reaches none of targetWriterMethods.
var reviewedLoopCapabilities = []string{
	"observe.fetch_signal", "reason.evaluate", "reason.plan", "reason.synthesize",
	"software.act.create_pr", "software.act.create_ticket", "software.act.delegate_to_cli",
	"software.act.draft_adr", "software.act.propose_roadmap_phase", "software.act.run_remediation",
	"software.act.send_message", "software.act.shell_exec", "software.act.write_code",
	"software.act.write_design_doc", "software.act.write_test", "software.decide.prioritize_tasks",
	"software.learn.extract_patterns", "software.learn.update_tech_radar",
	"software.observe.fetch_commits", "software.observe.fetch_logs", "software.observe.fetch_metrics",
	"software.observe.fetch_prs", "software.observe.read_codebase",
	"software.reason.analyse_repo", "software.reason.analyse_usage", "software.reason.architecture_review",
	"software.reason.research",
	"software.verify.alerts_resolved", "software.verify.ci_green", "software.verify.lint",
	"software.verify.paths_declared", "software.verify.review", "software.verify.run_tests",
	"software.verify.stream_pr", "software.verify.tech_lead_review",
}

// loopCapabilitySet is what bootstrap registers for the software domain: the
// universal capabilities and the software pack's, over the tool registry built
// from config/default.yaml, with the delegation issuer attached as bootstrap
// attaches it.
func loopCapabilitySet(t *testing.T) ([]capability.Capability, []environment.Factory) {
	t.Helper()
	cfg, err := config.Load("../../config/default.yaml")
	if err != nil {
		t.Fatalf("load config/default.yaml: %v", err)
	}
	toolReg := tools.NewRegistryFromConfig(cfg.Tools)
	pack := domainsw.NewWithTools(toolReg)
	pack.AttachDelegationIssuer(delegationIssuer{delegation.NewIssuer(nil, nil, auth.TokenConfig{}, nil)})

	caps := slices.Clone(capability.Universal)
	caps = append(caps, pack.Capabilities()...)
	caps = append(caps, toolReg.MCPCapabilities()...)
	factories := append(pack.EnvironmentFactories(), toolReg.MCPEnvironmentFactories()...)
	return caps, factories
}

// TestTheTargetWritersAreThePersonFacingServiceMethods pins the writer names
// to the real methods, so renaming one cannot quietly make the scan below look
// for a method nothing has.
//
// docs/roadmap.md, Phase 41 step 9.
func TestTheTargetWritersAreThePersonFacingServiceMethods(t *testing.T) {
	svc := reflect.TypeOf((*featureobjective.Service)(nil))
	for _, m := range []string{"SetTarget", "SetBaseline"} {
		if _, ok := svc.MethodByName(m); !ok {
			t.Errorf("featureobjective.Service has no %s: the person-facing writers moved, and targetWriterMethods must follow", m)
		}
	}
	if _, ok := reflect.TypeOf((*storage.StorageAdapter)(nil)).Elem().MethodByName("SaveObjective"); !ok {
		t.Errorf("storage.StorageAdapter has no SaveObjective: the objective store's write moved, and targetWriterMethods must follow")
	}

	// The scan finds a planted writer, so a pass below means something.
	planted := struct {
		inner struct{ objectives *featureobjective.Service }
		store any
	}{}
	planted.inner.objectives = featureobjective.NewService(nil)
	if path := findTargetWriter(reflect.ValueOf(planted)); path == "" {
		t.Errorf("findTargetWriter missed a *featureobjective.Service held two fields deep")
	}
	var noStore storage.StorageAdapter
	if path := findTargetWriter(reflect.ValueOf(struct{ s storage.StorageAdapter }{noStore})); path == "" {
		t.Errorf("findTargetWriter missed a nil storage.StorageAdapter field: its static type can write")
	}
}

// TestEveryLoopCapabilityIsReviewed enumerates every capability the loop can
// execute and compares it with the reviewed list. A capability that is not on
// it fails here until somebody has read what it executes.
//
// docs/roadmap.md, Phase 41 step 9.
func TestEveryLoopCapabilityIsReviewed(t *testing.T) {
	caps, _ := loopCapabilitySet(t)
	var ids []string
	for _, c := range caps {
		ids = append(ids, string(c.ID))
	}
	// One discovered tool stands for the reserved namespace: whatever an MCP
	// server calls its tool, it runs on that server, through the MCP
	// environment scanned below, never in this process.
	ids = append(ids, string(capability.MCPCapabilityID("any", "set_target")))
	sort.Strings(ids)

	reviewed := make(map[string]bool, len(reviewedLoopCapabilities))
	for _, id := range reviewedLoopCapabilities {
		reviewed[id] = true
	}
	for _, id := range ids {
		if capability.IsMCPCapability(capability.CapabilityID(id)) {
			continue
		}
		if !reviewed[id] {
			t.Errorf("capability %s is not in reviewedLoopCapabilities: read what it executes, and add it only if "+
				"it reaches none of %v; the only writers of a criterion's target, baseline, reducer, samples or "+
				"sustain are featureobjective.Service.SetTarget and SetBaseline, which a person calls through the API", id, targetWriterMethods)
		}
	}
	for id := range reviewed {
		if !slices.Contains(ids, id) {
			t.Errorf("reviewedLoopCapabilities lists %s, which the loop can no longer execute; remove it", id)
		}
	}
}

// TestNoEnvironmentTheLoopExecutesThroughCanWriteATarget builds every
// environment that serves a loop capability and scans everything it holds.
//
// docs/roadmap.md, Phase 41 step 9.
func TestNoEnvironmentTheLoopExecutesThroughCanWriteATarget(t *testing.T) {
	if path := findTargetWriterInType(reflect.TypeOf(environment.BuildContext{}), "BuildContext", map[reflect.Type]bool{}); path != "" {
		t.Errorf("environment.BuildContext reaches a criterion writer at %s: every environment could be handed it", path)
	}

	caps, factories := loopCapabilitySet(t)
	served := map[capability.CapabilityID]bool{}
	for _, f := range factories {
		for _, id := range f.Serves {
			served[id] = true
		}
		env, err := f.Build(environment.BuildContext{})
		if err != nil {
			// An environment that needs a binding is still scanned by its type.
			t.Logf("environment %s did not build without bindings (%v); scanning nothing it would hold", f.EnvID, err)
			continue
		}
		if path := findTargetWriter(reflect.ValueOf(env)); path != "" {
			t.Errorf("environment %s, which the loop executes %v through, reaches a criterion writer at %s; "+
				"only a person writes a target or baseline, through featureobjective.Service.SetTarget and SetBaseline",
				f.EnvID, f.Serves, path)
		}
	}
	if len(served) == 0 && len(caps) > 0 {
		t.Fatalf("no environment serves any of the %d capabilities; the scan saw nothing", len(caps))
	}

	// The reserved namespace: no MCP server is configured by default, so the
	// MCP environment is scanned by its type.
	if path := findTargetWriterInType(reflect.TypeOf(mcp.Environment{}), "mcp.Environment", map[reflect.Type]bool{}); path != "" {
		t.Errorf("the MCP environment, which executes every mcp.* capability, reaches a criterion writer at %s", path)
	}
}

// TestTheDelegatedCredentialCannotUpdateAnObjective: the cli agent's way back
// into Karakuri is a credential bound to the viewer role, and the viewer role
// grants no objective:update, which gates the target and baseline PUTs.
//
// docs/roadmap.md, Phase 41 step 9.
func TestTheDelegatedCredentialCannotUpdateAnObjective(t *testing.T) {
	for _, r := range karakuriauth.BuiltinRoles() {
		if r.Name != karakuriauth.RoleViewer {
			continue
		}
		if len(r.Inherits) != 0 {
			t.Errorf("viewer inherits %v; re-check that none of them grants objective:update", r.Inherits)
		}
		for _, p := range r.Policies {
			a := string(p.Action)
			wildcard := strings.HasSuffix(a, "*") && strings.HasPrefix(string(karakuriauth.ActionObjectiveUpdate), strings.TrimSuffix(a, "*"))
			if p.Action == karakuriauth.ActionObjectiveUpdate || wildcard {
				t.Errorf("viewer policy %s grants %s: a delegated cli agent could write a target through the API", p.ID, a)
			}
		}
		return
	}
	t.Fatalf("no built-in viewer role")
}

// findTargetWriter walks every value reachable from v and returns the path to
// the first one whose type, or the static interface type holding it, has a
// method in targetWriterMethods. Empty means none.
func findTargetWriter(v reflect.Value) string {
	return walkValue(v, "env", map[uintptr]bool{}, 0)
}

func writes(t reflect.Type) bool {
	for _, m := range targetWriterMethods {
		if _, ok := t.MethodByName(m); ok {
			return true
		}
		if t.Kind() != reflect.Interface && t.Kind() != reflect.Pointer {
			if _, ok := reflect.PointerTo(t).MethodByName(m); ok {
				return true
			}
		}
	}
	return false
}

func walkValue(v reflect.Value, path string, seen map[uintptr]bool, depth int) string {
	if !v.IsValid() || depth > 40 {
		return ""
	}
	if writes(v.Type()) {
		return path + " (" + v.Type().String() + ")"
	}
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return ""
		}
		return walkValue(v.Elem(), path, seen, depth+1)
	case reflect.Pointer:
		if v.IsNil() || seen[v.Pointer()] {
			return ""
		}
		seen[v.Pointer()] = true
		return walkValue(v.Elem(), path, seen, depth+1)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if p := walkValue(v.Field(i), path+"."+v.Type().Field(i).Name, seen, depth+1); p != "" {
				return p
			}
		}
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && (v.IsNil() || seen[v.Pointer()]) {
			return ""
		}
		if v.Kind() == reflect.Slice {
			seen[v.Pointer()] = true
		}
		for i := 0; i < v.Len(); i++ {
			if p := walkValue(v.Index(i), path+"[]", seen, depth+1); p != "" {
				return p
			}
		}
	case reflect.Map:
		if v.IsNil() || seen[v.Pointer()] {
			return ""
		}
		seen[v.Pointer()] = true
		it := v.MapRange()
		for it.Next() {
			if p := walkValue(it.Value(), path+"[k]", seen, depth+1); p != "" {
				return p
			}
		}
	}
	return ""
}

// findTargetWriterInType is the walk over types alone, for what cannot be
// built here: every field's static type, and an interface field's own method
// set.
func findTargetWriterInType(t reflect.Type, path string, seen map[reflect.Type]bool) string {
	if seen[t] {
		return ""
	}
	seen[t] = true
	if writes(t) {
		return path + " (" + t.String() + ")"
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return findTargetWriterInType(t.Elem(), path, seen)
	case reflect.Map:
		return findTargetWriterInType(t.Elem(), path+"[k]", seen)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if p := findTargetWriterInType(t.Field(i).Type, path+"."+t.Field(i).Name, seen); p != "" {
				return p
			}
		}
	}
	return ""
}
