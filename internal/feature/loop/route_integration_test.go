package loop

import (
	"testing"

	"github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// The routing rules above are proved against stubs. This proves the wiring:
// the real software pack, registered the way bootstrap registers it, routes
// the real capability IDs to the environments that implement them.
//
// It is the test that would have failed on the live run that started Phase 26.
// A self_improve plan asked for software.act.write_code with no env_id, got
// noopEnv, and reported "unimplemented" after a git worktree and branch had
// been created for it.
func TestSoftwarePackRoutesItsWriteCapabilities(t *testing.T) {
	pack := software.New()

	reg := environment.NewRegistry()
	var envs []environment.Environment
	for _, f := range pack.EnvironmentFactories() {
		if err := reg.Register(f); err != nil {
			t.Fatalf("register %q: %v", f.EnvID, err)
		}
		env, err := f.Build(environment.BuildContext{})
		if err != nil {
			t.Fatalf("build %q: %v", f.EnvID, err)
		}
		envs = append(envs, env)
	}
	svc := &serviceImpl{envReg: reg}

	for _, tc := range []struct {
		capID   string
		envID   string
		planned string // the env_id a plan might name, right or wrong
	}{
		// The three that write source, with no env_id at all — the shape of
		// the plan that failed live.
		{capID: "software.act.write_code", envID: "software.env.cli_agent"},
		{capID: "software.act.write_test", envID: "software.env.cli_agent"},
		{capID: "software.act.delegate_to_cli", envID: "software.env.cli_agent"},

		// And with an env_id the model got wrong, which the pack overrides.
		{capID: "software.act.write_code", envID: "software.env.cli_agent", planned: "software.env.codebase"},
		{capID: "software.act.create_pr", envID: "software.env.git", planned: "software.env.ci"},

		// The rest of the pack's acting capabilities, so this is a claim about
		// routing rather than about three names.
		{capID: "software.act.create_ticket", envID: "software.env.ticket"},
		{capID: "software.act.send_message", envID: "software.env.communication"},
		{capID: "software.act.shell_exec", envID: "software.env.shell"},
		{capID: "software.act.write_design_doc", envID: "software.env.git"},
		{capID: "software.reason.analyse_usage", envID: "software.env.platform_telemetry"},
	} {
		t.Run(tc.capID, func(t *testing.T) {
			got, routedBy := svc.resolveEnv(envs, plannedAction{
				CapabilityID: tc.capID,
				EnvID:        tc.planned,
			})
			if got == nil {
				t.Fatalf("%s routed nowhere (routed_by=%s)", tc.capID, routedBy)
			}
			if string(got.ID()) != tc.envID {
				t.Errorf("%s routed to %q, want %q", tc.capID, got.ID(), tc.envID)
			}
			if routedBy != "capability" {
				t.Errorf("%s routed by %q, want the pack's declaration", tc.capID, routedBy)
			}
		})
	}
}

// The worktree and the route have to agree. A capability that declares it
// writes files and routes nowhere gets a git branch created for it and then
// fails — which is precisely what write_code did for four phases.
func TestEveryWorkspaceCapabilityInTheSoftwarePackRoutes(t *testing.T) {
	pack := software.New()

	reg := environment.NewRegistry()
	for _, f := range pack.EnvironmentFactories() {
		if err := reg.Register(f); err != nil {
			t.Fatalf("register %q: %v", f.EnvID, err)
		}
	}

	checked := 0
	for _, c := range pack.Capabilities() {
		if !c.NeedsWorkspace {
			continue
		}
		checked++
		if len(reg.ServedBy(c.ID)) == 0 {
			t.Errorf("%q is given a worktree and no environment serves it", c.ID)
		}
	}
	if checked == 0 {
		t.Fatal("no capability declares NeedsWorkspace; this test is checking nothing")
	}
}
