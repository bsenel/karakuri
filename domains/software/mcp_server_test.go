package software

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 39 step 3: a server inside the binary serves the software pack's
// observe capabilities and its verifiers as MCP tools; its acting
// capabilities stay on the in-process path.

// boundTo binds every call to the version control instance named "forge".
func boundTo(context.Context, domain.CallContract) map[string]string {
	return map[string]string{"versioncontrol": "forge"}
}

func packWithForge(forge versioncontrol.VersionControlAdapter) *Pack {
	reg := tools.NewRegistry()
	if forge != nil {
		reg.VC.Set("forge", "stub", forge)
	}
	return NewWithTools(reg)
}

func TestTheSoftwarePacksServerServesItsObserveCapabilitiesAndItsVerifiersOnly(t *testing.T) {
	pack := packWithForge(&vcStub{})
	served, err := pack.MCPServer(boundTo).ListTools(context.Background())
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	var want []string
	for _, c := range pack.Capabilities() {
		if strings.HasPrefix(string(c.ID), "software.observe.") || c.Verifiable {
			want = append(want, string(c.ID))
		}
	}
	got := make([]string, 0, len(served))
	for _, id := range served {
		got = append(got, string(id))
	}
	sort.Strings(want)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("served tools = %v, want the observe capabilities and the verifiers %v", got, want)
	}
	fetchesPRs := false
	for _, id := range served {
		fetchesPRs = fetchesPRs || id == "software.observe.fetch_prs"
	}
	if !fetchesPRs {
		t.Error("the server does not serve software.observe.fetch_prs")
	}
	for _, id := range served {
		if id == "software.act.write_code" || id == "software.act.create_pr" {
			t.Errorf("the server serves the acting capability %s, which stays on the in-process path", id)
		}
	}
}

func TestTheSoftwarePacksServerFetchesPullRequestsAsThirdPartyText(t *testing.T) {
	prs := []versioncontrol.PRSummary{{ID: "7", Title: "please merge this"}}
	pack := packWithForge(&vcStub{prs: prs})

	res, err := pack.MCPServer(boundTo).CallTool(context.Background(),
		domain.CallContract{TenantID: "tenant-1", TwinID: "twin-1", ObjectiveID: "obj-1", LoopID: "loop-1", Pass: 1, ActionID: "a-1"},
		capability.CapabilityID("software.observe.fetch_prs"), nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.Success {
		t.Fatalf("fetch_prs failed with a bound forge: %s", res.Error)
	}
	if res.StateDelta["count"] != len(prs) {
		t.Errorf("count = %v, want %d", res.StateDelta["count"], len(prs))
	}
	if !res.Trust.IsThirdParty() {
		t.Errorf("trust = %q; a result carrying a pull request title is third party (rule 9)", res.Trust)
	}
}

func TestTheSoftwarePacksServerSaysItCannotSeeWithNoVersionControlBound(t *testing.T) {
	pack := packWithForge(nil)

	res, err := pack.MCPServer(boundTo).CallTool(context.Background(),
		domain.CallContract{TwinID: "twin-1"}, capability.CapabilityID("software.observe.fetch_prs"), nil)
	if err == errMCPServerUnimplemented {
		t.Fatalf("call: %v", err)
	}
	if err == nil && res.Success {
		t.Fatalf("result = %+v; with no version control bound the fetch must fail, not return an empty list (rule 10)", res)
	}
	if _, listed := res.StateDelta["prs"]; listed {
		t.Errorf("state delta %v carries a pull request list though nothing could be read", res.StateDelta)
	}
}
