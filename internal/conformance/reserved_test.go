package conformance_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/conformance"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
)

func checkResult(t *testing.T, p domain.Pack, check string) conformance.Result {
	t.Helper()
	for _, r := range conformance.New().Run(context.Background(), p) {
		if r.Check == check {
			return r
		}
	}
	t.Fatalf("the suite no longer runs %s", check)
	return conformance.Result{}
}

// The fourth bound, in the one direction conformance can check: a pack may not
// claim the namespace discovery owns — not as a capability, not as an
// environment's domain, and not as a route. Each way has to fail, or the check
// passes everything.
func TestReservedNamespaceRefusesAPackThatClaimsIt(t *testing.T) {
	mcpID := capability.MCPCapabilityID("files", "write")

	for _, tc := range []struct {
		name     string
		pack     func() *routingPack
		contains string
	}{
		{
			name: "capability",
			pack: func() *routingPack {
				c := routingCap("routing.act.real", false)
				c.ID = mcpID
				return &routingPack{caps: []capability.Capability{c}}
			},
			contains: string(mcpID),
		},
		{
			name: "environment domain",
			pack: func() *routingPack {
				env := routingEnv("routing.env.real")
				env.Domain = capability.MCPDomain
				return &routingPack{
					caps: []capability.Capability{routingCap("routing.act.real", false)},
					envs: []environment.Factory{env},
				}
			},
			contains: "routing.env.real",
		},
		{
			name: "serves",
			pack: func() *routingPack {
				return &routingPack{
					caps: []capability.Capability{routingCap("routing.act.real", false)},
					envs: []environment.Factory{routingEnv("routing.env.real", "routing.act.real", mcpID)},
				}
			},
			contains: string(mcpID),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := checkResult(t, tc.pack(), "reserved_namespace")
			if res.Passed {
				t.Fatalf("a pack claiming the reserved namespace by its %s passed: %s", tc.name, res.Message)
			}
			if !strings.Contains(res.Message, tc.contains) {
				t.Errorf("message %q does not name %q", res.Message, tc.contains)
			}
		})
	}
}

func TestReservedNamespacePassesAPackThatLeavesItAlone(t *testing.T) {
	p := &routingPack{
		caps: []capability.Capability{routingCap("routing.act.real", false)},
		envs: []environment.Factory{routingEnv("routing.env.real", "routing.act.real")},
	}
	if res := checkResult(t, p, "reserved_namespace"); !res.Passed {
		t.Errorf("a pack outside the namespace failed: %s", res.Message)
	}
}

// The second bound at pack time. The reserved verifier is refused before the
// cross-pack escape: a criterion naming Domain "mcp" would otherwise read as a
// deliberate reference to another pack, and there is no such pack.
func TestCriterionVerifiedByADiscoveredToolIsRefused(t *testing.T) {
	for _, critDomain := range []string{"", capability.MCPDomain} {
		p := packWithTemplate(critDomain, "mcp.files.read_file")
		res := checkResult(t, p, "criterion_verifier_refs")
		if res.Passed {
			t.Fatalf("criterion verified by mcp.files.read_file (domain %q) passed: %s", critDomain, res.Message)
		}
		if !strings.Contains(res.Message, "reserved") {
			t.Errorf("message %q does not say the namespace is reserved", res.Message)
		}
	}
}
