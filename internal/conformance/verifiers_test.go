package conformance_test

import (
	"strings"
	"testing"

	"github.com/bsenel/karakuri/domains/agriculture"
	"github.com/bsenel/karakuri/domains/consulting"
	"github.com/bsenel/karakuri/domains/healthcare"
	"github.com/bsenel/karakuri/domains/legal"
	"github.com/bsenel/karakuri/domains/mechanical"
	"github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/conformance"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/objective"
)

func allShippedPacks() []domain.Pack {
	return []domain.Pack{
		software.New(), agriculture.New(), consulting.New(),
		healthcare.New(), legal.New(), mechanical.New(),
	}
}

// With every shipped pack enabled, every criterion verifier resolves.
//
// This is the check the per-pack suite cannot make: it deliberately does not
// resolve a foreign domain, because a pack is valid on its own (ADR 017). So a
// criterion naming another pack's capability was checked by nothing at all —
// it would score zero at verify with no explanation.
func TestShippedPacksHaveNoDanglingVerifiers(t *testing.T) {
	for _, res := range conformance.CheckDanglingVerifiers(allShippedPacks()...) {
		if !res.Passed {
			t.Errorf("%s: %s", res.Check, res.Message)
		}
	}
}

// The software pack is self-contained: enabled on its own, every criterion it
// declares still resolves.
//
// This is the case an operator most often runs — one pack switched on — and
// the one where a check that only ever passes would be invisible. The failure
// direction is asserted in the test below, against fixtures where the answer
// is known rather than incidental.
func TestASelfContainedPackResolvesAlone(t *testing.T) {
	results := conformance.CheckDanglingVerifiers(software.New())
	if len(results) != 1 || !results[0].Passed {
		for _, res := range results {
			if !res.Passed {
				t.Errorf("software alone has a dangling verifier: %s", res.Message)
			}
		}
		return
	}
	if !strings.Contains(results[0].Message, "resolves") {
		t.Errorf("passing message %q does not say what it checked", results[0].Message)
	}
}

// A verifier no pack anywhere exports is a bug in the pack, not a deployment
// decision, and the message has to distinguish them — they have different
// fixes and different owners.
func TestDanglingVerifierDistinguishesDisabledPackFromTypo(t *testing.T) {
	for _, tc := range []struct {
		name     string
		critDom  string
		verifier string
		contains string
	}{
		{"pack switched off", "healthcare", "healthcare.verify.chart_review", "is not enabled on this deployment"},
		{"nothing exports it", "", "routing.verify.imaginary", "no enabled pack exports it"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := packWithTemplate(tc.critDom, tc.verifier)
			results := conformance.CheckDanglingVerifiers(p)
			if len(results) != 1 || results[0].Passed {
				t.Fatalf("expected one failure, got %+v", results)
			}
			if !strings.Contains(results[0].Message, tc.contains) {
				t.Errorf("message %q does not say %q", results[0].Message, tc.contains)
			}
		})
	}
}

// packWithTemplate returns a pack whose single template has one criterion
// verified by the given capability, which the pack does not export.
func packWithTemplate(critDomain, verifier string) domain.Pack {
	p := &templatePack{}
	p.caps = []capability.Capability{routingCap("routing.act.real", false)}
	p.templates = []objective.Template{{
		ID:     "routing.objective.fixture",
		Title:  "fixture",
		Domain: "routing",
		SuccessCriteria: []objective.Criterion{{
			ID:          "the-one",
			Description: "verified by something this pack does not export",
			Verifier:    capability.CapabilityID(verifier),
			Domain:      critDomain,
			Weight:      1.0,
		}},
	}}
	return p
}

type templatePack struct {
	routingPack
	templates []objective.Template
}

func (p *templatePack) ObjectiveTemplates() []objective.Template { return p.templates }
