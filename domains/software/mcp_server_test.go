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
// observe capabilities and its verifiers as MCP tools. Step 4 adds the acting
// capabilities that write files (pack_acting_test.go).

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

// Which acting tools it serves besides is pinned by step 4's
// TestTheSoftwarePacksServerListsItsFileWritingActingToolsAndNoOtherActingTool.
func TestTheSoftwarePacksServerServesItsObserveCapabilitiesAndItsVerifiers(t *testing.T) {
	pack := packWithForge(&vcStub{})
	served, err := pack.MCPServer(boundTo).ListTools(context.Background())
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	listed := map[string]bool{}
	for _, id := range served {
		listed[string(id)] = true
	}
	var missing []string
	for _, c := range pack.Capabilities() {
		if (strings.HasPrefix(string(c.ID), "software.observe.") || c.Verifiable) && !listed[string(c.ID)] {
			missing = append(missing, string(c.ID))
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("the server does not serve the observe capabilities and verifiers %v", missing)
	}
	if !listed["software.observe.fetch_prs"] {
		t.Error("the server does not serve software.observe.fetch_prs")
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
