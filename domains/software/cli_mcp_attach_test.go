package software

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/cliagent"
)

const (
	testMCPURL   = "https://karakuri.example/api/v1/mcp"
	testMCPToken = "tok-delegation-secret-1"
)

// recordingCLI is a wired coding-agent adapter that keeps what it was handed.
type recordingCLI struct {
	calls []cliagent.DelegateInput
	err   error
}

func (*recordingCLI) Name() string { return "recording" }
func (*recordingCLI) Active() bool { return true }
func (c *recordingCLI) Delegate(_ context.Context, in cliagent.DelegateInput) (cliagent.DelegateOutput, error) {
	c.calls = append(c.calls, in)
	if c.err != nil {
		return cliagent.DelegateOutput{}, c.err
	}
	return cliagent.DelegateOutput{Summary: "did " + in.Prompt}, nil
}
func (*recordingCLI) Stream(context.Context, cliagent.DelegateInput) (<-chan cliagent.DelegateChunk, error) {
	ch := make(chan cliagent.DelegateChunk)
	close(ch)
	return ch, nil
}

// recordingIssuer hands out one fixed credential and keeps every call.
type recordingIssuer struct {
	twins    []string
	timeouts []time.Duration
	revoked  []DelegationCredential
	expires  time.Time
}

func (i *recordingIssuer) Issue(_ context.Context, twinID string, timeout time.Duration) (DelegationCredential, error) {
	i.twins = append(i.twins, twinID)
	i.timeouts = append(i.timeouts, timeout)
	return DelegationCredential{
		Token: testMCPToken, PrincipalID: "principal-1", TwinID: twinID, ExpiresAt: i.expires,
	}, nil
}

func (i *recordingIssuer) Revoke(_ context.Context, c DelegationCredential) error {
	i.revoked = append(i.revoked, c)
	return nil
}

func newRecordingIssuer() *recordingIssuer {
	return &recordingIssuer{expires: time.Date(2026, 10, 10, 12, 10, 0, 0, time.UTC)}
}

// delegateAction is a delegation that asks for the given Karakuri tools; with
// none, params carries no karakuri_tools at all.
func delegateAction(karakuriTools ...string) environment.Action {
	params := map[string]any{"prompt": "read the audit log", "worktree_path": "/tmp/wt"}
	if len(karakuriTools) > 0 {
		list := make([]any, 0, len(karakuriTools))
		for _, n := range karakuriTools {
			list = append(list, n)
		}
		params["karakuri_tools"] = list
	}
	return environment.Action{CapabilityID: "software.act.delegate_to_cli", Params: params}
}

func attachingEnv(cli cliagent.CLIAgentAdapter, iss delegationIssuer) *cliEnv {
	return &cliEnv{id: "software.env.cli_agent", cli: cli, twinID: "twin-1",
		attachMCP: true, mcpURL: testMCPURL, issuer: iss}
}

// Asking for Karakuri tools on an instance that does not attach them is a
// failure, not a delegation without them: the run would go ahead, bill, and
// report on a question it could not look up.
func TestKarakuriToolsRefusedWhenAttachmentIsOff(t *testing.T) {
	cli, iss := &recordingCLI{}, newRecordingIssuer()
	env := &cliEnv{id: "software.env.cli_agent", cli: cli, twinID: "twin-1", issuer: iss}

	res, err := env.Act(context.Background(), delegateAction("audit_list"))
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if res.Success {
		t.Error("delegated without the Karakuri tools the action asked for")
	}
	if !strings.Contains(res.Error, "karakuri_tools") || !strings.Contains(res.Error, "attach_karakuri_mcp") {
		t.Errorf("refusal does not name what was asked for and the option that is off: %q", res.Error)
	}
	if len(cli.calls) != 0 {
		t.Errorf("adapter was called %d times", len(cli.calls))
	}
	if len(iss.twins) != 0 {
		t.Error("a credential was issued for a delegation that did not run")
	}
}

// An action that asks for no Karakuri tools is delegated exactly as it is
// today, attachment enabled or not, and nothing is issued for it.
func TestNoKarakuriToolsMeansNoAttachment(t *testing.T) {
	base := &recordingCLI{}
	if _, err := (&cliEnv{id: "software.env.cli_agent", cli: base}).Act(context.Background(), delegateAction()); err != nil {
		t.Fatalf("act: %v", err)
	}
	if len(base.calls) != 1 {
		t.Fatalf("baseline adapter calls = %d, want 1", len(base.calls))
	}

	empty := delegateAction()
	empty.Params["karakuri_tools"] = []any{}

	for name, a := range map[string]environment.Action{
		"absent":     delegateAction(),
		"empty list": empty,
	} {
		t.Run(name, func(t *testing.T) {
			cli, iss := &recordingCLI{}, newRecordingIssuer()
			res, err := attachingEnv(cli, iss).Act(context.Background(), a)
			if err != nil {
				t.Fatalf("act: %v", err)
			}
			if !res.Success {
				t.Fatalf("delegation failed: %s", res.Error)
			}
			if len(cli.calls) != 1 {
				t.Fatalf("adapter calls = %d, want 1", len(cli.calls))
			}
			if cli.calls[0].MCP != nil {
				t.Errorf("an MCP server was attached to an action that asked for none: %+v", cli.calls[0].MCP)
			}
			if !reflect.DeepEqual(cli.calls[0], base.calls[0]) {
				t.Errorf("input changed:\n got %+v\nwant %+v", cli.calls[0], base.calls[0])
			}
			if len(iss.twins) != 0 {
				t.Error("a credential was issued with nothing to attach")
			}
			if _, ok := res.StateDelta["delegation_credential"]; ok {
				t.Error("StateDelta reports a delegation credential that was never issued")
			}
		})
	}
}

// The credential is for this twin, lives no longer than the run, reaches the
// adapter with the configured endpoint and only the tools asked for, and is
// gone when Act returns.
func TestKarakuriToolsAttachedWithAScopedCredential(t *testing.T) {
	cli, iss := &recordingCLI{}, newRecordingIssuer()

	res, err := attachingEnv(cli, iss).Act(context.Background(), delegateAction("audit_list"))
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if !res.Success {
		t.Fatalf("delegation failed: %s", res.Error)
	}

	if len(iss.twins) != 1 || iss.twins[0] != "twin-1" {
		t.Fatalf("issued for %v, want exactly once for twin-1", iss.twins)
	}
	if got := iss.timeouts[0]; got <= 0 || got > 600*time.Second {
		t.Errorf("credential timeout = %s, want within the action's 600s", got)
	}

	if len(cli.calls) != 1 {
		t.Fatalf("adapter calls = %d, want 1", len(cli.calls))
	}
	in := cli.calls[0]
	if in.TimeoutSeconds != 600 {
		t.Errorf("TimeoutSeconds = %d, want 600", in.TimeoutSeconds)
	}
	if in.MCP == nil {
		t.Fatal("no MCP server attached")
	}
	if in.MCP.URL != testMCPURL {
		t.Errorf("MCP.URL = %q, want %q", in.MCP.URL, testMCPURL)
	}
	if in.MCP.Token != testMCPToken {
		t.Error("MCP.Token is not the issued token")
	}
	if in.MCP.ServerName == "" {
		t.Error("MCP.ServerName is empty")
	}
	if !reflect.DeepEqual(in.MCP.Tools, []string{"audit_list"}) {
		t.Errorf("MCP.Tools = %v, want exactly [audit_list]", in.MCP.Tools)
	}

	if len(iss.revoked) != 1 || iss.revoked[0].PrincipalID != "principal-1" {
		t.Errorf("revoked = %+v, want the issued credential once", iss.revoked)
	}
}

// A run that fails still ends: the credential does not outlive it.
func TestCredentialRevokedWhenTheAdapterFails(t *testing.T) {
	cli, iss := &recordingCLI{err: errors.New("exit status 1")}, newRecordingIssuer()

	res, err := attachingEnv(cli, iss).Act(context.Background(), delegateAction("audit_list"))
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if res.Success {
		t.Error("a failed delegation reported success")
	}
	if len(cli.calls) != 1 || cli.calls[0].MCP == nil {
		t.Fatalf("adapter was not handed the attachment: %+v", cli.calls)
	}
	if len(iss.revoked) != 1 || iss.revoked[0].PrincipalID != "principal-1" {
		t.Errorf("revoked = %+v, want the issued credential once", iss.revoked)
	}
}

// Without a twin there is nothing to scope the credential to, and without an
// issuer or an endpoint there is nothing to attach. Each is said, and nothing
// runs.
func TestKarakuriToolsRefusedWithoutTwinOrIssuer(t *testing.T) {
	iss := newRecordingIssuer()
	for name, tc := range map[string]struct {
		env  *cliEnv
		want string
	}{
		"no twin":   {&cliEnv{attachMCP: true, mcpURL: testMCPURL, issuer: iss}, "twin"},
		"no issuer": {&cliEnv{twinID: "twin-1", attachMCP: true, mcpURL: testMCPURL}, "issuer"},
		"no url":    {&cliEnv{twinID: "twin-1", attachMCP: true, issuer: iss}, "karakuri_mcp_url"},
	} {
		t.Run(name, func(t *testing.T) {
			cli := &recordingCLI{}
			tc.env.id, tc.env.cli = "software.env.cli_agent", cli
			res, err := tc.env.Act(context.Background(), delegateAction("audit_list"))
			if err != nil {
				t.Fatalf("act: %v", err)
			}
			if res.Success {
				t.Error("delegated without a credential it could scope")
			}
			if !strings.Contains(res.Error, tc.want) {
				t.Errorf("refusal does not mention %q: %q", tc.want, res.Error)
			}
			if len(cli.calls) != 0 {
				t.Errorf("adapter was called %d times", len(cli.calls))
			}
		})
	}
	if len(iss.twins) != 0 {
		t.Errorf("credentials issued for refused actions: %v", iss.twins)
	}
}

// The result is persisted and shown; the token is neither. What is recorded
// is that a credential existed and how far it reached.
func TestActionResultRecordsTheScopeAndNeverTheToken(t *testing.T) {
	for name, adapterErr := range map[string]error{
		"success": nil,
		"failure": fmt.Errorf("mcp config rejected for bearer %s", testMCPToken),
	} {
		t.Run(name, func(t *testing.T) {
			cli, iss := &recordingCLI{err: adapterErr}, newRecordingIssuer()

			res, err := attachingEnv(cli, iss).Act(context.Background(), delegateAction("audit_list"))
			if err != nil {
				t.Fatalf("act: %v", err)
			}
			if strings.Contains(fmt.Sprintf("%v", res.StateDelta), testMCPToken) {
				t.Error("StateDelta contains the token")
			}
			if strings.Contains(res.Error, testMCPToken) {
				t.Error("Error contains the token")
			}

			cred, ok := res.StateDelta["delegation_credential"].(map[string]any)
			if !ok {
				t.Fatalf("StateDelta[delegation_credential] = %#v, want a map", res.StateDelta["delegation_credential"])
			}
			if cred["twin_id"] != "twin-1" {
				t.Errorf("twin_id = %v, want twin-1", cred["twin_id"])
			}
			if cred["read_only"] != true {
				t.Errorf("read_only = %v, want true", cred["read_only"])
			}
			if cred["expires_at"] != iss.expires.Format(time.RFC3339) {
				t.Errorf("expires_at = %v, want %s", cred["expires_at"], iss.expires.Format(time.RFC3339))
			}
		})
	}
}

// attach_karakuri_mcp is off unless it is set, and the URL is read with it.
func TestCLIMCPOptionsDefaultOff(t *testing.T) {
	for name, tc := range map[string]struct {
		opts   map[string]any
		attach bool
		url    string
	}{
		"nil":    {nil, false, ""},
		"absent": {map[string]any{"binary": "claude"}, false, ""},
		"false":  {map[string]any{"attach_karakuri_mcp": false, "karakuri_mcp_url": testMCPURL}, false, testMCPURL},
		"true":   {map[string]any{"attach_karakuri_mcp": true, "karakuri_mcp_url": testMCPURL}, true, testMCPURL},
	} {
		t.Run(name, func(t *testing.T) {
			attach, url := cliMCPOptions(tc.opts)
			if attach != tc.attach || url != tc.url {
				t.Errorf("cliMCPOptions = (%v, %q), want (%v, %q)", attach, url, tc.attach, tc.url)
			}
		})
	}
}

// The option decides what the adapter is handed. If it were read and dropped,
// on and off would delegate the same input.
func TestAttachOptionChangesTheDelegation(t *testing.T) {
	run := func(opts map[string]any) (*recordingCLI, environment.ActionResult) {
		t.Helper()
		cli := &recordingCLI{}
		env := &cliEnv{id: "software.env.cli_agent", cli: cli, twinID: "twin-1", issuer: newRecordingIssuer()}
		env.attachMCP, env.mcpURL = cliMCPOptions(opts)
		res, err := env.Act(context.Background(), delegateAction("audit_list"))
		if err != nil {
			t.Fatalf("act: %v", err)
		}
		return cli, res
	}

	off, offRes := run(map[string]any{"karakuri_mcp_url": testMCPURL})
	if len(off.calls) != 0 || offRes.Success {
		t.Errorf("attachment off: adapter calls = %d, success = %v; want a refusal", len(off.calls), offRes.Success)
	}

	on, onRes := run(map[string]any{"attach_karakuri_mcp": true, "karakuri_mcp_url": testMCPURL})
	if !onRes.Success {
		t.Fatalf("attachment on: %s", onRes.Error)
	}
	if len(on.calls) != 1 || on.calls[0].MCP == nil || on.calls[0].MCP.URL != testMCPURL {
		t.Errorf("attachment on: adapter was handed %+v, want the configured MCP endpoint", on.calls)
	}
}

// The twin the credential is scoped to is the one the environment was built
// for: an Action carries no twin, BuildContext does.
func TestCLIEnvIsBuiltForItsTwin(t *testing.T) {
	reg := tools.NewRegistry()
	reg.CLIAgents.Set("recording", "stub", &recordingCLI{})

	for _, f := range softwareEnvironmentFactories(reg) {
		if f.EnvID != "software.env.cli_agent" {
			continue
		}
		built, err := f.Build(environment.BuildContext{TwinID: "twin-7"})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		env, ok := built.(*cliEnv)
		if !ok {
			t.Fatalf("built %T, want *cliEnv", built)
		}
		if env.twinID != "twin-7" {
			t.Errorf("twinID = %q, want twin-7", env.twinID)
		}
		return
	}
	t.Fatal("no software.env.cli_agent factory")
}
