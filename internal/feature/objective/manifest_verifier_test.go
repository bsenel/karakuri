package objective

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 39 step 2: the manifest nominates verifiers and grants nothing (ADR
// 028 section a and Decision 4). The orchestrator's rule today is
// lookupVerifier: a verifier is trusted only when it is a registered capability
// outside the `mcp.` namespace. A tool the manifest names as a verifier but the
// pack does not register as a tool is still not trusted.

// registeredTools is the verifier lookup the orchestrator would build from the
// tools packs registered.
type registeredTools []capability.Capability

func (r registeredTools) Get(id capability.CapabilityID) (capability.Capability, bool) {
	for _, c := range r {
		if c.ID == id {
			return c, true
		}
	}
	return capability.Capability{}, false
}

func TestManifestNominationDoesNotMakeAVerifierTrusted(t *testing.T) {
	const (
		registered capability.CapabilityID = "trivial.verify"
		nominated  capability.CapabilityID = "trivial.only_in_the_manifest"
	)
	tools := []capability.Capability{{
		ID: registered, Domain: "trivial", Verifiable: true,
		InputSchema: capability.Schema{Type: "object"}, OutputSchema: capability.Schema{Type: "object"},
	}}
	m := domain.Manifest{Verifiers: []capability.CapabilityID{registered, nominated}}

	reg := domain.NewRegistry()
	// Whether registration refuses a nomination with no tool behind it is the
	// implementation's choice; either way the nomination must not be trusted.
	regErr := reg.RegisterManifest("trivial", tools, m)

	svc := newTestService(t).WithVerifiers(registeredTools(reg.Capabilities()))
	verified := func(v capability.CapabilityID) CreateRequest {
		return CreateRequest{Title: "check", Domain: "trivial", SuccessCriteria: []coreobjective.Criterion{
			{ID: "done", Description: "the check passes", Kind: coreobjective.KindVerified, Verifier: v},
		}}
	}

	// Control: when registration succeeded, a nominated tool the pack does
	// register is accepted, so the refusal below is about the nomination alone.
	if regErr == nil {
		if _, err := svc.Create(context.Background(), verified(registered)); err != nil {
			t.Fatalf("a registered, nominated verifier was refused: %v", err)
		}
	} else if strings.Contains(regErr.Error(), "not implemented") {
		t.Fatalf("register the manifest: %v", regErr)
	}

	_, err := svc.Create(context.Background(), verified(nominated))
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want a *ValidationError: a verifier only the manifest names is not trusted", err)
	}
	if !strings.HasSuffix(verr.Field, ".verifier") {
		t.Errorf("rejected field = %q, want the criterion's verifier", verr.Field)
	}
}
